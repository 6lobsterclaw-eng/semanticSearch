package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"semantic-search/internal/detect"
	"semantic-search/internal/indexer"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Simple debug log to file
func debugLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	f, err := os.OpenFile("debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		f.WriteString(msg + "\n")
		f.Close()
	}
}

// Global panic handler
func init() {
	defer func() {
		if r := recover(); r != nil {
			debugLog("GLOBAL PANIC: %v", r)
			fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
		}
	}()
}

var (
	app           *tview.Application
	pages         *tview.Pages
	indexerObj    *indexer.Indexer
	llamaProc     *exec.Cmd
	llamaMutex    sync.Mutex
	serverRunning bool

	// Two servers: one for embedding, one for LLM
	llmProc          *exec.Cmd
	llmServerRunning bool

	// Server status indicators (for UI)
	embedStatusView *tview.TextView
	llmStatusView   *tview.TextView

	// UI components
	statusLabel    *tview.TextView
	searchInput    *tview.InputField
	resultsView    *tview.List

	// State
	currentMode    string // "semantic", "keyword", "hybrid"
	currentResults []SearchResult

	// Auto-detected paths
	exeFolder      string
	ggufFiles      []string

	// Selected models (indices into ggufFiles)
	embedModelIndex int
	llmModelIndex   int
)

type SearchResult struct {
	Index   int
	Extract string
	Score   float64
	File    string
	Mode    string
}

func main() {
	// Catch any panic in main
	defer func() {
		if r := recover(); r != nil {
			debugLog("PANIC in main: %v", r)
		}
	}()
	
	// Auto-detect exe folder and GGUF files
	var err error
	exeFolder, err = detect.FindExeFolder()
	if err != nil {
		log.Printf("Warning: Could not find exe folder: %v", err)
		exeFolder = "."
	}

	debugLog("Startup: exeFolder=%s", exeFolder)

	// Auto-detect GGUF files
	ggufFiles, _ = detect.FindGGUFFiles(exeFolder)
	debugLog("Startup: found %d GGUF files: %v", len(ggufFiles), ggufFiles)
	if len(ggufFiles) == 0 {
		debugLog("No GGUF files found in %s", exeFolder)
	}

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

	// Set root pages
	app.SetRoot(pages, true)

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
		SetText("Llama Server - Auto-detected from exe folder").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorBlack)
	flex.AddItem(title, 3, 0, false)

	// Auto-detect info
	infoText := fmt.Sprintf("Exe Folder: %s\nGGUF Files: %d found",
		exeFolder, len(ggufFiles))
	infoView := tview.NewTextView().
		SetText(infoText).
		SetTextColor(tcell.ColorDarkGray)
	flex.AddItem(infoView, 4, 0, false)

	// Prepare options for dropdowns
	var modelOptions []string
	if len(ggufFiles) == 0 {
		modelOptions = []string{"(No GGUF files found)"}
	} else {
		modelOptions = ggufFiles
	}

	// Default selection
	if len(ggufFiles) >= 1 {
		embedModelIndex = 0
	}
	if len(ggufFiles) >= 2 {
		llmModelIndex = 1
	} else {
		llmModelIndex = 0
	}

	// Form
	form := tview.NewForm()

	// Embedding model dropdown with status
	embedFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	embedDropdown := tview.NewDropDown().
		SetLabel("Embedding Model: ").
		SetOptions(modelOptions, func(option string, optionIndex int) {
			embedModelIndex = optionIndex
		})
	embedDropdown.SetCurrentOption(embedModelIndex)
	embedStatusView = tview.NewTextView().
		SetText("[red]●[white]").
		SetDynamicColors(true)
	embedFlex.AddItem(embedDropdown, 0, 1, true)
	embedFlex.AddItem(embedStatusView, 5, 0, false)
	flex.AddItem(embedFlex, 1, 0, false)

	// LLM section header
	llmHeader := tview.NewTextView().
		SetText("=== LLM MODEL (for answer generation) ===").
		SetTextColor(tcell.ColorBlue)
	flex.AddItem(llmHeader, 1, 0, false)

	// LLM model dropdown with status
	llmFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	llmDropdown := tview.NewDropDown().
		SetLabel("LLM Model: ").
		SetOptions(modelOptions, func(option string, optionIndex int) {
			llmModelIndex = optionIndex
		})
	llmDropdown.SetCurrentOption(llmModelIndex)
	llmStatusView = tview.NewTextView().
		SetText("[red]●[white]").
		SetDynamicColors(true)
	llmFlex.AddItem(llmDropdown, 0, 1, true)
	llmFlex.AddItem(llmStatusView, 5, 0, false)
	flex.AddItem(llmFlex, 1, 0, false)

	form.AddButton("Start Both", func() {
		// Catch any panic
		defer func() {
			if r := recover(); r != nil {
				debugLog("PANIC in Start Both button: %v", r)
				updateStatus(fmt.Sprintf("Panic: %v", r))
			}
		}()
		
		debugLog("Start button clicked")
		debugLog("ggufFiles length: %d", len(ggufFiles))
		debugLog("embedModelIndex: %d, llmModelIndex: %d", embedModelIndex, llmModelIndex)
		
		if len(ggufFiles) == 0 || embedModelIndex >= len(ggufFiles) {
			updateStatus("No embedding model selected")
			debugLog("No embedding model")
			return
		}
		if len(ggufFiles) == 0 || llmModelIndex >= len(ggufFiles) {
			updateStatus("No LLM model selected")
			debugLog("No LLM model")
			return
		}
		
		embedModelPath := filepath.Join(exeFolder, ggufFiles[embedModelIndex])
		llmModelPath := filepath.Join(exeFolder, ggufFiles[llmModelIndex])
		
		debugLog("embedModelPath: %s", embedModelPath)
		debugLog("llmModelPath: %s", llmModelPath)
		
		// Check files exist
		if _, err := os.Stat(embedModelPath); os.IsNotExist(err) {
			updateStatus(fmt.Sprintf("Embed model not found: %s", embedModelPath))
			debugLog("Embed model not found: %v", err)
			return
		}
		if _, err := os.Stat(llmModelPath); os.IsNotExist(err) {
			updateStatus(fmt.Sprintf("LLM model not found: %s", llmModelPath))
			debugLog("LLM model not found: %v", err)
			return
		}
		
		debugLog("Starting servers in goroutines...")
		go startLlamaServer(embedModelPath, "8080")
		debugLog("After startLlamaServer goroutine")
		go startLLMServer(llmModelPath, "8081")
		debugLog("After startLLMServer goroutine")
		debugLog("Servers started")
	})

	form.AddButton("Stop Both", func() {
		stopLlamaServer()
		stopLLMServer()
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
	debugLog("startLlamaServer: modelPath=%s, port=%s", modelPath, port)

	// Catch any panic in this function
	defer func() {
		if r := recover(); r != nil {
			debugLog("PANIC in startLlamaServer: %v", r)
		}
	}()

	// Catch any panic in this function
	defer func() {
		if r := recover(); r != nil {
			debugLog("PANIC in startLlamaServer: %v", r)
		}
	}()

	if serverRunning {
		debugLog("Llama server already running")
		updateStatus("Llama server already running")
		return
	}

	// Check if model exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		debugLog("Model not found: %v", err)
		updateStatus(fmt.Sprintf("Model not found: %s", modelPath))
		return
	}

	// Start llama-server (always CPU)
	// Use full path to llama-server.exe in exe folder
	llamaServerPath := filepath.Join(exeFolder, "llama-server.exe")
	args := []string{
		llamaServerPath,
		"-m", modelPath,
		"--port", port,
		"-ngl", "0",
	}
	// Note: embedding mode is determined by the model itself, no flag needed

	debugLog("Running command: %v", args)
	
	llamaProc = exec.Command(args[0], args[1:]...)
	debugLog("After exec.Command")
	
	// Redirect output to log files (after exec.Command)
	logFile, err := os.OpenFile("llama-server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	debugLog("After OpenFile, err=%v", err)
	if err == nil {
		llamaProc.Stdout = logFile
		llamaProc.Stderr = logFile
	}

	debugLog("Before llamaProc.Start()")
	if err := llamaProc.Start(); err != nil {
		debugLog("Failed to start: %v", err)
		return
	}

	debugLog("llamaProc.Start() succeeded, pid=%d", llamaProc.Process.Pid)

	serverRunning = true
	debugLog("Before updateEmbedStatus")
	updateEmbedStatus(true)
	debugLog("After updateEmbedStatus")
	// Note: Don't call updateStatus from goroutine - it can block

	// Wait in background
	go func() {
		llamaProc.Wait()
		llamaMutex.Lock()
		serverRunning = false
		llamaProc = nil
		llamaMutex.Unlock()
		updateEmbedStatus(false)
		debugLog("Embedding server stopped")
	}()
}

func startLLMServer(modelPath, port string) {
	debugLog("startLLMServer: modelPath=%s, port=%s", modelPath, port)
	
	// Catch panic
	defer func() {
		if r := recover(); r != nil {
			debugLog("PANIC in startLLMServer: %v", r)
		}
	}()
	
	if llmServerRunning {
		updateStatus("LLM server already running")
		return
	}

	// Check if model exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		updateStatus(fmt.Sprintf("LLM Model not found: %s", modelPath))
		debugLog("LLM Model not found: %v", err)
		return
	}

	// Start llama-server (always CPU, no embedding flag for LLM)
	// Use full path to llama-server.exe in exe folder
	llamaServerPath := filepath.Join(exeFolder, "llama-server.exe")
	debugLog("LLM: Running command: %s -m %s --port %s -ngl 0", llamaServerPath, modelPath, port)
	llmProc = exec.Command(
		llamaServerPath,
		"-m", modelPath,
		"--port", port,
		"-ngl", "0",
	)
	
	// Redirect output to log file
	logFile, err := os.OpenFile("llama-llm.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		llmProc.Stdout = logFile
		llmProc.Stderr = logFile
	}

	if err := llmProc.Start(); err != nil {
		debugLog("LLM Failed to start: %v", err)
		return
	}

	debugLog("LLM server started, pid=%d", llmProc.Process.Pid)
	llmServerRunning = true
	updateLLMStatus(true)
	// Note: Don't call updateStatus from goroutine

	// Wait in background
	go func() {
		llmProc.Wait()
		llmServerRunning = false
		llmProc = nil
		updateLLMStatus(false)
		debugLog("LLM server stopped")
	}()
}

func stopLlamaServer() {
	llamaMutex.Lock()
	defer llamaMutex.Unlock()

	if llamaProc != nil && llamaProc.Process != nil {
		llamaProc.Process.Kill()
		llamaProc = nil
		serverRunning = false
		updateStatus("Embedding server stopped")
	}
}

func stopLLMServer() {
	if llmProc != nil && llmProc.Process != nil {
		llmProc.Process.Kill()
		llmProc = nil
		llmServerRunning = false
		updateStatus("LLM server stopped")
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
		// Use PowerShell to open Windows folder browser dialog (runs in background)
		go func() {
			cmd := exec.Command("powershell", "-Command", `
				Add-Type -AssemblyName System.Windows.Forms
				$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
				$dialog.Description = "Select folder to index"
				if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
					Write-Output $dialog.SelectedPath
				}
			`)
			output, err := cmd.Output()
			if err != nil {
				app.QueueUpdate(func() {
					updateStatus("Browse cancelled or error")
				})
				return
			}
			selectedPath := strings.TrimSpace(string(output))
			if selectedPath != "" {
				app.QueueUpdate(func() {
					folderPath = selectedPath
					form.GetFormItemByLabel("Folder Path:").(*tview.InputField).SetText(selectedPath)
					updateStatus("Selected: " + selectedPath)
				})
			}
		}()
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

	// Index folder (handles scanning and indexing)
	updateStatus(fmt.Sprintf("Indexing folder: %s", folderPath))
	err = idx.IndexFolder(folderPath, func(done, total, chunks int) {
		updateStatus(fmt.Sprintf("Progress: %d/%d files, %d chunks", done, total, chunks))
	})

	if err != nil {
		updateStatus(fmt.Sprintf("Index error: %v", err))
		return
	}

	indexerObj = idx
	updateStatus(fmt.Sprintf("Indexed %d files", idx.FileCount()))
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

	// Load index
	if err := idx.LoadIndex(path); err != nil {
		updateStatus(fmt.Sprintf("Import error: %v", err))
		return
	}

	indexerObj = idx
	updateStatus(fmt.Sprintf("Imported! Files: %d", idx.FileCount()))
}

// ============ UTILITIES ============

func updateStatus(msg string) {
	app.QueueUpdate(func() {
		if statusLabel != nil {
			statusLabel.SetText(msg)
		}
	})
}

// Update embed server status indicator
func updateEmbedStatus(running bool) {
	debugLog("updateEmbedStatus called: running=%v, embedStatusView=%v", running, embedStatusView != nil)
	app.QueueUpdate(func() {
		if embedStatusView != nil {
			debugLog("Setting embed status: %v", running)
			if running {
				embedStatusView.SetText("[green]●[white]")
			} else {
				embedStatusView.SetText("[red]●[white]")
			}
		}
	})
}

// Update LLM server status indicator
func updateLLMStatus(running bool) {
	debugLog("updateLLMStatus called: running=%v, llmStatusView=%v", running, llmStatusView != nil)
	app.QueueUpdate(func() {
		if llmStatusView != nil {
			debugLog("Setting LLM status: %v", running)
			if running {
				llmStatusView.SetText("[green]●[white]")
			} else {
				llmStatusView.SetText("[red]●[white]")
			}
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
