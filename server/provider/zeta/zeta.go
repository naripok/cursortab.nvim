package zeta

import (
	"bytes"
	"context"
	"cursortab/types"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Provider implements the types.Provider interface for Zeta (vLLM with OpenAI-style API)
type Provider struct {
	config      *types.ProviderConfig
	httpClient  *http.Client
	url         string
	model       string
	temperature float64
	maxTokens   int
}

// completionRequest matches the OpenAI Completion API format used by vLLM
type completionRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Temperature float64  `json:"temperature"`
	MaxTokens   int      `json:"max_tokens"`
	Stop        []string `json:"stop,omitempty"`
	N           int      `json:"n"`
	Echo        bool     `json:"echo"`
}

// completionResponse matches the OpenAI Completion API response format
type completionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int    `json:"index"`
		Text         string `json:"text"`
		Logprobs     any    `json:"logprobs"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// NewProvider creates a new Zeta provider instance
func NewProvider(config *types.ProviderConfig) (*Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	return &Provider{
		config:      config,
		httpClient:  &http.Client{},
		url:         config.ProviderURL,
		model:       config.ProviderModel,
		temperature: config.ProviderTemperature,
		maxTokens:   config.ProviderMaxTokens,
	}, nil
}

// GetCompletion implements types.Provider.GetCompletion for Zeta
// This provider supports multi-line completions using special tokens
func (p *Provider) GetCompletion(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	// Build the user excerpt with special tokens
	userExcerpt := p.buildPromptWithSpecialTokens(req)

	// Build the user edits from diff history
	userEdits := p.buildUserEditsFromDiffHistory(req)

	// Format diagnostics for inclusion in prompt
	diagnosticsText := p.formatDiagnosticsForPrompt(req)

	// Build the full prompt with instruction template
	// Extended format includes diagnostics section
	prompt := p.buildInstructionPrompt(userEdits, diagnosticsText, userExcerpt)

	// Create the completion request
	completionReq := completionRequest{
		Model:       p.model,
		Prompt:      prompt,
		Temperature: p.temperature,
		MaxTokens:   p.maxTokens,
		Stop:        []string{"\n<|editable_region_end|>"},
		N:           1,
		Echo:        false,
	}

	// Marshal the request without HTML escaping
	var reqBodyBuf bytes.Buffer
	encoder := json.NewEncoder(&reqBodyBuf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(completionReq); err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	reqBody := reqBodyBuf.Bytes()

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.url+"/v1/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read the response body for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the response
	var completionResp completionResponse
	if err := json.Unmarshal(body, &completionResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check if we got any completions
	if len(completionResp.Choices) == 0 {
		return &types.CompletionResponse{
			Completions:  []*types.Completion{},
			CursorTarget: nil,
		}, nil
	}

	// Extract the completion text
	completionText := completionResp.Choices[0].Text

	// If the completion is empty or just whitespace, return empty response
	if strings.TrimSpace(completionText) == "" {
		return &types.CompletionResponse{
			Completions:  []*types.Completion{},
			CursorTarget: nil,
		}, nil
	}

	// Parse the completion into lines and build the result
	completion := p.parseCompletion(req, completionText)
	if completion == nil {
		return &types.CompletionResponse{
			Completions:  []*types.Completion{},
			CursorTarget: nil,
		}, nil
	}

	return &types.CompletionResponse{
		Completions:  []*types.Completion{completion},
		CursorTarget: nil,
	}, nil
}

// formatDiagnosticsForPrompt formats linter errors in a diff-like format for the prompt
// Format similar to User Edits section:
// Diagnostics in "path/to/file":
// ```diagnostics
// line 10: [error] Unused variable 'x' (source: eslint)
// line 15: [warning] Missing return type (source: typescript)
// ```
func (p *Provider) formatDiagnosticsForPrompt(req *types.CompletionRequest) string {
	if req.LinterErrors == nil || len(req.LinterErrors.Errors) == 0 {
		return ""
	}

	var diagBuilder strings.Builder

	diagBuilder.WriteString("Diagnostics in \"")
	diagBuilder.WriteString(req.LinterErrors.RelativeWorkspacePath)
	diagBuilder.WriteString("\":\n")
	diagBuilder.WriteString("```diagnostics\n")

	for _, err := range req.LinterErrors.Errors {
		// Format: line X: [severity] message (source)
		if err.Range != nil {
			diagBuilder.WriteString(fmt.Sprintf("line %d: ", err.Range.StartLine))
		}

		diagBuilder.WriteString(fmt.Sprintf("[%s] %s", err.Severity, err.Message))

		if err.Source != "" {
			diagBuilder.WriteString(fmt.Sprintf(" (source: %s)", err.Source))
		}
		diagBuilder.WriteString("\n")
	}

	diagBuilder.WriteString("```")
	return diagBuilder.String()
}

// buildUserEditsFromDiffHistory formats the diff history into Zed's "User Edits" format
// Example format:
// User edited "path/to/file.py":
// ```diff
// @@ -1,1 +1,1 @@
// -def test
// +def testi
// ```
func (p *Provider) buildUserEditsFromDiffHistory(req *types.CompletionRequest) string {
	if len(req.FileDiffHistories) == 0 {
		return ""
	}

	var editsBuilder strings.Builder
	firstEdit := true

	for _, fileHistory := range req.FileDiffHistories {
		if len(fileHistory.DiffHistory) == 0 {
			continue
		}

		// Each file's diffs are concatenated with double newlines
		for _, diff := range fileHistory.DiffHistory {
			if !firstEdit {
				editsBuilder.WriteString("\n\n")
			}
			firstEdit = false

			// Format: User edited "filename":
			editsBuilder.WriteString("User edited \"")
			editsBuilder.WriteString(fileHistory.FileName)
			editsBuilder.WriteString("\":\n")
			editsBuilder.WriteString("```diff\n")
			editsBuilder.WriteString(diff)
			editsBuilder.WriteString("\n```")
		}
	}

	return editsBuilder.String()
}

// buildInstructionPrompt wraps the user excerpt in the instruction template
// Extended version includes diagnostics section
func (p *Provider) buildInstructionPrompt(userEdits, diagnostics, userExcerpt string) string {
	var promptBuilder strings.Builder

	promptBuilder.WriteString("### Instruction:\n")
	promptBuilder.WriteString("You are a code completion assistant and your task is to analyze user edits and then rewrite an excerpt that the user provides, suggesting the appropriate edits within the excerpt, taking into account the cursor location.\n\n")

	promptBuilder.WriteString("### User Edits:\n\n")
	promptBuilder.WriteString(userEdits)
	promptBuilder.WriteString("\n\n")

	// Add diagnostics section if available
	if diagnostics != "" {
		promptBuilder.WriteString("### Diagnostics:\n\n")
		promptBuilder.WriteString(diagnostics)
		promptBuilder.WriteString("\n\n")
	}

	promptBuilder.WriteString("### User Excerpt:\n\n")
	promptBuilder.WriteString(userExcerpt)
	promptBuilder.WriteString("\n\n")

	promptBuilder.WriteString("### Response:\n")

	return promptBuilder.String()
}

// buildPromptWithSpecialTokens constructs the prompt with special tokens matching Zed's format
func (p *Provider) buildPromptWithSpecialTokens(req *types.CompletionRequest) string {
	var promptBuilder strings.Builder

	cursorRow := req.CursorRow // 1-indexed
	cursorCol := req.CursorCol // 0-indexed

	// Convert cursor to 0-indexed for calculations
	cursorLine := cursorRow - 1

	// Determine the editable region (expand around cursor)
	// Zed uses token-based limits: editableTokenLimit=350, contextTokenLimit=150
	// For simplicity, we use line-based expansion which approximates this

	// Editable region: about 10 lines before and after cursor
	editableLinesBefore := 10
	editableLinesAfter := 10

	editableStart := max(0, cursorLine-editableLinesBefore)
	editableEnd := min(len(req.Lines), cursorLine+editableLinesAfter+1)

	// Context region: additional lines around editable region
	contextLinesBefore := 5
	contextLinesAfter := 5

	contextStart := max(0, editableStart-contextLinesBefore)
	contextEnd := min(len(req.Lines), editableEnd+contextLinesAfter)

	// Build the prompt in Zed's format: ```filename\n<|start_of_file|>\n...
	promptBuilder.WriteString("```")
	promptBuilder.WriteString(req.FilePath)
	promptBuilder.WriteString("\n")

	// Add start of file marker if we're at the beginning
	if contextStart == 0 {
		promptBuilder.WriteString("<|start_of_file|>\n")
	}

	// Add context lines before editable region
	for i := contextStart; i < editableStart; i++ {
		promptBuilder.WriteString(req.Lines[i])
		promptBuilder.WriteString("\n")
	}

	// Mark the start of the editable region (writeln adds newline after)
	promptBuilder.WriteString("<|editable_region_start|>\n")

	// Add lines in the editable region up to the cursor
	for i := editableStart; i < cursorLine; i++ {
		promptBuilder.WriteString(req.Lines[i])
		promptBuilder.WriteString("\n")
	}

	// Add the current line split at cursor position
	if cursorLine < len(req.Lines) {
		currentLine := req.Lines[cursorLine]
		if cursorCol <= len(currentLine) {
			beforeCursor := currentLine[:cursorCol]
			afterCursor := currentLine[cursorCol:]

			promptBuilder.WriteString(beforeCursor)
			promptBuilder.WriteString("<|user_cursor_is_here|>")
			promptBuilder.WriteString(afterCursor)
		} else {
			promptBuilder.WriteString(currentLine)
			promptBuilder.WriteString("<|user_cursor_is_here|>")
		}
	} else {
		promptBuilder.WriteString("<|user_cursor_is_here|>")
	}

	// Add remaining lines in the editable region after the cursor
	for i := cursorLine + 1; i < editableEnd; i++ {
		promptBuilder.WriteString("\n")
		promptBuilder.WriteString(req.Lines[i])
	}

	// Mark the end of the editable region (write adds newline before, not after)
	promptBuilder.WriteString("\n<|editable_region_end|>")

	// Add context lines after editable region
	for i := editableEnd; i < contextEnd; i++ {
		promptBuilder.WriteString("\n")
		promptBuilder.WriteString(req.Lines[i])
	}

	// Close the code fence (newline before the closing ```)
	promptBuilder.WriteString("\n```")

	return promptBuilder.String()
}

// parseCompletion parses the model's completion text matching Zed's parsing logic
func (p *Provider) parseCompletion(req *types.CompletionRequest, completionText string) *types.Completion {
	// Remove cursor markers
	content := strings.ReplaceAll(completionText, "<|user_cursor_is_here|>", "")

	// Extract text between editable markers
	startMarker := "<|editable_region_start|>"
	endMarker := "<|editable_region_end|>"

	// Find the start marker
	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return p.parseSimpleCompletion(req, completionText)
	}

	// Slice from the start marker position onward
	content = content[startIdx:]

	// Find the newline after the start marker and skip it
	newlineIdx := strings.Index(content, "\n")
	if newlineIdx == -1 {
		return nil
	}
	content = content[newlineIdx+1:]

	// Find the end marker (looking for "\n<|editable_region_end|>")
	endIdx := strings.Index(content, "\n"+endMarker)
	var newText string
	if endIdx == -1 {
		// If end marker not found, use rest of content
		newText = content
	} else {
		newText = content[:endIdx]
	}

	// Calculate the editable region that was sent in the prompt
	cursorRow := req.CursorRow - 1 // Convert to 0-indexed
	editableStart := max(0, cursorRow-10)
	editableEnd := min(len(req.Lines), cursorRow+10+1)

	// Get the old text of the editable region
	oldLines := req.Lines[editableStart:editableEnd]
	oldText := strings.Join(oldLines, "\n")

	// If the new text equals old text, no completion needed
	if newText == oldText {
		return nil
	}

	// Split new text into lines
	newLines := strings.Split(newText, "\n")

	return &types.Completion{
		StartLine:  editableStart + 1, // Convert back to 1-indexed
		EndLineInc: editableEnd,       // Already 1-indexed exclusive -> inclusive
		Lines:      newLines,
	}
}

// parseSimpleCompletion is a fallback parser for when markers aren't found
func (p *Provider) parseSimpleCompletion(req *types.CompletionRequest, completionText string) *types.Completion {
	// Split into lines
	completionLines := strings.Split(completionText, "\n")

	if len(completionLines) == 0 {
		return nil
	}

	cursorRow := req.CursorRow
	cursorCol := req.CursorCol

	// Build the replacement lines
	var resultLines []string

	// First line: combine text before cursor + completion first line
	if cursorRow <= len(req.Lines) {
		currentLine := req.Lines[cursorRow-1]
		beforeCursor := ""
		if cursorCol <= len(currentLine) {
			beforeCursor = currentLine[:cursorCol]
		} else {
			beforeCursor = currentLine
		}
		resultLines = append(resultLines, beforeCursor+completionLines[0])
	} else {
		resultLines = append(resultLines, completionLines[0])
	}

	// Add remaining completion lines
	resultLines = append(resultLines, completionLines[1:]...)

	// Determine the end line
	endLine := cursorRow + len(completionLines) - 1

	return &types.Completion{
		StartLine:  cursorRow,
		EndLineInc: endLine,
		Lines:      resultLines,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
