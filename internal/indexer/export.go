package indexer

import (
	"encoding/gob"
	"log"
	"os"
	"time"
)

// Chunk represents a single indexed sentence with its embedding
type Chunk struct {
	ID                   string
	Source               string
	Sentence             string
	ParentID             string  // Reference to parent chunk for parent-child chunking
	Embedding            []float32
	GeneratedQuestions   []string  // LLM-generated questions for this chunk
	QuestionEmbeddings  [][]float32  // Embeddings for generated questions
}

// ExportedIndex represents the full exported index data
type ExportedIndex struct {
	Version   string
	Exported  time.Time
	Chunks    []Chunk
}

// Export saves the index as a binary gob file
func (idx *Indexer) Export(path string) error {
	// Gather all chunks from the embedder
	chunks := idx.gatherChunks()
	
	exported := ExportedIndex{
		Version:  "1.0",
		Exported: time.Now(),
		Chunks:   chunks,
	}
	
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	
	enc := gob.NewEncoder(f)
	if err := enc.Encode(exported); err != nil {
		return err
	}
	
	log.Printf("Exported %d chunks to %s", len(chunks), path)
	return nil
}

// Import loads an index from a binary gob file
func (idx *Indexer) Import(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var exported ExportedIndex
	dec := gob.NewDecoder(f)
	if err := dec.Decode(&exported); err != nil {
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

	log.Printf("Imported %d chunks from %s", len(exported.Chunks), path)
	return nil
}

// gatherChunks returns all stored chunks for export
func (idx *Indexer) gatherChunks() []Chunk {
	return idx.storedChunks
}

// ChunkCount returns the number of indexed chunks
func (idx *Indexer) ChunkCount() int {
	return idx.DocumentCount()
}
