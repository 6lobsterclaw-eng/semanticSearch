package indexer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kelindar/search"
)

// Indexer orchestrates document indexing using an embedder
type Indexer struct {
	index    *search.Index[string]
	embedder interface {
		Embed(string) (search.Vector, error)
		AddDocument(string, search.Vector, string)
		Search(search.Vector, int) []search.Result[string]
		SaveIndex(string) error
		LoadIndex(string) error
	}
}

// NewIndexer creates a new indexer with the given embedder
func NewIndexer(embedder interface {
	Embed(string) (search.Vector, error)
	AddDocument(string, search.Vector, string)
	Search(search.Vector, int) []search.Result[string]
	SaveIndex(string) error
	LoadIndex(string) error
}) *Indexer {
	return &Indexer{
		index:    search.NewIndex[string](),
		embedder: embedder,
	}
}

// IndexFolder indexes all PDF and MD files in a directory
func (idx *Indexer) IndexFolder(dirPath string) error {
	var files []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".pdf" || ext == ".md" || ext == ".txt" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := idx.indexFile(file); err != nil {
			continue
		}
	}

	return nil
}

func (idx *Indexer) indexFile(path string) error {
	ext := strings.ToLower(filepath.Ext(path))

	var content string
	var err error

	switch ext {
	case ".pdf":
		content, err = extractPDFText(path)
	case ".md":
		content, err = extractMarkdown(path)
	case ".txt":
		data, err := os.ReadFile(path)
		if err == nil {
			content = string(data)
		}
	default:
		return nil
	}

	if err != nil || content == "" {
		return err
	}

	title := extractTitle(path)
	docID := filepath.Base(path)

	// Generate embedding
	vec, err := idx.embedder.Embed(content)
	if err != nil {
		return err
	}

	// Add to search index
	idx.embedder.AddDocument(docID, vec, title+" | "+content)

	return nil
}

func extractPDFText(path string) (string, error) {
	// Simple text extraction - could be enhanced
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func extractMarkdown(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func extractTitle(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		return base[:len(base)-len(ext)]
	}
	return base
}

// SaveIndex saves the index to a file
func (idx *Indexer) SaveIndex(path string) error {
	return idx.index.WriteFile(path)
}

// LoadIndex loads the index from a file
func (idx *Indexer) LoadIndex(path string) error {
	return idx.index.ReadFile(path)
}

// DocumentCount returns the number of indexed documents
func (idx *Indexer) DocumentCount() int {
	return idx.index.Len()
}

// Search searches indexed documents
func (idx *Indexer) Search(query string, k int) []search.Result[string] {
	vec, err := idx.embedder.Embed(query)
	if err != nil {
		return nil
	}
	return idx.embedder.Search(vec, k)
}
