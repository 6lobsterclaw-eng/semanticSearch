package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/blevesearch/bleve/v2"
	"semantic-search/internal/indexer"
)

var (
	idx       bleve.Index
	modelPath string
)

type SearchResult struct {
	Path  string  `json:"path"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

func main() {
	log.Println("Starting Semantic Search (BM25)...")

	// Create default in-memory index with default mapping
	var err error
	idx, err = bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		log.Fatalf("Failed to create index: %v", err)
	}
	log.Println("Search index ready (BM25 - no model needed)")

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
        button { padding: 10px 20px; cursor: pointer; background: #007bff; color: white; border: none; border-radius: 3px; }
        input { padding: 8px; width: 400px; }
        #results { margin-top: 20px; }
        .result { padding: 15px; margin: 10px 0; background: white; border: 1px solid #ddd; border-radius: 5px; }
        .score { color: #666; font-size: 12px; }
        .success { background: #d4edda; color: #155724; }
        .error { background: #f8d7da; color: #721c24; }
        .info { background: #d1ecf1; color: #0c5460; }
        .note { color: #666; font-size: 12px; }
    </style>
</head>
<body>
    <h1>Semantic Search (BM25)</h1>
    
    <div class="step">
        <h3>Step 1: (Optional) GGUF Model for Semantic Search</h3>
        <input type="text" id="modelPath" placeholder="C:\path\to\model.gguf">
        <button onclick="setModel()">Set Model</button>
        <div id="modelStatus"></div>
        <p class="note">Note: BM25 doesn't need a model. This is for future semantic search (AI embeddings).</p>
    </div>
    
    <div class="step">
        <h3>Step 2: Select Folder to Index</h3>
        <input type="text" id="folderPath" placeholder="C:\path\to\documents">
        <button onclick="indexFolder()">Index Folder</button>
        <div id="indexStatus"></div>
    </div>
    
    <div class="step">
        <h3>Step 3: Search</h3>
        <input type="text" id="query" placeholder="Enter search query">
        <button onclick="doSearch()">Search</button>
        <button onclick="dumpIndex()">Dump Index</button>
    </div>
    
    <div id="results"></div>

    <script>
        function setModel() {
            var path = document.getElementById('modelPath').value;
            if(!path) { alert('Please enter model path'); return; }
            document.getElementById('modelStatus').innerHTML = '<div class="info">Setting model path...</div>';
            fetch('/setModel?path=' + encodeURIComponent(path))
                .then(r => r.json())
                .then(d => {
                    if(d.success) document.getElementById('modelStatus').innerHTML = '<div class="success">Model path saved!</div>';
                    else document.getElementById('modelStatus').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
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
        
        function doSearch() {
            var query = document.getElementById('query').value;
            if(!query) { alert('Please enter query'); return; }
            document.getElementById('results').innerHTML = '<div class="info">Searching...</div>';
            fetch('/search?q=' + encodeURIComponent(query))
                .then(r => r.json())
                .then(d => {
                    console.log('Search result:', d);
                    if(d.error) {
                        document.getElementById('results').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                        return;
                    }
                    if(d.results.length == 0) {
                        document.getElementById('results').innerHTML = '<div>No results found - try indexing a folder first</div>';
                        return;
                    }
                    var html = '';
                    d.results.forEach(function(r) {
                        html += '<div class="result"><strong>' + r.title + '</strong><br>' +
                                '<span class="score">Score: ' + r.score.toFixed(4) + '</span><br>' +
                                '<small>' + r.path + '</small></div>';
                    });
                    document.getElementById('results').innerHTML = html;
                });
        }
        
        function dumpIndex() {
            document.getElementById('results').innerHTML = '<div class="info">Dumping index...</div>';
            fetch('/debug/dump')
                .then(r => r.json())
                .then(d => {
                    console.log('Dump result:', d);
                    if(d.error) {
                        document.getElementById('results').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                        return;
                    }
                    var html = '<h3>Indexed Documents: ' + d.count + '</h3>';
                    d.documents.forEach(function(doc) {
                        html += '<div class="result"><strong>' + doc.id + '</strong><br>' +
                                '<small>Title: ' + (doc.title || 'N/A') + '</small><br>' +
                                '<small>Content length: ' + (doc.content ? doc.content.length : 0) + ' chars</small><br>' +
                                '<pre style="max-height:100px;overflow:scroll;background:#f5f5f5;padding:5px;">' + 
                                (doc.content ? doc.content.substring(0, 500) + '...' : 'No content') + '</pre></div>';
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

	// Set model path endpoint
	http.HandleFunc("/setModel", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "no path"})
			return
		}
		modelPath = path
		log.Printf("Model path set to: %s", path)
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

		count, err := indexer.IndexFolder(idx, path)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}

		log.Printf("Indexed %d files", count)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "count": count})
	})

	// Search endpoint
	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "no query"})
			return
		}

		log.Printf("Searching for: %s", query)

		// Get index stats
		docCount, _ := idx.DocCount()
		log.Printf("Index has %d documents", docCount)

		// Simple BM25 search
		log.Printf("Creating QueryStringQuery for: %s", query)
		q := bleve.NewQueryStringQuery(query)
		log.Printf("Query created: %+v", q)
		
		search := bleve.NewSearchRequestOptions(q, 10, 0, true)
		search.Highlight = nil // disable highlight to simplify
		
		log.Printf("Executing search...")
		result, err := idx.Search(search)
		if err != nil {
			log.Printf("Search error: %v", err)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		log.Printf("Search result: Took=%v, Total=%d, MaxScore=%f", 
			result.Took, result.Total, result.MaxScore)

		log.Printf("Search returned %d hits", len(result.Hits))

		var results []SearchResult
		for _, hit := range result.Hits {
			pathVal, _ := hit.Fields["path"].(string)
			titleVal, _ := hit.Fields["title"].(string)
			if titleVal == "" {
				titleVal = pathVal
			}
			
			results = append(results, SearchResult{
				Path:  pathVal,
				Title: titleVal,
				Score: hit.Score,
			})
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"results": results, "total": len(results)})
	})

	// Debug: dump all indexed content
	http.HandleFunc("/debug/dump", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Dumping index contents...")
		
		allQuery := bleve.NewMatchAllQuery()
		allSearch := bleve.NewSearchRequestOptions(allQuery, 100, 0, false)
		allSearch.Fields = []string{"*"} // Get all fields
		
		allResults, err := idx.Search(allSearch)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		
		var docs []map[string]interface{}
		for _, hit := range allResults.Hits {
			doc := map[string]interface{}{
				"id":    hit.ID,
				"score": hit.Score,
			}
			log.Printf("Doc %s fields: %+v", hit.ID, hit.Fields)
			for k, v := range hit.Fields {
				doc[k] = v
			}
			docs = append(docs, doc)
		}
		
		log.Printf("Dumped %d documents", len(docs))
		json.NewEncoder(w).Encode(map[string]interface{}{"documents": docs, "count": len(docs)})
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
