package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/go-resty/resty/v2"
)

var ErrEmbedFailed = errors.New("embedding generation failed")

type Embedder struct {
	apiURL  string
	apiKey  string
	model   string
	useLocal bool
}

type Embedding []float32

type EmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func NewEmbedder() *Embedder {
	apiKey := os.Getenv("EMBEDDING_API_KEY")
	return &Embedder{
		apiURL:  "https://api.openai.com/v1/embeddings",
		apiKey:  apiKey,
		model:   "text-embedding-3-small",
		useLocal: apiKey == "",
	}
}

func (e *Embedder) Embed(ctx context.Context, text string) (Embedding, error) {
	if e.useLocal {
		return e.embedLocal(ctx, text)
	}
	return e.embedAPI(ctx, text)
}

func (e *Embedder) embedAPI(ctx context.Context, text string) (Embedding, error) {
	req := map[string]interface{}{
		"input": text,
		"model": e.model,
	}

	resp, err := resty.New().R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+e.apiKey).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		Post(e.apiURL)

	if err != nil {
		return nil, err
	}

	var result EmbeddingResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, ErrEmbedFailed
	}

	// Convert []float64 to []float32
	emb := make(Embedding, len(result.Data[0].Embedding))
	for i, v := range result.Data[0].Embedding {
		emb[i] = float32(v)
	}
	return emb, nil
}

func (e *Embedder) embedLocal(ctx context.Context, text string) (Embedding, error) {
	// Fallback: return zero vector for offline mode
	// TODO: Add local ONNX model support
	return make(Embedding, 384), nil
}
