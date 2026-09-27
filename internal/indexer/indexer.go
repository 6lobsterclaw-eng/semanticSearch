package indexer

import (
	"log"
	"os"
	"path/filepath"

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

	if len(content) == 0 {
		log.Printf("Skipping empty file: %s", path)
		return nil
	}

	doc := map[string]string{
		"title":   filepath.Base(path),
		"content": string(content),
		"path":    path,
	}

	return idx.Index(path, doc)
}

func extractPDF(path string) ([]byte, error) {
	log.Printf("Extracting PDF: %s", path)
	
	// Try to open PDF
	f, r, err := pdf.Open(path)
	if err != nil {
		// Log the error but continue
		log.Printf("PDF open error: %v", err)
		
		// Try to read anyway - some PDFs can still be read even with encryption warning
		// The library might have opened it partially
	}
	
	// Even if there's an error, try to read if we got a valid reader
	if r != nil {
		defer f.Close()
		
		var text []byte
		numPages := r.NumPage()
		log.Printf("PDF has %d pages", numPages)
		
		for i := 1; i <= numPages; i++ {
			p := r.Page(i)
			if p.V.IsNull() {
				continue
			}
			txt, err := p.GetPlainText(nil)
			if err != nil {
				continue
			}
			text = append(text, []byte(txt)...)
			text = append(text, '\n')
		}
		
		if len(text) > 0 {
			log.Printf("Extracted %d bytes from PDF", len(text))
			return text, nil
		}
	}
	
	// If we get here, the PDF couldn't be read properly
	// Return error so it's logged
	return nil, err
}
