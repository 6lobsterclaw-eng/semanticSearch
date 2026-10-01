package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kelindar/search"
)

var errEmbedderInit = errors.New("failed to initialize HTTP embedder")
type HTTPEmbedder struct {
	serverURL string
	modelPath string
	index     *search.Index[string]
	dim       int
	client    *http.Client
}

// NewHTTPEmbedder creates a new HTTP-based embedder
// serverURL: base URL of llama-server (e.g., "http://localhost:8080")
// modelPath: path to GGUF model file (for display/info only)
func NewHTTPEmbedder(serverURL, modelPath string) (*HTTPEmbedder, error) {
	if serverURL == "" {
		return nil, errors.Join(errEmbedderInit, errors.New("server URL is required"))
	}

	return &HTTPEmbedder{
		serverURL: serverURL,
		modelPath: modelPath,
		index:     search.NewIndex[string](),
		dim:       1024,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

// Embed generates a vector for the given text via HTTP
func (e *HTTPEmbedder) Embed(text string) (search.Vector, error) {
	// OpenAI-compatible embedding request format
	type EmbedRequest struct {
		Input string `json:"input"`
		Model string `json:"model,omitempty"`
	}

	type EmbedResponse struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}

	reqBody := EmbedRequest{Input: text}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", e.serverURL+"/v1/embeddings", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Read body for error details
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llama-server returned status %d: %s", resp.StatusCode, string(body))
	}

	var embedResp EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(embedResp.Data) == 0 {
		return nil, errors.New("no embeddings returned")
	}

	// Convert to search.Vector
	vec := make(search.Vector, len(embedResp.Data[0].Embedding))
	for i, v := range embedResp.Data[0].Embedding {
		vec[i] = float32(v)
	}

	return vec, nil
}

// AddDocument adds a document to the embedder's index
func (e *HTTPEmbedder) AddDocument(id string, vec search.Vector, content string) {
	// Store ID as value so we can look up the chunk later
	e.index.Add(vec, id)
}

// Search searches the index
func (e *HTTPEmbedder) Search(query search.Vector, k int) []search.Result[string] {
	return e.index.Search(query, k)
}

// SaveIndex saves the index to a file
func (e *HTTPEmbedder) SaveIndex(path string) error {
	return e.index.WriteFile(path)
}

// LoadIndex loads the index from a file
func (e *HTTPEmbedder) LoadIndex(path string) error {
	return e.index.ReadFile(path)
}

// Dim returns the embedding dimension
func (e *HTTPEmbedder) Dim() int {
	return e.dim
}

// Close releases resources
func (e *HTTPEmbedder) Close() error {
	e.client.CloseIdleConnections()
	return nil
}

// IsServerReady checks if llama-server is responding
func (e *HTTPEmbedder) IsServerReady() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", e.serverURL+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}
