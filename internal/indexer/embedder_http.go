package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kelindar/search"
)

// vectorEntry stores a single vector with its metadata
type vectorEntry struct {
	id      string
	vector  []float32
	content string
}

// SimpleVectorStore is a simple in-memory vector store with cosine similarity
type SimpleVectorStore struct {
	mu      sync.RWMutex
	vectors []vectorEntry
}

func NewSimpleVectorStore() *SimpleVectorStore {
	log.Println("[DEBUG] Creating new SimpleVectorStore")
	return &SimpleVectorStore{
		vectors: make([]vectorEntry, 0),
	}
}

func (s *SimpleVectorStore) Add(id string, vec search.Vector, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vectors = append(s.vectors, vectorEntry{
		id:      id,
		vector:  vec,
		content: content,
	})
	// DEBUG: Log vector first 10 values when adding
	if len(vec) > 0 {
		first10 := vec
		if len(first10) > 10 {
			first10 = first10[:10]
		}
		log.Printf("[DEBUG SimpleVectorStore] Added: id=%q, total=%d, vec first10=%v, content=%q", 
			id, len(s.vectors), first10, content)
	} else {
		log.Printf("[DEBUG SimpleVectorStore] Added: id=%q, total=%d, vec=EMPTY, content=%q", 
			id, len(s.vectors), content)
	}
}

func (s *SimpleVectorStore) Search(query search.Vector, k int) []search.Result[string] {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Printf("[DEBUG SimpleVectorStore.Search] Searching in %d vectors", len(s.vectors))
	
	// DEBUG: Log query vector first 10 values
	if len(query) > 0 {
		first10 := query
		if len(first10) > 10 {
			first10 = first10[:10]
		}
		log.Printf("[DEBUG SimpleVectorStore.Search] Query vector first10: %v", first10)
	}

	type scoredResult struct {
		id        string
		score     float64
	}

	var results []scoredResult
	for _, v := range s.vectors {
		score := cosineSimilarity(query, v.vector)
		results = append(results, scoredResult{id: v.id, score: score})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// DEBUG: Log top 20 raw scores before filtering
	log.Printf("[DEBUG] Top 20 raw scores before filtering:")
	brainCount := 0
	for i := 0; i < len(results) && i < 20; i++ {
		// Try to find content for this ID
		content := ""
		for _, v := range s.vectors {
			if v.id == results[i].id {
				// Show first 80 chars of content
				if len(v.content) > 80 {
					content = v.content[:80] + "..."
				} else {
					content = v.content
				}
				// Check if content has "brain"
				if strings.Contains(strings.ToLower(v.content), "brain") {
					brainCount++
					log.Printf("[DEBUG BRAIN FOUND] %d: id=%q score=%.4f content=%q", i+1, results[i].id, results[i].score, content)
				}
				break
			}
		}
		if !strings.Contains(strings.ToLower(content), "brain") {
			log.Printf("[DEBUG]   %d: id=%q score=%.4f content=%q", i+1, results[i].id, results[i].score, content)
		}
	}
	log.Printf("[DEBUG] Total 'brain' in top 20: %d", brainCount)
	
	// Count ALL brain instances in index
	totalBrainInIndex := 0
	belowThreshold := 0
	for _, v := range s.vectors {
		if strings.Contains(strings.ToLower(v.content), "brain") {
			totalBrainInIndex++
		}
	}
	log.Printf("[DEBUG] Total 'brain' instances in index: %d", totalBrainInIndex)

	// Apply threshold to filter out noise
	// Key insight: In 1024D space, random vectors have expected similarity ~0.5
	// Use noise floor of 0.50 to filter truly random matches while keeping valid results
	const noiseFloor = 0.50
	threshold := noiseFloor
	
	// If top score is above noise floor, include everything above noise floor
	if len(results) > 0 && results[0].score > noiseFloor {
		threshold = noiseFloor
	}
	
	var filteredResults []scoredResult
	for _, r := range results {
		if r.score >= threshold {
			filteredResults = append(filteredResults, r)
		}
	}
	if len(filteredResults) == 0 {
		log.Printf("[DEBUG SimpleVectorStore.Search] No results above noise floor (%.2f)", noiseFloor)
		return nil // Return empty - no real matches
	}
	results = filteredResults
	log.Printf("[DEBUG SimpleVectorStore.Search] Threshold=%.2f, got %d results", threshold, len(results))

	// Take top k
	if len(results) > k {
		results = results[:k]
	}

	var out []search.Result[string]
	for _, r := range results {
		out = append(out, search.Result[string]{
			Value:      r.id,
			Relevance: r.score,
		})
	}
	return out
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

var errEmbedderInit = errors.New("failed to initialize HTTP embedder")
type HTTPEmbedder struct {
	serverURL string
	modelPath string
	index     *search.Index[string]
	// Use our simple store instead
	simpleStore *SimpleVectorStore
	dim         int
	client      *http.Client
}

// NewHTTPEmbedder creates a new HTTP-based embedder
// serverURL: base URL of llama-server (e.g., "http://localhost:8080")
// modelPath: path to GGUF model file (for display/info only)
func NewHTTPEmbedder(serverURL, modelPath string) (*HTTPEmbedder, error) {
	if serverURL == "" {
		return nil, errors.Join(errEmbedderInit, errors.New("server URL is required"))
	}

	return &HTTPEmbedder{
		serverURL:    serverURL,
		modelPath:    modelPath,
		simpleStore:  NewSimpleVectorStore(),
		dim:          1024,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

// Embed generates a vector for the given text via HTTP
func (e *HTTPEmbedder) Embed(text string) (search.Vector, error) {
	log.Printf("[DEBUG Embed] Input text: %q (len=%d)", text, len(text))

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

	log.Printf("[DEBUG Embed] Output vector: dim=%d, first5=%v, norm=%.4f", len(vec), vec[:5], computeNorm(vec))

	return vec, nil
}

// computeNorm computes L2 norm of a vector
func computeNorm(v search.Vector) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x * x)
	}
	return math.Sqrt(sum)
}

// AddDocument adds a document to the embedder's index
func (e *HTTPEmbedder) AddDocument(id string, vec search.Vector, content string) {
	log.Printf("[DEBUG AddDocument] id=%q, content=%q, vec dim=%d, norm=%.4f, first5=%v", id, content, len(vec), computeNorm(vec), vec[:5])
	// Use our simple vector store instead of kelindar/search
	e.simpleStore.Add(id, vec, content)
}

// Search searches the index
func (e *HTTPEmbedder) Search(query search.Vector, k int) []search.Result[string] {
	log.Printf("[DEBUG Search] Query vector: dim=%d, first5=%v, norm=%.4f", len(query), query[:5], computeNorm(query))
	
	// Use our simple vector store instead of kelindar/search
	results := e.simpleStore.Search(query, k)
	
	log.Printf("[DEBUG Search] Got %d results", len(results))
	for i, r := range results {
		log.Printf("[DEBUG Search] Result %d: value=%q, relevance=%.4f", i+1, r.Value, r.Relevance)
	}
	return results
}

// SaveIndex saves the index to a file
func (e *HTTPEmbedder) SaveIndex(path string) error {
	// SimpleVectorStore doesn't support persistence yet - user re-indexes each time
	log.Println("[DEBUG] SaveIndex not implemented for SimpleVectorStore - user will re-index")
	return nil
}

// LoadIndex loads the index from a file
func (e *HTTPEmbedder) LoadIndex(path string) error {
	// SimpleVectorStore doesn't support persistence yet - user re-indexes each time
	log.Println("[DEBUG] LoadIndex not implemented for SimpleVectorStore - user will re-index")
	return nil
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
