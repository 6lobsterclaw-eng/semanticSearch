package indexer

import (
	"encoding/json"
	"fmt"
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
	// Create export directory
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create export dir: %w", err)
	}

	// 1. Export vector index
	vectorPath := filepath.Join(dirPath, "vectors.bin")
	if err := idx.index.WriteFile(vectorPath); err != nil {
		return fmt.Errorf("failed to export vectors: %w", err)
	}

	// 2. Export Bleve index
	if idx.bleveIdx != nil {
		blevePath := filepath.Join(dirPath, "bleve")
		if err := idx.bleveIdx.Close(); err != nil {
			return fmt.Errorf("failed to close bleve: %w", err)
		}
		// Move temp bleve index to export location
		tempBlevePath := filepath.Join(os.TempDir(), "bleve_ngram_index")
		if _, err := os.Stat(tempBlevePath); err == nil {
			if err := os.Rename(tempBlevePath, blevePath); err != nil {
				return fmt.Errorf("failed to move bleve index: %w", err)
			}
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
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// Import imports the entire index from a directory
func (idx *Indexer) Import(dirPath string) error {
	// 1. Import vector index
	vectorPath := filepath.Join(dirPath, "vectors.bin")
	if err := idx.index.ReadFile(vectorPath); err != nil {
		return fmt.Errorf("failed to import vectors: %w", err)
	}

	// 2. Import Bleve index
	blevePath := filepath.Join(dirPath, "bleve")
	if _, err := os.Stat(blevePath); err == nil {
		// Close existing bleve if any
		if idx.bleveIdx != nil {
			idx.bleveIdx.Close()
		}
		// Remove temp bleve path
		tempBlevePath := filepath.Join(os.TempDir(), "bleve_ngram_index")
		os.RemoveAll(tempBlevePath)
		// Move imported bleve to temp location
		if err := os.Rename(blevePath, tempBlevePath); err != nil {
			return fmt.Errorf("failed to move bleve index: %w", err)
		}
		// Open the bleve index
		var err error
		idx.bleveIdx, err = bleve.Open(tempBlevePath)
		if err != nil {
			return fmt.Errorf("failed to open bleve index: %w", err)
		}
	}

	// 3. Import metadata
	metaPath := filepath.Join(dirPath, "metadata.json")
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
