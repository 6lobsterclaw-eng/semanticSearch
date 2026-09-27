// Package embed provides semantic embedding search using GGUF models via yzma + llama.dll
package embed

import (
	"fmt"
	"log"
	"math"
	"runtime"

	"github.com/hybridgroup/yzma/pkg/llama"
)

var instance *LlamaEmbedder

type LlamaEmbedder struct {
	model   llama.Model
	ctx     llama.Context
	dim     int
	pooling llama.PoolingType
}

// NewEmbedder creates a new embedding embedder
// libPath is the path to llama.dll (e.g., "llama.dll" or "C:\\path\\to\\llama.dll")
func NewEmbedder(modelPath, libPath string) (*LlamaEmbedder, error) {
	if instance != nil {
		return instance, nil
	}

	log.Printf("Loading llama.dll from: %s", libPath)

	// Load llama.dll dynamically using purego
	if err := llama.Load(libPath); err != nil {
		return nil, fmt.Errorf("failed to load llama.dll: %w", err)
	}

	// Initialize llama backend
	llama.Init()

	log.Printf("Loading model: %s", modelPath)

	// Load model
	model, err := llama.ModelLoadFromFile(modelPath, llama.ModelDefaultParams())
	if err != nil {
		llama.Close()
		return nil, fmt.Errorf("failed to load model: %w", err)
	}
	if model == 0 {
		llama.Close()
		return nil, fmt.Errorf("model returned null handle")
	}

	// Get embedding dimension
	dim := int(llama.ModelNEmbd(model))
	log.Printf("Embedding dimension: %d", dim)

	// Create context with embeddings enabled
	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = 512
	ctxParams.NBatch = 512
	ctxParams.NThreads = int32(runtime.NumCPU())
	ctxParams.NThreadsBatch = int32(runtime.NumCPU())
	ctxParams.PoolingType = llama.PoolingTypeMean // MEAN pooling for embeddings
	ctxParams.Embeddings = 1

	ctx, err := llama.InitFromModel(model, ctxParams)
	if err != nil {
		llama.ModelFree(model)
		llama.Close()
		return nil, fmt.Errorf("failed to create context: %w", err)
	}

	instance = &LlamaEmbedder{
		model:   model,
		ctx:     ctx,
		dim:     dim,
		pooling: llama.PoolingTypeMean,
	}

	return instance, nil
}

// Embed generates an embedding vector for the given text
func (e *LlamaEmbedder) Embed(text string) ([]float32, error) {
	// Tokenize
	vocab := llama.ModelGetVocab(e.model)
	tokens := llama.Tokenize(vocab, text, true, true)

	// Create batch
	batch := llama.BatchGetOne(tokens)

	// Decode - Decode returns (int32, error)
	_, err := llama.Decode(e.ctx, batch)
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	// Get embeddings - use int32 for nEmbd
	vec, err := llama.GetEmbeddingsSeq(e.ctx, 0, int32(e.dim))
	if err != nil {
		return nil, fmt.Errorf("failed to get embeddings: %w", err)
	}

	// Normalize
	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}
	sum = math.Sqrt(sum)
	if sum > 0 {
		norm := float32(1.0 / sum)
		for i := range vec {
			vec[i] *= norm
		}
	}

	return vec, nil
}

// EmbedDimension returns the embedding dimension
func (e *LlamaEmbedder) EmbedDimension() int {
	return e.dim
}

// Close closes the embedder and releases resources
func Close() {
	if instance != nil {
		if instance.ctx != 0 {
			llama.Free(instance.ctx)
		}
		if instance.model != 0 {
			llama.ModelFree(instance.model)
		}
		llama.Close()
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
