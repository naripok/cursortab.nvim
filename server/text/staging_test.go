package text

import (
	"cursortab/assert"
	"fmt"
	"testing"
)

func TestClusterDistanceFromCursor(t *testing.T) {
	// Cluster with relative coordinates 1-6, baseLineOffset=10 means buffer lines 10-15
	cluster := &ChangeCluster{
		StartLine: 1,
		EndLine:   6,
		Changes: map[int]LineDiff{
			1: {Type: LineModification, LineNumber: 1, OldLineNum: 1, NewLineNum: 1},
			6: {Type: LineModification, LineNumber: 6, OldLineNum: 6, NewLineNum: 6},
		},
	}
	baseLineOffset := 10
	diff := &DiffResult{
		Changes:      cluster.Changes,
		OldLineCount: 6,
		NewLineCount: 6,
	}

	tests := []struct {
		cursorRow int // buffer coordinates
		expected  int
	}{
		{5, 5},  // cursor before cluster (buffer line 5, cluster starts at buffer 10)
		{10, 0}, // cursor at start (buffer line 10)
		{12, 0}, // cursor inside (buffer line 12)
		{15, 0}, // cursor at end (buffer line 15)
		{20, 5}, // cursor after cluster (buffer line 20, cluster ends at buffer 15)
	}

	for _, tt := range tests {
		result := clusterDistanceFromCursor(cluster, tt.cursorRow, baseLineOffset, diff)
		assert.Equal(t, tt.expected, result, fmt.Sprintf("distance for cursor at %d", tt.cursorRow))
	}
}

func TestClusterDistanceFromCursor_NoOffset(t *testing.T) {
	// When baseLineOffset=1, cluster coordinates match buffer coordinates
	cluster := &ChangeCluster{
		StartLine: 10,
		EndLine:   15,
		Changes: map[int]LineDiff{
			10: {Type: LineModification, LineNumber: 10, OldLineNum: 10, NewLineNum: 10},
			15: {Type: LineModification, LineNumber: 15, OldLineNum: 15, NewLineNum: 15},
		},
	}
	baseLineOffset := 1
	diff := &DiffResult{
		Changes:      cluster.Changes,
		OldLineCount: 15,
		NewLineCount: 15,
	}

	tests := []struct {
		cursorRow int
		expected  int
	}{
		{5, 5},  // cursor before cluster
		{10, 0}, // cursor at start
		{12, 0}, // cursor inside
		{15, 0}, // cursor at end
		{20, 5}, // cursor after cluster
	}

	for _, tt := range tests {
		result := clusterDistanceFromCursor(cluster, tt.cursorRow, baseLineOffset, diff)
		assert.Equal(t, tt.expected, result, fmt.Sprintf("distance for cursor at %d", tt.cursorRow))
	}
}

func TestJoinLines(t *testing.T) {
	lines := []string{"line1", "line2", "line3"}
	result := JoinLines(lines)
	expected := "line1\nline2\nline3"

	assert.Equal(t, expected, result, "JoinLines result")
}

func TestCreateStages_EmptyDiff(t *testing.T) {
	diff := &DiffResult{Changes: map[int]LineDiff{}}
	stages := CreateStages(diff, 10, 1, 50, 1, 3, "test.go", []string{})

	assert.Nil(t, stages, "stages for empty diff")
}

func TestCreateStages_SingleCluster(t *testing.T) {
	// All changes within proximity threshold - should return nil (no staging needed)
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			10: {Type: LineModification, LineNumber: 10, Content: "new10", OldContent: "old10"},
			11: {Type: LineModification, LineNumber: 11, Content: "new11", OldContent: "old11"},
			12: {Type: LineModification, LineNumber: 12, Content: "new12", OldContent: "old12"},
		},
	}
	newLines := make([]string, 20)
	for i := range newLines {
		newLines[i] = "line"
	}

	stages := CreateStages(diff, 10, 1, 50, 1, 3, "test.go", newLines)

	assert.Nil(t, stages, "stages for single cluster")
}

func TestCreateStages_TwoClusters(t *testing.T) {
	// Changes at lines 10-11 and 25-26 (gap > threshold of 3)
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			10: {Type: LineModification, LineNumber: 10, Content: "new10", OldContent: "old10"},
			11: {Type: LineModification, LineNumber: 11, Content: "new11", OldContent: "old11"},
			25: {Type: LineModification, LineNumber: 25, Content: "new25", OldContent: "old25"},
			26: {Type: LineModification, LineNumber: 26, Content: "new26", OldContent: "old26"},
		},
	}
	newLines := make([]string, 30)
	for i := range newLines {
		newLines[i] = "content"
	}

	// Cursor at line 15, baseLineOffset=1, so cluster 10-11 is closer
	stages := CreateStages(diff, 15, 1, 50, 1, 3, "test.go", newLines)

	assert.Len(t, 2, stages, "stages")

	// First stage should be the closer cluster (10-11)
	assert.Equal(t, 10, stages[0].Completion.StartLine, "first stage start line")
	assert.Equal(t, 11, stages[0].Completion.EndLineInc, "first stage end line")

	// Second stage should be 25-26
	assert.Equal(t, 25, stages[1].Completion.StartLine, "second stage start line")

	// First stage cursor target should point to next stage
	assert.Equal(t, 25, stages[0].CursorTarget.LineNumber, "first stage cursor target line")
	assert.False(t, stages[0].CursorTarget.ShouldRetrigger, "first stage ShouldRetrigger")

	// Last stage should have ShouldRetrigger=true
	assert.True(t, stages[1].CursorTarget.ShouldRetrigger, "last stage ShouldRetrigger")
	assert.True(t, stages[1].IsLastStage, "second stage IsLastStage")
}

func TestCreateStages_CursorDistanceSorting(t *testing.T) {
	// Three clusters: 5-6, 20-21, 35-36
	// Cursor at 22 - closest to cluster 20-21
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			5:  {Type: LineModification, LineNumber: 5, Content: "new5", OldContent: "old5"},
			6:  {Type: LineModification, LineNumber: 6, Content: "new6", OldContent: "old6"},
			20: {Type: LineModification, LineNumber: 20, Content: "new20", OldContent: "old20"},
			21: {Type: LineModification, LineNumber: 21, Content: "new21", OldContent: "old21"},
			35: {Type: LineModification, LineNumber: 35, Content: "new35", OldContent: "old35"},
			36: {Type: LineModification, LineNumber: 36, Content: "new36", OldContent: "old36"},
		},
	}
	newLines := make([]string, 40)
	for i := range newLines {
		newLines[i] = "content"
	}

	stages := CreateStages(diff, 22, 1, 50, 1, 3, "test.go", newLines)

	assert.Len(t, 3, stages, "stages")

	// First stage should be closest to cursor (20-21)
	assert.Equal(t, 20, stages[0].Completion.StartLine, "first stage should be closest cluster (20-21)")
}

func TestCreateStages_ViewportPartitioning(t *testing.T) {
	// Changes at lines 10 (in viewport) and 100 (out of viewport)
	// Viewport is 1-50
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			10:  {Type: LineModification, LineNumber: 10, Content: "new10", OldContent: "old10"},
			100: {Type: LineModification, LineNumber: 100, Content: "new100", OldContent: "old100"},
		},
	}
	newLines := make([]string, 110)
	for i := range newLines {
		newLines[i] = "content"
	}

	// Cursor at 10, viewport 1-50
	stages := CreateStages(diff, 10, 1, 50, 1, 3, "test.go", newLines)

	assert.Len(t, 2, stages, "stages")

	// In-view changes should come first (line 10)
	assert.Equal(t, 10, stages[0].Completion.StartLine, "first stage should be in-viewport change")

	// Out-of-view change should be second (line 100)
	assert.Equal(t, 100, stages[1].Completion.StartLine, "second stage should be out-of-viewport change")
}

func TestCreateStages_ProximityGrouping(t *testing.T) {
	// Changes at lines 10, 12, 14 (all within threshold of 3)
	// Should form single cluster, so no staging
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			10: {Type: LineModification, LineNumber: 10, Content: "new10", OldContent: "old10"},
			12: {Type: LineModification, LineNumber: 12, Content: "new12", OldContent: "old12"},
			14: {Type: LineModification, LineNumber: 14, Content: "new14", OldContent: "old14"},
		},
	}
	newLines := make([]string, 20)
	for i := range newLines {
		newLines[i] = "content"
	}

	stages := CreateStages(diff, 10, 1, 50, 1, 3, "test.go", newLines)

	// Gap between 10->12 is 2, 12->14 is 2 - all within threshold
	assert.Nil(t, stages, "stages for single cluster (all within threshold)")
}

func TestCreateStages_ProximityGrouping_SplitByGap(t *testing.T) {
	// Changes at lines 10, 12 (gap=2) and 20 (gap=8 from 12)
	// With threshold=3, should split into two clusters
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			10: {Type: LineModification, LineNumber: 10, Content: "new10", OldContent: "old10"},
			12: {Type: LineModification, LineNumber: 12, Content: "new12", OldContent: "old12"},
			20: {Type: LineModification, LineNumber: 20, Content: "new20", OldContent: "old20"},
		},
	}
	newLines := make([]string, 25)
	for i := range newLines {
		newLines[i] = "content"
	}

	stages := CreateStages(diff, 10, 1, 50, 1, 3, "test.go", newLines)

	assert.Len(t, 2, stages, "stages (gap > threshold)")

	// First cluster should be 10-12
	assert.Equal(t, 10, stages[0].Completion.StartLine, "first stage start line")
	assert.Equal(t, 12, stages[0].Completion.EndLineInc, "first stage end line")

	// Second cluster should be 20
	assert.Equal(t, 20, stages[1].Completion.StartLine, "second stage start line")
}

func TestCreateStages_WithBaseLineOffset(t *testing.T) {
	// Diff coordinates are relative (1-indexed from start of extraction)
	// baseLineOffset=50 means diff line 1 = buffer line 50
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			1:  {Type: LineModification, LineNumber: 1, Content: "new1", OldContent: "old1"},
			10: {Type: LineModification, LineNumber: 10, Content: "new10", OldContent: "old10"},
		},
	}
	newLines := make([]string, 15)
	for i := range newLines {
		newLines[i] = "content"
	}

	// baseLineOffset=50, so diff line 1 = buffer line 50, diff line 10 = buffer line 59
	stages := CreateStages(diff, 55, 1, 100, 50, 3, "test.go", newLines)

	assert.Len(t, 2, stages, "stages")

	// Verify buffer coordinates in completion
	assert.True(t, stages[0].Completion.StartLine == 50 || stages[0].Completion.StartLine == 59,
		fmt.Sprintf("stage should have buffer coordinates (50 or 59), got %d", stages[0].Completion.StartLine))
}

func TestCreateStages_VisualGroupsComputed(t *testing.T) {
	// Verify that visual groups are computed for stages
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			1: {Type: LineModification, LineNumber: 1, Content: "new1", OldContent: "old1"},
			2: {Type: LineModification, LineNumber: 2, Content: "new2", OldContent: "old2"},
			// Gap
			10: {Type: LineModification, LineNumber: 10, Content: "new10", OldContent: "old10"},
		},
	}
	newLines := []string{"new1", "new2", "", "", "", "", "", "", "", "new10"}

	stages := CreateStages(diff, 1, 1, 50, 1, 3, "test.go", newLines)

	assert.Len(t, 2, stages, "stages")

	// First stage (lines 1-2) should have visual groups
	assert.NotNil(t, stages[0].VisualGroups, "first stage visual groups")
}

func TestGroupChangesByProximity(t *testing.T) {
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			5:  {Type: LineModification, LineNumber: 5},
			6:  {Type: LineModification, LineNumber: 6},
			7:  {Type: LineModification, LineNumber: 7},
			20: {Type: LineModification, LineNumber: 20},
			21: {Type: LineModification, LineNumber: 21},
		},
	}

	lineNumbers := []int{5, 6, 7, 20, 21}
	clusters := groupChangesByProximity(diff, lineNumbers, 3)

	assert.Len(t, 2, clusters, "clusters")

	// First cluster: 5-7
	assert.Equal(t, 5, clusters[0].StartLine, "first cluster start line")
	assert.Equal(t, 7, clusters[0].EndLine, "first cluster end line")
	assert.Equal(t, 3, len(clusters[0].Changes), "first cluster change count")

	// Second cluster: 20-21
	assert.Equal(t, 20, clusters[1].StartLine, "second cluster start line")
	assert.Equal(t, 21, clusters[1].EndLine, "second cluster end line")
}

func TestGroupChangesByProximity_EmptyInput(t *testing.T) {
	diff := &DiffResult{Changes: map[int]LineDiff{}}
	clusters := groupChangesByProximity(diff, []int{}, 3)

	assert.Nil(t, clusters, "clusters for empty input")
}

func TestGroupChangesByProximity_WithGroups(t *testing.T) {
	// Test that group types (LineModificationGroup) use EndLine for cluster boundaries
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			5: {
				Type:       LineModificationGroup,
				LineNumber: 5,
				StartLine:  5,
				EndLine:    10, // Group spans 5-10
			},
			15: {Type: LineModification, LineNumber: 15},
		},
	}

	lineNumbers := []int{5, 15}
	clusters := groupChangesByProximity(diff, lineNumbers, 3)

	assert.Len(t, 2, clusters, "clusters")

	// First cluster should end at 10 (group EndLine), not 5
	assert.Equal(t, 10, clusters[0].EndLine, "first cluster should end at group EndLine")
}

func TestComputeVisualGroups(t *testing.T) {
	changes := map[int]LineDiff{
		1: {Type: LineModification, LineNumber: 1, Content: "new1", OldContent: "old1"},
		2: {Type: LineModification, LineNumber: 2, Content: "new2", OldContent: "old2"},
		3: {Type: LineModification, LineNumber: 3, Content: "new3", OldContent: "old3"},
		// Gap
		7: {Type: LineAddition, LineNumber: 7, Content: "added7"},
		8: {Type: LineAddition, LineNumber: 8, Content: "added8"},
	}
	newLines := []string{"new1", "new2", "new3", "", "", "", "added7", "added8"}
	oldLines := []string{"old1", "old2", "old3", "", "", "", "", ""}

	groups := computeVisualGroups(changes, newLines, oldLines)

	assert.Len(t, 2, groups, "groups")

	// First group: modifications 1-3
	assert.Equal(t, "modification", groups[0].Type, "first group type")
	assert.Equal(t, 1, groups[0].StartLine, "first group start line")
	assert.Equal(t, 3, groups[0].EndLine, "first group end line")
	assert.Equal(t, 3, len(groups[0].Lines), "first group line count")
	assert.Equal(t, 3, len(groups[0].OldLines), "first group old line count")

	// Second group: additions 7-8
	assert.Equal(t, "addition", groups[1].Type, "second group type")
	assert.Equal(t, 7, groups[1].StartLine, "second group start line")
	assert.Equal(t, 8, groups[1].EndLine, "second group end line")
}

func TestComputeVisualGroups_NonConsecutive(t *testing.T) {
	// Non-consecutive changes should form separate groups
	changes := map[int]LineDiff{
		1: {Type: LineModification, LineNumber: 1, Content: "new1", OldContent: "old1"},
		3: {Type: LineModification, LineNumber: 3, Content: "new3", OldContent: "old3"},
		5: {Type: LineModification, LineNumber: 5, Content: "new5", OldContent: "old5"},
	}
	newLines := []string{"new1", "", "new3", "", "new5"}
	oldLines := []string{"old1", "", "old3", "", "old5"}

	groups := computeVisualGroups(changes, newLines, oldLines)

	// Each non-consecutive change should be its own group
	assert.Equal(t, 3, len(groups), "group count for non-consecutive changes")
}

// =============================================================================
// Tests for staging with unequal line counts
// =============================================================================

func TestCreateStages_WithInsertions(t *testing.T) {
	// Test staging when completion adds lines (net line increase)
	// Old: 3 lines, New: 5 lines (2 insertions at different locations)
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			2: {Type: LineAddition, LineNumber: 2, NewLineNum: 2, OldLineNum: -1, Content: "inserted1"},
			5: {Type: LineAddition, LineNumber: 5, NewLineNum: 5, OldLineNum: -1, Content: "inserted2"},
		},
		OldLineCount: 3,
		NewLineCount: 5,
		LineMapping: &LineMapping{
			NewToOld: []int{1, -1, 2, 3, -1}, // line 2 and 5 are insertions
			OldToNew: []int{1, 3, 4},         // old lines map to new positions
		},
	}
	newLines := []string{"line1", "inserted1", "line2", "line3", "inserted2"}

	// Gap between line 2 and line 5 is 3, with threshold 2 they should be separate
	stages := CreateStages(diff, 1, 1, 50, 1, 2, "test.go", newLines)

	assert.Len(t, 2, stages, "stages for separated insertions")

	// Verify stages have valid completions
	for i, stage := range stages {
		assert.NotNil(t, stage.Completion, fmt.Sprintf("stage %d completion", i))
		assert.True(t, len(stage.Completion.Lines) > 0, fmt.Sprintf("stage %d has lines", i))
	}
}

func TestCreateStages_WithDeletions(t *testing.T) {
	// Test staging when completion removes lines (net line decrease)
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			2:  {Type: LineDeletion, LineNumber: 2, OldLineNum: 2, NewLineNum: -1, Content: "deleted1"},
			10: {Type: LineDeletion, LineNumber: 10, OldLineNum: 10, NewLineNum: -1, Content: "deleted2"},
		},
		OldLineCount: 12,
		NewLineCount: 10,
		LineMapping: &LineMapping{
			NewToOld: []int{1, 3, 4, 5, 6, 7, 8, 9, 11, 12},
			OldToNew: []int{1, -1, 2, 3, 4, 5, 6, 7, 8, -1, 9, 10},
		},
	}
	newLines := make([]string, 10)
	for i := range newLines {
		newLines[i] = "content"
	}

	// Gap is large (10-2=8), should create 2 stages
	stages := CreateStages(diff, 1, 1, 50, 1, 3, "test.go", newLines)

	assert.Len(t, 2, stages, "stages for separated deletions")
}

func TestCreateStages_MixedInsertionDeletion(t *testing.T) {
	// Test with both insertions and deletions in different regions
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			2:  {Type: LineAddition, LineNumber: 2, NewLineNum: 2, OldLineNum: 1, Content: "inserted"},
			15: {Type: LineDeletion, LineNumber: 15, OldLineNum: 15, NewLineNum: 14, Content: "deleted"},
		},
		OldLineCount: 20,
		NewLineCount: 20, // net zero change
		LineMapping: &LineMapping{
			NewToOld: make([]int, 20),
			OldToNew: make([]int, 20),
		},
	}
	// Initialize mappings
	for i := range diff.LineMapping.NewToOld {
		diff.LineMapping.NewToOld[i] = i + 1
	}
	for i := range diff.LineMapping.OldToNew {
		diff.LineMapping.OldToNew[i] = i + 1
	}
	diff.LineMapping.NewToOld[1] = -1 // line 2 is insertion

	newLines := make([]string, 20)
	for i := range newLines {
		newLines[i] = "content"
	}

	// Large gap (15-2=13), should create 2 stages
	stages := CreateStages(diff, 1, 1, 50, 1, 3, "test.go", newLines)

	assert.Len(t, 2, stages, "stages")
}

func TestGetBufferLineForChange_Insertion(t *testing.T) {
	// Test that insertions correctly calculate buffer position
	mapping := &LineMapping{
		NewToOld: []int{1, -1, -1, 2}, // lines 2,3 are insertions after line 1
		OldToNew: []int{1, 4},
	}

	// Pure insertion at new line 2, should anchor to old line 1
	change := LineDiff{
		Type:       LineAddition,
		NewLineNum: 2,
		OldLineNum: -1, // pure insertion
	}

	bufferLine := getBufferLineForChange(change, 2, 1, mapping)

	// Should find old line 1 as anchor (the mapped line before insertion)
	assert.Equal(t, 1, bufferLine, "buffer line for insertion (anchor)")
}

func TestGetBufferLineForChange_Modification(t *testing.T) {
	// Test that modifications use OldLineNum directly
	mapping := &LineMapping{
		NewToOld: []int{1, 2, 3},
		OldToNew: []int{1, 2, 3},
	}

	change := LineDiff{
		Type:       LineModification,
		NewLineNum: 2,
		OldLineNum: 2,
	}

	bufferLine := getBufferLineForChange(change, 2, 10, mapping)

	// baseLineOffset=10, oldLineNum=2: 2 + 10 - 1 = 11
	assert.Equal(t, 11, bufferLine, "buffer line for modification")
}

func TestGetClusterBufferRange_WithInsertions(t *testing.T) {
	// Cluster containing insertions should still compute valid buffer range
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			2: {Type: LineAddition, LineNumber: 2, NewLineNum: 2, OldLineNum: -1},
			3: {Type: LineAddition, LineNumber: 3, NewLineNum: 3, OldLineNum: -1},
		},
		LineMapping: &LineMapping{
			NewToOld: []int{1, -1, -1, 2}, // insertions at lines 2,3
			OldToNew: []int{1, 4},
		},
	}

	cluster := &ChangeCluster{
		StartLine: 2,
		EndLine:   3,
		Changes:   diff.Changes,
	}

	start, end := getClusterBufferRange(cluster, 1, diff)

	// Both insertions anchor to line 1, so range should be 1-1
	assert.True(t, start <= end, fmt.Sprintf("invalid range: start=%d > end=%d", start, end))
}

func TestCreateCompletionFromCluster_UnequalLineCounts(t *testing.T) {
	// Test that completion correctly extracts content when line counts differ
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			2: {Type: LineModification, LineNumber: 2, NewLineNum: 2, OldLineNum: 2, Content: "modified"},
			3: {Type: LineAddition, LineNumber: 3, NewLineNum: 3, OldLineNum: -1, Content: "inserted"},
		},
		OldLineCount: 3,
		NewLineCount: 4,
		LineMapping: &LineMapping{
			NewToOld: []int{1, 2, -1, 3},
			OldToNew: []int{1, 2, 4},
		},
	}

	cluster := &ChangeCluster{
		StartLine: 2,
		EndLine:   3,
		Changes:   diff.Changes,
	}

	newLines := []string{"line1", "modified", "inserted", "line3"}

	completion := createCompletionFromCluster(cluster, newLines, 1, diff)

	// Should have 2 lines (from new line 2 to new line 3)
	assert.Equal(t, 2, len(completion.Lines), "completion line count")

	// Verify content
	assert.Equal(t, "modified", completion.Lines[0], "first completion line")
	assert.Equal(t, "inserted", completion.Lines[1], "second completion line")
}

// =============================================================================
// Edge case tests for staging with extreme coordinate scenarios
// =============================================================================

func TestGetBufferLineForChange_DeletionAtLine1(t *testing.T) {
	// Edge case: deletion at line 1 with no preceding anchor
	mapping := &LineMapping{
		NewToOld: []int{2, 3}, // old line 1 deleted
		OldToNew: []int{-1, 1, 2},
	}

	change := LineDiff{
		Type:       LineDeletion,
		OldLineNum: 1,
		NewLineNum: -1, // no new line for deletions
	}

	bufferLine := getBufferLineForChange(change, 1, 1, mapping)

	// Should use OldLineNum directly: 1 + 1 - 1 = 1
	assert.Equal(t, 1, bufferLine, "buffer line for deletion at line 1")
}

func TestGetBufferLineForChange_InsertionWithNoAnchor(t *testing.T) {
	// Edge case: insertion at line 1 with no preceding mapped line
	mapping := &LineMapping{
		NewToOld: []int{-1, 1}, // new line 1 is insertion, new line 2 maps to old line 1
		OldToNew: []int{2},
	}

	change := LineDiff{
		Type:       LineAddition,
		NewLineNum: 1,
		OldLineNum: -1, // pure insertion
	}

	bufferLine := getBufferLineForChange(change, 1, 1, mapping)

	// No anchor found, should fallback to mapKey: 1 + 1 - 1 = 1
	assert.Equal(t, 1, bufferLine, "buffer line for insertion at line 1 with no anchor")
}

func TestCreateStages_CumulativeOffsetScenario(t *testing.T) {
	// Scenario: first stage increases line count, affecting second stage coordinates
	// This tests the scenario where cumulative offset might exceed remaining line count
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			2:  {Type: LineAddition, LineNumber: 2, NewLineNum: 2, OldLineNum: 1, Content: "insert1"},
			3:  {Type: LineAddition, LineNumber: 3, NewLineNum: 3, OldLineNum: 1, Content: "insert2"},
			20: {Type: LineModification, LineNumber: 20, NewLineNum: 22, OldLineNum: 20, Content: "modified"},
		},
		OldLineCount: 25,
		NewLineCount: 27, // +2 from insertions
		LineMapping: &LineMapping{
			NewToOld: make([]int, 27),
			OldToNew: make([]int, 25),
		},
	}
	// Initialize mappings
	for i := range diff.LineMapping.OldToNew {
		diff.LineMapping.OldToNew[i] = i + 3 // offset by 2 after line 1
	}
	diff.LineMapping.OldToNew[0] = 1
	for i := range diff.LineMapping.NewToOld {
		switch i {
		case 0:
			diff.LineMapping.NewToOld[i] = 1
		case 1, 2:
			diff.LineMapping.NewToOld[i] = -1 // insertions
		default:
			diff.LineMapping.NewToOld[i] = i - 1
		}
	}

	newLines := make([]string, 27)
	for i := range newLines {
		newLines[i] = fmt.Sprintf("line%d", i+1)
	}

	// Large gap (20-3=17), should create 2 stages
	stages := CreateStages(diff, 1, 1, 50, 1, 3, "test.go", newLines)

	assert.Len(t, 2, stages, "stages for insertions + distant modification")

	// Verify both stages have valid completions
	for i, stage := range stages {
		assert.NotNil(t, stage.Completion, fmt.Sprintf("stage %d completion", i))
		assert.True(t, stage.Completion.StartLine > 0, fmt.Sprintf("stage %d has valid StartLine", i))
	}
}

func TestCreateStages_AllDeletions(t *testing.T) {
	// Edge case: completion that only contains deletions
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			2: {Type: LineDeletion, LineNumber: 2, OldLineNum: 2, NewLineNum: -1, Content: "deleted1"},
			3: {Type: LineDeletion, LineNumber: 3, OldLineNum: 3, NewLineNum: -1, Content: "deleted2"},
		},
		OldLineCount: 5,
		NewLineCount: 3,
		LineMapping: &LineMapping{
			NewToOld: []int{1, 4, 5},
			OldToNew: []int{1, -1, -1, 2, 3},
		},
	}
	newLines := []string{"line1", "line4", "line5"}

	// Deletions are at same location, should form single cluster
	stages := CreateStages(diff, 1, 1, 50, 1, 3, "test.go", newLines)

	// Single cluster = nil (no staging needed)
	assert.Nil(t, stages, "single cluster of deletions")
}

func TestGetClusterBufferRange_AllInsertions(t *testing.T) {
	// Cluster containing only insertions (all OldLineNum = -1)
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			2: {Type: LineAddition, LineNumber: 2, NewLineNum: 2, OldLineNum: -1},
			3: {Type: LineAddition, LineNumber: 3, NewLineNum: 3, OldLineNum: -1},
			4: {Type: LineAddition, LineNumber: 4, NewLineNum: 4, OldLineNum: -1},
		},
		LineMapping: &LineMapping{
			NewToOld: []int{1, -1, -1, -1, 2}, // lines 2,3,4 are insertions
			OldToNew: []int{1, 5},
		},
	}

	cluster := &ChangeCluster{
		StartLine: 2,
		EndLine:   4,
		Changes:   diff.Changes,
	}

	start, end := getClusterBufferRange(cluster, 1, diff)

	// All insertions anchor to line 1 (the mapped line before them)
	assert.True(t, start <= end, fmt.Sprintf("valid range: start=%d end=%d", start, end))
	assert.Equal(t, 1, start, "insertions anchor to line 1")
}

// =============================================================================
// Test for the bug: visual groups containing lines outside stage's content
// =============================================================================

func TestStageVisualGroups_ShouldNotExceedStageContent(t *testing.T) {
	// This test reproduces the bug from cursortab.2.log where:
	// - Full completion has 54 lines
	// - First stage has 17 lines
	// - But the stage's VisualGroups contained a group at lines 41-54
	//
	// The stage's visual groups should ONLY reference lines within the stage's content.

	// Create a diff that mimics the log scenario:
	// - Original: 3 lines (buffer lines 37-39)
	// - New: 54 lines (full completion)
	// - Changes at various line numbers including high ones (41-54)
	changes := make(map[int]LineDiff)

	// Add changes at low line numbers (will be in first cluster)
	for i := 1; i <= 17; i++ {
		changes[i] = LineDiff{
			Type:       LineAddition,
			LineNumber: i,
			NewLineNum: i,
			OldLineNum: -1,
			Content:    fmt.Sprintf("line%d", i),
		}
	}

	// Add changes at high line numbers (should be in separate cluster)
	for i := 41; i <= 54; i++ {
		changes[i] = LineDiff{
			Type:       LineAddition,
			LineNumber: i,
			NewLineNum: i,
			OldLineNum: -1,
			Content:    fmt.Sprintf("line%d", i),
		}
	}

	diff := &DiffResult{
		Changes:      changes,
		OldLineCount: 3,
		NewLineCount: 54,
	}

	// Create newLines for full completion
	newLines := make([]string, 54)
	for i := range newLines {
		newLines[i] = fmt.Sprintf("content%d", i+1)
	}

	// Create stages with proximity threshold of 3
	// Gap between line 17 and 41 is 24, so they should be in separate clusters
	stages := CreateStages(diff, 1, 1, 100, 1, 3, "test.go", newLines)

	// Should have at least 2 stages (may have more due to viewport partitioning)
	assert.True(t, len(stages) >= 2, fmt.Sprintf("should have at least 2 stages, got %d", len(stages)))

	// CRITICAL: For ALL stages, VisualGroups should NOT reference lines
	// beyond the stage's content line count
	for i, stage := range stages {
		stageLineCount := len(stage.Completion.Lines)

		for _, vg := range stage.VisualGroups {
			assert.True(t, vg.StartLine <= stageLineCount,
				fmt.Sprintf("Stage %d: VisualGroup StartLine (%d) exceeds stage content line count (%d)",
					i, vg.StartLine, stageLineCount))
			assert.True(t, vg.EndLine <= stageLineCount,
				fmt.Sprintf("Stage %d: VisualGroup EndLine (%d) exceeds stage content line count (%d)",
					i, vg.EndLine, stageLineCount))
		}
	}
}

func TestComputeVisualGroups_ShouldNotReferenceOutOfBoundsLines(t *testing.T) {
	// Test that computeVisualGroups doesn't create groups with lines beyond newLines length

	// Cluster has changes at lines 1-5 and 41-45
	// But newLines only has 10 entries
	changes := map[int]LineDiff{
		1:  {Type: LineAddition, LineNumber: 1, NewLineNum: 1, Content: "line1"},
		2:  {Type: LineAddition, LineNumber: 2, NewLineNum: 2, Content: "line2"},
		3:  {Type: LineAddition, LineNumber: 3, NewLineNum: 3, Content: "line3"},
		41: {Type: LineAddition, LineNumber: 41, NewLineNum: 41, Content: "line41"},
		42: {Type: LineAddition, LineNumber: 42, NewLineNum: 42, Content: "line42"},
	}

	newLines := make([]string, 10) // Only 10 lines
	for i := range newLines {
		newLines[i] = fmt.Sprintf("content%d", i+1)
	}
	oldLines := make([]string, 10)

	groups := computeVisualGroups(changes, newLines, oldLines)

	// Visual groups should only reference lines within newLines bounds
	for _, vg := range groups {
		assert.True(t, vg.StartLine <= len(newLines),
			fmt.Sprintf("VisualGroup StartLine (%d) exceeds newLines length (%d)",
				vg.StartLine, len(newLines)))
		assert.True(t, vg.EndLine <= len(newLines),
			fmt.Sprintf("VisualGroup EndLine (%d) exceeds newLines length (%d)",
				vg.EndLine, len(newLines)))

		// Group's Lines should not be empty if StartLine is within bounds
		if vg.StartLine <= len(newLines) {
			assert.True(t, len(vg.Lines) > 0,
				fmt.Sprintf("VisualGroup at line %d has empty Lines", vg.StartLine))
		}
	}
}

func TestBuildStagesFromClusters_VisualGroupsUseClusterNewLines(t *testing.T) {
	// Test that buildStagesFromClusters computes visual groups using
	// only the lines relevant to each cluster, not the full newLines

	// Create a cluster with changes at lines 1-3
	cluster := &ChangeCluster{
		StartLine: 1,
		EndLine:   3,
		Changes: map[int]LineDiff{
			1: {Type: LineAddition, LineNumber: 1, NewLineNum: 1, OldLineNum: -1, Content: "new1"},
			2: {Type: LineAddition, LineNumber: 2, NewLineNum: 2, OldLineNum: -1, Content: "new2"},
			3: {Type: LineAddition, LineNumber: 3, NewLineNum: 3, OldLineNum: -1, Content: "new3"},
		},
	}

	// Full newLines has 50 entries, but the cluster only uses lines 1-3
	fullNewLines := make([]string, 50)
	for i := range fullNewLines {
		fullNewLines[i] = fmt.Sprintf("fullcontent%d", i+1)
	}

	diff := &DiffResult{
		Changes:      cluster.Changes,
		OldLineCount: 3,
		NewLineCount: 50,
	}

	stages := buildStagesFromClusters([]*ChangeCluster{cluster}, fullNewLines, "test.go", 1, diff)

	assert.Len(t, 1, stages, "should have 1 stage")

	stage := stages[0]

	// The stage's completion should only have 3 lines
	assert.Equal(t, 3, len(stage.Completion.Lines), "stage completion should have 3 lines")

	// The visual groups should only reference lines 1-3
	for _, vg := range stage.VisualGroups {
		assert.True(t, vg.EndLine <= 3,
			fmt.Sprintf("VisualGroup EndLine (%d) should be <= 3 (stage content size)", vg.EndLine))
	}
}
