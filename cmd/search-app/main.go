package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"semantic-search/internal/indexer"
	"semantic-search/internal/search"
	"semantic-search/internal/storage"
)

var (
	db        *storage.DB
	indexerObj *indexer.Indexer
	searcher  *search.Searcher
	modelPath string
	folderPath string
)

type SearchResult struct {
	Path  string  `json:"path"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

func main() {
	log.Println("Starting Semantic Search...")

	// Initialize database
	var err error
	db, err = storage.NewDB("semantic.db")
	if err != nil {
		log.Printf("DB init error: %v", err)
	}
	log.Println("Database initialized")

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
        button:hover { background: #0056b3; }
        input { padding: 8px; width: 300px; }
        #results { margin-top: 20px; }
        .result { padding: 15px; margin: 10px 0; background: white; border: 1px solid #ddd; border-radius: 5px; }
        .score { color: #666; font-size: 12px; }
        #status { padding: 10px; margin: 10px 0; border-radius: 3px; }
        .success { background: #d4edda; color: #155724; }
        .error { background: #f8d7da; color: #721c24; }
        .info { background: #d1ecf1; color: #0c5460; }
    </style>
</head>
<body>
    <h1>Semantic Search</h1>
    
    <div class="step">
        <h3>Step 1: Select GGUF Model</h3>
        <input type="text" id="modelPath" placeholder="C:\path\to\model.gguf" style="width: 500px;">
        <button onclick="setModel()">Set Model</button>
        <div id="modelStatus"></div>
    </div>
    
    <div class="step">
        <h3>Step 2: Select Folder to Index</h3>
        <input type="text" id="folderPath" placeholder="C:\path\to\documents" style="width: 500px;">
        <button onclick="indexFolder()">Index Folder</button>
        <div id="indexStatus"></div>
    </div>
    
    <div class="step">
        <h3>Step 3: Search</h3>
        <input type="text" id="query" placeholder="Enter search query" style="width: 500px;">
        <button onclick="doSearch()">Search</button>
    </div>
    
    <div id="results"></div>

    <script>
        function setModel() {
            var path = document.getElementById('modelPath').value;
            if(!path) { alert('Please enter model path'); return; }
            document.getElementById('modelStatus').innerHTML = '<div class="info">Loading model...</div>';
            fetch('/setModel?path=' + encodeURIComponent(path))
                .then(r => r.json())
                .then(d => {
                    if(d.success) document.getElementById('modelStatus').innerHTML = '<div class="success">Model loaded!</div>';
                    else document.getElementById('modelStatus').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                });
        }
        
        function indexFolder() {
            var path = document.getElementById('folderPath').value;
            if(!path) { alert('Please enter folder path'); return; }
            document.getElementById('indexStatus').innerHTML = '<div class="info">Indexing...</div>';
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
                        html += '<div class="result"><strong>' + r.title + '</strong><br>' +
                                '<span class="score">Score: ' + r.score.toFixed(4) + '</span><br>' +
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

	http.HandleFunc("/setModel", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "no path"})
			return
		}
		
		if _, err := os.Stat(path); os.IsNotExist(err) {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "file not found"})
			return
		}
		
		modelPath = path
		log.Printf("Model set to: %s", modelPath)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	})

	http.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "no path"})
			return
		}
		
		if modelPath == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "set model first"})
			return
		}
		
		folderPath = path
		log.Printf("Indexing folder: %s", folderPath)
		
		// Create indexer
		idx, err := indexer.NewIndexer(db, modelPath)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		
		// Index folder
		ctx := context.Background()
		err = idx.IndexDir(ctx, folderPath)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		
		indexerObj = idx
		log.Printf("Indexed folder: %s", folderPath)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "count": 0})
	})

	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "no query"})
			return
		}
		
		if indexerObj == nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "index folder first"})
			return
		}
		
		// Create searcher
		s, err := search.NewSearcher(db, modelPath)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		
		ctx := context.Background()
		results, err := s.Search(ctx, query, 10)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		
		var out []SearchResult
		for _, r := range results {
			out = append(out, SearchResult{
				Path:  r.Path,
				Title: r.Title,
				Score: r.Score,
			})
		}
		
		json.NewEncoder(w).Encode(map[string]interface{}{"results": out})
	})

	addr := ":8080"
	log.Printf("Open browser: http://localhost%s", addr)
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
