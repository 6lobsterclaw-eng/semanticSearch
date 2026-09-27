// Package embed provides semantic embedding search using GGUF models via go-llama (WASM)
package embed

import (
	"fmt"
	"log"
	"math"

	llama "github.com/goccy/go-llama"
)

var (
	instance *LlamaEmbedder
)

// LlamaEmbedder wraps go-llama for embedding generation
type LlamaEmbedder struct {
	llama  *llama.Llama
	model  *llama.Model
	ctx    *llama.Context
	dim    int
}

// NewEmbedder creates a new embedding embedder with the given model path
func NewEmbedder(modelPath string) (*LlamaEmbedder, error) {
	// If already initialized, return existing instance
	if instance != nil {
		return instance, nil
	}

	log.Printf("Initializing embedding model: %s", modelPath)

	// Create llama instance
	ll, err := llama.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create llama instance: %w", err)
	}

	// Load model
	model, err := ll.LoadModel(modelPath)
	if err != nil {
		ll.Close()
		return nil, fmt.Errorf("failed to load model: %w", err)
	}

	// Create context with embeddings enabled
	ctx, err := model.NewContext(llama.ContextParams{
		NCtx:       512,
		Embeddings: true,
	})
	if err != nil {
		model.Close()
		ll.Close()
		return nil, fmt.Errorf("failed to create context: %w", err)
	}

	dim := 384 // Default, will be detected from first embedding
	log.Printf("Embedding context created")

	instance = &LlamaEmbedder{
		llama: ll,
		model: model,
		ctx:   ctx,
		dim:   dim,
	}

	return instance, nil
}

// Embed generates an embedding vector for the given text
func (e *LlamaEmbedder) Embed(text string) ([]float32, error) {
	emb, err := e.ctx.Embed(text, false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Update dimension on first call
	if e.dim != len(emb) {
		e.dim = len(emb)
		log.Printf("Detected embedding dimension: %d", e.dim)
	}

	return emb, nil
}

// EmbedDimension returns the embedding dimension
func (e *LlamaEmbedder) EmbedDimension() int {
	return e.dim
}

// Close closes the embedder and releases resources
func Close() {
	if instance != nil {
		if instance.ctx != nil {
			instance.ctx.Close()
		}
		if instance.model != nil {
			instance.model.Close()
		}
		if instance.llama != nil {
			instance.llama.Close()
		}
		instance = nil
	}
}

// CosineSimilarity computes cosine similarity between two embedding vectors
func CosineSimilarity(a, b []float32) float64 {
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
