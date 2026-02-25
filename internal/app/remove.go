package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alicanalbayrak/sikifanso/internal/gitops"
)

// Remove deletes the coordinate and values files, then commits.
func Remove(gitOpsPath, name string) error {
	coordPath := filepath.Join("apps", "coordinates", name+".yaml")
	absCoord := filepath.Join(gitOpsPath, coordPath)

	if _, err := os.Stat(absCoord); os.IsNotExist(err) {
		return fmt.Errorf("app %q not found", name)
	}

	if err := os.Remove(absCoord); err != nil {
		return fmt.Errorf("removing coordinates file: %w", err)
	}

	valuesPath := filepath.Join("apps", "values", name+".yaml")
	absValues := filepath.Join(gitOpsPath, valuesPath)
	if _, err := os.Stat(absValues); err == nil {
		if err := os.Remove(absValues); err != nil {
			return fmt.Errorf("removing values file: %w", err)
		}
	}

	if err := gitops.Commit(gitOpsPath, fmt.Sprintf("remove app %s", name), coordPath, valuesPath); err != nil {
		return fmt.Errorf("committing removal: %w", err)
	}

	return nil
}
