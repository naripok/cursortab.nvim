package utils

// Token estimation constants
const (
	AvgCharsPerToken = 4 // Rough estimation: 1 token ≈ 4 characters
)

// EstimateTokenCount estimates the number of tokens in a slice of strings
func EstimateTokenCount(lines []string) int {
	if len(lines) == 0 {
		return 0
	}

	totalChars := 0
	for _, line := range lines {
		totalChars += len(line) + 1 // +1 for newline
	}

	return (totalChars + AvgCharsPerToken - 1) / AvgCharsPerToken // Ceiling division
}

// EstimateCharsFromTokens estimates the number of characters for a given token count
func EstimateCharsFromTokens(tokens int) int {
	return tokens * AvgCharsPerToken
}

// TrimContentAroundCursor trims the content to fit within maxTokens while preserving
// context around the cursor position. Returns the trimmed lines, adjusted cursor position, and trim offset.
func TrimContentAroundCursor(lines []string, cursorRow, cursorCol, maxTokens int) ([]string, int, int, int) {
	if maxTokens <= 0 {
		return lines, cursorRow, cursorCol, 0
	}

	maxChars := EstimateCharsFromTokens(maxTokens)

	// Calculate total content size
	totalChars := 0
	for _, line := range lines {
		totalChars += len(line) + 1 // +1 for newline
	}

	// If content is already within limits, return as-is
	if totalChars <= maxChars {
		return lines, cursorRow, cursorCol, 0
	}

	// Calculate how many lines we can keep around the cursor
	targetChars := maxChars

	// Start from cursor line and expand outward
	startLine := cursorRow
	endLine := cursorRow
	currentChars := len(lines[cursorRow]) + 1

	// Expand around cursor position while staying within token limit
	for currentChars < targetChars && (startLine > 0 || endLine < len(lines)-1) {
		// Try expanding up first
		if startLine > 0 {
			newChars := len(lines[startLine-1]) + 1
			if currentChars+newChars <= targetChars {
				startLine--
				currentChars += newChars
			}
		}

		// Then try expanding down
		if endLine < len(lines)-1 && currentChars < targetChars {
			newChars := len(lines[endLine+1]) + 1
			if currentChars+newChars <= targetChars {
				endLine++
				currentChars += newChars
			}
		}

		// If we can't expand in either direction, break
		if (startLine == 0 || currentChars+len(lines[startLine-1])+1 > targetChars) &&
			(endLine == len(lines)-1 || currentChars+len(lines[endLine+1])+1 > targetChars) {
			break
		}
	}

	// Extract the trimmed lines
	trimmedLines := make([]string, endLine-startLine+1)
	copy(trimmedLines, lines[startLine:endLine+1])

	// Adjust cursor position
	newCursorRow := cursorRow - startLine

	// Return trim offset (how many lines were removed from the start)
	trimOffset := startLine

	return trimmedLines, newCursorRow, cursorCol, trimOffset
}

// TrimDiffHistory trims diff history content to fit within maxTokens per file
func TrimDiffHistory(diffHistory []string, maxTokens int) []string {
	if maxTokens <= 0 || len(diffHistory) == 0 {
		return diffHistory
	}

	maxChars := EstimateCharsFromTokens(maxTokens)

	// Calculate total content size
	totalChars := 0
	for _, diff := range diffHistory {
		totalChars += len(diff) + 1 // +1 for newline
	}

	// If content is already within limits, return as-is
	if totalChars <= maxChars {
		return diffHistory
	}

	// Keep the most recent diffs (from the end) to stay within token limit
	var trimmedDiffs []string
	currentChars := 0

	// Start from the end and work backwards
	for i := len(diffHistory) - 1; i >= 0; i-- {
		diffChars := len(diffHistory[i]) + 1
		if currentChars+diffChars <= maxChars {
			// Prepend to maintain chronological order
			trimmedDiffs = append([]string{diffHistory[i]}, trimmedDiffs...)
			currentChars += diffChars
		} else {
			break
		}
	}

	return trimmedDiffs
}
