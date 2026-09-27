package ui

import (
	"testing"

	"fyne.io/fyne/test"
)

func TestAppLayout(t *testing.T) {
	// Test that app can be created
	app := test.NewApp()
	defer app.Quit()

	// Create window
	w := app.NewWindow("Semantic Search")
	if w == nil {
		t.Error("expected window, got nil")
	}

	// Verify title
	if w.Title() != "Semantic Search" {
		t.Errorf("expected title 'Semantic Search', got '%s'", w.Title())
	}
}
