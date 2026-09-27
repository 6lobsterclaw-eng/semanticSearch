package ui

import (
	"fyne.io/fyne"
	"fyne.io/fyne/app"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/widget"
)

type App struct {
	fyneApp fyne.App
	window  fyne.Window
}

func NewApp() *App {
	a := app.New()
	w := a.NewWindow("Semantic Search")
	
	return &App{
		fyneApp: a,
		window:  w,
	}
}

func (a *App) Run() {
	// Create UI components
	title := widget.NewLabel("Semantic Search")
	title.TextStyle.Bold = true
	
	folderBtn := widget.NewButton("Select Folder", func() {
		// TODO: Implement folder selection
	})
	
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Enter search query...")
	
	resultsList := widget.NewListWithData(
		nil,
		func() fyne.CanvasObject {
			return widget.NewLabel("result")
		},
		func(i interface{}, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(i.(string))
		},
	)
	
	// Layout
	content := fyne.NewContainerWithLayout(
		layout.NewVBoxLayout(),
		title,
		widget.NewSeparator(),
		folderBtn,
		searchEntry,
		widget.NewSeparator(),
		resultsList,
	)
	
	a.window.SetContent(content)
	a.window.Resize(fyne.NewSize(800, 600))
	a.window.ShowAndRun()
}
