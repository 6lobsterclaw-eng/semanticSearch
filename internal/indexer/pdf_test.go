package indexer

import (
	"os"
	"testing"
)

func TestPDFExtraction(t *testing.T) {
	// Skip if no sample PDF available
	t.Skip("Sample PDF needed for testing")
}

func TestExtractTextFromFile(t *testing.T) {
	// Create a temp file to test error handling
	tmpfile, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	
	content := []byte("test content")
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Should return error for non-PDF file
	_, err = ExtractText(tmpfile.Name())
	if err == nil {
		t.Error("expected error for non-PDF file")
	}
}
