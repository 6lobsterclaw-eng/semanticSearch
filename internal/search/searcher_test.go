package search

import (
	"context"
	"testing"

	"github.com/kelindar/search"
	"semantic-search/internal/storage"
)

func TestSearcher(t *testing.T) {
	t.Skip("Requires network for rembed model download")

	db, err := storage.NewDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.InitSchema(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Insert test document
	doc := storage.Document{
		ID:      "test-1",
		Path:    "/docs/test.md",
		Content: "Hello world this is a test document",
		Title:   "Test",
	}
	err = db.InsertDocument(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}

	// Insert embedding (all zeros for testing)
	emb := make([]float32, 384)
	err = db.InsertEmbedding(context.Background(), doc.ID, emb)
	if err != nil {
		t.Fatal(err)
	}

	s, err := NewSearcher(db)
	if err != nil {
		t.Fatal(err)
	}
	results, err := s.Search(context.Background(), "test query", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("expected results, got none")
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := search.Vector{1, 0, 0}
	b := search.Vector{1, 0, 0}
	sim := cosineSimilarity(a, b)
	if sim != 1.0 {
		t.Errorf("expected 1.0, got %f", sim)
	}

	// Orthogonal vectors
	c := search.Vector{1, 0, 0}
	d := search.Vector{0, 1, 0}
	sim = cosineSimilarity(c, d)
	if sim != 0.0 {
		t.Errorf("expected 0.0, got %f", sim)
	}
}
