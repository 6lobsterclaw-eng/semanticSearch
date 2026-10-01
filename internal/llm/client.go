package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Client wraps an HTTP client for LLM inference
type Client struct {
	serverURL string
	client    *http.Client
	model     string
}

// NewClient creates a new LLM client
func NewClient(serverURL, model string) *Client {
	return &Client{
		serverURL: strings.TrimRight(serverURL, "/"),
		client: &http.Client{
			Timeout: 120 * time.Second, // LLM calls take longer
		},
		model: model,
	}
}

// GenerateQuestions generates 2-3 user questions from the given text
func (c *Client) GenerateQuestions(text string) ([]string, error) {
	// Prompt for question generation
	prompt := fmt.Sprintf(`Given this text, generate 2-3 natural questions that a user might ask that this text would answer. Be casual and varied. Return ONLY the questions, one per line, no numbering:

%s

Questions:`, text)

	// Build request
	reqBody := map[string]interface{}{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a helpful assistant that generates search questions."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens": 256,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	req, err := http.NewRequest("POST", c.serverURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[DEBUG LLM] Sending request to %s with model %s", c.serverURL, c.model)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	content := response.Choices[0].Message.Content

	// Parse questions (one per line)
	questions := strings.Split(strings.TrimSpace(content), "\n")
	var result []string
	for _, q := range questions {
		q = strings.TrimSpace(q)
		// Remove numbering like "1.", "2." etc
		q = strings.TrimLeft(q, "1234567890.) -")
		q = strings.TrimSpace(q)
		if len(q) > 10 && len(q) < 200 { // Reasonable question length
			result = append(result, q)
		}
	}

	// Limit to 3 questions
	if len(result) > 3 {
		result = result[:3]
	}

	log.Printf("[DEBUG LLM] Generated %d questions: %v", len(result), result)

	return result, nil
}

// SetModel updates the model name
func (c *Client) SetModel(model string) {
	c.model = model
}
