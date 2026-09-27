package indexer

import (
	"testing"
)

func TestIndexerInterface(t *testing.T) {
	// Verify Indexer can be created (will fail until we implement it)
	// For now, just verify Embedder works
	emb, err := NewEmbedder()
	if err != nil {
		t.Skipf("Embedder needs network to download model: %v", err)
	}
	if emb == nil {
		t.Error("expected embedder, got nil")
	}
}
