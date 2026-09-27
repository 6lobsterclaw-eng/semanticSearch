package indexer

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/ledongthuc/pdf"
)

func IndexFolder(idx bleve.Index, dirPath string) (int, error) {
	log.Printf("Indexing directory: %s", dirPath)

	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		log.Printf("Directory does not exist: %s", dirPath)
		return 0, err
	}

	var count int
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Walk error on %s: %v", path, err)
			return nil
		}

		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		log.Printf("Found file: %s (ext: %s)", path, ext)
		
		if ext != ".pdf" && ext != ".md" {
			return nil
		}

		log.Printf("Indexing: %s", path)

		if err := indexFile(idx, path); err != nil {
			log.Printf("Error indexing %s: %v", path, err)
			return nil
		}

		count++
		return nil
	})

	if err != nil {
		log.Printf("Walk error: %v", err)
		return count, err
	}

	log.Printf("Indexed %d files", count)
	return count, nil
}

func indexFile(idx bleve.Index, path string) error {
	ext := filepath.Ext(path)
	var content []byte
	var err error

	if ext == ".pdf" {
		content, err = extractPDF(path)
	} else if ext == ".md" {
		content, err = os.ReadFile(path)
	}

	if err != nil {
		log.Printf("Error reading %s: %v", path, err)
		return err
	}

	log.Printf("=== INDEXING DEBUG ===")
	log.Printf("File: %s", path)
	log.Printf("Extracted content length: %d bytes", len(content))
	
	if len(content) == 0 {
		log.Printf("WARNING: Empty content - skipping")
		log.Printf("=======================")
		return nil
	}

	// Show first 500 chars
	preview := string(content)
	if len(preview) > 500 {
		preview = preview[:500]
	}
	log.Printf("Content preview:\n%s", preview)
	log.Printf("=======================")

	doc := map[string]string{
		"title":   filepath.Base(path),
		"content": string(content),
		"path":    path,
	}

	log.Printf("Calling idx.Index(%s, doc)", path)
	if err := idx.Index(path, doc); err != nil {
		log.Printf("idx.Index error: %v", err)
		return err
	}
	log.Printf("Indexing complete for: %s", path)
	
	return nil
}

func extractPDF(path string) ([]byte, error) {
	log.Printf("=== PDF EXTRACTION DEBUG ===")
	log.Printf("Opening PDF: %s", path)
	
	// Try to open PDF
	f, r, err := pdf.Open(path)
	if err != nil {
		log.Printf("ERROR: PDF open failed: %v", err)
		log.Printf("============================")
		return nil, err
	}
	defer f.Close()
	
	log.Printf("PDF opened successfully")
	
	numPages := r.NumPage()
	log.Printf("PDF has %d pages", numPages)
	
	var allText []string
	var totalChars int
	
	for i := 1; i <= numPages; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			log.Printf("Page %d: V is null, skipping", i)
			continue
		}
		
		txt, err := p.GetPlainText(nil)
		if err != nil {
			log.Printf("Page %d: GetPlainText error: %v", i, err)
			continue
		}
		
		trimmed := strings.TrimSpace(txt)
		pageChars := len(trimmed)
		totalChars += pageChars
		
		if pageChars > 0 {
			log.Printf("Page %d: extracted %d chars", i, pageChars)
			// Show first 100 chars of each non-empty page
			preview := trimmed
			if len(preview) > 100 {
				preview = preview[:100]
			}
			log.Printf("  Preview: %s...", preview)
			allText = append(allText, trimmed)
		} else {
			log.Printf("Page %d: no text", i)
		}
	}
	
	log.Printf("Total extracted: %d chars from %d pages", totalChars, len(allText))
	
	if totalChars == 0 {
		log.Printf("WARNING: No text extracted from PDF!")
		log.Printf("============================")
		return nil, nil
	}
	
	result := []byte(strings.Join(allText, "\n\n"))
	log.Printf("Final result: %d bytes", len(result))
	log.Printf("============================")
	
	return result, nil
}
