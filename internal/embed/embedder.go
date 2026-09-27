// Package embed provides semantic embedding search - DISABLED due to WASM compatibility issues
// Using BM25 search instead, which works fully offline
package embed

import (
	"fmt"
)

// DISABLED: goccy/go-llama WASM has GGUF compatibility issues on Windows
// Using BM25 search instead

type LlamaEmbedder struct{}

func NewEmbedder(modelPath string) (*LlamaEmbedder, error) {
	return nil, fmt.Errorf("semantic search temporarily disabled - use BM25 mode instead")
}

func (e *LlamaEmbedder) Embed(text string) ([]float32, error) {
	return nil, fmt.Errorf("disabled")
}

func (e *LlamaEmbedder) EmbedDimension() int {
	return 0
}

func Close() {}

func CosineSimilarity(a, b []float32) float64 {
	return 0
}
