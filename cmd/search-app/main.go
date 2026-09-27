package main

import (
	"fmt"
	"log"
	"os"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func main() {
	// Try to create window using raw Walk API to avoid declarative bug
	mw := new(walk.MainWindow)

	// Create main window
	if err := (MainWindow{
		Title:    "Semantic Search",
		MinSize:  Size{800, 600},
		AssignTo: &mw,
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "Semantic Search - Click buttons to use"},
		},
	}.Create()); err != nil {
		log.Printf("Error: %v", err)
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	log.Println("Window created!")
	mw.Run()
}
