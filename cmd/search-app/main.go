package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"semantic-search/internal/indexer"
	sem "semantic-search/internal/semantic"
)

var (
	idx        bleve.Index
	modelPath  string
	semMu sync.RWMutex
	semOn bool
)

type SearchResult struct {
	Path    string  `json:"path"`
	Title   string  `json:"title"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

func main() {
	log.Println("Starting Semantic Search App...")

	// Create index with proper mapping
	mapping := bleve.NewIndexMapping()

	// Define document mapping
	docMapping := bleve.NewDocumentMapping()

	// Title field
	titleField := bleve.NewTextFieldMapping()
	titleField.Index = true
	titleField.Store = true
	docMapping.AddFieldMappingsAt("title", titleField)

	// Content field
	contentField := bleve.NewTextFieldMapping()
	contentField.Index = true
	contentField.Store = true
	docMapping.AddFieldMappingsAt("content", contentField)

	// Path field
	pathField := bleve.NewTextFieldMapping()
	pathField.Index = false
	pathField.Store = true
	docMapping.AddFieldMappingsAt("path", pathField)

	mapping.DefaultMapping = docMapping
	mapping.DefaultType = "text"
	mapping.DefaultAnalyzer = "standard"

	var err error
	idx, err = bleve.NewMemOnly(mapping)
	if err != nil {
		log.Fatalf("Failed to create index: %v", err)
	}
	log.Println("BM25 index ready")

	// HTML UI
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Semantic Search</title>
    <style>
        body { font-family: Arial; padding: 20px; max-width: 900px; margin: 0 auto; background: #f5f5f5; }
        h1 { color: #333; }
        .step { margin: 20px 0; padding: 15px; background: white; border: 1px solid #ddd; border-radius: 5px; }
        button { padding: 10px 20px; cursor: pointer; background: #007bff; color: white; border: none; border-radius: 3px; margin-right: 5px; }
        button.secondary { background: #6c757d; }
        .score { color: #666; font-size: 12px; }
        .snippet { margin: 10px 0; padding: 10px; background: #f0f0f0; border-left: 3px solid #007bff; font-style: italic; }
        #results { margin-top: 20px; }
        .result { padding: 15px; margin: 10px 0; background: white; border: 1px solid #ddd; border-radius: 5px; }
        .error { background: #f8d7da; color: #721c24; }
        .info { background: #d1ecf1; color: #0c5460; }
        .success { background: #d4edda; color: #155724; }
        .note { color: #666; font-size: 12px; }
        .mode-badge { display: inline-block; padding: 3px 8px; border-radius: 3px; font-size: 11px; margin-left: 10px; }
        .mode-bm25 { background: #ffc107; color: #000; }
        .mode-sem { background: #28a745; color: #fff; }
    </style>
</head>
<body>
    <h1>Semantic Search <span id="modeBadge" class="mode-badge mode-bm25">BM25</span></h1>
    
    <div class="step">
        <h3>Step 1: Configure GGUF Model (for Semantic Search)</h3>
        <input type="text" id="modelPath" placeholder="C:\path\to\nomic-embed-text-v1.5.gguf" style="width: 400px;">
        <input type="text" id="libPath" placeholder="C:\path\to\llama.dll (optional)" style="width: 250px;">
        <button onclick="setModel()">Enable Semantic Search</button>
        <button class="secondary" onclick="disableSemantic()">Use BM25 Only</button>
        <div id="modelStatus"></div>
        <p class="note">Note: BM25 works without a model. Semantic search needs GGUF model + llama.dll</p>
    </div>
    
    <div class="step">
        <h3>Step 2: Select Folder to Index</h3>
        <input type="text" id="folderPath" placeholder="C:\path\to\documents">
        <button onclick="indexFolder()">Index Folder</button>
        <button class="secondary" onclick="clearIndex()">Clear Index</button>
        <div id="indexStatus"></div>
    </div>
    
    <div class="step">
        <h3>Step 3: Search</h3>
        <input type="text" id="query" placeholder="Enter search query" style="width: 400px;">
        <button onclick="doSearch()">Search</button>
    </div>
    
    <div id="results"></div>

    <script>
        var semMode = false;
        
        function setModel() {
            var modelPath = document.getElementById('modelPath').value;
            var libPath = document.getElementById('libPath').value;
            if(!modelPath) { alert('Please enter model path'); return; }
            document.getElementById('modelStatus').innerHTML = '<div class="info">Loading model... (this may take a minute)</div>';
            var url = '/setModel?model=' + encodeURIComponent(modelPath);
            if(libPath) url += '&lib=' + encodeURIComponent(libPath);
            fetch(url)
                .then(r => r.json())
                .then(d => {
                    if(d.success) {
                        semMode = true;
                        document.getElementById('modelStatus').innerHTML = '<div class="success">Semantic search enabled! (dimension: ' + d.dimension + ')</div>';
                        document.getElementById('modeBadge').textContent = 'SEMANTIC';
                        document.getElementById('modeBadge').className = 'mode-badge mode-sem';
                    } else {
                        document.getElementById('modelStatus').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                    }
                });
        }
        
        function disableSemantic() {
            fetch('/disableSemantic')
                .then(r => r.json())
                .then(d => {
                    semMode = false;
                    document.getElementById('modelStatus').innerHTML = '<div class="success">BM25 mode enabled</div>';
                    document.getElementById('modeBadge').textContent = 'BM25';
                    document.getElementById('modeBadge').className = 'mode-badge mode-bm25';
                });
        }
        
        function indexFolder() {
            var path = document.getElementById('folderPath').value;
            if(!path) { alert('Please enter folder path'); return; }
            document.getElementById('indexStatus').innerHTML = '<div class="info">Indexing... (this may take a while)</div>';
            fetch('/index?path=' + encodeURIComponent(path))
                .then(r => r.json())
                .then(d => {
                    if(d.success) document.getElementById('indexStatus').innerHTML = '<div class="success">Indexed ' + d.count + ' files!</div>';
                    else document.getElementById('indexStatus').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                });
        }
        
        function clearIndex() {
            fetch('/clearIndex')
                .then(r => r.json())
                .then(d => {
                    document.getElementById('indexStatus').innerHTML = '<div class="success">Index cleared</div>';
                    document.getElementById('results').innerHTML = '';
                });
        }
        
        function doSearch() {
            var query = document.getElementById('query').value;
            if(!query) { alert('Please enter query'); return; }
            document.getElementById('results').innerHTML = '<div class="info">Searching...</div>';
            fetch('/search?q=' + encodeURIComponent(query) + '&sem=' + semMode)
                .then(r => r.json())
                .then(d => {
                    console.log('Search result:', d);
                    if(d.error) {
                        document.getElementById('results').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                        return;
                    }
                    if(d.results.length == 0) {
                        document.getElementById('results').innerHTML = '<div>No results found</div>';
                        return;
                    }
                    var html = '';
                    d.results.forEach(function(r) {
                        var snippetHtml = r.snippet ? '<div class="snippet">' + r.snippet + '</div>' : '';
                        html += '<div class="result"><strong>' + r.title + '</strong><br>' +
                                '<span class="score">Score: ' + r.score.toFixed(4) + '</span><br>' +
                                snippetHtml +
                                '<small>' + r.path + '</small></div>';
                    });
                    document.getElementById('results').innerHTML = html;
                });
        }
    </script>
</body>
</html>`

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, html)
	})

	// Set model path endpoint - shows BM25 mode (semantic disabled)
	http.HandleFunc("/setModel", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error": "Semantic search disabled - using BM25 mode (works fully offline)",
		})
	})

	// Disable sem search - fall back to BM25
	http.HandleFunc("/disableSemantic", func(w http.ResponseWriter, r *http.Request) {
		semMu.Lock()
		semOn = false
		semMu.Unlock()
		sem.Close()
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	})

	// Clear index
	http.HandleFunc("/clearIndex", func(w http.ResponseWriter, r *http.Request) {
		// Create new empty index
		mapping := bleve.NewIndexMapping()
		docMapping := bleve.NewDocumentMapping()
		titleField := bleve.NewTextFieldMapping()
		titleField.Index = true
		titleField.Store = true
		docMapping.AddFieldMappingsAt("title", titleField)
		contentField := bleve.NewTextFieldMapping()
		contentField.Index = true
		contentField.Store = true
		docMapping.AddFieldMappingsAt("content", contentField)
		pathField := bleve.NewTextFieldMapping()
		pathField.Index = false
		pathField.Store = true
		docMapping.AddFieldMappingsAt("path", pathField)
		mapping.DefaultMapping = docMapping
		mapping.DefaultType = "text"
		mapping.DefaultAnalyzer = "standard"

		idx, _ = bleve.NewMemOnly(mapping)
		sem.Close()

		log.Println("Index cleared")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	})

	// Index folder endpoint
	http.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "no path"})
			return
		}

		log.Printf("Indexing folder: %s", path)

		// Check if sem mode is on
		semMu.RLock()
		useSemantic := semOn
		semMu.RUnlock()

		count, err := indexer.IndexFolder(idx, path)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}

		// If sem mode is on, also build embedding index
		if useSemantic {
			log.Printf("Building sem index...")
			err = indexer.IndexForSemantic(idx, path, sem.AddChunk)
			if err != nil {
				log.Printf("Semantic indexing error: %v", err)
				// Continue anyway - BM25 still works
			}
			log.Printf("Semantic index built with %d chunks", sem.ChunkCount())
		}

		log.Printf("Indexed %d files", count)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "count": count})
	})

	// Search endpoint
	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		useSemantic := r.URL.Query().Get("sem") == "true"

		if query == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "no query"})
			return
		}

		log.Printf("Searching for: %s (sem: %v)", query, useSemantic)

		// Check sem mode
		semMu.RLock()
		semEnabled := semOn
		semMu.RUnlock()

		// Use sem search if enabled
		if semEnabled && useSemantic {
			log.Println("Using sem search")
			results, err := sem.Search(query, 10)
			if err != nil {
				log.Printf("Semantic search error: %v", err)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			log.Printf("Semantic search returned %d results", len(results))
			json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
			return
		}

		// Fall back to BM25
		log.Println("Using BM25 search")
		docCount, _ := idx.DocCount()
		log.Printf("Index has %d documents", docCount)

		q := bleve.NewMatchQuery(query)
		search := bleve.NewSearchRequestOptions(q, 10, 0, true)
		search.Fields = []string{"title", "content", "path"}
		search.Highlight = bleve.NewHighlightWithStyle("html")
		search.Highlight.Fields = []string{"content"}

		result, err := idx.Search(search)
		if err != nil {
			log.Printf("Search error: %v", err)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		log.Printf("BM25 search returned %d hits", len(result.Hits))

		var results []SearchResult
		for _, hit := range result.Hits {
			pathVal, _ := hit.Fields["path"].(string)
			titleVal, _ := hit.Fields["title"].(string)
			if titleVal == "" {
				titleVal = pathVal
			}

			snippet := ""
			if hit.Fragments["content"] != nil && len(hit.Fragments["content"]) > 0 {
				snippet = hit.Fragments["content"][0]
			}

			results = append(results, SearchResult{
				Path:    pathVal,
				Title:   titleVal,
				Score:   hit.Score,
				Snippet: snippet,
			})
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
	})

	addr := ":8080"
	log.Println("Press Ctrl+C to stop")

	url := "http://localhost" + addr
	openBrowser(url)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Could not open browser: %v", err)
	}
}
