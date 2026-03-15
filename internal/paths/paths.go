package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const baseDir = ".sikifanso"

// RootDir returns the sikifanso root directory.
// It checks SIKIFANSO_HOME first, falling back to ~/.sikifanso.
func RootDir() (string, error) {
	if v := os.Getenv("SIKIFANSO_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, baseDir), nil
}
