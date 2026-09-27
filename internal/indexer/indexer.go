package indexer

import (
	"fmt"
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

	// Split content into paragraphs/chunks for better search granularity
	chunks := splitIntoChunks(string(content), 500) // 500 chars per chunk
	log.Printf("Split into %d chunks", len(chunks))
	
	// Index each chunk as separate document
	for i, chunk := range chunks {
		chunkID := fmt.Sprintf("%s#chunk%d", path, i)
		chunkDoc := map[string]string{
			"title":   fmt.Sprintf("%s [Part %d]", filepath.Base(path), i+1),
			"content": chunk,
			"path":    path,
			"chunk":   fmt.Sprintf("%d", i),
		}
		
		if err := idx.Index(chunkID, chunkDoc); err != nil {
			log.Printf("Error indexing chunk %d: %v", i, err)
			continue
		}
	}
	
	log.Printf("Indexed %d chunks for: %s", len(chunks), path)
	log.Printf("=======================")
	
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

// splitIntoChunks splits text into chunks of approximately maxSize characters
// It splits on paragraph boundaries when possible
func splitIntoChunks(text string, maxSize int) []string {
	if len(text) <= maxSize {
		return []string{text}
	}
	
	var chunks []string
	paragraphs := strings.Split(text, "\n")
	
	var currentChunk string
	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		
		// If single paragraph is too big, split by sentences
		if len(para) > maxSize {
			// Flush current chunk
			if currentChunk != "" {
				chunks = append(chunks, currentChunk)
				currentChunk = ""
			}
			
			// Split long paragraph by sentences
			sentences := strings.Split(para, ". ")
			var sentenceChunk string
			for _, sent := range sentences {
				sent = strings.TrimSpace(sent)
				if sent == "" {
					continue
				}
				// Add period back
				if !strings.HasSuffix(sent, ".") {
					sent = sent + "."
				}
				
				if len(sentenceChunk)+len(sent) > maxSize {
					chunks = append(chunks, sentenceChunk)
					sentenceChunk = sent
				} else {
					if sentenceChunk != "" {
						sentenceChunk += " " + sent
					} else {
						sentenceChunk = sent
					}
				}
			}
			if sentenceChunk != "" {
				chunks = append(chunks, sentenceChunk)
			}
			continue
		}
		
		// Normal paragraph
		if len(currentChunk)+len(para) > maxSize {
			chunks = append(chunks, currentChunk)
			currentChunk = para
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n" + para
			} else {
				currentChunk = para
			}
		}
	}
	
	// Don't forget last chunk
	if currentChunk != "" {
		chunks = append(chunks, currentChunk)
	}
	
	return chunks
}

// AddChunkFunc is a callback function type for adding chunks to semantic index
type AddChunkFunc func(path, title, content string) error

// IndexForSemantic indexes documents for semantic search using embeddings
// It walks the folder and calls addChunk for each document's content chunks
func IndexForSemantic(idx bleve.Index, dirPath string, addChunk AddChunkFunc) error {
	log.Printf("Semantic indexing directory: %s", dirPath)

	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return err
	}

	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".pdf" && ext != ".md" {
			return nil
		}

		log.Printf("Semantic indexing: %s", path)

		var content string
		var contentBytes []byte

		if ext == ".pdf" {
			var err error
			contentBytes, err = extractPDF(path)
			if err != nil {
				log.Printf("Error extracting PDF %s: %v", path, err)
				return nil
			}
			content = string(contentBytes)
		} else if ext == ".md" {
			var err error
			contentBytes, err = os.ReadFile(path)
			if err != nil {
				log.Printf("Error reading MD %s: %v", path, err)
				return nil
			}
			content = string(contentBytes)
		}

		if len(content) == 0 {
			return nil
		}

		// Get title from filename
		title := filepath.Base(path)

		// Chunk the content
		chunks := splitIntoChunks(content, 500)
		log.Printf("Split into %d chunks", len(chunks))

		// Add each chunk to semantic index
		for i, chunk := range chunks {
			chunkTitle := fmt.Sprintf("%s (chunk %d)", title, i+1)
			if err := addChunk(path, chunkTitle, chunk); err != nil {
				log.Printf("Error adding chunk %d for %s: %v", i, path, err)
				continue
			}
		}

		return nil
	})
}
