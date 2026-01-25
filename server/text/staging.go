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
	// Use OldLineNum for buffer coordinate calculation (where change appears in current buffer)
	var inViewChanges, outViewChanges []int // map keys (line numbers)
	for lineNum, change := range diff.Changes {
		// Calculate buffer line using OldLineNum if available, otherwise use the map key
		bufferLine := getBufferLineForChange(change, lineNum, baseLineOffset, diff.LineMapping)
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
		distI := clusterDistanceFromCursor(allClusters[i], cursorRow, baseLineOffset, diff)
		distJ := clusterDistanceFromCursor(allClusters[j], cursorRow, baseLineOffset, diff)
		if distI != distJ {
			return distI < distJ
		}
		return allClusters[i].StartLine < allClusters[j].StartLine
	})

	// Step 4: Create stages from clusters
	return buildStagesFromClusters(allClusters, newLines, filePath, baseLineOffset, diff)
}

// getBufferLineForChange calculates the buffer line for a change using the appropriate coordinate.
// For modifications/deletions, uses OldLineNum (where it exists in current buffer).
// For pure additions, uses the anchor point from the mapping.
func getBufferLineForChange(change LineDiff, mapKey int, baseLineOffset int, mapping *LineMapping) int {
	// If OldLineNum is set, use it directly (modifications, deletions)
	if change.OldLineNum > 0 {
		return change.OldLineNum + baseLineOffset - 1
	}

	// For pure additions, use the mapping to find the anchor point
	if mapping != nil && change.NewLineNum > 0 && change.NewLineNum <= len(mapping.NewToOld) {
		oldLine := mapping.NewToOld[change.NewLineNum-1]
		if oldLine > 0 {
			return oldLine + baseLineOffset - 1
		}
		// No direct mapping - find nearest mapped old line
		// Look backwards for nearest mapped line
		for i := change.NewLineNum - 2; i >= 0; i-- {
			if mapping.NewToOld[i] > 0 {
				return mapping.NewToOld[i] + baseLineOffset - 1
			}
		}
	}

	// Fallback: use mapKey (backward compatibility)
	return mapKey + baseLineOffset - 1
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

// ChangeCluster represents a group of nearby changes (within threshold lines)
type ChangeCluster struct {
	StartLine int             // First line with changes (1-indexed)
	EndLine   int             // Last line with changes (1-indexed)
	Changes   map[int]LineDiff // Map of line number to diff operation
}

// clusterDistanceFromCursor calculates the minimum distance from cursor to a cluster,
// using the coordinate mapping for accurate buffer positions.
func clusterDistanceFromCursor(cluster *ChangeCluster, cursorRow int, baseLineOffset int, diff *DiffResult) int {
	// Find the buffer line range for this cluster using the mapping
	bufferStartLine, bufferEndLine := getClusterBufferRange(cluster, baseLineOffset, diff)

	if cursorRow >= bufferStartLine && cursorRow <= bufferEndLine {
		return 0 // Cursor is within the cluster
	}
	if cursorRow < bufferStartLine {
		return bufferStartLine - cursorRow
	}
	return cursorRow - bufferEndLine
}

// getClusterBufferRange determines the buffer line range for a cluster using coordinate mapping.
// Returns (startLine, endLine) in buffer coordinates.
func getClusterBufferRange(cluster *ChangeCluster, baseLineOffset int, diff *DiffResult) (int, int) {
	minOldLine := -1
	maxOldLine := -1

	for lineNum, change := range cluster.Changes {
		bufferLine := getBufferLineForChange(change, lineNum, baseLineOffset, diff.LineMapping)

		if minOldLine == -1 || bufferLine < minOldLine {
			minOldLine = bufferLine
		}
		if bufferLine > maxOldLine {
			maxOldLine = bufferLine
		}

		// For group types, also consider EndLine
		if change.Type == LineModificationGroup || change.Type == LineAdditionGroup {
			endBufferLine := change.EndLine + baseLineOffset - 1
			if endBufferLine > maxOldLine {
				maxOldLine = endBufferLine
			}
		}
	}

	// Fallback if no valid range found
	if minOldLine == -1 {
		minOldLine = cluster.StartLine + baseLineOffset - 1
	}
	if maxOldLine == -1 {
		maxOldLine = cluster.EndLine + baseLineOffset - 1
	}

	return minOldLine, maxOldLine
}

// getClusterNewLineRange determines the new line range for content extraction.
// Returns (startLine, endLine) in new-text coordinates (1-indexed).
func getClusterNewLineRange(cluster *ChangeCluster) (int, int) {
	minNewLine := -1
	maxNewLine := -1

	for _, change := range cluster.Changes {
		if change.NewLineNum > 0 {
			if minNewLine == -1 || change.NewLineNum < minNewLine {
				minNewLine = change.NewLineNum
			}
			if change.NewLineNum > maxNewLine {
				maxNewLine = change.NewLineNum
			}
		}

		// For group types, also consider EndLine (in new coordinates)
		if change.Type == LineModificationGroup || change.Type == LineAdditionGroup {
			if change.EndLine > maxNewLine {
				maxNewLine = change.EndLine
			}
		}
	}

	// Fallback to cluster coordinates
	if minNewLine == -1 {
		minNewLine = cluster.StartLine
	}
	if maxNewLine == -1 {
		maxNewLine = cluster.EndLine
	}

	return minNewLine, maxNewLine
}

// createCompletionFromCluster creates a Completion from a cluster of changes.
// Uses separate old/new coordinates to handle unequal line counts correctly.
func createCompletionFromCluster(cluster *ChangeCluster, newLines []string, baseLineOffset int, diff *DiffResult) *types.Completion {
	// Get buffer range (old coordinates) for StartLine/EndLineInc
	bufferStartLine, bufferEndLine := getClusterBufferRange(cluster, baseLineOffset, diff)

	// Get new line range for content extraction
	newStartLine, newEndLine := getClusterNewLineRange(cluster)

	// Extract the new content using new coordinates
	var lines []string
	for i := newStartLine; i <= newEndLine && i-1 < len(newLines); i++ {
		if i > 0 {
			lines = append(lines, newLines[i-1])
		}
	}

	// Handle pure deletions (no new content)
	if len(lines) == 0 && bufferStartLine > 0 && bufferEndLine >= bufferStartLine {
		// This is a deletion - Lines stays empty, which means delete the range
	}

	return &types.Completion{
		StartLine:  bufferStartLine,
		EndLineInc: bufferEndLine,
		Lines:      lines,
	}
}

// buildStagesFromClusters creates CompletionStages from clusters.
func buildStagesFromClusters(clusters []*ChangeCluster, newLines []string, filePath string, baseLineOffset int, diff *DiffResult) []*types.CompletionStage {
	var stages []*types.CompletionStage

	for i, cluster := range clusters {
		isLastStage := i == len(clusters)-1

		// Create completion using mapping for correct coordinates
		completion := createCompletionFromCluster(cluster, newLines, baseLineOffset, diff)

		// Create cursor target
		var cursorTarget *types.CursorPredictionTarget
		if isLastStage {
			// Last stage: cursor target to end of this cluster with retrigger
			_, bufferEndLine := getClusterBufferRange(cluster, baseLineOffset, diff)
			cursorTarget = &types.CursorPredictionTarget{
				RelativePath:    filePath,
				LineNumber:      int32(bufferEndLine),
				ShouldRetrigger: true,
			}
		} else {
			// Not last stage: cursor target to the start of the next cluster
			nextCluster := clusters[i+1]
			nextBufferStart, _ := getClusterBufferRange(nextCluster, baseLineOffset, diff)
			cursorTarget = &types.CursorPredictionTarget{
				RelativePath:    filePath,
				LineNumber:      int32(nextBufferStart),
				ShouldRetrigger: false,
			}
		}

		// Compute visual groups for this cluster's changes
		// Use the stage's lines (completion.Lines) rather than full newLines to ensure
		// visual groups only reference lines within the stage's content
		stageLines := completion.Lines
		oldLinesForCluster := make([]string, len(stageLines))

		// Get the new line range for this cluster to calculate relative line numbers
		newStartLine, _ := getClusterNewLineRange(cluster)

		for lineNum, change := range cluster.Changes {
			// Use NewLineNum if available for proper alignment
			newLineNum := lineNum
			if change.NewLineNum > 0 {
				newLineNum = change.NewLineNum
			}
			// Convert to relative index within the stage's content
			relativeIdx := newLineNum - newStartLine
			if relativeIdx >= 0 && relativeIdx < len(oldLinesForCluster) {
				oldLinesForCluster[relativeIdx] = change.OldContent
			}
		}

		// Create a remapped changes map with line numbers relative to the stage content
		remappedChanges := make(map[int]LineDiff)
		for lineNum, change := range cluster.Changes {
			newLineNum := lineNum
			if change.NewLineNum > 0 {
				newLineNum = change.NewLineNum
			}
			// Convert to relative line number (1-indexed within stage content)
			relativeLine := newLineNum - newStartLine + 1
			if relativeLine > 0 && relativeLine <= len(stageLines) {
				remappedChange := change
				remappedChange.LineNumber = relativeLine
				remappedChange.NewLineNum = relativeLine
				remappedChanges[relativeLine] = remappedChange
			}
		}

		visualGroups := computeVisualGroups(remappedChanges, stageLines, oldLinesForCluster)

		stages = append(stages, &types.CompletionStage{
			Completion:   completion,
			CursorTarget: cursorTarget,
			IsLastStage:  isLastStage,
			VisualGroups: visualGroups,
		})
	}

	return stages
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

	// Get sorted line numbers, filtering out any that exceed newLines bounds
	var lineNumbers []int
	for ln := range changes {
		// Only include line numbers that are within the available content
		if ln > 0 && ln <= len(newLines) {
			lineNumbers = append(lineNumbers, ln)
		}
	}
	if len(lineNumbers) == 0 {
		return nil
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
