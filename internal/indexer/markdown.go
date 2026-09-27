package indexer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

func ExtractMarkdown(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	
	// Convert markdown to HTML
	ext := parser.CommonExtensions | parser.Attributes
	p := parser.NewWithExtensions(ext)
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)
	
	// Convert markdown to HTML
	result := markdown.ToHTML(data, p, renderer)
	
	// Strip HTML tags
	text := stripHTML(string(result))
	return text, nil
}

func stripHTML(s string) string {
	result := ""
	inTag := false
	for _, c := range s {
		if c == '<' {
			inTag = true
		} else if c == '>' {
			inTag = false
		} else if !inTag {
			result += string(c)
		}
	}
	// Clean up whitespace
	result = strings.Join(strings.Fields(result), " ")
	return result
}

// ExtractTitle returns the first heading or filename as title
func ExtractTitle(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		// Fallback to filename
		return filenameToTitle(path)
	}
	
	// Try to find first heading
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	
	return filenameToTitle(path)
}

func filenameToTitle(path string) string {
	filename := filepath.Base(path)
	ext := filepath.Ext(filename)
	if ext != "" {
		return filename[:len(filename)-len(ext)]
	}
	return filename
}
