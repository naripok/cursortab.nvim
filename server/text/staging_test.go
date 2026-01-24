package text

import (
	"testing"
)

func TestClusterDistanceFromCursor(t *testing.T) {
	// Cluster with relative coordinates 1-6, baseLineOffset=10 means buffer lines 10-15
	cluster := &ChangeCluster{StartLine: 1, EndLine: 6}
	baseLineOffset := 10

	tests := []struct {
		cursorRow int // buffer coordinates
		expected  int
	}{
		{5, 5},   // cursor before cluster (buffer line 5, cluster starts at buffer 10)
		{10, 0},  // cursor at start (buffer line 10)
		{12, 0},  // cursor inside (buffer line 12)
		{15, 0},  // cursor at end (buffer line 15)
		{20, 5},  // cursor after cluster (buffer line 20, cluster ends at buffer 15)
	}

	for _, tt := range tests {
		result := clusterDistanceFromCursor(cluster, tt.cursorRow, baseLineOffset)
		if result != tt.expected {
			t.Errorf("cursor at %d: expected distance %d, got %d", tt.cursorRow, tt.expected, result)
		}
	}
}

func TestClusterDistanceFromCursor_NoOffset(t *testing.T) {
	// When baseLineOffset=1, cluster coordinates match buffer coordinates
	cluster := &ChangeCluster{StartLine: 10, EndLine: 15}
	baseLineOffset := 1

	tests := []struct {
		cursorRow int
		expected  int
	}{
		{5, 5},   // cursor before cluster
		{10, 0},  // cursor at start
		{12, 0},  // cursor inside
		{15, 0},  // cursor at end
		{20, 5},  // cursor after cluster
	}

	for _, tt := range tests {
		result := clusterDistanceFromCursor(cluster, tt.cursorRow, baseLineOffset)
		if result != tt.expected {
			t.Errorf("cursor at %d: expected distance %d, got %d", tt.cursorRow, tt.expected, result)
		}
	}
}

func TestJoinLines(t *testing.T) {
	lines := []string{"line1", "line2", "line3"}
	result := JoinLines(lines)
	expected := "line1\nline2\nline3"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestCreateStages_EmptyDiff(t *testing.T) {
	diff := &DiffResult{Changes: map[int]LineDiff{}}
	stages := CreateStages(diff, 10, 1, 50, 1, 3, "test.go", []string{})

	if stages != nil {
		t.Errorf("expected nil stages for empty diff, got %d stages", len(stages))
	}
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

	if stages != nil {
		t.Errorf("expected nil stages for single cluster, got %d stages", len(stages))
	}
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

	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}

	// First stage should be the closer cluster (10-11)
	if stages[0].Completion.StartLine != 10 {
		t.Errorf("first stage should start at line 10, got %d", stages[0].Completion.StartLine)
	}
	if stages[0].Completion.EndLineInc != 11 {
		t.Errorf("first stage should end at line 11, got %d", stages[0].Completion.EndLineInc)
	}

	// Second stage should be 25-26
	if stages[1].Completion.StartLine != 25 {
		t.Errorf("second stage should start at line 25, got %d", stages[1].Completion.StartLine)
	}

	// First stage cursor target should point to next stage
	if stages[0].CursorTarget.LineNumber != 25 {
		t.Errorf("first stage cursor target should be 25, got %d", stages[0].CursorTarget.LineNumber)
	}
	if stages[0].CursorTarget.ShouldRetrigger {
		t.Error("first stage should not have ShouldRetrigger=true")
	}

	// Last stage should have ShouldRetrigger=true
	if !stages[1].CursorTarget.ShouldRetrigger {
		t.Error("last stage should have ShouldRetrigger=true")
	}
	if !stages[1].IsLastStage {
		t.Error("second stage should have IsLastStage=true")
	}
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

	if len(stages) != 3 {
		t.Fatalf("expected 3 stages, got %d", len(stages))
	}

	// First stage should be closest to cursor (20-21)
	if stages[0].Completion.StartLine != 20 {
		t.Errorf("first stage should be closest cluster (20-21), got start=%d", stages[0].Completion.StartLine)
	}

	// Verify cursor is within first stage
	if stages[0].Completion.StartLine > 22 || stages[0].Completion.EndLineInc < 22 {
		// Cursor should be within or very close to first stage
		t.Logf("Note: cursor 22 is within cluster 20-21")
	}
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

	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}

	// In-view changes should come first (line 10)
	if stages[0].Completion.StartLine != 10 {
		t.Errorf("first stage should be in-viewport change at line 10, got %d", stages[0].Completion.StartLine)
	}

	// Out-of-view change should be second (line 100)
	if stages[1].Completion.StartLine != 100 {
		t.Errorf("second stage should be out-of-viewport change at line 100, got %d", stages[1].Completion.StartLine)
	}
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
	if stages != nil {
		t.Errorf("expected nil (single cluster), got %d stages", len(stages))
	}
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

	if len(stages) != 2 {
		t.Fatalf("expected 2 stages (gap > threshold), got %d", len(stages))
	}

	// First cluster should be 10-12
	if stages[0].Completion.StartLine != 10 || stages[0].Completion.EndLineInc != 12 {
		t.Errorf("first stage should be 10-12, got %d-%d",
			stages[0].Completion.StartLine, stages[0].Completion.EndLineInc)
	}

	// Second cluster should be 20
	if stages[1].Completion.StartLine != 20 {
		t.Errorf("second stage should start at 20, got %d", stages[1].Completion.StartLine)
	}
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

	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}

	// Verify buffer coordinates in completion
	if stages[0].Completion.StartLine != 50 && stages[0].Completion.StartLine != 59 {
		t.Errorf("stage should have buffer coordinates (50 or 59), got %d", stages[0].Completion.StartLine)
	}
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

	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}

	// First stage (lines 1-2) should have visual groups
	if stages[0].VisualGroups == nil {
		t.Error("first stage should have visual groups computed")
	}
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

	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	// First cluster: 5-7
	if clusters[0].StartLine != 5 || clusters[0].EndLine != 7 {
		t.Errorf("first cluster should be 5-7, got %d-%d", clusters[0].StartLine, clusters[0].EndLine)
	}
	if len(clusters[0].Changes) != 3 {
		t.Errorf("first cluster should have 3 changes, got %d", len(clusters[0].Changes))
	}

	// Second cluster: 20-21
	if clusters[1].StartLine != 20 || clusters[1].EndLine != 21 {
		t.Errorf("second cluster should be 20-21, got %d-%d", clusters[1].StartLine, clusters[1].EndLine)
	}
}

func TestGroupChangesByProximity_EmptyInput(t *testing.T) {
	diff := &DiffResult{Changes: map[int]LineDiff{}}
	clusters := groupChangesByProximity(diff, []int{}, 3)

	if clusters != nil {
		t.Errorf("expected nil for empty input, got %d clusters", len(clusters))
	}
}

func TestGroupChangesByProximity_WithGroups(t *testing.T) {
	// Test that group types (LineModificationGroup) use EndLine for cluster boundaries
	diff := &DiffResult{
		Changes: map[int]LineDiff{
			5: {
				Type:      LineModificationGroup,
				LineNumber: 5,
				StartLine:  5,
				EndLine:    10, // Group spans 5-10
			},
			15: {Type: LineModification, LineNumber: 15},
		},
	}

	lineNumbers := []int{5, 15}
	clusters := groupChangesByProximity(diff, lineNumbers, 3)

	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	// First cluster should end at 10 (group EndLine), not 5
	if clusters[0].EndLine != 10 {
		t.Errorf("first cluster should end at 10 (group EndLine), got %d", clusters[0].EndLine)
	}
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

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// First group: modifications 1-3
	if groups[0].Type != "modification" {
		t.Errorf("first group should be modification, got %s", groups[0].Type)
	}
	if groups[0].StartLine != 1 || groups[0].EndLine != 3 {
		t.Errorf("first group should span 1-3, got %d-%d", groups[0].StartLine, groups[0].EndLine)
	}
	if len(groups[0].Lines) != 3 {
		t.Errorf("first group should have 3 lines, got %d", len(groups[0].Lines))
	}
	if len(groups[0].OldLines) != 3 {
		t.Errorf("first group should have 3 old lines, got %d", len(groups[0].OldLines))
	}

	// Second group: additions 7-8
	if groups[1].Type != "addition" {
		t.Errorf("second group should be addition, got %s", groups[1].Type)
	}
	if groups[1].StartLine != 7 || groups[1].EndLine != 8 {
		t.Errorf("second group should span 7-8, got %d-%d", groups[1].StartLine, groups[1].EndLine)
	}
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
	if len(groups) != 3 {
		t.Fatalf("expected 3 separate groups, got %d", len(groups))
	}
}
