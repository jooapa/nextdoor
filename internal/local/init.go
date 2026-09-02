package local

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jooapa/nextdoor/internal/state"
)

// Init initializes a new Nextdoor sync environment in the given path.
// It creates a .nextdoor directory based on our Architecture.md.
func Init(targetPath string, remote string, rebuild bool) error {
	nextdoorDir := filepath.Join(targetPath, ".nextdoor")
	
	// Create the .nextdoor directory
	if err := os.MkdirAll(nextdoorDir, 0755); err != nil {
		return fmt.Errorf("failed to create .nextdoor directory: %w", err)
	}

	st, err := state.Load(targetPath)
	if err != nil {
		return err
	}

	if rebuild {
		st.Files = make(map[string]state.FileInfo)
	}

	if remote != "" {
		st.RemoteTarget = remote
	}

	if err := state.Save(targetPath, st); err != nil {
		return err
	}

	fmt.Printf("Initialized Nextdoor repository in %s\n", nextdoorDir)
	return nil
}
