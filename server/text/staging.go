package text

import (
	"cursortab/types"
	"sort"
	"strings"
)

// CreateStages is the main entry point for creating stages from a diff result.
// It handles viewport partitioning, proximity grouping, and cursor distance sorting.
// Returns nil if no staging is needed (single stage or no changes).
//
// Parameters:
//   - diff: The diff result (can be grouped or ungrouped - this function handles both)
//   - cursorRow: Current cursor position (1-indexed buffer coordinate)
//   - viewportTop, viewportBottom: Visible viewport (1-indexed buffer coordinates)
//   - baseLineOffset: Where the diff range starts in the buffer (1-indexed)
//   - proximityThreshold: Max gap between changes to be in same stage
//   - filePath: File path for cursor targets
//   - newLines: New content lines for extracting stage content
//
// Returns stages sorted by cursor distance, or nil if ≤1 stage needed.
func CreateStages(
	diff *DiffResult,
	cursorRow int,
	viewportTop, viewportBottom int,
	baseLineOffset int,
	proximityThreshold int,
	filePath string,
	newLines []string,
) []*types.CompletionStage {
	if len(diff.Changes) == 0 {
		return nil
	}

	// Step 1: Partition changes by viewport visibility
	var inViewChanges, outViewChanges []int // line numbers
	for lineNum, change := range diff.Changes {
		bufferLine := lineNum + baseLineOffset - 1
		endBufferLine := bufferLine

		// For group types (if diff is already grouped), use EndLine
		if change.Type == LineModificationGroup || change.Type == LineAdditionGroup {
			endBufferLine = change.EndLine + baseLineOffset - 1
		}

		// A change is visible if its entire range is within viewport
		isVisible := viewportTop == 0 && viewportBottom == 0 || // No viewport info = all visible
			(bufferLine >= viewportTop && endBufferLine <= viewportBottom)

		if isVisible {
			inViewChanges = append(inViewChanges, lineNum)
		} else {
			outViewChanges = append(outViewChanges, lineNum)
		}
	}

	// Sort both partitions
	sort.Ints(inViewChanges)
	sort.Ints(outViewChanges)

	// Step 2: Group changes by proximity within each partition
	inViewClusters := groupChangesByProximity(diff, inViewChanges, proximityThreshold)
	outViewClusters := groupChangesByProximity(diff, outViewChanges, proximityThreshold)

	// Combine: in-view first, then out-of-view
	allClusters := append(inViewClusters, outViewClusters...)

	// If only 1 cluster (or none), no staging needed
	if len(allClusters) <= 1 {
		return nil
	}

	// Step 3: Sort clusters by cursor distance
	sort.SliceStable(allClusters, func(i, j int) bool {
		distI := clusterDistanceFromCursor(allClusters[i], cursorRow, baseLineOffset)
		distJ := clusterDistanceFromCursor(allClusters[j], cursorRow, baseLineOffset)
		if distI != distJ {
			return distI < distJ
		}
		return allClusters[i].StartLine < allClusters[j].StartLine
	})

	// Step 4: Create stages from clusters
	return buildStagesFromClusters(allClusters, newLines, filePath, baseLineOffset)
}

// groupChangesByProximity groups sorted line numbers into clusters based on proximity.
// Changes within proximityThreshold lines of each other are grouped together.
func groupChangesByProximity(diff *DiffResult, lineNumbers []int, proximityThreshold int) []*ChangeCluster {
	if len(lineNumbers) == 0 {
		return nil
	}

	var clusters []*ChangeCluster
	var currentCluster *ChangeCluster

	for _, lineNum := range lineNumbers {
		change := diff.Changes[lineNum]

		// Get the end line for this change
		endLine := lineNum
		if change.Type == LineModificationGroup || change.Type == LineAdditionGroup {
			endLine = change.EndLine
		}

		if currentCluster == nil {
			// Start new cluster
			currentCluster = &ChangeCluster{
				StartLine: lineNum,
				EndLine:   endLine,
				Changes:   make(map[int]LineDiff),
			}
			currentCluster.Changes[lineNum] = change
		} else {
			// Check if this change is within threshold of current cluster
			// Gap is the number of lines between the end of current cluster and this change
			// e.g., lines 47 and 50 have gap = 50 - 47 = 3
			gap := lineNum - currentCluster.EndLine
			if gap <= proximityThreshold {
				// Add to current cluster
				currentCluster.Changes[lineNum] = change
				if endLine > currentCluster.EndLine {
					currentCluster.EndLine = endLine
				}
			} else {
				// Gap too large - finalize current cluster and start new one
				clusters = append(clusters, currentCluster)
				currentCluster = &ChangeCluster{
					StartLine: lineNum,
					EndLine:   endLine,
					Changes:   make(map[int]LineDiff),
				}
				currentCluster.Changes[lineNum] = change
			}
		}
	}

	// Don't forget the last cluster
	if currentCluster != nil {
		clusters = append(clusters, currentCluster)
	}

	return clusters
}

// buildStagesFromClusters creates CompletionStages from clusters.
func buildStagesFromClusters(clusters []*ChangeCluster, newLines []string, filePath string, baseLineOffset int) []*types.CompletionStage {
	var stages []*types.CompletionStage

	for i, cluster := range clusters {
		isLastStage := i == len(clusters)-1

		// Create completion for this cluster
		completion := createCompletionFromCluster(cluster, newLines, baseLineOffset)

		// Create cursor target
		var cursorTarget *types.CursorPredictionTarget
		if isLastStage {
			// Last stage: cursor target to end of this cluster with retrigger
			bufferEndLine := cluster.EndLine + baseLineOffset - 1
			cursorTarget = &types.CursorPredictionTarget{
				RelativePath:    filePath,
				LineNumber:      int32(bufferEndLine),
				ShouldRetrigger: true,
			}
		} else {
			// Not last stage: cursor target to the start of the next cluster
			nextCluster := clusters[i+1]
			bufferStartLine := nextCluster.StartLine + baseLineOffset - 1
			cursorTarget = &types.CursorPredictionTarget{
				RelativePath:    filePath,
				LineNumber:      int32(bufferStartLine),
				ShouldRetrigger: false,
			}
		}

		// Compute visual groups for this cluster's changes
		// Build oldLinesForCluster aligned with newLines indices (not cluster line range)
		// This ensures computeVisualGroups receives correctly aligned data
		oldLinesForCluster := make([]string, len(newLines))
		for lineNum, change := range cluster.Changes {
			if lineNum-1 >= 0 && lineNum-1 < len(oldLinesForCluster) {
				oldLinesForCluster[lineNum-1] = change.OldContent
			}
		}
		visualGroups := computeVisualGroups(cluster.Changes, newLines, oldLinesForCluster)

		stages = append(stages, &types.CompletionStage{
			Completion:   completion,
			CursorTarget: cursorTarget,
			IsLastStage:  isLastStage,
			VisualGroups: visualGroups,
		})
	}

	return stages
}

// ChangeCluster represents a group of nearby changes (within threshold lines)
type ChangeCluster struct {
	StartLine int             // First line with changes (1-indexed)
	EndLine   int             // Last line with changes (1-indexed)
	Changes   map[int]LineDiff // Map of line number to diff operation
}

// clusterDistanceFromCursor calculates the minimum distance from cursor to a cluster
// baseLineOffset converts cluster-relative coordinates to buffer coordinates
func clusterDistanceFromCursor(cluster *ChangeCluster, cursorRow int, baseLineOffset int) int {
	// Convert cluster coordinates to buffer coordinates
	bufferStartLine := cluster.StartLine + baseLineOffset - 1
	bufferEndLine := cluster.EndLine + baseLineOffset - 1

	if cursorRow >= bufferStartLine && cursorRow <= bufferEndLine {
		return 0 // Cursor is within the cluster
	}
	if cursorRow < bufferStartLine {
		return bufferStartLine - cursorRow
	}
	return cursorRow - bufferEndLine
}

// createCompletionFromCluster creates a Completion from a cluster of changes
// It determines the minimal line range that needs to be replaced
// baseLineOffset converts cluster-relative coordinates to buffer coordinates
func createCompletionFromCluster(cluster *ChangeCluster, newLines []string, baseLineOffset int) *types.Completion {
	// Cluster coordinates are relative to the extracted text (1-indexed)
	// We need to map them back to buffer coordinates
	startLine := cluster.StartLine
	endLine := cluster.EndLine

	// Convert to buffer coordinates
	bufferStartLine := startLine + baseLineOffset - 1
	bufferEndLine := endLine + baseLineOffset - 1

	// Extract the new content for this range (using cluster-relative indices)
	var lines []string
	for i := startLine; i <= endLine && i-1 < len(newLines); i++ {
		lines = append(lines, newLines[i-1])
	}

	// Ensure we have at least the lines from startLine to endLine
	// If newLines is shorter, use empty strings (shouldn't happen in normal cases)
	for len(lines) < endLine-startLine+1 {
		lines = append(lines, "")
	}

	return &types.Completion{
		StartLine:  bufferStartLine,
		EndLineInc: bufferEndLine,
		Lines:      lines,
	}
}

// AnalyzeDiffForStagingWithViewport analyzes the diff with viewport-aware grouping
// viewportTop and viewportBottom are 1-indexed buffer line numbers
// baseLineOffset is the 1-indexed line number where the diff range starts in the buffer
func AnalyzeDiffForStagingWithViewport(originalText, newText string, viewportTop, viewportBottom, baseLineOffset int) *DiffResult {
	return analyzeDiffWithViewport(originalText, newText, viewportTop, viewportBottom, baseLineOffset)
}

// JoinLines joins a slice of strings with newlines
func JoinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// computeVisualGroups groups consecutive changes of the same type for UI rendering alignment
func computeVisualGroups(changes map[int]LineDiff, newLines, oldLines []string) []*types.VisualGroup {
	if len(changes) == 0 {
		return nil
	}

	// Get sorted line numbers
	var lineNumbers []int
	for ln := range changes {
		lineNumbers = append(lineNumbers, ln)
	}
	sort.Ints(lineNumbers)

	var groups []*types.VisualGroup
	var current *types.VisualGroup

	for _, ln := range lineNumbers {
		change := changes[ln]

		// Only group modifications and additions
		var groupType string
		switch change.Type {
		case LineModification, LineModificationGroup:
			groupType = "modification"
		case LineAddition, LineAdditionGroup:
			groupType = "addition"
		default:
			// Flush and skip non-groupable changes
			if current != nil {
				groups = append(groups, current)
				current = nil
			}
			continue
		}

		// Check if consecutive with current group of same type
		if current != nil && current.Type == groupType && ln == current.EndLine+1 {
			// Extend current group
			current.EndLine = ln
			if ln-1 < len(newLines) {
				current.Lines = append(current.Lines, newLines[ln-1])
			}
			if groupType == "modification" && ln-1 < len(oldLines) {
				current.OldLines = append(current.OldLines, oldLines[ln-1])
			}
		} else {
			// Flush current, start new
			if current != nil {
				groups = append(groups, current)
			}
			current = &types.VisualGroup{
				Type:      groupType,
				StartLine: ln,
				EndLine:   ln,
			}
			if ln-1 < len(newLines) {
				current.Lines = []string{newLines[ln-1]}
			}
			if groupType == "modification" && ln-1 < len(oldLines) {
				current.OldLines = []string{oldLines[ln-1]}
			}
		}
	}

	if current != nil {
		groups = append(groups, current)
	}

	return groups
}
