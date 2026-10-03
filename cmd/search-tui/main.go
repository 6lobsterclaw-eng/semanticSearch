package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"semantic-search/internal/indexer"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	app           *tview.Application
	pages         *tview.Pages
	indexerObj    *indexer.Indexer
	llamaProc     *exec.Cmd
	llamaMutex    sync.Mutex
	serverRunning bool

	// UI components
	statusLabel    *tview.TextView
	searchInput    *tview.InputField
	resultsView    *tview.List

	// State
	currentMode    string // "semantic", "keyword", "hybrid"
	currentResults []SearchResult
)

type SearchResult struct {
	Index   int
	Extract string
	Score   float64
	File    string
	Mode    string
}

func main() {
	// Initialize tview
	app = tview.NewApplication()

	// Create pages for different screens
	pages = tview.NewPages()

	// Setup main layout
	setupMainMenu()
	setupLlamaScreen()
	setupIndexScreen()
	setupSearchScreen()
	setupExportScreen()
	setupImportScreen()

	// Show main menu
	pages.SwitchToPage("menu")

	// Enable mouse
	app.EnableMouse(true)

	// Run the app
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// ============ MAIN MENU ============

func setupMainMenu() {
	menu := tview.NewFlex().SetDirection(tview.FlexRow)

	// Header
	header := tview.NewTextView().
		SetText("semanticSearch TUI v1.0").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorBlack)

	// Menu buttons - using Flex
	menuFlex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Create clickable menu items
	menuItems := []struct {
		name string
		key  string
		page string
	}{
		{"Start LlamaServer", "1", "llama"},
		{"Index Files", "2", "index"},
		{"Search", "3", "search"},
		{"Export Index", "4", "export"},
		{"Import Index", "5", "import"},
		{"Quit", "q", "quit"},
	}

	for _, item := range menuItems {
		btn := tview.NewButton(fmt.Sprintf("[%s] %s", item.key, item.name))
		btn.SetSelectedFunc(func() {
			if item.page == "quit" {
				app.Stop()
				return
			}
			pages.SwitchToPage(item.page)
		})
		menuFlex.AddItem(btn, 1, 0, false)
	}

	menu.AddItem(header, 3, 0, false)
	menu.AddItem(menuFlex, 0, 1, false)

	// Status bar
	statusLabel = tview.NewTextView().
		SetText("Ready").
		SetTextAlign(tview.AlignLeft).
		SetTextColor(tcell.ColorDarkGray)

	menu.AddItem(statusLabel, 1, 0, false)

	pages.AddPage("menu", menu, true, true)
}

// ============ LLAMA SERVER SCREEN ============

func setupLlamaScreen() {
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Title
	title := tview.NewTextView().
		SetText("Llama Server").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorBlack)
	flex.AddItem(title, 3, 0, false)

	// Model path input
	form := tview.NewForm()

	modelPath := "C:\\llama.cpp\\models\\qwen3-0.6b-q4_k_m.gguf"
	port := "8080"

	form.AddInputField("Model Path:", modelPath, 60, nil, func(text string) {
		modelPath = text
	})

	form.AddInputField("Port:", port, 10, nil, func(text string) {
		port = text
	})

	form.AddButton("Start", func() {
		startLlamaServer(modelPath, port)
	})

	form.AddButton("Stop", func() {
		stopLlamaServer()
	})

	form.AddButton("Back", func() {
		pages.SwitchToPage("menu")
	})

	flex.AddItem(form, 0, 1, false)

	// Status output
	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	flex.AddItem(statusView, 10, 0, false)

	pages.AddPage("llama", flex, true, false)
}

func startLlamaServer(modelPath, port string) {
	llamaMutex.Lock()
	defer llamaMutex.Unlock()

	if serverRunning {
		updateStatus("Llama server already running")
		return
	}

	// Check if model exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		updateStatus(fmt.Sprintf("Model not found: %s", modelPath))
		return
	}

	// Start llama-server
	llamaProc = exec.Command(
		"llama-server.exe",
		"-m", modelPath,
		"-p", port,
		"--embedding", "true",
		"-ngl", "0",
	)
	llamaProc.Stdout = os.Stdout
	llamaProc.Stderr = os.Stderr

	if err := llamaProc.Start(); err != nil {
		updateStatus(fmt.Sprintf("Failed to start: %v", err))
		return
	}

	serverRunning = true
	updateStatus(fmt.Sprintf("Server started on port %s", port))

	// Wait in background
	go func() {
		llamaProc.Wait()
		llamaMutex.Lock()
		serverRunning = false
		llamaProc = nil
		llamaMutex.Unlock()
		updateStatus("Server stopped")
	}()
}

func stopLlamaServer() {
	llamaMutex.Lock()
	defer llamaMutex.Unlock()

	if llamaProc != nil && llamaProc.Process != nil {
		llamaProc.Process.Kill()
		llamaProc = nil
		serverRunning = false
		updateStatus("Server stopped")
	}
}

// ============ INDEX SCREEN ============

func setupIndexScreen() {
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Title
	title := tview.NewTextView().
		SetText("Index Files").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorBlack)
	flex.AddItem(title, 3, 0, false)

	// Form
	form := tview.NewForm()

	folderPath := ""

	form.AddInputField("Folder Path:", "", 60, nil, func(text string) {
		folderPath = text
	})

	form.AddButton("Browse...", func() {
		updateStatus("Please type the full path")
	})

	form.AddButton("Index", func() {
		if folderPath == "" {
			updateStatus("Please enter a folder path")
			return
		}
		go indexFolder(folderPath)
	})

	form.AddButton("Back", func() {
		pages.SwitchToPage("menu")
	})

	flex.AddItem(form, 0, 1, false)

	// Progress view
	progressView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	flex.AddItem(progressView, 10, 0, false)

	pages.AddPage("index", flex, true, false)
}

func indexFolder(folderPath string) {
	updateStatus("Starting indexing...")

	// Create embedder - needs serverURL and modelPath
	emb, err := indexer.NewHTTPEmbedder("http://localhost:8080", "qwen3-0.6b")
	if err != nil {
		updateStatus(fmt.Sprintf("Embedder error: %v", err))
		return
	}

	// Create indexer
	idx := indexer.NewIndexer(emb)

	// Scan files
	files, err := scanFolder(folderPath)
	if err != nil {
		updateStatus(fmt.Sprintf("Scan error: %v", err))
		return
	}

	if len(files) == 0 {
		updateStatus("No files found")
		return
	}

	// Index files
	for i, file := range files {
		updateStatus(fmt.Sprintf("Indexing %d/%d: %s", i+1, len(files), filepath.Base(file)))
		if err := idx.AddFile(file); err != nil {
			log.Printf("Error indexing %s: %v", file, err)
		}
	}

	indexerObj = idx
	updateStatus(fmt.Sprintf("Indexed %d files, %d chunks", idx.DocCount(), idx.ChunkCount()))
}

// ============ SEARCH SCREEN ============

func setupSearchScreen() {
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Title
	title := tview.NewTextView().
		SetText("Search").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorBlack)
	flex.AddItem(title, 3, 0, false)

	// Mode selector
	modeFlex := tview.NewFlex()

	modeFlex.AddItem(tview.NewTextView().SetText("Mode: "), 7, 0, false)

	semanticBtn := tview.NewButton("[S]emantic")
	semanticBtn.SetSelectedFunc(func() {
		currentMode = "semantic"
		updateModeDisplay()
	})
	keywordBtn := tview.NewButton("[K]eyword")
	keywordBtn.SetSelectedFunc(func() {
		currentMode = "keyword"
		updateModeDisplay()
	})
	hybridBtn := tview.NewButton("[H]ybrid")
	hybridBtn.SetSelectedFunc(func() {
		currentMode = "hybrid"
		updateModeDisplay()
	})

	modeFlex.AddItem(semanticBtn, 0, 1, false)
	modeFlex.AddItem(keywordBtn, 0, 1, false)
	modeFlex.AddItem(hybridBtn, 0, 1, false)

	flex.AddItem(modeFlex, 3, 0, false)

	// Search input
	searchInput = tview.NewInputField().
		SetLabel("Query: ").
		SetPlaceholder("Enter search query...")

	flex.AddItem(searchInput, 3, 0, false)

	// Results header
	header := tview.NewTextView().
		SetText("# | Extract                                | Score  | File               | Mode").
		SetTextColor(tcell.ColorDarkGray)
	flex.AddItem(header, 1, 0, false)

	// Results list
	resultsView = tview.NewList()

	flex.AddItem(resultsView, 0, 1, false)

	// Help text
	help := tview.NewTextView().
		SetText("Enter to search, Esc to go back").
		SetTextColor(tcell.ColorDarkGray).
		SetTextAlign(tview.AlignCenter)
	flex.AddItem(help, 1, 0, false)

	currentMode = "semantic"
	pages.AddPage("search", flex, true, false)
}

func updateModeDisplay() {
	updateStatus(fmt.Sprintf("Mode: %s", currentMode))
}

func performSearch(query string) {
	if indexerObj == nil {
		updateStatus("No index loaded. Please index files first.")
		return
	}

	if query == "" {
		updateStatus("Please enter a search query")
		return
	}

	updateStatus(fmt.Sprintf("Searching for: %s", query))

	// Use indexer's search methods
	results := indexerObj.Search(query, 50, currentMode)

	// Convert to display results
	currentResults = make([]SearchResult, 0, len(results))
	app.QueueUpdate(func() {
		resultsView.Clear()

		for i, r := range results {
			// Extract text snippet
			extract := r.Extract
			if len(extract) > 35 {
				extract = extract[:35] + "..."
			}
			// Clean newlines
			extract = strings.ReplaceAll(extract, "\n", " ")

			result := SearchResult{
				Index:   i + 1,
				Extract: extract,
				Score:   r.Score,
				File:    r.Path,
				Mode:    r.Type,
			}
			currentResults = append(currentResults, result)

			// Format: # | Extract | Score | File | Mode
			text := fmt.Sprintf("%2d | %-35s | %.4f | %-20s | %s",
				result.Index, result.Extract, result.Score, filepath.Base(result.File), result.Mode)

			resultsView.AddItem(text, "", 0, nil)
		}

		updateStatus(fmt.Sprintf("Found %d results", len(results)))
	})
}

// ============ EXPORT SCREEN ============

func setupExportScreen() {
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Title
	title := tview.NewTextView().
		SetText("Export Index").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorBlack)
	flex.AddItem(title, 3, 0, false)

	// Form
	form := tview.NewForm()

	exportPath := ""

	form.AddInputField("Export Folder:", "", 60, nil, func(text string) {
		exportPath = text
	})

	form.AddButton("Export", func() {
		if indexerObj == nil {
			updateStatus("No index to export")
			return
		}
		if exportPath == "" {
			updateStatus("Please enter export folder path")
			return
		}
		go exportIndex(exportPath)
	})

	form.AddButton("Back", func() {
		pages.SwitchToPage("menu")
	})

	flex.AddItem(form, 0, 1, false)

	// Status view
	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	flex.AddItem(statusView, 10, 0, false)

	pages.AddPage("export", flex, true, false)
}

func exportIndex(path string) {
	if indexerObj == nil {
		updateStatus("No index to export")
		return
	}

	updateStatus(fmt.Sprintf("Exporting to: %s", path))

	if err := indexerObj.Export(path); err != nil {
		updateStatus(fmt.Sprintf("Export error: %v", err))
		return
	}

	updateStatus("Export complete!")
}

// ============ IMPORT SCREEN ============

func setupImportScreen() {
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Title
	title := tview.NewTextView().
		SetText("Import Index").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorBlack)
	flex.AddItem(title, 3, 0, false)

	// Form
	form := tview.NewForm()

	importPath := ""

	form.AddInputField("Import Folder:", "", 60, nil, func(text string) {
		importPath = text
	})

	form.AddButton("Import", func() {
		if importPath == "" {
			updateStatus("Please enter import folder path")
			return
		}
		go importIndex(importPath)
	})

	form.AddButton("Back", func() {
		pages.SwitchToPage("menu")
	})

	flex.AddItem(form, 0, 1, false)

	// Status view
	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	flex.AddItem(statusView, 10, 0, false)

	pages.AddPage("import", flex, true, false)
}

func importIndex(path string) {
	updateStatus(fmt.Sprintf("Importing from: %s", path))

	// Create embedder
	emb, err := indexer.NewHTTPEmbedder("http://localhost:8080", "qwen3-0.6b")
	if err != nil {
		updateStatus(fmt.Sprintf("Embedder error: %v", err))
		return
	}

	// Create new indexer
	idx := indexer.NewIndexer(emb)

	if err := idx.Import(path); err != nil {
		updateStatus(fmt.Sprintf("Import error: %v", err))
		return
	}

	indexerObj = idx
	updateStatus(fmt.Sprintf("Imported! Files: %d, Chunks: %d", idx.DocCount(), idx.ChunkCount()))
}

// ============ UTILITIES ============

func updateStatus(msg string) {
	app.QueueUpdate(func() {
		if statusLabel != nil {
			statusLabel.SetText(msg)
		}
	})
}

func scanFolder(folder string) ([]string, error) {
	var files []string

	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
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

	return files, err
}
