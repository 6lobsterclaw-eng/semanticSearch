package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"sync"

	"semantic-search/internal/detect"
	"semantic-search/internal/indexer"

	"github.com/kelindar/search"
)

var (
	serverCmd    *exec.Cmd
	serverURL    string
	serverPath   string
	modelPath    string
	embedder     interface {
		Embed(string) (search.Vector, error)
		AddDocument(string, search.Vector, string)
		Search(search.Vector, int) []search.Result[string]
		SaveIndex(string) error
		LoadIndex(string) error
		Close() error
	}
	idx          *indexer.Indexer
	mu           sync.RWMutex
)

type SearchResult struct {
	Path    string  `json:"path"`
	Title   string  `json:"title"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

func main() {
	log.Println("Starting Semantic Search App...")

	// Auto-detect exe folder
	exeDir, err := detect.FindExeFolder()
	if err != nil {
		log.Printf("Warning: Could not find exe folder: %v", err)
		exeDir = "."
	}

	// Auto-detect llama-server.exe
	serverPath, err = detect.FindLlamaServer(exeDir)
	if err != nil {
		log.Printf("llama-server.exe not found in %s", exeDir)
	}

	// Auto-detect GGUF files
	ggufFiles, _ := detect.FindGGUFFiles(exeDir)
	log.Printf("Found GGUF files: %v", ggufFiles)

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
        button:disabled { background: #ccc; }
        button.secondary { background: #6c757d; }
        .score { color: #666; font-size: 12px; }
        .snippet { margin: 10px 0; padding: 10px; background: #f0f0f0; border-left: 3px solid #007bff; font-style: italic; }
        #results { margin-top: 20px; }
        .result { padding: 15px; margin: 10px 0; background: white; border: 1px solid #ddd; border-radius: 5px; }
        .error { background: #f8d7da; color: #721c24; }
        .info { background: #d1ecf1; color: #0c5460; }
        .success { background: #d4edda; color: #155724; }
        input[type="text"] { padding: 8px; border: 1px solid #ddd; border-radius: 3px; }
        select { padding: 8px; border: 1px solid #ddd; border-radius: 3px; }
        .auto-detect { background: #fff3cd; padding: 10px; border-radius: 3px; margin-bottom: 15px; }
    </style>
</head>
<body>
    <h1>Semantic Search</h1>
    
    <div class="auto-detect">
        <strong>Auto-detect:</strong> <span id="serverStatus">Scanning...</span>
    </div>
    
    <div class="step">
        <h3>Step 1: Configure Model</h3>
        <select id="modelSelect" style="width: 300px;">
            <option value="">-- Select GGUF Model --</option>
        </select>
        <button id="startServerBtn" onclick="startServer()" disabled>Start Server</button>
        <div id="serverInfo"></div>
    </div>
    
    <div class="step">
        <h3>Step 2: Select Folder</h3>
        <input type="text" id="folderPath" placeholder="C:\path\to\documents" style="width: 400px;">
        <button onclick="indexFolder()">Index Folder</button>
        <div id="indexStatus"></div>
    </div>
    
    <div class="step">
        <h3>Step 3: Search</h3>
        <input type="text" id="query" placeholder="Enter search query" style="width: 400px;">
        <button onclick="doSearch()">Search</button>
    </div>
    
    <div id="results"></div>

    <script>
        var serverRunning = false;
        var indexed = false;
        
        // Auto-detect on load
        window.onload = function() {
            fetch('/detect')
                .then(r => r.json())
                .then(d => {
                    var status = '';
                    if (d.server) {
                        status += 'llama-server.exe: FOUND ';
                    } else {
                        status += 'llama-server.exe: NOT FOUND ';
                    }
                    status += '| GGUF files: ' + d.gguf.length;
                    document.getElementById('serverStatus').textContent = status;
                    
                    var select = document.getElementById('modelSelect');
                    d.gguf.forEach(function(f) {
                        var opt = document.createElement('option');
                        opt.value = f;
                        opt.textContent = f;
                        select.appendChild(opt);
                    });
                    
                    if (d.server && d.gguf.length > 0) {
                        document.getElementById('startServerBtn').disabled = false;
                    }
                });
        };
        
        function startServer() {
            var model = document.getElementById('modelSelect').value;
            if (!model) { alert('Please select a model'); return; }
            
            document.getElementById('serverInfo').innerHTML = '<div class="info">Starting server...</div>';
            
            fetch('/startServer?model=' + encodeURIComponent(model))
                .then(r => r.json())
                .then(d => {
                    if (d.success) {
                        serverRunning = true;
                        document.getElementById('serverInfo').innerHTML = '<div class="success">Server running on port ' + d.port + '</div>';
                    } else {
                        document.getElementById('serverInfo').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                    }
                });
        }
        
        function indexFolder() {
            var path = document.getElementById('folderPath').value;
            if (!path) { alert('Please enter folder path'); return; }
            if (!serverRunning) { alert('Please start server first'); return; }
            
            document.getElementById('indexStatus').innerHTML = '<div class="info">Indexing...</div>';
            fetch('/index?path=' + encodeURIComponent(path))
                .then(r => r.json())
                .then(d => {
                    if (d.success) {
                        indexed = true;
                        document.getElementById('indexStatus').innerHTML = '<div class="success">Indexed ' + d.count + ' documents!</div>';
                    } else {
                        document.getElementById('indexStatus').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                    }
                });
        }
        
        function doSearch() {
            var query = document.getElementById('query').value;
            if (!query) { alert('Please enter query'); return; }
            if (!indexed) { alert('Please index a folder first'); return; }
            
            document.getElementById('results').innerHTML = '<div class="info">Searching...</div>';
            fetch('/search?q=' + encodeURIComponent(query))
                .then(r => r.json())
                .then(d => {
                    if (d.error) {
                        document.getElementById('results').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                        return;
                    }
                    if (d.results.length == 0) {
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

	// Auto-detect endpoint
	http.HandleFunc("/detect", func(w http.ResponseWriter, r *http.Request) {
		exeDir, err := detect.FindExeFolder()
		log.Printf("detect: exeDir=%s, err=%v", exeDir, err)
		if err != nil {
			fmt.Fprintf(w, `{"server": false, "gguf": [], "error": "%v"}`, err)
			return
		}
		
		server, _ := detect.FindLlamaServer(exeDir)
		gguf, _ := detect.FindGGUFFiles(exeDir)
		log.Printf("detect: server=%s, gguf=%v", server, gguf)
		
		fmt.Fprintf(w, `{"server": %v, "gguf": %v, "exeDir": %v}`, server != "", gguf, exeDir)
	})

	// Start server endpoint
	http.HandleFunc("/startServer", func(w http.ResponseWriter, r *http.Request) {
		model := r.URL.Query().Get("model")
		if model == "" {
			fmt.Fprint(w, `{"success": false, "error": "no model selected"}`)
			return
		}

		exeDir, _ := detect.FindExeFolder()
		modelPath = exeDir + "/" + model
		serverPath, _ = detect.FindLlamaServer(exeDir)

		port := 8080
		serverURL = fmt.Sprintf("http://localhost:%d", port)

		log.Printf("Starting server: %s -m %s --port %d", serverPath, modelPath, port)

		serverCmd = exec.Command(serverPath, "-m", modelPath, "--port", fmt.Sprintf("%d", port))
		serverCmd.Stdout = log.Writer()
		serverCmd.Stderr = log.Writer()

		if err := serverCmd.Start(); err != nil {
			fmt.Fprintf(w, `{"success": false, "error": "%v"}`, err)
			return
		}

		// Wait for server to be ready
		for i := 0; i < 30; i++ {
			if resp, err := http.Get(serverURL + "/health"); err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					// Create embedder
					emb, err := indexer.NewHTTPEmbedder(serverURL, modelPath)
					if err != nil {
						fmt.Fprintf(w, `{"success": false, "error": "embedder: %v"}`, err)
						return
					}
					embedder = emb
					idx = indexer.NewIndexer(emb)

					fmt.Fprintf(w, `{"success": true, "port": %d}`, port)
					return
				}
			}
		}

		fmt.Fprint(w, `{"success": false, "error": "timeout"}`)
	})

	// Index folder endpoint
	http.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			fmt.Fprint(w, `{"success": false, "error": "no path"}`)
			return
		}

		if idx == nil {
			fmt.Fprint(w, `{"success": false, "error": "server not running"}`)
			return
		}

		err := idx.IndexFolder(path)
		if err != nil {
			fmt.Fprintf(w, `{"success": false, "error": "%v"}`, err)
			return
		}

		count := idx.DocumentCount()
		fmt.Fprintf(w, `{"success": true, "count": %d}`, count)
	})

	// Search endpoint
	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			fmt.Fprint(w, `{"error": "no query"}`)
			return
		}

		if idx == nil {
			fmt.Fprint(w, `{"error": "not indexed"}`)
			return
		}

		results := idx.Search(query, 10)
		
		var out []SearchResult
		for _, r := range results {
			out = append(out, SearchResult{
				Path:  r.Value,
				Title: r.Value,
				Score: float64(r.Relevance),
			})
		}

		fmt.Fprintf(w, `{"results": %v}`, out)
	})

	addr := ":8081"
	log.Println("Server starting on", addr)
	openBrowser("http://localhost" + addr)
	http.ListenAndServe(addr, nil)
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
	cmd.Start()
}
