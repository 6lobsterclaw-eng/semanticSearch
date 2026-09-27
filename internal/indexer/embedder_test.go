package indexer

import (
	"context"
	"testing"
)

func TestEmbedder(t *testing.T) {
	e := NewEmbedder()
	vec, err := e.Embed(context.Background(), "Hello world")
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	// Default embedding dimension
	if len(vec) != 384 {
		t.Errorf("expected 384 dimensions, got %d", len(vec))
	}
}

func TestEmbedderEmpty(t *testing.T) {
	e := NewEmbedder()
	vec, err := e.Embed(context.Background(), "")
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vec) != 384 {
		t.Errorf("expected 384 dimensions, got %d", len(vec))
	}
}
