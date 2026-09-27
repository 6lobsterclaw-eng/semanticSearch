// Package search provides semantic search using vector embeddings
package search

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"semantic-search/internal/embed"
)

var (
	// chunks stores all text chunks with their embeddings
	chunks    []Chunk
	embedder  *embed.LlamaEmbedder
	dim       int
)

// Chunk represents a text chunk with its embedding vector
type Chunk struct {
	Path    string    `json:"path"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
	Vector  []float32 `json:"vector"`
}

// Result represents a search result with score
type Result struct {
	Path    string  `json:"path"`
	Title   string  `json:"title"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

// Init initializes the semantic search with an embedding model
func Init(modelPath string) error {
	log.Printf("Initializing semantic search with model: %s", modelPath)

	var err error
	embedder, err = embed.NewEmbedder(modelPath)
	if err != nil {
		return fmt.Errorf("failed to create embedder: %w", err)
	}

	dim = 384 // Default, will be auto-detected
	chunks = make([]Chunk, 0)

	log.Printf("Semantic search initialized with dimension: %d", dim)
	return nil
}

// AddChunk adds a text chunk to the index
func AddChunk(path, title, content string) error {
	// Generate embedding for the content
	vector, err := embedder.Embed(content)
	if err != nil {
		return fmt.Errorf("failed to embed content: %w", err)
	}

	chunk := Chunk{
		Path:    path,
		Title:   title,
		Content: content,
		Vector:  vector,
	}

	chunks = append(chunks, chunk)
	log.Printf("Added chunk: %s (vector dim: %d)", path, len(vector))

	return nil
}

// Search performs semantic search using cosine similarity
func Search(query string, topK int) ([]Result, error) {
	// Embed the query
	queryVec, err := embedder.Embed(query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// Calculate similarities
	type scoredChunk struct {
		idx   int
		score float64
	}

	scores := make([]scoredChunk, 0, len(chunks))

	for i, chunk := range chunks {
		sim := embed.CosineSimilarity(queryVec, chunk.Vector)
		scores = append(scores, scoredChunk{idx: i, score: sim})
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Get top K results
	if topK > len(scores) {
		topK = len(scores)
	}

	results := make([]Result, 0, topK)
	for i := 0; i < topK; i++ {
		chunk := chunks[scores[i].idx]
		results = append(results, Result{
			Path:    chunk.Path,
			Title:   chunk.Title,
			Score:   scores[i].score,
			Snippet: truncateContent(chunk.Content, 200),
		})
	}

	return results, nil
}

// SaveIndex saves the chunk index to a file
func SaveIndex(path string) error {
	data, err := json.Marshal(chunks)
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	log.Printf("Saved index with %d chunks to %s", len(chunks), path)
	return nil
}

// LoadIndex loads the chunk index from a file
func LoadIndex(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read index: %w", err)
	}

	if err := json.Unmarshal(data, &chunks); err != nil {
		return fmt.Errorf("failed to unmarshal index: %w", err)
	}

	log.Printf("Loaded index with %d chunks from %s", len(chunks), path)
	return nil
}

// Close releases resources
func Close() {
	embed.Close()
	chunks = nil
}

// ChunkCount returns the number of indexed chunks
func ChunkCount() int {
	return len(chunks)
}

func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

// FindModelFile searches for GGUF model file in common locations
func FindModelFile() string {
	searchPaths := []string{
		"model.gguf",
		"./model.gguf",
		"./models/model.gguf",
		filepath.Join(filepath.Dir(os.Args[0]), "model.gguf"),
		"./embedding-model.gguf",
		"./models/embedding-model.gguf",
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			log.Printf("Found model: %s", path)
			return path
		}
	}

	return ""
}
