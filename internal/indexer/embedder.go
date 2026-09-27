package indexer

import (
	"context"
	"errors"

	"github.com/kelindar/search"
	"github.com/rostamlabs/rembed"
)

var ErrEmbedderInit = errors.New("failed to initialize embedder")

// Embedder generates embeddings using rembed and indexes with kelindar/search
type Embedder struct {
	model *rembed.Embedder
	index *search.Index[string]
	dim  int
}

// NewEmbedder creates a new local embedder using rembed + kelindar/search
func NewEmbedder() (*Embedder, error) {
	// Load model from HuggingFace (auto-downloads)
	model, err := rembed.Load("sentence-transformers/all-MiniLM-L6-v2")
	if err != nil {
		return nil, errors.Join(ErrEmbedderInit, err)
	}

	return &Embedder{
		model: model,
		index: search.NewIndex[string](),
		dim:   model.Dim(),
	}, nil
}

// Embed generates a vector for the given text
func (e *Embedder) Embed(text string) (search.Vector, error) {
	vecs, err := e.model.Embed(context.Background(), []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

// AddDocument adds a document to the embedder's index
func (e *Embedder) AddDocument(id string, vec search.Vector, content string) {
	e.index.Add(vec, content)
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
