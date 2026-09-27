package indexer

import (
	"os"
	"testing"
)

func TestMarkdownExtraction(t *testing.T) {
	// Create a temp markdown file
	tmpfile, err := os.CreateTemp("", "test*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	
	content := []byte("# Hello World\n\nThis is a test.")
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := ExtractMarkdown(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to extract: %v", err)
	}
	if len(result) == 0 {
		t.Error("content should not be empty")
	}
	// Should contain the text content
	if result == "" {
		t.Error("should have extracted text")
	}
}

func TestExtractTitle(t *testing.T) {
	title := ExtractTitle("/docs/readme.md")
	if title != "readme" {
		t.Errorf("expected 'readme', got '%s'", title)
	}
}
