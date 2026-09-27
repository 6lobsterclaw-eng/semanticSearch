package indexer

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/blevesearch/bleve/v2"
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
	
	// Try pdftotext - handles encrypted PDFs well
	// -layout preserves formatting
	// -enc UTF-8 ensures proper encoding
	cmd := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", path, "-")
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	if err != nil {
		log.Printf("pdftotext error: %v", err)
		if stderr.Len() > 0 {
			log.Printf("pdftotext stderr: %s", stderr.String())
		}
		return nil, err
	}
	
	content := stdout.Bytes()
	if len(content) == 0 {
		log.Printf("pdftotext returned empty content")
		return nil, nil
	}
	
	log.Printf("Extracted %d bytes from PDF using pdftotext", len(content))
	
	// Log first 200 chars of content for debugging
	preview := string(content)
	if len(preview) > 200 {
		preview = preview[:200]
	}
	log.Printf("Content preview: %s", preview)
	
	return content, nil
}
