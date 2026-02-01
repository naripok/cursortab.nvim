package engine

import (
	"cursortab/assert"
	"cursortab/types"
	"testing"
)

func TestCheckTypingMatchesPrediction_NoCompletions(t *testing.T) {
	buf := newMockBuffer()
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	matches, hasRemaining := eng.checkTypingMatchesPrediction()
	assert.False(t, matches, "matches when no completions")
	assert.False(t, hasRemaining, "hasRemaining when no completions")
}

func TestCheckTypingMatchesPrediction_MatchesPrefix(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"hello wo"}
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"hello world"},
	}}
	eng.completionOriginalLines = []string{"hello "}

	matches, hasRemaining := eng.checkTypingMatchesPrediction()
	assert.True(t, matches, "match when buffer is prefix of target")
	assert.True(t, hasRemaining, "hasRemaining when buffer hasn't fully matched target")
}

func TestCheckTypingMatchesPrediction_FullyTyped(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"hello world"}
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"hello world"},
	}}
	eng.completionOriginalLines = []string{"hello "}

	matches, hasRemaining := eng.checkTypingMatchesPrediction()
	assert.True(t, matches, "match when buffer matches target")
	assert.False(t, hasRemaining, "hasRemaining when buffer fully matches target")
}

func TestCheckTypingMatchesPrediction_NoMatch(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"hello universe"}
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"hello world"},
	}}
	eng.completionOriginalLines = []string{"hello "}

	matches, _ := eng.checkTypingMatchesPrediction()
	assert.False(t, matches, "match when buffer diverges from target")
}

func TestCheckTypingMatchesPrediction_MultiLine(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"line 1", "line 2 co"}
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 2,
		Lines:      []string{"line 1", "line 2 complete"},
	}}
	eng.completionOriginalLines = []string{"line 1", "line 2 "}

	matches, hasRemaining := eng.checkTypingMatchesPrediction()
	assert.True(t, matches, "match for multi-line partial completion")
	assert.True(t, hasRemaining, "hasRemaining for multi-line partial completion")
}

func TestCheckTypingMatchesPrediction_DeletionNotSupported(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"line 1"}
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 2,
		Lines:      []string{"combined line"},
	}}
	eng.completionOriginalLines = []string{"line 1", "line 2"}

	matches, _ := eng.checkTypingMatchesPrediction()
	assert.False(t, matches, "match when completion deletes lines")
}

func TestHandleCursorTarget_Disabled(t *testing.T) {
	buf := newMockBuffer()
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)
	eng.config.CursorPrediction.Enabled = false

	eng.cursorTarget = &types.CursorPredictionTarget{LineNumber: 10}
	eng.state = stateHasCursorTarget

	eng.handleCursorTarget()

	assert.Equal(t, stateIdle, eng.state, "state when cursor prediction disabled")
	assert.Nil(t, eng.cursorTarget, "cursorTarget when cursor prediction disabled")
}

func TestHandleCursorTarget_CloseEnough(t *testing.T) {
	buf := newMockBuffer()
	buf.row = 8
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)
	eng.config.CursorPrediction.Enabled = true
	eng.config.CursorPrediction.ProximityThreshold = 3

	eng.cursorTarget = &types.CursorPredictionTarget{LineNumber: 10}

	eng.handleCursorTarget()

	assert.Equal(t, stateIdle, eng.state, "state when close enough")
}

func TestHandleCursorTarget_FarAway(t *testing.T) {
	buf := newMockBuffer()
	buf.row = 1
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)
	eng.config.CursorPrediction.Enabled = true
	eng.config.CursorPrediction.ProximityThreshold = 3

	eng.cursorTarget = &types.CursorPredictionTarget{LineNumber: 10}

	eng.handleCursorTarget()

	assert.Equal(t, stateHasCursorTarget, eng.state, "state when far away")
	assert.Equal(t, 10, buf.showCursorTargetLine, "showCursorTargetLine")
}
