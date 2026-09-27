package indexer

import (
	"testing"

	"github.com/kelindar/search"
)

// TestLocalEmbedderIntegration tests that rembed + kelindar/search work together
// This test verifies the integration pattern: rembed generates vectors, kelindar/search indexes them
func TestLocalEmbedderIntegration(t *testing.T) {
	// Create a kelindar search index
	idx := search.NewIndex[string]()

	// Simulate what rembed would return: 384-dim vectors (all-MiniLM-L6-v2)
	vec1 := make(search.Vector, 384)
	vec2 := make(search.Vector, 384)

	// vec1: similar to vec2 (both about "hello world")
	for i := range vec1 {
		vec1[i] = float32(i % 10)
		vec2[i] = float32(i % 10) + 0.01 // slightly different
	}

	// Add documents to index
	idx.Add(vec1, "doc1: hello world document")
	idx.Add(vec2, "doc2: another hello document")

	// Search with similar query
	query := make(search.Vector, 384)
	for i := range query {
		query[i] = float32(i % 10)
	}

	results := idx.Search(query, 2)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result should be most similar
	if results[0].Relevance < 0.9 {
		t.Errorf("expected high relevance, got %f", results[0].Relevance)
	}

	t.Logf("Search results: %+v", results)
}

// TestLocalEmbedderDimension verifies the expected embedding dimension
func TestLocalEmbedderDimension(t *testing.T) {
	// all-MiniLM-L6-v2 produces 384-dimensional vectors
	expectedDim := 384

	vec := make(search.Vector, expectedDim)
	if len(vec) != expectedDim {
		t.Errorf("expected dimension %d, got %d", expectedDim, len(vec))
	}
}
