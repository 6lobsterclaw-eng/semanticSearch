package ui

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"semantic-search/internal/detect"
	"semantic-search/internal/indexer"

	"github.com/kelindar/search"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type App struct {
	mw          *walk.MainWindow
	folderPath  string
	modelPath   string
	serverURL   string
	serverPath  string
	port        int
	embedder    interface {
		Embed(string) (search.Vector, error)
		AddDocument(string, search.Vector, string)
		Search(search.Vector, int) []search.Result[string]
		Dim() int
		SaveIndex(string) error
		LoadIndex(string) error
		Close() error
	}
	indexer     *indexer.Indexer
	results     *walk.ListBox
	searchTE    *walk.TextEdit
	statusLabel *walk.Label
	modelCombo  *walk.ComboBox
	serverBtn   *walk.PushButton
	serverCmd   *exec.Cmd
}

func NewApp() *App {
	return &App{
		port: 8080,
	}
}

// scanAndPopulate scans exe folder and populates UI (call AFTER UI is created)
func (a *App) scanAndPopulate() {
	log.Println("scanAndPopulate called")
	
	// Check if widgets are ready
	if a.statusLabel == nil || a.modelCombo == nil || a.serverBtn == nil {
		log.Println("Widgets not ready yet, skipping scan")
		return
	}

	log.Println("Widgets ready, scanning...")
	
	exeDir, err := detect.FindExeFolder()
	if err != nil {
		log.Printf("FindExeFolder error: %v", err)
		a.statusLabel.SetText("Status: Error - Cannot find exe folder")
		return
	}

	log.Printf("Exe folder: %s", exeDir)

	serverPath, err := detect.FindLlamaServer(exeDir)
	if err != nil {
		a.statusLabel.SetText("Status: llama-server.exe NOT FOUND. Press F5 to refresh.")
		a.serverPath = ""
		a.serverBtn.SetEnabled(false)
	} else {
		a.serverPath = serverPath
		a.statusLabel.SetText(fmt.Sprintf("Status: Found llama-server.exe at %s", filepath.Base(serverPath)))
		a.serverBtn.SetEnabled(true)
	}

	ggufFiles, err := detect.FindGGUFFiles(exeDir)
	if err != nil {
		log.Printf("Error scanning for GGUF files: %v", err)
		ggufFiles = []string{}
	}

	a.modelCombo.SetModel(ggufFiles)
	if len(ggufFiles) > 0 {
		a.modelCombo.SetCurrentIndex(0)
		a.modelPath = filepath.Join(exeDir, ggufFiles[0])
		a.serverBtn.SetEnabled(a.serverPath != "")
	} else {
		a.modelPath = ""
		a.statusLabel.SetText(a.statusLabel.Text() + " | No GGUF files found. Press F5 to refresh.")
	}
}

// startServer starts llama-server
func (a *App) startServer() {
	if a.serverPath == "" || a.modelPath == "" {
		a.statusLabel.SetText("Status: Please select llama-server and model first")
		return
	}

	a.statusLabel.SetText("Status: Starting llama-server...")
	a.port = 8080

	a.serverCmd = exec.Command(a.serverPath, "-m", a.modelPath, "--port", fmt.Sprintf("%d", a.port))
	a.serverCmd.Stdout = log.Writer()
	a.serverCmd.Stderr = log.Writer()

	if err := a.serverCmd.Start(); err != nil {
		a.statusLabel.SetText(fmt.Sprintf("Status: Failed to start server: %v", err))
		return
	}

	a.serverURL = fmt.Sprintf("http://localhost:%d", a.port)
	emb, err := indexer.NewHTTPEmbedder(a.serverURL, a.modelPath)
	if err != nil {
		a.statusLabel.SetText(fmt.Sprintf("Status: Failed to create embedder: %v", err))
		return
	}

	for i := 0; i < 20; i++ {
		if emb.IsServerReady() {
			a.statusLabel.SetText(fmt.Sprintf("Status: Server running on port %d", a.port))
			return
		}
		for j := 0; j < 5000000; j++ {
		}
	}

	emb.Close()
	a.statusLabel.SetText("Status: Server failed to start (timeout)")
}

func (a *App) Run() error {
	var statusText string = "Status: Scanning for llama-server.exe and GGUF files..."
	log.Println("Run() started")

	if _, err := (MainWindow{
		Title:    "Semantic Search",
		MinSize:  Size{800, 600},
		Layout:   VBox{},
		OnKeyPress: func(key walk.Key) {
			if key == walk.KeyF5 {
				log.Println("F5 pressed")
				a.scanAndPopulate()
			}
		},
		Children: []Widget{
			Label{Text: "=== Step 1: Server Setup ==="},
			Label{
				AssignTo: &a.statusLabel,
				Text:     statusText,
			},
			Label{Text: "Model:"},
			ComboBox{
				AssignTo:  &a.modelCombo,
				Editable:  false,
				OnCurrentIndexChanged: func() {
					idx := a.modelCombo.CurrentIndex()
					if idx >= 0 {
						exeDir, _ := detect.FindExeFolder()
						items := a.modelCombo.Model().([]string)
						if idx < len(items) {
							a.modelPath = filepath.Join(exeDir, items[idx])
						}
					}
				},
			},
			PushButton{
				AssignTo: &a.serverBtn,
				Text:     "Start Server",
				OnClicked: func() {
					a.startServer()
				},
			},
			Label{Text: "Press F5 to refresh scan"},
			Label{Text: "=== Step 2: Select Folder ==="},
			PushButton{
				Text: "Select Folder",
				OnClicked: func() {
					dlg := new(walk.FileDialog)
					dlg.Title = "Select Folder"
					if ok, err := dlg.ShowBrowseFolder(a.mw); err == nil && ok {
						a.folderPath = dlg.FilePath
					}
				},
			},
			Label{Text: "Status: Ready"},
			PushButton{
				Text: "Start Indexing",
				OnClicked: func() {
					if a.serverURL == "" {
						a.statusLabel.SetText("Status: Please start server first")
						return
					}
					if a.folderPath == "" {
						a.statusLabel.SetText("Status: Please select folder first")
						return
					}
					a.statusLabel.SetText("Status: Creating HTTP embedder...")

					emb, err := indexer.NewHTTPEmbedder(a.serverURL, a.modelPath)
					if err != nil {
						a.statusLabel.SetText(fmt.Sprintf("Status: Failed to create embedder: %v", err))
						return
					}
					a.embedder = emb

					a.statusLabel.SetText("Status: Indexing...")
					idx := indexer.NewIndexer(emb)
					if err := idx.IndexFolder(a.folderPath); err != nil {
						a.statusLabel.SetText(fmt.Sprintf("Status: Indexing failed: %v", err))
						return
					}
					a.indexer = idx

					indexPath := filepath.Join(a.folderPath, ".semantic-index")
					if err := emb.SaveIndex(indexPath); err != nil {
						log.Printf("Warning: failed to save index: %v", err)
					}

					a.statusLabel.SetText(fmt.Sprintf("Status: Indexed %d documents", idx.DocumentCount()))
				},
			},
			Label{Text: "=== Step 3: Search ==="},
			TextEdit{
				AssignTo: &a.searchTE,
				MinSize:  Size{0, 30},
			},
			PushButton{
				Text: "Search",
				OnClicked: func() {
					if a.embedder == nil {
						a.statusLabel.SetText("Status: Please index first")
						return
					}
					query := a.searchTE.Text()
					if query == "" {
						return
					}
					vec, err := a.embedder.Embed(query)
					if err != nil {
						a.statusLabel.SetText(fmt.Sprintf("Status: Embed failed: %v", err))
						return
					}
					searchResults := a.embedder.Search(vec, 10)
					a.results.SetModel(nil)
					models := make([]string, len(searchResults))
					for i, r := range searchResults {
						preview := r.Value
						if len(preview) > 100 {
							preview = preview[:100] + "..."
						}
						models[i] = fmt.Sprintf("%.2f - %s", r.Relevance, preview)
					}
					a.results.SetModel(models)
					a.statusLabel.SetText(fmt.Sprintf("Status: Found %d results", len(searchResults)))
				},
			},
			Label{Text: "Results:"},
			ListBox{
				AssignTo: &a.results,
			},
		},
	}.Run()); err != nil {
		log.Fatal(err)
		return err
	}

	a.scanAndPopulate()

	return nil
}
