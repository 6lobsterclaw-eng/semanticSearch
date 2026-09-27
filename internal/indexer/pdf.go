package indexer

import (
	"errors"
	"io"
	"os"

	"github.com/ledongthuc/pdf"
)

var ErrPDFRead = errors.New("failed to read PDF")

func ExtractText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", err
	}

	reader, err := pdf.NewReader(f, fi.Size())
	if err != nil {
		return "", ErrPDFRead
	}

	var text []string
	numPages := reader.NumPage()
	for i := 1; i <= numPages; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text = append(text, content)
	}

	return joinWithSpace(text), nil
}

func joinWithSpace(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}

// SkipPDF returns true if file should be skipped (not a PDF)
func SkipPDF(path string) bool {
	// This is a placeholder - actual implementation checks extension
	return false
}

// OpenFile opens a file and returns its reader
func OpenFile(path string) (io.ReadCloser, error) {
	return os.Open(path)
}
