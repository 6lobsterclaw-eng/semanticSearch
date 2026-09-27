package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
)

func main() {
	log.Println("Starting Semantic Search Web UI...")

	// Simple HTML UI
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Semantic Search</title>
    <style>
        body { font-family: Arial; padding: 20px; max-width: 800px; margin: 0 auto; }
        h1 { color: #333; }
        .step { margin: 20px 0; padding: 15px; border: 1px solid #ddd; border-radius: 5px; }
        button { padding: 10px 20px; cursor: pointer; }
        input { padding: 8px; width: 300px; }
        #results { margin-top: 20px; }
        .result { padding: 10px; border-bottom: 1px solid #eee; }
    </style>
</head>
<body>
    <h1>Semantic Search</h1>
    <div class="step">
        <h3>Step 1: Select GGUF Model</h3>
        <input type="text" id="modelPath" placeholder="Path to .gguf file">
    </div>
    <div class="step">
        <h3>Step 2: Select Folder</h3>
        <input type="text" id="folderPath" placeholder="Path to folder">
    </div>
    <div class="step">
        <h3>Step 3: Search</h3>
        <input type="text" id="query" placeholder="Enter search query">
        <button onclick="doSearch()">Search</button>
    </div>
    <div id="results"></div>
    <script>
        function doSearch() {
            document.getElementById('results').innerHTML = '<p>Searching...</p>';
        }
    </script>
</body>
</html>`

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, html)
	})

	addr := ":8080"
	log.Printf("Open browser: http://localhost%s", addr)
	log.Println("Press Ctrl+C to stop")

	// Open browser automatically
	url := "http://localhost" + addr
	openBrowser(url)

	log.Fatal(http.ListenAndServe(addr, nil))
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
