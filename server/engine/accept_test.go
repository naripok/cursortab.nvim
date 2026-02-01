package engine

import (
	"cursortab/assert"
	"cursortab/text"
	"cursortab/types"
	"testing"
)

func TestReject(t *testing.T) {
	buf := newMockBuffer()
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{StartLine: 1, EndLineInc: 1, Lines: []string{"test"}}}
	eng.cursorTarget = &types.CursorPredictionTarget{LineNumber: 5}

	eng.reject()

	assert.Equal(t, stateIdle, eng.state, "state after reject")
	assert.Nil(t, eng.completions, "completions after reject")
	assert.Nil(t, eng.cursorTarget, "cursorTarget after reject")
	assert.Greater(t, buf.clearUICalls, 0, "ClearUI should have been called")
}

func TestClearState_Options(t *testing.T) {
	buf := newMockBuffer()
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.completions = []*types.Completion{{StartLine: 1, EndLineInc: 1, Lines: []string{"test"}}}
	eng.stagedCompletion = &types.StagedCompletion{CurrentIdx: 0}
	eng.cursorTarget = &types.CursorPredictionTarget{LineNumber: 5}

	eng.clearState(ClearOptions{
		ClearStaged:       false,
		ClearCursorTarget: true,
		CallOnReject:      true,
	})

	if eng.stagedCompletion == nil {
		assert.NotNil(t, eng.stagedCompletion, "stagedCompletion should be preserved when ClearStaged=false")
	}
	assert.Nil(t, eng.cursorTarget, "cursorTarget should be cleared when ClearCursorTarget=true")
	assert.Nil(t, eng.completions, "completions should always be cleared")
}

func TestPartialAccept_AppendChars_SingleWord(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"func"}
	buf.row = 1
	buf.col = 4
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"function foo()"},
	}}
	eng.completionOriginalLines = []string{"func"}
	eng.currentGroups = []*text.Group{{
		Type:       "modification",
		BufferLine: 1,
		RenderHint: "append_chars",
		ColStart:   4,
		Lines:      []string{"function foo()"},
	}}

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.Equal(t, "tion ", buf.lastInsertedText, "inserted text")
	assert.Equal(t, stateHasCompletion, eng.state, "state after partial accept")
}

func TestPartialAccept_AppendChars_Punctuation(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"foo"}
	buf.row = 1
	buf.col = 3
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"foo.bar.baz"},
	}}
	eng.completionOriginalLines = []string{"foo"}
	eng.currentGroups = []*text.Group{{
		Type:       "modification",
		BufferLine: 1,
		RenderHint: "append_chars",
		ColStart:   3,
		Lines:      []string{"foo.bar.baz"},
	}}

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.Equal(t, ".", buf.lastInsertedText, "inserted text at punctuation")
	assert.Equal(t, stateHasCompletion, eng.state, "state after partial accept")
}

func TestPartialAccept_AppendChars_NoRemaining(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"hello"}
	buf.row = 1
	buf.col = 5
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"hello!"},
	}}
	eng.completionOriginalLines = []string{"hello"}
	eng.currentGroups = []*text.Group{{
		Type:       "modification",
		BufferLine: 1,
		RenderHint: "append_chars",
		ColStart:   5,
		Lines:      []string{"hello!"},
	}}

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.Equal(t, "!", buf.lastInsertedText, "inserted text")
	assert.Equal(t, stateIdle, eng.state, "state when nothing remaining")
}

func TestPartialAccept_MultiLine_FirstLine(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"line 1", "line 2", "line 3"}
	buf.row = 1
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 3,
		Lines:      []string{"new line 1", "new line 2", "new line 3"},
	}}
	eng.completionOriginalLines = []string{"line 1", "line 2", "line 3"}
	eng.currentGroups = []*text.Group{{
		Type:       "modification",
		BufferLine: 1,
		Lines:      []string{"new line 1", "new line 2", "new line 3"},
	}}

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.Equal(t, 1, buf.lastReplacedLine, "replaced line number")
	assert.Equal(t, "new line 1", buf.lastReplacedContent, "replaced content")
	assert.Equal(t, stateHasCompletion, eng.state, "state after partial line accept")
	assert.Equal(t, 2, len(eng.completions[0].Lines), "remaining lines")
	assert.Equal(t, 2, eng.completions[0].StartLine, "updated start line")
	assert.Equal(t, 3, eng.completions[0].EndLineInc, "end line unchanged for equal line count")
}

func TestPartialAccept_MultiLine_LastLine(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"old line"}
	buf.row = 1
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"new line"},
	}}
	eng.completionOriginalLines = []string{"old line"}
	eng.currentGroups = []*text.Group{{
		Type:       "modification",
		BufferLine: 1,
		Lines:      []string{"new line"},
	}}

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.Equal(t, "new line", buf.lastReplacedContent, "replaced content")
	assert.Equal(t, stateIdle, eng.state, "state after accepting last line")
}

func TestPartialAccept_WithUserTyping(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"functi"}
	buf.row = 1
	buf.col = 6
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"function foo()"},
	}}
	eng.completionOriginalLines = []string{"func"}
	eng.currentGroups = []*text.Group{{
		Type:       "modification",
		BufferLine: 1,
		RenderHint: "append_chars",
		ColStart:   4,
		Lines:      []string{"function foo()"},
	}}

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.Equal(t, "on ", buf.lastInsertedText, "inserted text after user typing")
}

func TestPartialAccept_NoCompletions(t *testing.T) {
	buf := newMockBuffer()
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = nil

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.Equal(t, stateHasCompletion, eng.state, "state unchanged when no completions")
}

func TestPartialAccept_NoGroups(t *testing.T) {
	buf := newMockBuffer()
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"test"},
	}}
	eng.currentGroups = nil

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.Equal(t, stateHasCompletion, eng.state, "state unchanged when no groups")
}

func TestPartialAccept_AdditionGroup(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"func main() {", "}"}
	buf.row = 1
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 2,
		Lines:      []string{"func main() {", "    fmt.Println(\"hello\")", "}"},
	}}
	eng.completionOriginalLines = []string{"func main() {", "}"}
	eng.currentGroups = []*text.Group{{
		Type:       "addition",
		BufferLine: 2,
		StartLine:  2,
		EndLine:    2,
		Lines:      []string{"    fmt.Println(\"hello\")"},
	}}

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.Equal(t, 1, buf.lastReplacedLine, "replaced line number")
	assert.Equal(t, "func main() {", buf.lastReplacedContent, "replaced content")
	assert.Equal(t, stateHasCompletion, eng.state, "state after first partial")
	assert.Equal(t, 2, len(eng.completions[0].Lines), "remaining lines")
	assert.Equal(t, 2, eng.completions[0].StartLine, "updated start line")
	assert.Equal(t, 3, eng.completions[0].EndLineInc, "updated end line for addition")
}

func TestPartialAccept_FinishSyncsBuffer_NonStaged(t *testing.T) {
	buf := newMockBuffer()
	buf.lines = []string{"test"}
	buf.row = 1
	buf.col = 4
	prov := newMockProvider()
	clock := newMockClock()
	eng := createTestEngine(buf, prov, clock)

	eng.state = stateHasCompletion
	eng.completions = []*types.Completion{{
		StartLine:  1,
		EndLineInc: 1,
		Lines:      []string{"test!"},
	}}
	eng.completionOriginalLines = []string{"test"}
	eng.currentGroups = []*text.Group{{
		Type:       "modification",
		BufferLine: 1,
		RenderHint: "append_chars",
		ColStart:   4,
		Lines:      []string{"test!"},
	}}
	eng.stagedCompletion = nil
	eng.cursorTarget = nil

	initialSyncCalls := buf.syncCalls

	eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

	assert.True(t, buf.syncCalls > initialSyncCalls, "buffer should be synced after finish")
	assert.Equal(t, stateIdle, eng.state, "should be idle after finish")
}

// TestPartialAccept_MultiLineCompletion_CursorTargetConsistency tests that cursor targets
// remain consistent when using partial accept vs full accept on the same multi-line completion.
func TestPartialAccept_MultiLineCompletion_CursorTargetConsistency(t *testing.T) {
	t.Run("full_accept_preserves_cursor_target", func(t *testing.T) {
		buf := newMockBuffer()
		buf.lines = []string{"old line 1", "old line 2", "old line 3", "old line 4"}
		buf.row = 1
		prov := newMockProvider()
		clock := newMockClock()
		eng, cancel := createTestEngineWithContext(buf, prov, clock)
		defer cancel()

		eng.state = stateHasCompletion
		eng.completions = []*types.Completion{{
			StartLine:  1,
			EndLineInc: 4,
			Lines:      []string{"new line 1", "new line 2", "new line 3", "new line 4"},
		}}
		eng.completionOriginalLines = buf.lines
		eng.currentGroups = []*text.Group{
			{Type: "modification", BufferLine: 1},
			{Type: "modification", BufferLine: 2},
			{Type: "modification", BufferLine: 3},
			{Type: "modification", BufferLine: 4},
		}

		expectedCursorTarget := int32(8)
		eng.cursorTarget = &types.CursorPredictionTarget{
			RelativePath:    "test.go",
			LineNumber:      expectedCursorTarget,
			ShouldRetrigger: true,
		}
		eng.applyBatch = &mockBatch{}
		eng.stagedCompletion = nil

		eng.doAcceptCompletion(Event{Type: EventAccept})

		assert.Equal(t, int(expectedCursorTarget), buf.showCursorTargetLine, "cursor target should be preserved after full accept")
	})

	t.Run("partial_accept_4_lines_one_by_one_same_target", func(t *testing.T) {
		buf := newMockBuffer()
		buf.lines = []string{"old line 1", "old line 2", "old line 3", "old line 4"}
		buf.row = 1
		prov := newMockProvider()
		clock := newMockClock()
		eng, cancel := createTestEngineWithContext(buf, prov, clock)
		defer cancel()

		eng.state = stateHasCompletion
		eng.completions = []*types.Completion{{
			StartLine:  1,
			EndLineInc: 4,
			Lines:      []string{"new line 1", "new line 2", "new line 3", "new line 4"},
		}}
		eng.completionOriginalLines = buf.lines
		eng.currentGroups = []*text.Group{
			{Type: "modification", BufferLine: 1},
			{Type: "modification", BufferLine: 2},
			{Type: "modification", BufferLine: 3},
			{Type: "modification", BufferLine: 4},
		}

		expectedCursorTarget := int32(8)
		eng.cursorTarget = &types.CursorPredictionTarget{
			RelativePath:    "test.go",
			LineNumber:      expectedCursorTarget,
			ShouldRetrigger: true,
		}
		eng.stagedCompletion = nil

		eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})
		assert.Equal(t, stateHasCompletion, eng.state, "should stay in HasCompletion after partial accept")
		assert.Equal(t, 3, len(eng.completions[0].Lines), "remaining lines")
		assert.Equal(t, 2, eng.completions[0].StartLine, "start line increments")

		eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})
		assert.Equal(t, 2, len(eng.completions[0].Lines), "remaining lines")
		assert.Equal(t, 3, eng.completions[0].StartLine, "start line increments")

		eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})
		assert.Equal(t, 1, len(eng.completions[0].Lines), "remaining lines")
		assert.Equal(t, 4, eng.completions[0].StartLine, "start line increments")

		eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

		assert.Equal(t, int(expectedCursorTarget), buf.showCursorTargetLine, "cursor target should be preserved through partial accepts")
	})

	t.Run("partial_accept_cursor_target_consistency_through_all_accepts", func(t *testing.T) {
		buf := newMockBuffer()
		buf.lines = []string{"x", "y", "z", "w"}
		buf.row = 1
		prov := newMockProvider()
		clock := newMockClock()
		eng, cancel := createTestEngineWithContext(buf, prov, clock)
		defer cancel()

		cursorTarget := int32(12)
		eng.state = stateHasCompletion
		eng.completions = []*types.Completion{{
			StartLine:  1,
			EndLineInc: 4,
			Lines:      []string{"X", "Y", "Z", "W"},
		}}
		eng.completionOriginalLines = buf.lines
		eng.currentGroups = []*text.Group{
			{Type: "modification", BufferLine: 1},
			{Type: "modification", BufferLine: 2},
			{Type: "modification", BufferLine: 3},
			{Type: "modification", BufferLine: 4},
		}
		eng.cursorTarget = &types.CursorPredictionTarget{
			RelativePath:    "test.go",
			LineNumber:      cursorTarget,
			ShouldRetrigger: false,
		}
		eng.stagedCompletion = nil

		for i := 0; i < 3; i++ {
			eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})
			if i < 2 {
				assert.Equal(t, cursorTarget, eng.cursorTarget.LineNumber, "cursor target should be unchanged")
			}
		}

		eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

		assert.Equal(t, int(cursorTarget), buf.showCursorTargetLine, "final cursor target should be original value")
	})

	t.Run("partial_accept_with_staged_completion", func(t *testing.T) {
		buf := newMockBuffer()
		buf.lines = []string{"a", "b", "c", "d", "e", "f"}
		buf.row = 1
		prov := newMockProvider()
		clock := newMockClock()
		eng, cancel := createTestEngineWithContext(buf, prov, clock)
		defer cancel()

		stage1 := &text.Stage{
			BufferStart: 1,
			BufferEnd:   2,
			Lines:       []string{"A", "B"},
			Groups:      []*text.Group{{Type: "modification", BufferLine: 1}},
			CursorLine:  1,
			CursorCol:   0,
			IsLastStage: false,
			CursorTarget: &types.CursorPredictionTarget{
				LineNumber:      3,
				ShouldRetrigger: false,
			},
		}

		stage2 := &text.Stage{
			BufferStart: 3,
			BufferEnd:   4,
			Lines:       []string{"C", "D"},
			Groups:      []*text.Group{{Type: "modification", BufferLine: 3}},
			CursorLine:  1,
			CursorCol:   0,
			IsLastStage: true,
			CursorTarget: &types.CursorPredictionTarget{
				LineNumber:      5,
				ShouldRetrigger: true,
			},
		}

		eng.state = stateHasCompletion
		eng.completions = []*types.Completion{{
			StartLine:  1,
			EndLineInc: 2,
			Lines:      []string{"A", "B"},
		}}
		eng.completionOriginalLines = []string{"a", "b"}
		eng.currentGroups = []*text.Group{{Type: "modification", BufferLine: 1}}
		eng.stagedCompletion = &types.StagedCompletion{
			Stages:     []any{stage1, stage2},
			CurrentIdx: 0,
		}
		eng.applyBatch = &mockBatch{}
		eng.cursorTarget = stage1.CursorTarget

		eng.doPartialAcceptCompletion(Event{Type: EventPartialAccept})

		assert.Equal(t, int32(3), eng.cursorTarget.LineNumber, "cursor target should be preserved from stage 1")
	})
}
