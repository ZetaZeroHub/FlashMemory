package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

// LLMParser uses a language model to enhance code understanding
type LLMParser struct {
	Lang        string
	BaseParser  Parser
	LLMEndpoint string
	APIKey      string
	Timeout     time.Duration
}

// NewLLMParser creates a new LLM-enhanced parser with a base parser for initial parsing
func NewLLMParser(lang string, llmEndpoint string, apiKey string) *LLMParser {
	baseParser := NewParser(lang)
	return &LLMParser{
		Lang:        lang,
		BaseParser:  baseParser,
		LLMEndpoint: llmEndpoint,
		APIKey:      apiKey,
		Timeout:     time.Second * 30,
	}
}

// ParseFile first uses the base parser, then enhances the results with LLM insights
func (lp *LLMParser) ParseFile(path string) ([]FunctionInfo, error) {
	// First use the base parser to get initial function information
	baseFuncs, err := lp.BaseParser.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("base parser error: %w", err)
	}

	// If no functions found or LLM enhancement is disabled, return base results
	if len(baseFuncs) == 0 || lp.LLMEndpoint == "" {
		return baseFuncs, nil
	}

	// Read the file content for LLM analysis
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return baseFuncs, fmt.Errorf("error reading file for LLM analysis: %w", err)
	}

	// Enhance each function with LLM insights
	enhancedFuncs := make([]FunctionInfo, len(baseFuncs))
	copy(enhancedFuncs, baseFuncs)

	for i, fn := range enhancedFuncs {
		// Only enhance if the function has a body (not just a declaration)
		if fn.Lines > 1 {
			enhancedFn, enhanceErr := lp.enhanceFunctionWithLLM(fn, string(data))
			if enhanceErr == nil {
				enhancedFuncs[i] = enhancedFn
			}
			// If enhancement fails, we keep the original function info
		}
	}

	return enhancedFuncs, nil
}

// enhanceFunctionWithLLM sends the function to an LLM to get enhanced understanding
func (lp *LLMParser) enhanceFunctionWithLLM(fn FunctionInfo, fileContent string) (FunctionInfo, error) {
	// Create a prompt for the LLM
	prompt := fmt.Sprintf(
		"Analyze this %s function and identify all internal function calls:\n\n%s\n\nFunction name: %s",
		lp.Lang,
		fileContent, // Ideally we would extract just the function body, but that's complex
		fn.Name,
	)

	// Call the LLM API
	response, err := lp.callLLMAPI(prompt)
	if err != nil {
		return fn, err
	}

	// Parse the response to extract function calls
	calls := lp.extractFunctionCalls(response, fn.Calls)
	if len(calls) > 0 {
		fn.Calls = calls
	}

	return fn, nil
}

// callLLMAPI makes an HTTP request to the LLM endpoint
func (lp *LLMParser) callLLMAPI(prompt string) (string, error) {
	// This is a simplified implementation - in a real system, you'd use the
	// specific API format required by your LLM provider (OpenAI, Anthropic, etc.)

	// Create request body
	reqBody, err := json.Marshal(map[string]interface{}{
		"prompt":      prompt,
		"max_tokens":  500,
		"temperature": 0.1, // Low temperature for more deterministic responses
	})
	if err != nil {
		return "", err
	}

	// Create HTTP client with timeout
	client := &http.Client{Timeout: lp.Timeout}

	// Create request
	req, err := http.NewRequest("POST", lp.LLMEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if lp.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+lp.APIKey)
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned non-OK status: %d", resp.StatusCode)
	}

	// Read response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse JSON response - this would need to be adapted to your LLM API's response format
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	// Extract the text from the response
	// This is a simplified example - adjust according to your LLM API's response structure
	if text, ok := result["text"].(string); ok {
		return text, nil
	}

	return "", errors.New("could not extract text from LLM response")
}

// extractFunctionCalls parses the LLM response to extract function calls
func (lp *LLMParser) extractFunctionCalls(llmResponse string, existingCalls []string) []string {
	// This is a simplified implementation
	// In a real system, you'd use more sophisticated parsing based on the LLM's output format

	// Start with existing calls
	callsMap := make(map[string]bool)
	for _, call := range existingCalls {
		callsMap[call] = true
	}

	// Look for function calls in the LLM response
	lines := strings.Split(llmResponse, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for patterns like "calls: function1, function2"
		if strings.Contains(line, "call") || strings.Contains(line, "function") {
			// Extract potential function names
			words := strings.FieldsFunc(line, func(r rune) bool {
				return r == ',' || r == ' ' || r == ':' || r == '(' || r == ')'
			})

			for _, word := range words {
				// Simple heuristic: words that look like function names
				if isLikelyFunctionName(word) {
					callsMap[word] = true
				}
			}
		}
	}

	// Convert map back to slice
	result := make([]string, 0, len(callsMap))
	for call := range callsMap {
		result = append(result, call)
	}

	return result
}

// isLikelyFunctionName uses simple heuristics to identify strings that look like function names
func isLikelyFunctionName(s string) bool {
	// Ignore common words and very short strings
	if len(s) < 2 || s == "the" || s == "and" || s == "function" || s == "call" || s == "calls" {
		return false
	}

	// Check if it starts with a letter or underscore (common for function names)
	firstChar := s[0]
	if !((firstChar >= 'a' && firstChar <= 'z') || (firstChar >= 'A' && firstChar <= 'Z') || firstChar == '_') {
		return false
	}

	// Check if it contains only valid identifier characters
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '.') {
			return false
		}
	}

	return true
}
