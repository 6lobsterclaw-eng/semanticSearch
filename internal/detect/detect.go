package detect

import (
	"os"
	"path/filepath"
)

// FindExeFolder returns the directory where the executable is located
func FindExeFolder() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}

// FindGGUFFiles returns all .gguf files in the given directory
func FindGGUFFiles(dir string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".gguf" {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

// FindLlamaServer checks if llama-server.exe exists in the directory
func FindLlamaServer(dir string) (string, error) {
	serverPath := filepath.Join(dir, "llama-server.exe")
	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		return "", err
	}
	return serverPath, nil
}
