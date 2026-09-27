package indexer

import (
	"testing"
)

// TestEmbedderGeneratesVectors verifies that rembed generates proper vectors
func TestEmbedderGeneratesVectors(t *testing.T) {
	// Skip if no network (rembed downloads models)
	t.Skip("Requires rembed model download - run manually with network")

	emb, err := NewEmbedder()
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	// Embed a simple text
	vec, err := emb.Embed("hello world")
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}

	// all-MiniLM-L6-v2 produces 384-dim vectors
	if len(vec) != 384 {
		t.Errorf("expected 384 dimensions, got %d", len(vec))
	}

	// Verify vector is not all zeros
	var sum float32
	for _, v := range vec {
		sum += v
	}
	if sum == 0 {
		t.Error("embedding is all zeros")
	}
}

// TestEmbedderConsistentResults verifies same text produces same embeddings
func TestEmbedderConsistentResults(t *testing.T) {
	t.Skip("Requires rembed model download - run manually with network")

	emb, err := NewEmbedder()
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}

	vec1, _ := emb.Embed("hello world")
	vec2, _ := emb.Embed("hello world")

	// Should be identical
	for i := range vec1 {
		if vec1[i] != vec2[i] {
			t.Error("same text produced different embeddings")
			break
		}
	}
}
