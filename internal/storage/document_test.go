package storage

import (
	"context"
	"testing"
)

func TestDocumentFields(t *testing.T) {
	doc := Document{
		ID:      "test-1",
		Path:    "/docs/readme.md",
		Content: "Hello world",
	}
	if doc.ID == "" {
		t.Error("ID should not be empty")
	}
	if doc.Path == "" {
		t.Error("Path should not be empty")
	}
}

func TestDBInitSchema(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	err = db.InitSchema(context.Background())
	if err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}
}

func TestDBInsertDocument(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	err = db.InitSchema(context.Background())
	if err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}

	doc := Document{
		ID:      "test-1",
		Path:    "/docs/readme.md",
		Content: "Hello world",
		Title:   "Readme",
	}

	err = db.InsertDocument(context.Background(), doc)
	if err != nil {
		t.Fatalf("failed to insert document: %v", err)
	}
}
