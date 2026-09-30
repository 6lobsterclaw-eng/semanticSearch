package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

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
        .drop-zone { 
            border: 2px dashed #aaa; 
            padding: 30px; 
            text-align: center; 
            background: #fafafa;
            border-radius: 5px;
            margin: 10px 0;
            transition: background 0.2s, border-color 0.2s;
        }
        .drop-zone.dragover { 
            border-color: #007bff; 
            background: #e7f1ff; 
        }
        .file-list { 
            max-height: 200px; 
            overflow-y: auto; 
            border: 1px solid #ddd; 
            border-radius: 3px;
            margin: 10px 0;
        }
        .file-item { 
            padding: 8px 12px; 
            border-bottom: 1px solid #eee; 
            display: flex; 
            align-items: center;
        }
        .file-item:last-child { border-bottom: none; }
        .file-item input[type="checkbox"] { margin-right: 10px; }
        .file-item .remove { 
            margin-left: auto; 
            color: #dc3545; 
            cursor: pointer; 
            font-weight: bold;
        }
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
        <h3>Step 2: Index Files</h3>
        <div style="display: flex; gap: 10px; margin-bottom: 10px;">
            <input type="text" id="folderPath" placeholder="C:\path\to\folder" style="width: 400px;">
            <button onclick="addFolder()">Add Folder</button>
        </div>
        <div id="dropZone" class="drop-zone" ondrop="handleDrop(event)" ondragover="handleDragOver(event)" ondragenter="handleDragEnter(event)" ondragleave="handleDragLeave(event)">
            Or drag & drop files here<br>
            <small>PDF, MD, TXT, DOC, INDEX</small>
        </div>
        <div id="fileList" class="file-list"></div>
        <div>
            <button onclick="indexFiles()">Index Selected</button>
            <button onclick="exportIndex()">Export Index</button>
            <button onclick="importIndex()">Import Index</button>
        </div>
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
                .then(r => {
                    console.log('detect response:', r);
                    return r.json();
                })
                .then(d => {
                    console.log('detect data:', d);
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
                        // Poll for status
                        pollServerStatus();
                    } else {
                        document.getElementById('serverInfo').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                    }
                });
        }
        
        function pollServerStatus() {
            fetch('/serverStatus')
                .then(r => r.json())
                .then(d => {
                    document.getElementById('serverInfo').innerHTML = '<div class="info">' + d.message + '</div>';
                    if (d.status === 'ready') {
                        serverRunning = true;
                        document.getElementById('serverInfo').innerHTML = '<div class="success">Server ready!</div>';
                    } else if (d.status === 'error') {
                        document.getElementById('serverInfo').innerHTML = '<div class="error">Error: ' + d.message + '</div>';
                    } else if (d.status === 'starting') {
                        setTimeout(pollServerStatus, 1000);
                    }
                });
        }
        
        // File handling for Step 2
        var pendingFiles = [];
        
        function addFolder() {
            var path = document.getElementById('folderPath').value;
            if (!path) { alert('Please enter folder path'); return; }
            addFile(path);
        }
        
        function handleDragOver(e) {
            e.preventDefault();
            e.stopPropagation();
        }
        
        function handleDragEnter(e) {
            e.preventDefault();
            e.stopPropagation();
            document.getElementById('dropZone').classList.add('dragover');
        }
        
        function handleDragLeave(e) {
            e.preventDefault();
            e.stopPropagation();
            document.getElementById('dropZone').classList.remove('dragover');
        }
        
        function handleDrop(e) {
            e.preventDefault();
            e.stopPropagation();
            document.getElementById('dropZone').classList.remove('dragover');
            
            var items = e.dataTransfer.files;
            if (items.length > 0) {
                for (var i = 0; i < items.length; i++) {
                    addFile(items[i].path || items[i].name);
                }
            }
        }
        
        function addFile(path) {
            // Check for duplicates
            for (var i = 0; i < pendingFiles.length; i++) {
                if (pendingFiles[i].path === path) return;
            }
            
            var ext = path.split('.').pop().toLowerCase();
            var isIndex = ext === 'index' || path.endsWith('.index');
            var isFolder = !path.includes('.') || path.endsWith('\\') || path.endsWith('/');
            
            pendingFiles.push({ path: path, isIndex: isIndex, checked: true, isFolder: isFolder });
            renderFileList();
        }
        
        function removeFile(index) {
            pendingFiles.splice(index, 1);
            renderFileList();
        }
        
        function handleDragOver(e) {
            e.preventDefault();
            e.stopPropagation();
        }
        
        function toggleFile(index) {
            pendingFiles[index].checked = !pendingFiles[index].checked;
        }
        
        function renderFileList() {
            var container = document.getElementById('fileList');
            if (pendingFiles.length === 0) {
                container.innerHTML = '';
                return;
            }
            
            var html = '';
            for (var i = 0; i < pendingFiles.length; i++) {
                var f = pendingFiles[i];
                var ext = f.path.split('.').pop().toLowerCase();
                var icon;
                if (f.isFolder) {
                    icon = '📁';
                } else if (f.isIndex) {
                    icon = '📦';
                } else if (ext === 'pdf') {
                    icon = '📄';
                } else if (ext === 'md') {
                    icon = '📝';
                } else if (ext === 'txt') {
                    icon = '📃';
                } else {
                    icon = '📄';
                }
                html += '<div class="file-item">';
                html += '<input type="checkbox" ' + (f.checked ? 'checked' : '') + ' onchange="toggleFile(' + i + ')">';
                html += '<span>' + icon + ' ' + f.path.split(/[/\\]/).pop() + '</span>';
                html += '<span class="remove" onclick="removeFile(' + i + ')">✕</span>';
                html += '</div>';
            }
            container.innerHTML = html;
        }
        
        function indexFiles() {
            if (!serverRunning) { alert('Please start server first'); return; }
            
            // Check if any .index files were dropped - import instead
            var indexFiles = pendingFiles.filter(function(f) { return f.isIndex && f.checked; });
            if (indexFiles.length > 0) {
                // Import the first .index file
                importIndexFile(indexFiles[0].path);
                return;
            }
            
            // Check for folders - index all at once
            var folders = pendingFiles.filter(function(f) { return f.isFolder && f.checked; });
            if (folders.length > 0) {
                indexFolderPath(folders[0].path);
                return;
            }
            
            // Regular indexing
            var filesToIndex = pendingFiles.filter(function(f) { return f.checked && !f.isIndex && !f.isFolder; });
            if (filesToIndex.length === 0) { alert('Please select files to index'); return; }
            
            document.getElementById('indexStatus').innerHTML = '<div class="info">Indexing ' + filesToIndex.length + ' files...</div>';
            
            // Index each file sequentially
            var indexed = 0;
            var errors = [];
            
            console.log("Starting indexing, files:", JSON.stringify(filesToIndex));
            
            function indexNext() {
                console.log("indexNext called, indexed=", indexed, "length=", filesToIndex.length);
                if (indexed >= filesToIndex.length) {
                    if (errors.length > 0) {
                        document.getElementById('indexStatus').innerHTML = '<div class="error">Indexed with errors: ' + errors.join(', ') + '</div>';
                    } else {
                        // Fetch actual counts from server
                        fetch('/documentCount')
                            .then(function(r) { return r.json(); })
                            .then(function(c) {
                                document.getElementById('indexStatus').innerHTML = '<div class="success">Indexed ' + c.files + ' document' + (c.files !== 1 ? 's' : '') + ' (' + c.count + ')</div>';
                            });
                    }
                    return;
                }
                
                var file = filesToIndex[indexed];
                console.log("Indexing file:", file.path);
                fetch('/index?path=' + encodeURIComponent(file.path))
                    .then(function(r) { 
                        console.log("Response status:", r.status);
                        return r.json(); 
                    })
                    .then(function(d) {
                        console.log("Response data:", JSON.stringify(d));
                        if (d.success) {
                            indexed++;
                        } else {
                            errors.push(file.path.split(/[/\\]/).pop());
                        }
                        indexNext();
                    })
                    .catch(function(e) {
                        console.error("Fetch error:", e);
                        errors.push(file.path.split(/[/\\]/).pop());
                        indexNext();
                    });
            }
            
            indexNext();
        }
        
        function importIndexFile(path) {
            document.getElementById('indexStatus').innerHTML = '<div class="info">Importing index...</div>';
            fetch('/import?path=' + encodeURIComponent(path))
                .then(function(r) { return r.json(); })
                .then(function(d) {
                    if (d.success) {
                        indexed = true;
                        document.getElementById('indexStatus').innerHTML = '<div class="success">Imported ' + d.count + ' chunks!</div>';
                    } else {
                        document.getElementById('indexStatus').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                    }
                });
        }
        
        var indexingPollInterval = null;

        function indexFolderPath(path) {
            document.getElementById('indexStatus').innerHTML = '<div class="info">Indexing folder...</div>';
            
            // Start indexing
            fetch('/index?path=' + encodeURIComponent(path))
                .then(function(r) { return r.json(); })
                .then(function(d) {
                    if (d.success) {
                        indexed = true;
                        // Poll for progress
                        indexingPollInterval = setInterval(function() {
                            fetch('/serverStatus')
                                .then(function(r) { return r.json(); })
                                .then(function(s) {
                                    if (s.status === 'indexing') {
                                        document.getElementById('indexStatus').innerHTML = '<div class="info">Indexing ' + s.done + '/' + s.total + ' (' + s.chunks + ' chunks)...</div>';
                                    } else if (s.status === 'ready' || s.status === 'idle') {
                                        clearInterval(indexingPollInterval);
                                        // Fetch final counts
                                        fetch('/documentCount')
                                            .then(function(r) { return r.json(); })
                                            .then(function(c) {
                                                document.getElementById('indexStatus').innerHTML = '<div class="success">Indexed ' + c.files + ' document' + (c.files !== 1 ? 's' : '') + ' (' + c.count + ')</div>';
                                            });
                                    }
                                });
                        }, 500);
                    } else {
                        document.getElementById('indexStatus').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                    }
                });
        }
        
        function exportIndex() {
            if (!indexed) { alert('Nothing to export'); return; }
            
            var path = prompt('Enter path to save index file:', 'index.idx');
            if (!path) return;
            
            document.getElementById('indexStatus').innerHTML = '<div class="info">Exporting index...</div>';
            fetch('/export?path=' + encodeURIComponent(path))
                .then(function(r) { return r.json(); })
                .then(function(d) {
                    if (d.success) {
                        document.getElementById('indexStatus').innerHTML = '<div class="success">Exported ' + d.count + ' chunks!</div>';
                    } else {
                        document.getElementById('indexStatus').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                    }
                });
        }
        
        function importIndex() {
            if (!serverRunning) { alert('Please start server first'); return; }
            
            var path = prompt('Enter path to index file:');
            if (!path) return;
            
            importIndexFile(path);
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
		w.Header().Set("Content-Type", "application/json")
		
		exeDir, err := detect.FindExeFolder()
		log.Printf("detect: exeDir=%s, err=%v", exeDir, err)
		if err != nil {
			fmt.Fprintf(w, `{"server": false, "gguf": [], "error": %q}`, err)
			return
		}
		
		server, _ := detect.FindLlamaServer(exeDir)
		gguf, _ := detect.FindGGUFFiles(exeDir)
		log.Printf("detect: server=%s, gguf=%v", server, gguf)
		
		// Build JSON manually to avoid formatting issues
		fmt.Fprintf(w, `{"server": %v, "gguf": [`, server != "")
		for i, f := range gguf {
			if i > 0 {
				fmt.Fprintf(w, ",")
			}
			fmt.Fprintf(w, "%q", f)
		}
		fmt.Fprintf(w, `], "exeDir": %q}`, exeDir)
	})

	var serverStatus string = "idle" // idle, starting, ready, error
	var serverStatusMsg string = ""
	var indexingProgress = struct {
		done   int
		total  int
		chunks int
		active bool
	}{}

	// Start server endpoint - starts async and returns immediately
	http.HandleFunc("/startServer", func(w http.ResponseWriter, r *http.Request) {
		model := r.URL.Query().Get("model")
		if model == "" {
			fmt.Fprint(w, `{"success": false, "error": "no model selected"}`)
			return
		}

		if serverStatus == "starting" {
			fmt.Fprint(w, `{"success": false, "error": "already starting"}`)
			return
		}

		exeDir, _ := detect.FindExeFolder()
		modelPath = exeDir + "/" + model
		serverPath, _ = detect.FindLlamaServer(exeDir)

		port := 8080
		serverURL = fmt.Sprintf("http://localhost:%d", port)

		log.Printf("Starting server: %s -m %s --port %d", serverPath, modelPath, port)

		serverStatus = "starting"
		serverStatusMsg = "Starting llama-server..."

		// Start server in background
		go func() {
			serverCmd = exec.Command(serverPath, "-m", modelPath, "--port", fmt.Sprintf("%d", port), "-c", "2048", "--embeddings")
			serverCmd.Stdout = log.Writer()
			serverCmd.Stderr = log.Writer()

			if err := serverCmd.Start(); err != nil {
				serverStatus = "error"
				serverStatusMsg = fmt.Sprintf("Failed to start: %v", err)
				log.Printf("Server start error: %v", err)
				return
			}

			// Wait for server to be ready
			for i := 0; i < 60; i++ {
				serverStatusMsg = fmt.Sprintf("Loading model... (%ds)", i)
				if resp, err := http.Get(serverURL + "/v1/models"); err == nil {
					resp.Body.Close()
					if resp.StatusCode == 200 {
						// Create embedder
						emb, err := indexer.NewHTTPEmbedder(serverURL, modelPath)
						if err != nil {
							serverStatus = "error"
							serverStatusMsg = fmt.Sprintf("Embedder error: %v", err)
							return
						}
						embedder = emb
						idx = indexer.NewIndexer(emb)
						serverStatus = "ready"
						serverStatusMsg = "Server ready!"
						log.Println("Server ready")
						return
					}
				}
				time.Sleep(1 * time.Second)
			}

			serverStatus = "error"
			serverStatusMsg = "Timeout waiting for server"
		}()

		fmt.Fprint(w, `{"success": true, "status": "starting"}`)
	})

	// Server status endpoint - poll this for progress
	http.HandleFunc("/serverStatus", func(w http.ResponseWriter, r *http.Request) {
		if indexingProgress.active {
			fmt.Fprintf(w, `{"status": "indexing", "message": "Indexing %d/%d (%d chunks)", "done": %d, "total": %d, "chunks": %d}`,
				indexingProgress.done, indexingProgress.total, indexingProgress.chunks, indexingProgress.done, indexingProgress.total, indexingProgress.chunks)
		} else {
			fmt.Fprintf(w, `{"status": %q, "message": %q}`, serverStatus, serverStatusMsg)
		}
	})

	// Document count endpoint
	http.HandleFunc("/documentCount", func(w http.ResponseWriter, r *http.Request) {
		if idx == nil {
			fmt.Fprintf(w, `{"count": 0, "files": 0}`)
			return
		}
		fmt.Fprintf(w, `{"count": %d, "files": %d}`, idx.DocumentCount(), idx.FileCount())
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

		// Start indexing in background and return immediately
		indexingProgress.active = true
		indexingProgress.done = 0
		indexingProgress.total = 0
		indexingProgress.chunks = 0

		go func() {
			err := idx.IndexFolder(path, func(done, total, chunks int) {
				indexingProgress.done = done
				indexingProgress.total = total
				indexingProgress.chunks = chunks
				log.Printf("Indexing progress: %d/%d (%d chunks)", done, total, chunks)
			})
			if err != nil {
				log.Printf("Indexing error: %v", err)
			}
			indexingProgress.active = false
		}()

		fmt.Fprint(w, `{"success": true, "message": "Indexing started"}`)
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

		data, _ := json.Marshal(out)
		fmt.Fprint(w, string(data))
	})

	// Export index endpoint
	http.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		if idx == nil {
			fmt.Fprint(w, `{"success": false, "error": "no index"}`)
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			fmt.Fprint(w, `{"success": false, "error": "no path"}`)
			return
		}

		err := idx.Export(path)
		if err != nil {
			fmt.Fprintf(w, `{"success": false, "error": "%v"}`, err)
			return
		}

		count := idx.ChunkCount()
		fmt.Fprintf(w, `{"success": true, "count": %d}`, count)
	})

	// Import index endpoint
	http.HandleFunc("/import", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			fmt.Fprint(w, `{"success": false, "error": "no path"}`)
			return
		}

		err := idx.Import(path)
		if err != nil {
			fmt.Fprintf(w, `{"success": false, "error": "%v"}`, err)
			return
		}

		count := idx.ChunkCount()
		fmt.Fprintf(w, `{"success": true, "count": %d}`, count)
	})

	addr := ":8081"
	openBrowser("http://localhost:8081")
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
