// Package embed provides semantic embedding search - DISABLED
// Purego libraries for llama.dll don't support Windows cross-compilation
// Using BM25 search instead, which works fully offline
package embed

import "fmt"

type LlamaEmbedder struct{}

func NewEmbedder(modelPath, libPath string) (*LlamaEmbedder, error) {
	return nil, fmt.Errorf("semantic search requires llama.dll + CGO build - use BM25 mode")
}

func (e *LlamaEmbedder) Embed(text string) ([]float32, error) {
	return nil, fmt.Errorf("disabled")
}

func (e *LlamaEmbedder) EmbedDimension() int { return 0 }

func Close() {}

func CosineSimilarity(a, b []float32) float64 { return 0 }
