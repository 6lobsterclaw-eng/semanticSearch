// Package embed provides semantic embedding search - DISABLED
// goccy/go-llama (WASM) has model compatibility issues
// Using BM25 search instead, which works fully offline
package embed

import "fmt"

type LlamaEmbedder struct{}

func NewEmbedder(modelPath, libPath string) (*LlamaEmbedder, error) {
	return nil, fmt.Errorf("semantic search has compatibility issues with some models - use BM25 mode")
}

func (e *LlamaEmbedder) Embed(text string) ([]float32, error) {
	return nil, fmt.Errorf("disabled")
}

func (e *LlamaEmbedder) EmbedDimension() int { return 0 }

func Close() {}

func CosineSimilarity(a, b []float32) float64 { return 0 }
