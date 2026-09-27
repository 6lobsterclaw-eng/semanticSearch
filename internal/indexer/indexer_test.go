package indexer

import (
	"context"
	"os"
	"testing"

	"semantic-search/internal/storage"
)

func TestIndexerIndexDir(t *testing.T) {
	// Create temp dir with test files
	tmpdir, err := os.MkdirTemp("", "test-index")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// Create a test markdown file
	os.WriteFile(tmpdir+"/test.md", []byte("# Test\n\nHello world"), 0644)

	db, err := storage.NewDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	err = db.InitSchema(context.Background())
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	idx := NewIndexer(db)
	err = idx.IndexDir(context.Background(), tmpdir)
	if err != nil {
		t.Fatalf("index failed: %v", err)
	}

	docs, err := db.GetAllDocuments(context.Background())
	if err != nil {
		t.Fatalf("failed to get docs: %v", err)
	}

	if len(docs) != 1 {
		t.Errorf("expected 1 document, got %d", len(docs))
	}
}
