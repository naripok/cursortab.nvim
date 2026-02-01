package engine

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"cursortab/buffer"
	"cursortab/logger"
	"cursortab/text"
	"cursortab/types"
)

// Timer represents a timer that can be stopped.
type Timer interface {
	Stop() bool
}

// Clock provides time-related operations for dependency injection.
type Clock interface {
	AfterFunc(d time.Duration, f func()) Timer
	Now() time.Time
}

// SystemClock is the default Clock implementation using the standard library.
var SystemClock Clock = systemClock{}

type systemClock struct{}

func (systemClock) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}

func (systemClock) Now() time.Time {
	return time.Now()
}

// Engine is the core state machine for managing completions.
type Engine struct {
	WorkspacePath string
	WorkspaceID   string

	provider        Provider
	buffer          Buffer
	clock           Clock
	state           state
	ctx             context.Context
	currentCancel   context.CancelFunc
	prefetchCancel  context.CancelFunc
	idleTimer       Timer
	textChangeTimer Timer
	mu              sync.RWMutex
	eventChan       chan Event

	// Main context and cancel for the engine lifecycle
	mainCtx    context.Context
	mainCancel context.CancelFunc
	stopped    bool
	stopOnce   sync.Once

	// Completion state
	completions  []*types.Completion
	applyBatch   buffer.Batch
	cursorTarget *types.CursorPredictionTarget

	// Staged completion state (for multi-stage completions)
	stagedCompletion *types.StagedCompletion

	// Original buffer lines when completion was shown (for partial typing optimization)
	completionOriginalLines []string

	// Current groups for partial accept (stored when showing completion)
	currentGroups []*text.Group

	// Prefetch state
	prefetchedCompletions  []*types.Completion
	prefetchedCursorTarget *types.CursorPredictionTarget
	prefetchState          prefetchState

	// Streaming state (line-by-line)
	streamingState          *StreamingState
	streamingCancel         context.CancelFunc
	streamLinesChan         <-chan string // Lines channel (nil when not streaming)
	streamLineNum           int           // Line counter for current stream
	acceptedDuringStreaming bool          // True if user accepted partial during streaming

	// Token streaming state (token-by-token for inline)
	tokenStreamingState *TokenStreamingState
	tokenStreamChan     <-chan string // Token stream channel (nil when not streaming)

	// Config options
	config EngineConfig

	// Per-file state that persists across file switches (for context restoration)
	fileStateStore map[string]*FileState
}

// NewEngine creates a new Engine instance.
func NewEngine(provider Provider, buf Buffer, config EngineConfig, clock Clock) (*Engine, error) {
	workspacePath, err := os.Getwd()
	if err != nil {
		logger.Warn("error getting current directory, using home: %v", err)
		workspacePath = "~"
	}
	workspaceID := fmt.Sprintf("%s-%d", workspacePath, os.Getpid())

	return &Engine{
		WorkspacePath:          workspacePath,
		WorkspaceID:            workspaceID,
		provider:               provider,
		buffer:                 buf,
		clock:                  clock,
		state:                  stateIdle,
		ctx:                    nil,
		eventChan:              make(chan Event, 100),
		config:                 config,
		idleTimer:              nil,
		textChangeTimer:        nil,
		mu:                     sync.RWMutex{},
		completions:            nil,
		cursorTarget:           nil,
		prefetchedCompletions:  nil,
		prefetchedCursorTarget: nil,
		prefetchState:          prefetchNone,
		stopped:                false,
		fileStateStore:         make(map[string]*FileState),
	}, nil
}

// Start begins the engine event loop.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return
	}

	e.mainCtx, e.mainCancel = context.WithCancel(ctx)
	e.mu.Unlock()

	go e.eventLoop(e.mainCtx)
	logger.Info("engine started")
}

// Stop gracefully shuts down the engine and cleans up all resources.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		e.mu.Lock()
		defer e.mu.Unlock()

		logger.Info("stopping engine...")

		e.stopped = true
		if e.mainCancel != nil {
			e.mainCancel()
		}
		if e.currentCancel != nil {
			e.currentCancel()
			e.currentCancel = nil
		}
		if e.prefetchCancel != nil {
			e.prefetchCancel()
			e.prefetchCancel = nil
		}
		e.stopIdleTimer()
		e.stopTextChangeTimer()
		e.state = stateIdle
		e.cursorTarget = nil
		e.completions = nil
		e.applyBatch = nil
		e.stagedCompletion = nil
		e.prefetchedCompletions = nil
		e.prefetchedCursorTarget = nil
		e.prefetchState = prefetchNone
		e.completionOriginalLines = nil
		close(e.eventChan)

		logger.Info("engine stopped")
	})
}

// ClearOptions configures what to clear in clearState
type ClearOptions struct {
	CancelCurrent     bool
	CancelPrefetch    bool
	ClearStaged       bool
	ClearCursorTarget bool
	CallOnReject      bool
}

// clearState consolidates all state clearing into one method with configurable options
func (e *Engine) clearState(opts ClearOptions) {
	if opts.CancelCurrent && e.currentCancel != nil {
		e.currentCancel()
		e.currentCancel = nil
	}
	if opts.CancelPrefetch && e.prefetchCancel != nil {
		e.prefetchCancel()
		e.prefetchCancel = nil
		e.prefetchState = prefetchNone
		e.prefetchedCompletions = nil
		e.prefetchedCursorTarget = nil
	}
	if opts.ClearCursorTarget {
		e.cursorTarget = nil
	}
	if opts.CallOnReject {
		e.buffer.ClearUI()
	}
	e.completions = nil
	e.applyBatch = nil
	if opts.ClearStaged {
		e.stagedCompletion = nil
	}
	e.completionOriginalLines = nil
	e.currentGroups = nil
}

// clearAll clears everything including prefetch and staged completions
func (e *Engine) clearAll() {
	e.clearState(ClearOptions{CancelCurrent: true, CancelPrefetch: true, ClearStaged: true, ClearCursorTarget: true, CallOnReject: true})
}

// RegisterEventHandler registers the event handler for nvim RPC callbacks.
func (e *Engine) RegisterEventHandler() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.stopped {
		return
	}

	if err := e.buffer.RegisterEventHandler(func(event string) {
		e.mu.RLock()
		stopped := e.stopped
		e.mu.RUnlock()

		if stopped {
			return
		}

		eventType := EventTypeFromString(event)
		if eventType != "" {
			select {
			case e.eventChan <- Event{Type: eventType, Data: nil}:
			case <-e.mainCtx.Done():
				return
			}
		}
	}); err != nil {
		logger.Error("error registering event handler for new connection: %v", err)
	}
}

// Timer management

func (e *Engine) startIdleTimer() {
	e.stopIdleTimer()
	e.idleTimer = e.clock.AfterFunc(e.config.IdleCompletionDelay, func() {
		e.mu.RLock()
		stopped := e.stopped
		mainCtx := e.mainCtx
		e.mu.RUnlock()

		if stopped || mainCtx == nil {
			return
		}

		select {
		case e.eventChan <- Event{Type: EventIdleTimeout}:
		case <-mainCtx.Done():
		}
	})
}

func (e *Engine) stopIdleTimer() {
	if e.idleTimer != nil {
		e.idleTimer.Stop()
		e.idleTimer = nil
	}
}

func (e *Engine) resetIdleTimer() {
	e.stopIdleTimer()
	e.startIdleTimer()
}

func (e *Engine) startTextChangeTimer() {
	e.stopTextChangeTimer()
	e.textChangeTimer = e.clock.AfterFunc(e.config.TextChangeDebounce, func() {
		e.mu.RLock()
		stopped := e.stopped
		mainCtx := e.mainCtx
		e.mu.RUnlock()

		if stopped || mainCtx == nil {
			return
		}

		select {
		case e.eventChan <- Event{Type: EventTextChangeTimeout, Data: nil}:
		case <-mainCtx.Done():
		}
	})
}

func (e *Engine) stopTextChangeTimer() {
	if e.textChangeTimer != nil {
		e.textChangeTimer.Stop()
		e.textChangeTimer = nil
	}
}
