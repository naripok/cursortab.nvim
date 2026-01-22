package text

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// DiffType represents the type of diff operation
type DiffType int

const (
	LineDeletion DiffType = iota
	LineAddition
	LineModification
	LineAppendChars
	LineDeleteChars
	LineReplaceChars
	LineModificationGroup
	LineAdditionGroup
)

// String returns the string representation of DiffType for Lua integration
func (dt DiffType) String() string {
	switch dt {
	case LineDeletion:
		return "deletion"
	case LineAddition:
		return "addition"
	case LineModification:
		return "modification"
	case LineAppendChars:
		return "append_chars"
	case LineDeleteChars:
		return "delete_chars"
	case LineReplaceChars:
		return "replace_chars"
	case LineModificationGroup:
		return "modification_group"
	case LineAdditionGroup:
		return "addition_group"
	default:
		return "unknown"
	}
}

// LineDiff represents a line-level diff operation
type LineDiff struct {
	Type       DiffType
	LineNumber int      // one-indexed
	Content    string   // new content
	OldContent string   // For modifications to compare changes
	ColStart   int      // Start column (0-based) for character-level changes
	ColEnd     int      // End column (0-based) for character-level changes
	GroupLines []string // For group types: array of content lines in the group
	StartLine  int      // For group types: starting line number of the group (1-indexed)
	EndLine    int      // For group types: ending line number of the group (1-indexed)
	MaxOffset  int      // For modification groups: maximum left offset for positioning
}

// DiffResult contains all categorized diff operations mapped by line number
type DiffResult struct {
	Changes              map[int]LineDiff // Map of line number (1-indexed) to diff operation
	IsOnlyLineDeletion   bool             // True if the diff contains only deletions
	LastDeletion         int              // The line number (1-indexed) of the last deletion, -1 if no deletion
	LastAddition         int              // The line number (1-indexed) of the last addition, -1 if no addition
	LastLineModification int              // The line number (1-indexed) of the last line modification, -1 if no line modification
	LastAppendChars      int              // The line number (1-indexed) of the last append chars, -1 if no append chars
	LastDeleteChars      int              // The line number (1-indexed) of the last delete chars, -1 if no delete chars
	LastReplaceChars     int              // The line number (1-indexed) of the last replace chars, -1 if no replace chars
	CursorLine           int              // The optimal line (1-indexed) to position cursor, -1 if no positioning needed
	CursorCol            int              // The optimal column (0-indexed) to position cursor, -1 if no positioning needed
}

// analyzeDiff computes and categorizes line-level diffs between two texts
func analyzeDiff(text1, text2 string) *DiffResult {
	result := &DiffResult{
		Changes: make(map[int]LineDiff),
	}

	// Use line-level diff to get the basic diff operations
	dmp := diffmatchpatch.New()
	chars1, chars2, lineArray := dmp.DiffLinesToChars(text1, text2)
	diffs := dmp.DiffMain(chars1, chars2, false)
	lineDiffs := dmp.DiffCharsToLines(diffs, lineArray)

	// Process the line diffs and intelligently merge delete+insert into modifications
	processLineDiffs(lineDiffs, result)

	// Apply grouping logic to consecutive modifications and additions
	applyGrouping(result, text1)

	// Process changes and calculate summary properties (after grouping)
	processChangesSummary(result)

	// Calculate optimal cursor position based on diff results
	calculateCursorPosition(result, text2)

	return result
}

// processChangesSummary processes all changes and calculates summary properties
func processChangesSummary(result *DiffResult) {
	// Initialize all properties to -1
	result.LastDeletion = -1
	result.LastAddition = -1
	result.LastLineModification = -1
	result.LastDeleteChars = -1
	result.LastReplaceChars = -1
	result.LastAppendChars = -1
	result.CursorLine = -1
	result.CursorCol = -1

	onlyDeletions := len(result.Changes) > 0
	for lineNum, change := range result.Changes {
		// IsOnlyDeletion
		if change.Type != LineDeletion {
			onlyDeletions = false
		}
		// LastDeletion - find the maximum line number
		if change.Type == LineDeletion && lineNum > result.LastDeletion {
			result.LastDeletion = lineNum
		}
		// LastAddition - find the maximum line number (for groups, use the first line)
		if (change.Type == LineAddition || change.Type == LineAdditionGroup) && lineNum > result.LastAddition {
			result.LastAddition = lineNum
		}
		// LastLineModification - find the maximum line number (for groups, use the first line)
		if (change.Type == LineModification || change.Type == LineModificationGroup) && lineNum > result.LastLineModification {
			result.LastLineModification = lineNum
		}
		// LastDeleteChars - find the maximum line number
		if change.Type == LineDeleteChars && lineNum > result.LastDeleteChars {
			result.LastDeleteChars = lineNum
		}
		// LastReplaceChars - find the maximum line number
		if change.Type == LineReplaceChars && lineNum > result.LastReplaceChars {
			result.LastReplaceChars = lineNum
		}
		// LastAppendChars - find the maximum line number
		if change.Type == LineAppendChars && lineNum > result.LastAppendChars {
			result.LastAppendChars = lineNum
		}
	}
	result.IsOnlyLineDeletion = onlyDeletions
}

// calculateCursorPosition determines optimal cursor positioning based on diff results
func calculateCursorPosition(result *DiffResult, newText string) {
	// Don't position cursor for pure deletions
	if result.IsOnlyLineDeletion {
		return
	}

	// Don't position cursor when there are no changes at all
	if len(result.Changes) == 0 {
		return
	}

	newLines := strings.Split(newText, "\n")

	// Priority order: modifications > additions > other changes
	var targetLine int = -1

	if result.LastLineModification != -1 {
		targetLine = result.LastLineModification
	} else if result.LastAddition != -1 {
		targetLine = result.LastAddition
	} else if result.LastAppendChars != -1 {
		targetLine = result.LastAppendChars
	} else if result.LastReplaceChars != -1 {
		targetLine = result.LastReplaceChars
	} else if result.LastDeleteChars != -1 {
		targetLine = result.LastDeleteChars
	} else if result.LastDeletion != -1 {
		targetLine = result.LastDeletion
	} else if len(newLines) > 0 {
		// Default to end of completion when there are changes
		targetLine = len(newLines)
	}

	// For group types, adjust target line to be the end of the group
	if targetLine > 0 {
		if change, exists := result.Changes[targetLine]; exists {
			if change.Type == LineModificationGroup || change.Type == LineAdditionGroup {
				targetLine = change.EndLine
			}
		}
	}

	// Set cursor position
	if targetLine > 0 && targetLine <= len(newLines) {
		result.CursorLine = targetLine
		// Position at end of the target line
		if targetLine-1 < len(newLines) {
			result.CursorCol = len(newLines[targetLine-1])
		}
	}
}

// applyGrouping identifies consecutive modifications and additions and groups them
func applyGrouping(result *DiffResult, oldText string) {
	oldLines := strings.Split(oldText, "\n")

	// Get sorted line numbers for processing
	var lineNumbers []int
	for lineNum := range result.Changes {
		lineNumbers = append(lineNumbers, lineNum)
	}

	// Sort line numbers
	for i := 0; i < len(lineNumbers); i++ {
		for j := i + 1; j < len(lineNumbers); j++ {
			if lineNumbers[i] > lineNumbers[j] {
				lineNumbers[i], lineNumbers[j] = lineNumbers[j], lineNumbers[i]
			}
		}
	}

	if len(lineNumbers) == 0 {
		return
	}

	// Find consecutive groups
	groupedChanges := make(map[int]LineDiff)
	i := 0

	for i < len(lineNumbers) {
		lineNum := lineNumbers[i]
		change := result.Changes[lineNum]

		// Check if this change should be grouped
		if change.Type == LineModification || change.Type == LineAddition {
			// Look for consecutive changes of the same type
			groupStart := i
			groupEnd := i
			groupType := change.Type

			// Find end of consecutive group
			for j := i + 1; j < len(lineNumbers); j++ {
				nextLineNum := lineNumbers[j]
				nextChange := result.Changes[nextLineNum]

				// Check if consecutive and same type
				if nextLineNum == lineNumbers[j-1]+1 && nextChange.Type == groupType {
					groupEnd = j
				} else {
					break
				}
			}

			// If we have multiple consecutive changes, create a group
			if groupEnd > groupStart {
				createGroup(result, lineNumbers[groupStart:groupEnd+1], groupType, oldLines, groupedChanges)
				i = groupEnd + 1
			} else {
				// Single change, keep as is
				groupedChanges[lineNum] = change
				i++
			}
		} else {
			// Not groupable, keep as is
			groupedChanges[lineNum] = change
			i++
		}
	}

	// Replace the changes with grouped changes
	result.Changes = groupedChanges
}

// createGroup creates a group change from consecutive individual changes
func createGroup(result *DiffResult, lineNumbers []int, groupType DiffType, oldLines []string, groupedChanges map[int]LineDiff) {
	if len(lineNumbers) == 0 {
		return
	}

	startLine := lineNumbers[0]
	endLine := lineNumbers[len(lineNumbers)-1]

	// Collect group content
	var groupLines []string
	var maxOffset int

	for _, lineNum := range lineNumbers {
		change := result.Changes[lineNum]
		groupLines = append(groupLines, change.Content)

		// For modification groups, calculate max offset based on old content
		if groupType == LineModification {
			if lineNum-1 < len(oldLines) {
				lineWidth := len(oldLines[lineNum-1])
				if lineWidth > maxOffset {
					maxOffset = lineWidth
				}
			}
		}
	}

	// Determine group type
	var finalGroupType DiffType
	if groupType == LineModification {
		finalGroupType = LineModificationGroup
	} else {
		finalGroupType = LineAdditionGroup
	}

	// Create the group change
	groupChange := LineDiff{
		Type:       finalGroupType,
		LineNumber: startLine,                      // Use first line as primary line number
		Content:    strings.Join(groupLines, "\n"), // Join all content
		GroupLines: groupLines,
		StartLine:  startLine,
		EndLine:    endLine,
		MaxOffset:  maxOffset,
	}

	// Add the group change (using the first line number as key)
	groupedChanges[startLine] = groupChange
}

// processLineDiffs processes line-level diffs and intelligently categorizes them
func processLineDiffs(lineDiffs []diffmatchpatch.Diff, result *DiffResult) {
	oldLineNum := 0
	newLineNum := 0
	i := 0

	for i < len(lineDiffs) {
		diff := lineDiffs[i]
		lines := strings.Split(diff.Text, "\n")
		// Remove empty last element if text ends with newline
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		switch diff.Type {
		case diffmatchpatch.DiffEqual:
			// Equal lines, just advance counters
			oldLineNum += len(lines)
			newLineNum += len(lines)
			i++

		case diffmatchpatch.DiffDelete:
			// Check if this is followed by an insert - potential modification
			if i+1 < len(lineDiffs) && lineDiffs[i+1].Type == diffmatchpatch.DiffInsert {
				// This is a delete followed by insert - treat as modification(s)
				insertDiff := lineDiffs[i+1]
				insertLines := strings.Split(insertDiff.Text, "\n")
				if len(insertLines) > 0 && insertLines[len(insertLines)-1] == "" {
					insertLines = insertLines[:len(insertLines)-1]
				}

				// Handle the modification(s)
				handleModifications(lines, insertLines, oldLineNum, newLineNum, result)

				oldLineNum += len(lines)
				newLineNum += len(insertLines)
				i += 2 // Skip both delete and insert
			} else {
				// Pure deletion
				for j, line := range lines {
					lineNum := oldLineNum + j + 1
					result.Changes[lineNum] = LineDiff{
						Type:       LineDeletion,
						LineNumber: lineNum,
						Content:    line,
					}
				}
				oldLineNum += len(lines)
				i++
			}

		case diffmatchpatch.DiffInsert:
			// Pure addition (not preceded by delete)
			for j, line := range lines {
				lineNum := newLineNum + j + 1
				result.Changes[lineNum] = LineDiff{
					Type:       LineAddition,
					LineNumber: lineNum,
					Content:    line,
				}
			}
			newLineNum += len(lines)
			i++
		}
	}
}

// lineSimilarity computes a simple similarity score between two lines (0.0 to 1.0)
// Higher score means more similar. Empty lines have 0 similarity with non-empty lines.
func lineSimilarity(line1, line2 string) float64 {
	// Empty lines
	if line1 == "" && line2 == "" {
		return 1.0
	}
	if line1 == "" || line2 == "" {
		return 0.0
	}

	// Use a simple approach: compute character-level diff ratio
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(line1, line2, false)

	// Count equal characters vs total characters
	equalChars := 0
	totalChars := 0
	for _, diff := range diffs {
		if diff.Type == diffmatchpatch.DiffEqual {
			equalChars += len(diff.Text)
		}
		totalChars += len(diff.Text)
	}

	if totalChars == 0 {
		return 0.0
	}

	return float64(equalChars) / float64(totalChars)
}

// findBestMatch finds the best matching line in insertedLines for the given deletedLine
// Returns the index of the best match and its similarity score
func findBestMatch(deletedLine string, insertedLines []string, usedInserts map[int]bool) (int, float64) {
	bestIdx := -1
	bestSimilarity := 0.0

	for i, insertedLine := range insertedLines {
		if usedInserts[i] {
			continue
		}

		similarity := lineSimilarity(deletedLine, insertedLine)
		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestIdx = i
		}
	}

	return bestIdx, bestSimilarity
}

// handleModifications processes delete+insert pairs as modifications
func handleModifications(deletedLines, insertedLines []string, oldLineStart, newLineStart int, result *DiffResult) {
	// If we have equal number of lines, treat each pair as a modification
	if len(deletedLines) == len(insertedLines) {
		for j := range len(deletedLines) {
			// Use old line numbers so modifications overlay the original content
			lineNum := oldLineStart + j + 1

			if deletedLines[j] != "" && insertedLines[j] != "" {
				diffType, colStart, colEnd := categorizeLineChangeWithColumns(deletedLines[j], insertedLines[j])
				result.Changes[lineNum] = LineDiff{
					Type:       diffType,
					LineNumber: lineNum,
					Content:    insertedLines[j],
					OldContent: deletedLines[j],
					ColStart:   colStart,
					ColEnd:     colEnd,
				}
			} else if deletedLines[j] != "" {
				// Deletion of non-empty line
				result.Changes[lineNum] = LineDiff{
					Type:       LineDeletion,
					LineNumber: lineNum,
					Content:    deletedLines[j],
				}
			} else if insertedLines[j] != "" {
				// Addition of non-empty line - use new line number
				lineNum = newLineStart + j + 1
				result.Changes[lineNum] = LineDiff{
					Type:       LineAddition,
					LineNumber: lineNum,
					Content:    insertedLines[j],
				}
			} else {
				// Both lines are empty - this is still a change
				result.Changes[lineNum] = LineDiff{
					Type:       LineModification,
					LineNumber: lineNum,
					Content:    insertedLines[j],
					OldContent: deletedLines[j],
				}
			}
		}
	} else {
		// Unequal number of lines - use similarity-based matching
		// Track which inserted lines have been matched
		usedInserts := make(map[int]bool)
		usedDeletes := make(map[int]bool)

		// First pass: Match similar non-empty lines with similarity threshold
		const similarityThreshold = 0.3 // Lines with >30% similarity are likely modifications
		matches := make(map[int]int)    // maps deleted index to inserted index

		for i, deletedLine := range deletedLines {
			if deletedLine == "" {
				continue
			}

			bestIdx, bestSimilarity := findBestMatch(deletedLine, insertedLines, usedInserts)
			if bestIdx != -1 && bestSimilarity >= similarityThreshold {
				matches[i] = bestIdx
				usedInserts[bestIdx] = true
				usedDeletes[i] = true
			}
		}

		// Second pass: Process matched pairs as modifications
		// Use OLD text coordinates for modifications so they overlay the original content
		for delIdx, insIdx := range matches {
			lineNum := oldLineStart + delIdx + 1
			diffType, colStart, colEnd := categorizeLineChangeWithColumns(deletedLines[delIdx], insertedLines[insIdx])
			result.Changes[lineNum] = LineDiff{
				Type:       diffType,
				LineNumber: lineNum,
				Content:    insertedLines[insIdx],
				OldContent: deletedLines[delIdx],
				ColStart:   colStart,
				ColEnd:     colEnd,
			}
		}

		// Third pass: Handle unmatched deletions
		// Deletions need to be mapped carefully to avoid conflicts with modifications/additions
		// We'll map them relative to new text positions, accounting for matched lines
		for i, deletedLine := range deletedLines {
			if usedDeletes[i] {
				continue
			}

			// Find the next matched or inserted line after this deletion to determine positioning
			// Start by computing the position as if it's in the new text
			lineNum := newLineStart + i + 1

			// Check if this line number is already used by a modification or addition
			// If so, find the next available line number
			for {
				if _, exists := result.Changes[lineNum]; !exists {
					break
				}
				lineNum++
			}

			result.Changes[lineNum] = LineDiff{
				Type:       LineDeletion,
				LineNumber: lineNum,
				Content:    deletedLine,
			}
		}

		// Fourth pass: Handle unmatched additions
		for i, insertedLine := range insertedLines {
			if usedInserts[i] {
				continue
			}

			lineNum := newLineStart + i + 1
			result.Changes[lineNum] = LineDiff{
				Type:       LineAddition,
				LineNumber: lineNum,
				Content:    insertedLine,
			}
		}
	}
}

// categorizeLineChangeWithColumns determines the type of change between two lines and returns column range
func categorizeLineChangeWithColumns(oldLine, newLine string) (DiffType, int, int) {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldLine, newLine, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	insertions := 0
	deletions := 0
	hasEqual := false

	// Track column positions for character-level changes
	colStart := -1
	oldPos := 0
	newPos := 0

	for _, diff := range diffs {
		switch diff.Type {
		case diffmatchpatch.DiffInsert:
			insertions++
			if colStart == -1 {
				colStart = newPos
			}
			newPos += len(diff.Text)
		case diffmatchpatch.DiffDelete:
			deletions++
			if colStart == -1 {
				colStart = oldPos
			}
			oldPos += len(diff.Text)
		case diffmatchpatch.DiffEqual:
			hasEqual = true
			oldPos += len(diff.Text)
			newPos += len(diff.Text)
		}
	}

	// If only insertions and equal parts (no deletions)
	if deletions == 0 && insertions > 0 && hasEqual {
		// Check if it's an append at the end
		if strings.HasPrefix(newLine, oldLine) {
			return LineAppendChars, len(oldLine), len(newLine)
		}
		// If it's a single insertion with equal parts, treat as replacement
		if insertions == 1 {
			// Find the insertion position and treat as replacement
			replaceStart := 0
			replaceEnd := 0
			pos := 0

			for _, diff := range diffs {
				if diff.Type == diffmatchpatch.DiffInsert {
					replaceStart = pos
					replaceEnd = pos + len(diff.Text)
					break
				} else if diff.Type == diffmatchpatch.DiffEqual {
					pos += len(diff.Text)
				}
			}

			return LineReplaceChars, replaceStart, replaceEnd
		}
		// Multiple insertions - treat as general modification
	}

	// If only deletions and equal parts (no insertions)
	if insertions == 0 && deletions > 0 && hasEqual {
		// Calculate deletion range properly
		deleteStart := 0
		deleteEnd := 0
		pos := 0

		// Find where deletion starts and ends
		for _, diff := range diffs {
			if diff.Type == diffmatchpatch.DiffDelete {
				deleteStart = pos
				deleteEnd = pos + len(diff.Text)
				break
			} else if diff.Type == diffmatchpatch.DiffEqual {
				pos += len(diff.Text)
			}
		}

		return LineDeleteChars, deleteStart, deleteEnd
	}

	// If there's both insertions and deletions in a single sequence
	if insertions == 1 && deletions == 1 && hasEqual {
		// Check if this is a simple replacement or a complex modification
		// Simple replacements should be small, single-word changes
		var deletedText, insertedText string

		for _, diff := range diffs {
			switch diff.Type {
			case diffmatchpatch.DiffDelete:
				deletedText = diff.Text
			case diffmatchpatch.DiffInsert:
				insertedText = diff.Text
			}
		}

		// If either the deleted or inserted text contains multiple words or is very different in length,
		// treat it as a complex modification
		deletedWords := len(strings.Fields(deletedText))
		insertedWords := len(strings.Fields(insertedText))

		// Consider it a modification if:
		// 1. Either has more than 2 words
		// 2. The word count difference is more than 1
		// 3. For multi-word changes, length difference is very large (more than 100% change)
		lengthRatio := float64(len(insertedText)) / float64(len(deletedText))

		// If either deleted or inserted text has multiple words, be more strict
		if deletedWords > 2 || insertedWords > 2 ||
			abs(deletedWords-insertedWords) > 1 {
			return LineModification, 0, 0
		}

		// For single-word to single-word changes, be more lenient with length ratio
		// Only consider it a modification if the length difference is extreme (>3x)
		if deletedWords == 1 && insertedWords == 1 {
			if lengthRatio > 3.0 || lengthRatio < 0.33 {
				return LineModification, 0, 0
			}
		} else {
			// For other cases, use stricter length ratio
			if lengthRatio > 2.0 || lengthRatio < 0.5 {
				return LineModification, 0, 0
			}
		}

		// For simple replace operations, find the range of the new content (inserted text)
		replaceStart := 0
		replaceEnd := 0
		pos := 0

		for _, diff := range diffs {
			if diff.Type == diffmatchpatch.DiffInsert {
				replaceStart = pos
				replaceEnd = pos + len(diff.Text)
				break
			} else if diff.Type == diffmatchpatch.DiffEqual {
				pos += len(diff.Text)
			}
		}

		return LineReplaceChars, replaceStart, replaceEnd
	}

	// Default to general modification
	return LineModification, 0, 0
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// generateCursorDiffFormat returns a diff string formatted for Cursor's AI server.
// Each addition line is formatted as: "<index>+|<content>"
// Each deletion line is formatted as: "<index>-|<content>"
// Indices are one-based positions in the new text sequence as changes are applied.
func generateCursorDiffFormat(oldText, newText string) string {
	dmp := diffmatchpatch.New()
	// Use line-level diff via chars mapping
	chars1, chars2, lineArray := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(chars1, chars2, false)
	// Convert back to line-level diffs
	lineDiffs := dmp.DiffCharsToLines(diffs, lineArray)

	var out []string
	printIdx := 0 // zero-based position in the new text as we apply changes

	// Helper to split a diff's text into lines without trailing empty
	splitLines := func(text string) []string {
		lines := strings.Split(text, "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		return lines
	}

	for i := range len(lineDiffs) {
		df := lineDiffs[i]
		switch df.Type {
		case diffmatchpatch.DiffEqual:
			// Advance index by the number of equal lines
			printIdx += len(splitLines(df.Text))

		case diffmatchpatch.DiffDelete:
			delLines := splitLines(df.Text)

			// Look-ahead: count consecutive insert lines in the next hunk
			insertCount := 0
			if i+1 < len(lineDiffs) && lineDiffs[i+1].Type == diffmatchpatch.DiffInsert {
				insertCount = len(splitLines(lineDiffs[i+1].Text))
			}

			// If this delete is paired with a following insert block of 2+ lines
			// and there is only one deleted line, keep index constant at printIdx+1 (1-based)
			if insertCount >= 2 && len(delLines) == 1 {
				out = append(out, fmt.Sprintf("%d-|%s", printIdx+1, delLines[0]))
			} else {
				// Otherwise, index increases per deleted line starting at printIdx (1-based)
				for k, l := range delLines {
					out = append(out, fmt.Sprintf("%d-|%s", printIdx+k+1, l))
				}
			}

		case diffmatchpatch.DiffInsert:
			insLines := splitLines(df.Text)

			// If immediately preceded by a delete, the first insert line replaces the deleted line
			// and subsequent lines are additions with incrementing line numbers
			if i-1 >= 0 && lineDiffs[i-1].Type == diffmatchpatch.DiffDelete {
				// First line replaces the deleted line at the same position
				out = append(out, fmt.Sprintf("%d+|%s", printIdx+1, insLines[0]))
				// Subsequent lines are additions with incrementing line numbers
				for k := 1; k < len(insLines); k++ {
					out = append(out, fmt.Sprintf("%d+|%s", printIdx+k+1, insLines[k]))
				}
			} else {
				// Otherwise, index increases per line starting at printIdx (1-based)
				for k, l := range insLines {
					out = append(out, fmt.Sprintf("%d+|%s", printIdx+k+1, l))
				}
			}
			// Advance the index by the number of inserted lines
			printIdx += len(insLines)
		}
	}

	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// ToLuaFormat converts a LineDiff to a Lua-friendly map format
func (ld LineDiff) ToLuaFormat() map[string]any {
	luaFormat := map[string]any{
		"type":       ld.Type.String(),
		"lineNumber": ld.LineNumber,
		"content":    ld.Content,
		"oldContent": ld.OldContent,
		"colStart":   ld.ColStart,
		"colEnd":     ld.ColEnd,
	}

	// Add group-specific fields if they exist
	if ld.Type == LineModificationGroup || ld.Type == LineAdditionGroup {
		luaFormat["startLine"] = ld.StartLine
		luaFormat["endLine"] = ld.EndLine
		luaFormat["maxOffset"] = ld.MaxOffset
		luaFormat["groupLines"] = ld.GroupLines
	}

	return luaFormat
}

// ToLuaFormat converts a DiffResult to a Lua-friendly map format
// Additional fields can be passed as key-value pairs: ToLuaFormat("startLine", 10, "endLineInclusive", 15)
func (dr *DiffResult) ToLuaFormat(additionalFields ...any) map[string]any {
	// Convert LineDiff changes to Lua-friendly format with string keys
	luaChanges := make(map[string]map[string]any)
	for lineNum, lineDiff := range dr.Changes {
		lineKey := fmt.Sprintf("%d", lineNum)
		luaChanges[lineKey] = lineDiff.ToLuaFormat()
	}

	luaFormat := map[string]any{
		"changes":              luaChanges,
		"isOnlyLineDeletion":   dr.IsOnlyLineDeletion,
		"lastDeletion":         dr.LastDeletion,
		"lastAddition":         dr.LastAddition,
		"lastLineModification": dr.LastLineModification,
		"lastAppendChars":      dr.LastAppendChars,
		"lastDeleteChars":      dr.LastDeleteChars,
		"lastReplaceChars":     dr.LastReplaceChars,
		"cursorLine":           dr.CursorLine,
		"cursorCol":            dr.CursorCol,
	}

	// Add additional fields from variadic parameters (key, value pairs)
	for i := 0; i < len(additionalFields)-1; i += 2 {
		if key, ok := additionalFields[i].(string); ok {
			luaFormat[key] = additionalFields[i+1]
		}
	}

	return luaFormat
}
