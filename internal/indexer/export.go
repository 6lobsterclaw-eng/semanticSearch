package indexer

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/blevesearch/bleve/v2"
)

// Chunk represents a single indexed sentence with its embedding
type Chunk struct {
	ID                  string
	Source              string
	Sentence            string
	ParentID            string  // Reference to parent chunk for parent-child chunking
	Embedding           []float32
	GeneratedQuestions  []string  // LLM-generated questions for this chunk
	QuestionEmbeddings  [][]float32  // Embeddings for generated questions
}

// ChunkCount returns the number of indexed chunks
func (idx *Indexer) ChunkCount() int {
	return idx.docCount
}

// Export exports the entire index (vector + bleve + metadata) to a directory
func (idx *Indexer) Export(dirPath string) error {
	log.Printf("[EXPORT] Starting export to %s", dirPath)
	
	// Create export directory
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create export dir: %w", err)
	}

	// 1. Export vector index
	vectorPath := filepath.Join(dirPath, "vectors.bin")
	log.Printf("[EXPORT] Writing vectors to %s", vectorPath)
	if err := idx.index.WriteFile(vectorPath); err != nil {
		return fmt.Errorf("failed to export vectors: %w", err)
	}

	// 2. Export Bleve index
	if idx.bleveIdx != nil {
		log.Printf("[EXPORT] Closing and exporting Bleve index")
		blevePath := filepath.Join(dirPath, "bleve")
		if err := idx.bleveIdx.Close(); err != nil {
			return fmt.Errorf("failed to close bleve: %w", err)
		}
		// Move temp bleve index to export location
		tempBlevePath := filepath.Join(os.TempDir(), "bleve_ngram_index")
		if _, err := os.Stat(tempBlevePath); err == nil {
			log.Printf("[EXPORT] Copying bleve from %s to %s", tempBlevePath, blevePath)
			// Use copy instead of rename (Windows can't rename across drives)
			if err := copyDir(tempBlevePath, blevePath); err != nil {
				return fmt.Errorf("failed to copy bleve index: %w", err)
			}
			// Remove temp
			os.RemoveAll(tempBlevePath)
		} else {
			log.Printf("[EXPORT] Warning: temp bleve path not found: %s", tempBlevePath)
		}
		// Re-create bleve index for continued use
		if err := idx.InitBleveIndex(); err != nil {
			return fmt.Errorf("failed to reinit bleve: %w", err)
		}
	}

	// 3. Export metadata (chunkMap, parentMap, docCount, fileCount, source paths)
	metaPath := filepath.Join(dirPath, "metadata.json")
	meta := struct {
		DocCount     int              `json:"docCount"`
		FileCount    int              `json:"fileCount"`
		ChunkMap     map[string]Chunk `json:"chunkMap"`
		ParentMap    map[string]Chunk `json:"parentMap"`
		StoredChunks []Chunk          `json:"storedChunks"`
	}{
		DocCount:     idx.docCount,
		FileCount:    idx.fileCount,
		ChunkMap:     idx.chunkMap,
		ParentMap:    idx.parentMap,
		StoredChunks: idx.storedChunks,
	}
	log.Printf("[EXPORT] Writing metadata: docCount=%d, fileCount=%d, storedChunks=%d", idx.docCount, idx.fileCount, len(idx.storedChunks))
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	log.Printf("[EXPORT] Complete")
	return nil
}

// Import imports the entire index from a directory
func (idx *Indexer) Import(dirPath string) error {
	log.Printf("[IMPORT] Starting import from %s", dirPath)
	
	// 1. Import vector index
	vectorPath := filepath.Join(dirPath, "vectors.bin")
	log.Printf("[IMPORT] Reading vectors from %s", vectorPath)
	if err := idx.index.ReadFile(vectorPath); err != nil {
		return fmt.Errorf("failed to import vectors: %w", err)
	}

	// 2. Import Bleve index
	blevePath := filepath.Join(dirPath, "bleve")
	if _, err := os.Stat(blevePath); err == nil {
		log.Printf("[IMPORT] Found bleve index, importing...")
		// Close existing bleve if any
		if idx.bleveIdx != nil {
			idx.bleveIdx.Close()
		}
		// Remove temp bleve path
		tempBlevePath := filepath.Join(os.TempDir(), "bleve_ngram_index")
		os.RemoveAll(tempBlevePath)
		// Copy imported bleve to temp location (Windows can't rename across drives)
		log.Printf("[IMPORT] Copying bleve from %s to %s", blevePath, tempBlevePath)
		if err := copyDir(blevePath, tempBlevePath); err != nil {
			return fmt.Errorf("failed to copy bleve index: %w", err)
		}
		// Open the bleve index
		var err error
		idx.bleveIdx, err = bleve.Open(tempBlevePath)
		if err != nil {
			return fmt.Errorf("failed to open bleve index: %w", err)
		}
	} else {
		log.Printf("[IMPORT] No bleve index found at %s", blevePath)
	}

	// 3. Import metadata
	metaPath := filepath.Join(dirPath, "metadata.json")
	log.Printf("[IMPORT] Reading metadata from %s", metaPath)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}
	meta := struct {
		DocCount     int              `json:"docCount"`
		FileCount    int              `json:"fileCount"`
		ChunkMap     map[string]Chunk `json:"chunkMap"`
		ParentMap    map[string]Chunk `json:"parentMap"`
		StoredChunks []Chunk          `json:"storedChunks"`
	}{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	idx.docCount = meta.DocCount
	idx.fileCount = meta.FileCount
	idx.chunkMap = meta.ChunkMap
	idx.parentMap = meta.ParentMap
	idx.storedChunks = meta.StoredChunks

	log.Printf("[IMPORT] Complete: docCount=%d, fileCount=%d, storedChunks=%d", idx.docCount, idx.fileCount, len(idx.storedChunks))
	return nil
}

// LegacyExport saves the index as a binary gob file (single file)
func (idx *Indexer) LegacyExport(path string) error {
	exported := struct {
		Version     string
		Exported    time.Time
		Chunks      []Chunk
	}{
		Version:  "1.0",
		Exported: time.Now(),
		Chunks:   idx.storedChunks,
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Use JSON for simplicity instead of gob
	data, err := json.Marshal(exported)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// LegacyImport loads an index from a binary gob file
func (idx *Indexer) LegacyImport(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var exported struct {
		Version  string
		Exported time.Time
		Chunks   []Chunk
	}
	if err := json.Unmarshal(data, &exported); err != nil {
		return err
	}

	// Reset counters
	idx.docCount = 0
	idx.fileCount = 0

	// Track files seen
	filesSeen := make(map[string]bool)

	// Add each chunk to the index
	for _, chunk := range exported.Chunks {
		vec := make([]float32, len(chunk.Embedding))
		copy(vec, chunk.Embedding)
		idx.embedder.AddDocument(chunk.ID, vec, chunk.Source+" | "+chunk.Sentence)
		idx.storedChunks = append(idx.storedChunks, chunk)
		idx.chunkMap[chunk.ID] = chunk
		idx.docCount++

		// Count unique files
		if !filesSeen[chunk.Source] {
			filesSeen[chunk.Source] = true
			idx.fileCount++
		}
	}

	return nil
}

// copyDir copies a directory recursively
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}
