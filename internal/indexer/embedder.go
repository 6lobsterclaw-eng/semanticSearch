package indexer

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/kelindar/search"
	"github.com/kelindar/search/llama"
)

var ErrEmbedderInit = errors.New("failed to initialize embedder")

// Embedder generates embeddings using kelindar/search/llama (GGUF models)
type Embedder struct {
	model *llama.Vectorizer
	index *search.Index[string]
	dim   int
	count int // Track number of vectors
}

// NewEmbedder creates a new local embedder using GGUF model
// modelPath: path to GGUF model file (e.g., "./model/embedding-model.gguf")
func NewEmbedder(modelPath string) (*Embedder, error) {
	// Check if model file exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, errors.Join(ErrEmbedderInit, errors.New("model file not found: "+modelPath))
	}

	// Load GGUF model (0 = CPU only, no GPU needed)
	model, err := llama.New(modelPath, 0)
	if err != nil {
		return nil, errors.Join(ErrEmbedderInit, err)
	}

	// Detect embedding dimension from model name
	// Default to 1024 for Qwen3-Embedding-0.6B
	dim := 1024
	if strings.Contains(strings.ToLower(modelPath), "384") {
		dim = 384
	} else if strings.Contains(strings.ToLower(modelPath), "768") {
		dim = 768
	} else if strings.Contains(strings.ToLower(modelPath), "1024") {
		dim = 1024
	}

	return &Embedder{
		model: model,
		index: search.NewIndex[string](),
		dim:   dim,
		count: 0,
	}, nil
}

// Embed generates a vector for the given text
func (e *Embedder) Embed(text string) (search.Vector, error) {
	vec, err := e.model.EmbedText(context.Background(), text)
	if err != nil {
		return nil, err
	}
	return vec, nil
}

// AddDocument adds a document to the embedder's index
func (e *Embedder) AddDocument(id string, vec search.Vector, content string) {
	e.index.Add(vec, content)
	e.count++
}

// Search searches the index
func (e *Embedder) Search(query search.Vector, k int) []search.Result[string] {
	return e.index.Search(query, k)
}

// SaveIndex saves the index to a file
func (e *Embedder) SaveIndex(path string) error {
	return e.index.WriteFile(path)
}

// LoadIndex loads the index from a file
func (e *Embedder) LoadIndex(path string) error {
	return e.index.ReadFile(path)
}

// Dim returns the embedding dimension
func (e *Embedder) Dim() int {
	return e.dim
}

// Size returns the number of vectors in the index
func (e *Embedder) Size() int {
	return e.count
}

// Close releases resources
func (e *Embedder) Close() error {
	if e.model != nil {
		e.model.Close()
	}
	return nil
}
