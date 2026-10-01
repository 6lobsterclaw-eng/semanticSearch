package indexer

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	chunkMap     map[string]Chunk // chunkID -> Chunk for lookup during search
	parentMap    map[string]Chunk // parentID -> Parent Chunk for parent-child retrieval
	docCount     int              // total chunks
	fileCount    int              // number of files indexed
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
		index:      search.NewIndex[string](),
		embedder:   embedder,
		chunkMap:   make(map[string]Chunk),
		parentMap:  make(map[string]Chunk),
	}
}

// IndexFolder indexes all PDF and MD files in a directory
// progressFn is called with (done, total, chunks) counts during indexing
func (idx *Indexer) IndexFolder(dirPath string, progressFn func(done, total, chunks int)) error {
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
		chunks, err := idx.indexFile(file)
		if err != nil {
			continue
		}
		if progressFn != nil {
			progressFn(i+1, total, chunks)
		}
	}

	return nil
}

func (idx *Indexer) indexFile(path string) (int, error) {
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
		return 0, nil
	}

	if err != nil || content == "" {
		return 0, err
	}

	title := extractTitle(path)
	docID := filepath.Base(path)

	// Parent-Child Chunking Strategy:
	// 1. Split into parent chunks (~1000 tokens each)
	// 2. Split each parent into child chunks (~150 tokens each)
	// 3. Embed child chunks, store parent reference

	// First, split into parent chunks (by paragraphs/sections)
	parentChunks := splitIntoParentChunks(content)
	log.Printf("Indexing %s: split into %d parent chunks", path, len(parentChunks))

	childCount := 0

	// Process each parent chunk
	for parentIdx, parentContent := range parentChunks {
		parentID := fmt.Sprintf("%s#parent#%d", docID, parentIdx)

		// Split parent into child chunks
		childChunks := splitIntoChildChunks(parentContent)
		log.Printf("  Parent %d: %d child chunks", parentIdx, len(childChunks))

		// Store parent chunk
		parentChunk := Chunk{
			ID:        parentID,
			Source:    title,
			Sentence:  parentContent,
			ParentID:  "", // Parent has no parent
		}
		idx.parentMap[parentID] = parentChunk

		// Embed each child chunk
		for childIdx, childContent := range childChunks {
			vec, err := idx.embedder.Embed(childContent)
			if err != nil {
				log.Printf("Warning: failed to embed child from %s: %v", path, err)
				continue
			}

			// Child chunk ID references parent
			childID := fmt.Sprintf("%s#child#%d#%d", docID, parentIdx, childIdx)
			idx.embedder.AddDocument(childID, vec, title+" | "+childContent)
			idx.docCount++
			childCount++

			// Store child chunk with parent reference
			chunk := Chunk{
				ID:        childID,
				Source:    title,
				Sentence:  childContent,
				ParentID:  parentID,
				Embedding: vec,
			}
			idx.storedChunks = append(idx.storedChunks, chunk)
			idx.chunkMap[childID] = chunk
		}
	}

	log.Printf("Indexing %s: total %d child chunks indexed", path, childCount)

	// Increment file count after successful indexing
	idx.fileCount++

	return childCount, nil
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

// splitIntoParentChunks splits text into larger parent chunks (~1000 tokens / ~5000 chars)
// Splits on double newlines (paragraphs) or headers
func splitIntoParentChunks(text string) []string {
	if len(text) == 0 {
		return nil
	}

	// Normalize whitespace
	text = strings.Join(strings.Fields(text), " ")

	// Split on double newlines (paragraph breaks) or markdown headers
	// This preserves logical sections as parent chunks
	re := regexp.MustCompile(`(?m)(?:\n\n+|#+\s)`)
	parts := re.Split(text, -1)

	var chunks []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if len(trimmed) > 50 { // Skip very small fragments
			chunks = append(chunks, trimmed)
		}
	}

	if len(chunks) == 0 && len(text) > 0 {
		return []string{text}
	}

	// If chunks are too large, split further
	var result []string
	for _, chunk := range chunks {
		// Split large chunks into ~2000 char pieces (parent chunk size)
		if len(chunk) > 2000 {
			for i := 0; i < len(chunk); i += 1800 {
				end := i + 1800
				if end > len(chunk) {
					end = len(chunk)
				}
				result = append(result, chunk[i:end])
			}
		} else if len(chunk) > 0 {
			result = append(result, chunk)
		}
	}

	return result
}

// splitIntoChildChunks splits parent chunk into smaller child chunks (~150 tokens / ~800 chars)
// Each child is 1-2 sentences for precise vector matching
func splitIntoChildChunks(parentText string) []string {
	if len(parentText) == 0 {
		return nil
	}

	// Use the existing sentence splitting logic
	sentences := splitIntoSentences(parentText)

	// Merge sentences into chunks of ~200 chars (~1-2 sentences, ~40 tokens)
	var chunks []string
	var current strings.Builder

	for _, sentence := range sentences {
		if current.Len()+len(sentence)+1 > 200 {
			// Current chunk is full, save it
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
		}
		if current.Len() > 0 {
			current.WriteString(". ")
		}
		current.WriteString(sentence)
	}

	// Don't forget the last chunk
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	// If we only have one chunk, just return it
	if len(chunks) == 0 {
		return []string{parentText}
	}

	return chunks
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
	return idx.docCount
}

// FileCount returns the number of indexed files
func (idx *Indexer) FileCount() int {
	return idx.fileCount
}

// SearchResult contains enriched search result data
type SearchResult struct {
	Index     int     `json:"index"`
	ChunkID   string  `json:"chunkId"`
	Path      string  `json:"path"`
	Title     string  `json:"title"`
	Extract   string  `json:"extract"`
	Location  string  `json:"location"`
	Score     float64 `json:"score"`
	IsKeyword bool    `json:"isKeyword"` // true if from keyword search
}

// Search searches indexed documents and returns enriched results
// mode: "semantic" for vector search, "keyword" for substring match, "hybrid" for both
// Uses Weighted Linear Combination: Score = α×Schild + (1-α)×Sparent
// α = 0.7 (70% child vector, 30% parent BM25)
func (idx *Indexer) Search(query string, k int, mode string) []SearchResult {
	log.Printf("[DEBUG Indexer.Search] Query: %q, mode: %s", query, mode)
	
	// If no mode specified, default to semantic
	if mode == "" {
		mode = "semantic"
	}

	var semanticResults, keywordResults []SearchResult
	
	// Semantic search (vector-based with parent-child weighted scoring)
	if mode == "semantic" || mode == "hybrid" {
		semanticResults = idx.searchSemanticWeighted(query, k)
	}

	// Keyword search (substring match)
	if mode == "keyword" || mode == "hybrid" {
		keywordResults = idx.searchKeyword(query, k)
	}

	// Merge results
	return idx.mergeResults(query, semanticResults, keywordResults, mode)
}

// searchSemanticWeighted performs vector search with parent-child weighted scoring
// Final Score = α×Schild + (1-α)×Sparent
// α = 0.7 (70% child, 30% parent)
func (idx *Indexer) searchSemanticWeighted(query string, k int) []SearchResult {
	const alpha = 0.7 // Weight for child score
	
	vec, err := idx.embedder.Embed(query)
	if err != nil {
		log.Printf("[DEBUG Indexer.searchSemanticWeighted] Embed error: %v", err)
		return nil
	}

	log.Printf("[DEBUG Indexer.searchSemanticWeighted] Query vec dim=%d", len(vec))
	results := idx.embedder.Search(vec, k*3) // Get more results for re-ranking

	log.Printf("[DEBUG Indexer.searchSemanticWeighted] Got %d raw results", len(results))

	// Group child results by parent
	type childResult struct {
		chunkID string
		child   Chunk
		schild  float64
	}
	
	parentChildren := make(map[string][]childResult)
	
	for _, r := range results {
		chunkID := r.Value
		chunk, ok := idx.chunkMap[chunkID]
		if !ok {
			continue
		}
		
		// Skip parent chunks (not children)
		if chunk.ParentID == "" {
			continue
		}
		
		schild := float64(r.Relevance)
		
		parentChildren[chunk.ParentID] = append(parentChildren[chunk.ParentID], childResult{
			chunkID: chunkID,
			child:   chunk,
			schild:  schild,
		})
	}

	// Calculate weighted scores for each parent
	type scoredParent struct {
		parentID    string
		parent      Chunk
		children    []childResult
		schildMax   float64  // Best child vector score
		sparent     float64  // Parent BM25 score
		finalScore  float64  // Weighted combination
	}
	
	var scoredParents []scoredParent
	
	for parentID, children := range parentChildren {
		parent, ok := idx.parentMap[parentID]
		if !ok {
			continue
		}
		
		// Calculate parent BM25 score (simplified: keyword frequency)
		sparent := calculateBM25(parent.Sentence, query)
		
		// Find max child score
		var schildMax float64
		for _, c := range children {
			if c.schild > schildMax {
				schildMax = c.schild
			}
		}
		
		// Weighted Linear Combination: α×Schild + (1-α)×Sparent
		// Normalize scores to 0-1 range first
		schildNorm := schildMax // Already 0-1 from cosine similarity
		sparentNorm := sparant  // Normalize BM25 to 0-1
		
		finalScore := alpha*schildNorm + (1-alpha)*sparentNorm
		
		scoredParents = append(scoredParents, scoredParent{
			parentID:   parentID,
			parent:     parent,
			children:   children,
			schildMax:  schildMax,
			sparent:    sparant,
			finalScore: finalScore,
		})
	}
	
	// Sort by final score descending
	sort.Slice(scoredParents, func(i, j int) bool {
		return scoredParents[i].finalScore > scoredParents[j].finalScore
	})
	
	// Build final results (return parent content with child match info)
	var out []SearchResult
	for i, p := range scoredParents {
		if i >= k {
			break
		}
		
		// Find best child match for this parent
		var bestChild childResult
		for _, c := range p.children {
			if bestChild.chunkID == "" || c.schild > bestChild.schild {
				bestChild = c
			}
		}
		
		log.Printf("[DEBUG] Parent %s: schild=%.4f, sparant=%.4f, final=%.4f", 
			p.parentID, p.schildMax, p.sparent, p.finalScore)
		
		extract := boldKeyword(p.parent.Sentence, query)
		
		out = append(out, SearchResult{
			Index:     i + 1,
			ChunkID:   bestChild.chunkID,
			Path:      p.parent.Source,
			Title:     p.parent.Source,
			Extract:   extract,
			Location:  findLocation(bestChild.chunkID, p.parent.Source),
			Score:     p.finalScore,
			IsKeyword: false,
		})
	}
	
	return out
}

// calculateBM25 calculates a simplified BM25 score for a document
// Returns normalized score between 0 and 1
func calculateBM25(doc, query string) float64 {
	docLower := strings.ToLower(doc)
	queryLower := strings.ToLower(query)
	
	// Simple term frequency approach (simplified BM25)
	words := strings.Fields(queryLower)
	tf := 0
	for _, word := range words {
		tf += strings.Count(docLower, word)
	}
	
	// Normalize by document length (longer docs can have more matches)
	docLen := float64(len(strings.Fields(docLower)))
	if docLen == 0 {
		return 0
	}
	
	// IDF would need collection stats, so we use simple TF normalization
	// Score = tf / (tf + 1) to cap at 1.0
	score := float64(tf) / (float64(tf) + 1)
	
	return score
}

// searchSemantic performs vector-based semantic search
func (idx *Indexer) searchSemantic(query string, k int) []SearchResult {
	vec, err := idx.embedder.Embed(query)
	if err != nil {
		log.Printf("[DEBUG Indexer.searchSemantic] Embed error: %v", err)
		return nil
	}

	log.Printf("[DEBUG Indexer.searchSemantic] Query vec dim=%d", len(vec))
	results := idx.embedder.Search(vec, k*2) // Get more to filter

	log.Printf("[DEBUG Indexer.searchSemantic] Got %d raw results", len(results))

	var out []SearchResult
	seen := make(map[string]bool) // Track seen chunkIDs
	
	for i, r := range results {
		chunkID := r.Value
		if seen[chunkID] {
			continue
		}
		seen[chunkID] = true
		
		log.Printf("[DEBUG Indexer.searchSemantic] Result %d: chunkID=%q, relevance=%.4f", i+1, chunkID, r.Relevance)

		chunk, ok := idx.chunkMap[chunkID]
		if !ok {
			parts := strings.Split(r.Value, " | ")
			title := ""
			sentence := ""
			if len(parts) >= 2 {
				title = parts[0]
				sentence = parts[1]
			}
			chunk = Chunk{
				ID:       chunkID,
				Source:   title,
				Sentence: sentence,
			}
		}

		// Parent-Child: Get parent content for context
		contentToShow := chunk.Sentence
		if chunk.ParentID != "" {
			if parent, ok := idx.parentMap[chunk.ParentID]; ok {
				contentToShow = parent.Sentence
				log.Printf("[DEBUG] Using parent content for child %s", chunkID)
			}
		}

		extract := boldKeyword(contentToShow, query)
		location := findLocation(chunkID, chunk.Source)

		out = append(out, SearchResult{
			Index:     len(out) + 1,
			ChunkID:   chunkID,
			Path:      chunk.Source,
			Title:     chunk.Source,
			Extract:   extract,
			Location:  location,
			Score:     float64(r.Relevance),
			IsKeyword: false, // Semantic result
		})
	}

	return out
}

// searchKeyword performs substring-based keyword search
func (idx *Indexer) searchKeyword(query string, k int) []SearchResult {
	log.Printf("[DEBUG Indexer.searchKeyword] Query: %q", query)
	
	queryLower := strings.ToLower(query)
	var results []SearchResult
	
	for chunkID, chunk := range idx.chunkMap {
		// Check if query appears in chunk content
		contentLower := strings.ToLower(chunk.Sentence)
		if strings.Contains(contentLower, queryLower) {
			// Parent-Child: Get parent content for context
			contentToShow := chunk.Sentence
			if chunk.ParentID != "" {
				if parent, ok := idx.parentMap[chunk.ParentID]; ok {
					contentToShow = parent.Sentence
				}
			}
			
			extract := boldKeyword(contentToShow, query)
			location := findLocation(chunkID, chunk.Source)
			
			// Position determines rank (earlier = better rank)
			pos := strings.Index(strings.ToLower(contentToShow), queryLower)
			
			results = append(results, SearchResult{
				ChunkID:   chunkID,
				Path:      chunk.Source,
				Title:     chunk.Source,
				Extract:   extract,
				Location:  location,
				Score:     float64(pos), // Use position as score (lower = better)
				IsKeyword: true,
			})
		}
	}
	
	// Sort by position (earlier = better rank = lower score)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score < results[j].Score
	})
	
	// Assign proper indices
	for i := range results {
		results[i].Index = i + 1
	}
	
	log.Printf("[DEBUG Indexer.searchKeyword] Found %d keyword matches", len(results))
	return results
}

// mergeResults combines semantic and keyword results using Reciprocal Rank Fusion (RRF)
// RRF Formula: Score = 1/(k+rank_semantic) + 1/(k+rank_keyword)
// k=60 is the smoothing constant to prevent top results from dominating
func (idx *Indexer) mergeResults(query string, semantic, keyword []SearchResult, mode string) []SearchResult {
	if mode == "semantic" {
		return semantic
	}
	if mode == "keyword" {
		return keyword
	}

	// Hybrid mode: use RRF to combine results
	const k = 60 // RRF smoothing constant
	
	// Build RRF scores map
	rrfScores := make(map[string]float64)
	seen := make(map[string]int) // Track which sources contributed (for IsKeyword flag)
	
	// Add semantic results with their ranks
	for rank, r := range semantic {
		rrfScores[r.ChunkID] += 1.0 / (k + float64(rank+1)) // rank is 0-indexed, convert to 1-indexed
		seen[r.ChunkID] = seen[r.ChunkID] | 1 // bit 1 = semantic
	}
	
	// Add keyword results with their ranks
	for rank, r := range keyword {
		rrfScores[r.ChunkID] += 1.0 / (k + float64(rank+1))
		seen[r.ChunkID] = seen[r.ChunkID] | 2 // bit 2 = keyword
	}
	
	// Convert to sorted slice
	type rrfResult struct {
		chunkID   string
		score     float64
		semantic  bool
		keyword   bool
	}
	var results []rrfResult
	for chunkID, score := range rrfScores {
		s := seen[chunkID]
		results = append(results, rrfResult{
			chunkID: chunkID,
			score:   score,
			semantic: s&1 != 0,
			keyword:  s&2 != 0,
		})
	}
	
	// Sort by RRF score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	
	// Build final results
	var out []SearchResult
	for i, r := range results {
		// Look up chunk info (child chunk)
		chunk, ok := idx.chunkMap[r.chunkID]
		if !ok {
			continue
		}
		
		// Determine if it's keyword (true if only keyword, false if only semantic)
		// If both, show as keyword since keyword is more precise for exact matches
		isKeyword := r.keyword
		
		// Parent-Child: Get parent content for context
		contentToShow := chunk.Sentence
		if chunk.ParentID != "" {
			if parent, ok := idx.parentMap[chunk.ParentID]; ok {
				contentToShow = parent.Sentence
			}
		}
		
		// Bold the keyword in extract
		extract := boldKeyword(contentToShow, query)
		
		out = append(out, SearchResult{
			Index:     i + 1,
			ChunkID:   r.chunkID,
			Path:      chunk.Source,
			Title:     chunk.Source,
			Extract:   extract,
			Location:  findLocation(r.chunkID, chunk.Source),
			Score:     r.score,
			IsKeyword: isKeyword,
		})
	}
	
	return out
}

// boldKeyword wraps the keyword in <b> tags for HTML bold display
func boldKeyword(sentence, keyword string) string {
	lowerSentence := strings.ToLower(sentence)
	lowerKeyword := strings.ToLower(keyword)

	// Find keyword position
	pos := strings.Index(lowerSentence, lowerKeyword)
	if pos == -1 {
		// Keyword not found, return first 200 chars
		if len(sentence) > 200 {
			return sentence[:200] + "..."
		}
		return sentence
	}

	// Wrap keyword in <b> tags
	before := sentence[:pos]
	kw := sentence[pos:pos+len(keyword)]
	after := sentence[pos+len(keyword):]

	return before + "<b>" + kw + "</b>" + after
}

// findSentenceStart finds the start of the sentence containing pos
func findSentenceStart(content string, pos int) int {
	if pos > len(content) {
		pos = len(content)
	}
	for i := pos; i >= 0; i-- {
		if i > 0 && (content[i-1] == '.' || content[i-1] == '!' || content[i-1] == '?') {
			return i
		}
	}
	return 0
}

// findSentenceEnd finds the end of the sentence after pos
func findSentenceEnd(content string, pos int) int {
	if pos > len(content) {
		pos = len(content)
	}
	for i := pos; i < len(content); i++ {
		if content[i] == '.' || content[i] == '!' || content[i] == '?' {
			return i + 1
		}
	}
	return len(content)
}

// findLocation returns location info: filename and chunk number
func findLocation(chunkID, source string) string {
	// Get just the filename from source path
	filename := filepath.Base(source)

	// Extract chunk index from chunkID (format: "filename#index")
	parts := strings.Split(chunkID, "#")
	chunkNum := 1
	if len(parts) >= 2 {
		fmt.Sscanf(parts[1], "%d", &chunkNum)
		chunkNum++ // Make it 1-indexed
	}

	return fmt.Sprintf("%s(%d)", filename, chunkNum)
}
