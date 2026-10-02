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
	"semantic-search/internal/llm"
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
	// LLM client for question generation
	llmClient    *llm.Client
	llmModelPath string
)

type SearchResult struct {
	Index    int     `json:"index"`
	ChunkID  string  `json:"chunkId"`
	Path     string  `json:"path"`
	Title    string  `json:"title"`
	Extract  string  `json:"extract"`
	Location string  `json:"location"`
	Score    float64 `json:"score"`
}

// JSONEscape escapes a string for safe JSON embedding
func JSONEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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
        .results-table { width: 100%; border-collapse: collapse; margin-top: 15px; background: white; }
        .results-table th, .results-table td { border: 1px solid #ddd; padding: 10px; text-align: left; }
        .results-table th { background: #007bff; color: white; }
        .results-table tr:hover { background: #f5f5f5; }
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
    
    <div class="step">
        <h3>Step 1: Configure Models</h3>
        
        <div style="padding: 10px; background: #f8f9fa; border-radius: 5px;">
            <strong>Embedding Model</strong> (for semantic search)<br>
            <select id="modelSelect" style="width: 300px; margin-top: 5px;" onchange="updateEmbedderStatus()">
                <option value="">-- Select GGUF Model --</option>
            </select>
            <button id="startServerBtn" onclick="startServer()" disabled>Start</button>
            <span id="embedderStatus" style="margin-left: 5px;">●</span>
        </div>
        
        <div style="padding: 10px; background: #f8f9fa; border-radius: 5px;">
            <strong>Question Generation Model</strong> (for generating search questions)<br>
            <select id="llmModelSelect" style="width: 300px; margin-top: 5px;">
                <option value="">-- Select GGUF Model --</option>
            </select>
            <button id="startLLMBtn" onclick="startLLMServer()" disabled>Start</button>
            <span id="llmStatus" style="margin-left: 5px;">●</span>
        </div>
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
        <button onclick="refineQuery()">Refine</button>
        <div style="margin-top: 10px;">
            <label style="margin-right: 15px;">
                <input type="checkbox" id="semanticCheck" checked> Semantic
            </label>
            <label style="margin-right: 15px;">
                <input type="checkbox" id="keywordCheck" checked> Keyword (BM25)
            </label>
        </div>
    </div>
    
    <div id="results"></div>

    <script>
        var serverRunning = false;
        var indexed = false;
        
        // Set status dot colors
        function setEmbedderStatus(color) {
            var dot = document.getElementById('embedderStatus');
            if (dot) {
                dot.style.color = color;
                dot.title = color === 'red' ? 'Unavailable' : color === 'orange' ? 'Loading' : 'Ready';
            }
        }
        
        function setLLMStatus(color) {
            var dot = document.getElementById('llmStatus');
            if (dot) {
                dot.style.color = color;
                dot.title = color === 'red' ? 'Unavailable' : color === 'orange' ? 'Loading' : 'Ready';
            }
        }
        
        // Auto-detect on load
        window.onload = function() {
            // Initialize dots as red (unavailable)
            setEmbedderStatus('red');
            setLLMStatus('red');
            
            fetch('/detect')
                .then(r => {
                    console.log('detect response:', r);
                    return r.json();
                })
                .then(d => {
                    console.log('detect data:', d);
                    
                    var select = document.getElementById('modelSelect');
                    var llmSelect = document.getElementById('llmModelSelect');
                    d.gguf.forEach(function(f) {
                        // Embedding models go to modelSelect
                        if (f.toLowerCase().includes('embedding')) {
                            var opt = document.createElement('option');
                            opt.value = f;
                            opt.textContent = f;
                            select.appendChild(opt);
                        }
                        
                        // Chat/Instruct models go to LLM dropdown (exclude embedding models)
                        if (!f.toLowerCase().includes('embedding') && 
                            (f.toLowerCase().includes('instruct') || f.toLowerCase().includes('chat'))) {
                            var opt2 = document.createElement('option');
                            opt2.value = f;
                            opt2.textContent = f;
                            llmSelect.appendChild(opt2);
                        }
                    });
                    
                    if (d.gguf.length > 0) {
                        document.getElementById('startServerBtn').disabled = false;
                        document.getElementById('startLLMBtn').disabled = false;
                    }
                });
        };
        
        // Global LLM state
        var llmServerRunning = false;
        var llmServerURL = "";
        
        function startServer() {
            var model = document.getElementById('modelSelect').value;
            if (!model) { alert('Please select a model'); return; }

            setEmbedderStatus('orange');
            
            fetch('/startServer?model=' + encodeURIComponent(model))
                .then(r => r.json())
                .then(d => {
                    if (d.success) {
                        // Poll for status
                        pollServerStatus();
                    } else {
                        setEmbedderStatus('red');
                        alert('Error: ' + d.error);
                    }
                });
        }
        
        function startLLMServer() {
            var model = document.getElementById('llmModelSelect').value;
            if (!model) { alert('Please select a model'); return; }
            
            setLLMStatus('orange');
            
            fetch('/startLLMServer?model=' + encodeURIComponent(model))
                .then(r => r.json())
                .then(d => {
                    if (d.success) {
                        llmServerRunning = true;
                        setLLMStatus('green');
                    } else {
                        setLLMStatus('red');
                        alert('Error: ' + d.error);
                    }
                });
        }
        
        function pollServerStatus() {
            fetch('/serverStatus')
                .then(r => r.json())
                .then(d => {
                    if (d.status === 'ready') {
                        serverRunning = true;
                        setEmbedderStatus('green');
                    } else if (d.status === 'error') {
                        setEmbedderStatus('red');
                    } else if (d.status === 'starting') {
                        setEmbedderStatus('orange');
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
            
            var semantic = document.getElementById('semanticCheck').checked;
            var keyword = document.getElementById('keywordCheck').checked;
            
            if (!semantic && !keyword) {
                alert('Please select at least one search mode'); return;
            }
            
            document.getElementById('results').innerHTML = '<div class="info">Searching...</div>';
            
            var url = '/search?q=' + encodeURIComponent(query) + 
                      '&semantic=' + semantic + 
                      '&keyword=' + keyword;
            fetch(url)
                .then(function(response) { return response.json(); })
                .then(function(d) {
                    if (!d) {
                        document.getElementById('results').innerHTML = '<div class="error">Server returned empty response. Please try again.</div>';
                        return;
                    }
                    if (d.error) {
                        document.getElementById('results').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                        return;
                    }
                    if (!d || d.length == 0) {
                        document.getElementById('results').innerHTML = '<div>No results found. Try different keywords or check if files are indexed.</div>';
                        return;
                    }
                    // Build results table
                    var html = '<table class="results-table">' +
                        '<thead><tr><th>#</th><th>Extract</th><th>Location</th><th>Score</th><th>Type</th></tr></thead>' +
                        '<tbody>';
                    d.forEach(function(r) {
                        var typeLabel = r.isKeyword ? '🔍 Keyword' : '🧠 Semantic';
                        html += '<tr>' +
                            '<td>' + r.index + '</td>' +
                            '<td>' + r.extract + '</td>' +
                            '<td>' + r.location + '</td>' +
                            '<td>' + r.score.toFixed(4) + '</td>' +
                            '<td>' + typeLabel + '</td>' +
                            '</tr>';
                    });
                    html += '</tbody></table>';
                    document.getElementById('results').innerHTML = html;
                })
                .catch(function(err) {
                    document.getElementById('results').innerHTML = '<div class="error">Error: ' + err + '</div>';
                });
        }

        function refineQuery() {
            var query = document.getElementById('query').value;
            if (!query) { alert('Please enter a query to refine'); return; }
            if (!llmServerRunning) { alert('Please start LLM server first'); return; }

            document.getElementById('results').innerHTML = '<div class="info">Refining query with LLM...</div>';

            fetch('/refine?q=' + encodeURIComponent(query))
                .then(function(response) { return response.json(); })
                .then(function(d) {
                    if (d.success) {
                        document.getElementById('query').value = d.refined;
                        document.getElementById('results').innerHTML = '<div class="success">Query refined! Press Search to find results.</div>';
                    } else {
                        document.getElementById('results').innerHTML = '<div class="error">Error: ' + d.error + '</div>';
                    }
                })
                .catch(function(err) {
                    document.getElementById('results').innerHTML = '<div class="error">Error: ' + err + '</div>';
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
						// Wire LLM client if already started
						if llmClient != nil {
							idx.SetLLMClient(llmClient)
							log.Println("LLM client wired to indexer")
						}
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

	// LLM Server endpoint - starts a separate llama-server for question generation
	var llmServerCmd *exec.Cmd
	var llmServerURL string
	var llmServerStatus string = "idle"

	http.HandleFunc("/startLLMServer", func(w http.ResponseWriter, r *http.Request) {
		model := r.URL.Query().Get("model")
		if model == "" {
			fmt.Fprint(w, `{"success": false, "error": "no model selected"}`)
			return
		}

		if llmServerStatus == "starting" || llmServerStatus == "ready" {
			fmt.Fprint(w, fmt.Sprintf(`{"success": true, "url": %q, "status": "already running"}`, llmServerURL))
			return
		}

		exeDir, _ := detect.FindExeFolder()
		modelPath := exeDir + "/" + model
		serverPath, _ := detect.FindLlamaServer(exeDir)

		port := 8082
		llmServerURL = fmt.Sprintf("http://localhost:%d", port)

		log.Printf("Starting LLM server: %s -m %s --port %d", serverPath, modelPath, port)

		llmServerStatus = "starting"

		// Start server in background
		go func() {
			// Kill existing if any
			if llmServerCmd != nil && llmServerCmd.Process != nil {
				llmServerCmd.Process.Kill()
			}

			llmServerCmd = exec.Command(serverPath, "-m", modelPath, "--port", fmt.Sprintf("%d", port), "-c", "4096")
			llmServerCmd.Stdout = log.Writer()
			llmServerCmd.Stderr = log.Writer()

			if err := llmServerCmd.Start(); err != nil {
				llmServerStatus = "error"
				log.Printf("LLM Server start error: %v", err)
				return
			}

			// Wait for server to be ready
			for i := 0; i < 60; i++ {
				if resp, err := http.Get(llmServerURL + "/v1/models"); err == nil {
					resp.Body.Close()
					if resp.StatusCode == 200 {
						llmServerStatus = "ready"
						log.Println("LLM Server ready")

						// Create LLM client and wire to indexer
						llmClient = llm.NewClient(llmServerURL, model)
						llmModelPath = model
						if idx != nil {
							idx.SetLLMClient(llmClient)
							log.Println("LLM client wired to indexer")
						}

						return
					}
				}
				time.Sleep(1 * time.Second)
			}

			llmServerStatus = "error"
			log.Printf("LLM Server timeout")
		}()

		// Return immediately with the URL that will be ready soon
		fmt.Fprintf(w, `{"success": true, "url": "http://localhost:%d", "status": "starting"}`, port)
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

		// Get search mode from query params
		semantic := r.URL.Query().Get("semantic") == "true"
		keyword := r.URL.Query().Get("keyword") == "true"
		
		mode := "semantic"
		if semantic && keyword {
			mode = "hybrid"
		} else if keyword {
			mode = "keyword"
		}

		log.Printf("Search: query=%q, mode=%s", query, mode)
		// Return up to 100 results to show all related items
		results := idx.Search(query, 100, mode)
		log.Printf("Search: got %d results", len(results))

		data, _ := json.Marshal(results)
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

	// Refine query endpoint - uses LLM to improve search query
	http.HandleFunc("/refine", func(w http.ResponseWriter, r *http.Request) {
		if llmClient == nil {
			fmt.Fprint(w, `{"success": false, "error": "LLM server not started"}`)
			return
		}

		query := r.URL.Query().Get("q")
		if query == "" {
			fmt.Fprint(w, `{"success": false, "error": "no query"}`)
			return
		}

		// Use LLM to refine the query for better semantic search
		refined, err := llmClient.RefineQuery(query)
		if err != nil {
			fmt.Fprintf(w, `{"success": false, "error": "%v"}`, err)
			return
		}

		fmt.Fprintf(w, `{"success": true, "refined": %s}`, JSONEscape(refined))
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
