package ui

import (
	"log"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type App struct {
	mw         *walk.MainWindow
	folderPath string
	modelPath  string
	results    *walk.ListBox
	searchTE   *walk.TextEdit
}

func NewApp() *App {
	return &App{}
}

func (a *App) Run() error {
	if _, err := (MainWindow{
		Title:    "Semantic Search",
		MinSize:  Size{800, 600},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "Semantic Search"},
			Label{Text: "Step 1: Select GGUF model file"},
			PushButton{
				Text: "Select Model",
				OnClicked: func() {
					dlg := new(walk.FileDialog)
					dlg.Title = "Select GGUF Model"
					dlg.Filter = "GGUF Files|*.gguf"
					if ok, err := dlg.ShowOpen(a.mw); err == nil && ok {
						a.modelPath = dlg.FilePath
					}
				},
			},
			Label{Text: "Step 2: Select folder to index"},
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
			Label{Text: "Step 3: Enter search query"},
			TextEdit{
				AssignTo: &a.searchTE,
				MinSize:  Size{0, 30},
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
	return nil
}
