package local

import (
	"fmt"
	"os"
	"path/filepath"
)

// Init initializes a new Nextdoor sync environment in the given path.
// It creates a .nextdoor directory based on our Architecture.md.
func Init(targetPath string) error {
	nextdoorDir := filepath.Join(targetPath, ".nextdoor")
	
	// Create the .nextdoor directory
	if err := os.MkdirAll(nextdoorDir, 0755); err != nil {
		return fmt.Errorf("failed to create .nextdoor directory: %w", err)
	}

	fmt.Printf("Initialized empty Nextdoor repository in %s\n", nextdoorDir)
	return nil
}
