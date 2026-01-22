package engine

import (
	"cursortab/types"
	"log"
)

type EventType string

// Event type constants
const (
	EventEsc               EventType = "esc"
	EventTextChanged       EventType = "text_changed"
	EventTextChangeTimeout EventType = "trigger_completion"
	EventCursorMovedNormal EventType = "cursor_moved_normal"
	EventInsertEnter       EventType = "insert_enter"
	EventInsertLeave       EventType = "insert_leave"
	EventTab               EventType = "tab"
	EventIdleTimeout       EventType = "idle_timeout"
	EventCompletionReady   EventType = "completion_ready"
	EventCompletionError   EventType = "completion_error"
	EventPrefetchReady     EventType = "prefetch_ready"
	EventPrefetchError     EventType = "prefetch_error"
)

var eventTypeMap map[string]EventType

func init() {
	eventTypeMap = buildEventTypeMap()
}

func buildEventTypeMap() map[string]EventType {
	eventMap := make(map[string]EventType)

	// Create a slice of all known EventType values
	allEventTypes := []EventType{
		EventEsc,
		EventTextChanged,
		EventTextChangeTimeout,
		EventCursorMovedNormal,
		EventInsertEnter,
		EventInsertLeave,
		EventTab,
		EventIdleTimeout,
		EventCompletionReady,
		EventCompletionError,
		EventPrefetchReady,
		EventPrefetchError,
	}

	// Build the map from EventType value to string
	for _, eventType := range allEventTypes {
		eventMap[string(eventType)] = eventType
	}

	return eventMap
}

func EventTypeFromString(s string) EventType {
	if eventType, exists := eventTypeMap[s]; exists {
		return eventType
	}
	return "" // or return a default EventType
}

type Event struct {
	Type EventType
	Data any
}

func (e *Engine) handleEsc() {
	e.reject()
	e.stopIdleTimer()
}

func (e *Engine) handleTextChange() {
	e.reject()
	e.startTextChangeTimer()
}

func (e *Engine) handleTextChangeTimeout() {
	e.requestCompletion(types.CompletionSourceTyping)
}

func (e *Engine) handleCursorMoveNormal() {
	e.reject()
	e.resetIdleTimer()
}

func (e *Engine) handleInsertEnter() {
	e.stopIdleTimer()
}

func (e *Engine) handleInsertLeave() {
	e.reject()
	e.startIdleTimer()
}

func (e *Engine) handleTab() {
	if e.n == nil {
		return
	}

	switch e.state {
	case stateHasCompletion:
		e.acceptCompletion()
	case stateHasCursorTarget:
		e.acceptCursorTarget()
	}
}

func (e *Engine) handleIdleTimeout() {
	if e.state == stateIdle {
		e.requestCompletion(types.CompletionSourceIdle)
	}
}

func (e *Engine) handleCompletionReady(response *types.CompletionResponse) {
	if e.n == nil {
		return
	}

	e.state = stateHasCompletion
	e.completions = response.Completions
	e.cursorTarget = response.CursorTarget

	if len(response.Completions) > 0 {
		if e.buffer.HasChanges(response.Completions[0].StartLine, response.Completions[0].EndLineInc, response.Completions[0].Lines) {
			e.applyBatch = e.buffer.OnCompletionReady(e.n, response.Completions[0].StartLine, response.Completions[0].EndLineInc, response.Completions[0].Lines)
		} else {
			log.Printf("no changes to completion")
			e.handleCursorTarget()
		}

		if len(response.Completions) > 1 {
			log.Printf("multiple completions: %v", response.Completions)
		}

	} else {
		e.handleCursorTarget()
	}
}
