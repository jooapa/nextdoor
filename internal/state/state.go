package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileInfo represents the tracked state of a single file.
type FileInfo struct {
	LocalXXHash3 string    `json:"local_xxhash3"`
	RemoteETag   string    `json:"remote_etag"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"modtime"`
}

type State struct {
	SchemaVersion  int                 `json:"schemaVersion"`
	RemoteTarget   string              `json:"remoteTarget"`
	RemoteRootETag string              `json:"remoteRootETag,omitempty"`
	LastSyncTime   time.Time           `json:"lastSyncTime"`
	Files          map[string]FileInfo `json:"files"`
}

const (
	StateDir      = ".nextdoor"
	StateFileName = "directory.json"
)

// Load reads the state from the given base directory (which should contain .nextdoor).
func Load(baseDir string) (*State, error) {
	statePath := filepath.Join(baseDir, StateDir, StateFileName)
	f, err := os.Open(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return a new empty state if it doesn't exist
			return &State{
				SchemaVersion: 2,
				Files:         make(map[string]FileInfo),
			}, nil
		}
		return nil, fmt.Errorf("failed to open state file: %w", err)
	}
	defer f.Close()

	var s State
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, fmt.Errorf("failed to decode state file: %w", err)
	}
	if s.Files == nil {
		s.Files = make(map[string]FileInfo)
	}
	return &s, nil
}

// Save writes the state to the .nextdoor/directory.json file atomically.
func Save(baseDir string, s *State) error {
	stateDirPath := filepath.Join(baseDir, StateDir)
	if err := os.MkdirAll(stateDirPath, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	statePath := filepath.Join(stateDirPath, StateFileName)

	// Open a unique temp file for writing
	f, err := os.CreateTemp(stateDirPath, StateFileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w", err)
	}
	tmpPath := f.Name()

	// Write JSON data
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to encode state: %w", err)
	}

	// Flush to disk (fsync) to ensure data is physically written before renaming
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync temp state file to disk: %w", err)
	}

	// Close the file before renaming (crucial for Windows, good practice generally)
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp state file: %w", err)
	}

	// Atomically rename tmp file to the actual state file
	if err := os.Rename(tmpPath, statePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp state file to %s: %w", StateFileName, err)
	}

	return nil
}
