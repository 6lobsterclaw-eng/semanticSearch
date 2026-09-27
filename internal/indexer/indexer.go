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
	
	// First try regular open
	f, r, err := pdf.Open(path)
	if err == nil {
		// Success - normal PDF
		defer f.Close()
		return extractTextFromReader(r)
	}
	
	// If regular open fails, try with encrypted reader using empty password
	// This should work for PDFs that are encrypted for "change" but allow reading
	log.Printf("Regular open failed, trying encrypted reader with empty password: %v", err)
	
	file, err := os.Open(path)
	if err != nil {
		log.Printf("Error opening file: %v", err)
		return nil, err
	}
	defer file.Close()
	
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	
	// Try with empty password - callback returns "" to try empty password
	pw := func() string { return "" }
	reader, err := pdf.NewReaderEncrypted(file, stat.Size(), pw)
	if err != nil {
		log.Printf("Encrypted reader failed: %v", err)
		return nil, err
	}
	
	// Create wrapper to match the Reader interface
	wrapper := &pdfReaderWrapper{reader}
	return extractTextFromReader(wrapper)
}

type pdfReaderWrapper struct {
	r *pdf.Reader
}

func (w *pdfReaderWrapper) NumPage() int {
	return w.r.NumPage()
}

func (w *pdfReaderWrapper) Page(num int) pdf.Page {
	return w.r.Page(num)
}

func extractTextFromReader(r interface{ NumPage() int; Page(int) pdf.Page }) ([]byte, error) {
	var text []byte
	numPages := r.NumPage()
	log.Printf("PDF has %d pages", numPages)
	
	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		txt, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text = append(text, []byte(txt)...)
		text = append(text, '\n')
	}
	
	if len(text) == 0 {
		log.Printf("No text extracted from PDF")
		return nil, nil
	}
	
	log.Printf("Extracted %d bytes from PDF", len(text))
	return text, nil
}
