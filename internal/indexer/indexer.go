package indexer

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/parser"
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
	// Store chunks with embeddings for export
	storedChunks []Chunk
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
// progressFn is called with (done, total) counts during indexing
func (idx *Indexer) IndexFolder(dirPath string, progressFn func(done, total int)) error {
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

	total := len(files)
	for i, file := range files {
		if err := idx.indexFile(file); err != nil {
			continue
		}
		if progressFn != nil {
			progressFn(i+1, total)
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
	case ".doc", ".docx":
		content, err = "", fmt.Errorf("DOC format not yet supported (convert to TXT)")
	default:
		return nil
	}

	if err != nil || content == "" {
		return err
	}

	title := extractTitle(path)
	docID := filepath.Base(path)

	// Split content into sentences for better search granularity
	sentences := splitIntoSentences(content)
	log.Printf("Indexing %s: split into %d sentences", path, len(sentences))

	// Generate embedding for each sentence and add to index
	for i, sentence := range sentences {
		vec, err := idx.embedder.Embed(sentence)
		if err != nil {
			log.Printf("Warning: failed to embed sentence from %s: %v", path, err)
			continue
		}

		// Add to search index with path as ID
		chunkID := fmt.Sprintf("%s#%d", docID, i)
		idx.embedder.AddDocument(chunkID, vec, title+" | "+sentence)
		
		// Store chunk for export
		idx.storedChunks = append(idx.storedChunks, Chunk{
			ID:        chunkID,
			Source:    title,
			Sentence:  sentence,
			Embedding: vec,
		})
	}

	return nil
}

func extractPDFText(path string) (string, error) {
	// Use ledongthuc/pdf for proper PDF text extraction
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	// Try to extract using pdf library
	reader := bytes.NewReader(data)
	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("create reader: %w", err)
	}

	var text []string
	numPages := pdfReader.NumPage()
	for i := 1; i <= numPages; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		if content != "" {
			text = append(text, content)
		}
	}

	if len(text) == 0 {
		return "", fmt.Errorf("no text extracted")
	}

	return strings.Join(text, "\n"), nil
}

func extractMarkdown(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Use gomarkdown to parse and convert to plain text
	ext := parser.CommonExtensions | parser.Attributes
	md := parser.NewWithExtensions(ext)
	html := markdown.ToHTML(data, md, nil)
	return stripHTML(string(html)), nil
}

func extractTitle(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		return base[:len(base)-len(ext)]
	}
	return base
}

func stripHTML(s string) string {
	var b strings.Builder
	in := false
	for _, c := range s {
		if c == '<' {
			in = true
		} else if c == '>' {
			in = false
		} else if !in {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// splitIntoSentences splits text into sentences at sentence boundaries
// Splits on . ! ? followed by space or newline, merges short chunks (<5 chars) with next
func splitIntoSentences(text string) []string {
	// Normalize whitespace first
	text = strings.Join(strings.Fields(text), " ")
	
	if len(text) == 0 {
		return nil
	}
	
	// Split on . ! ? followed by space or end of string
	re := regexp.MustCompile(`[.!?]+\s*`)
	parts := re.Split(text, -1)
	
	var sentences []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if len(trimmed) > 0 {
			sentences = append(sentences, trimmed)
		}
	}
	
	if len(sentences) == 0 {
		return []string{text}
	}
	
	// Merge short sentences with next
	var result []string
	var current string
	for _, s := range sentences {
		if len(current) == 0 {
			current = s
		} else if len(current) < 5 {
			// Merge with next if current is too short
			current = current + ". " + s
		} else {
			result = append(result, current)
			current = s
		}
	}
	// Don't forget the last one
	if len(current) > 0 {
		result = append(result, current)
	}
	
	if len(result) == 0 && len(text) > 0 {
		return []string{text}
	}
	
	return result
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
