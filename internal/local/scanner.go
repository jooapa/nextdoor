package local

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jooapa/nextdoor/internal/state"
	"github.com/zeebo/xxh3"
)

// ScannerOptions provides filtering for the scan process.
type ScannerOptions struct {
	Target        string
	Ignores       []string
	MaxSize       int64
	IncludeHidden bool
}

// Scanner handles local filesystem operations.
type Scanner struct {
	BaseDir string
	State   *state.State
	Options ScannerOptions
}

// NewScanner creates a new scanner initialized with the base directory and current state.
func NewScanner(baseDir string, currentState *state.State, opts ScannerOptions) *Scanner {
	return &Scanner{
		BaseDir: baseDir,
		State:   currentState,
		Options: opts,
	}
}

// Scan traverses the local directory and returns a map of all files and their computed state.
func (s *Scanner) Scan() (map[string]state.FileInfo, error) {
	currentFiles := make(map[string]state.FileInfo)

	err := filepath.WalkDir(s.BaseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the internal .nextdoor directory
		if d.IsDir() && d.Name() == state.StateDir {
			return filepath.SkipDir
		}

		// Skip hidden files/directories if not included
		if !s.Options.IncludeHidden && d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(s.BaseDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		// Ensure consistent path separators (forward slash) for cross-platform compatibility
		relPath = filepath.ToSlash(relPath)

		// Target filtering
		if s.Options.Target != "" {
			targetSlash := filepath.ToSlash(s.Options.Target)
			if relPath != targetSlash && !strings.HasPrefix(relPath, targetSlash+"/") {
				if d.IsDir() {
					// Need to descend if target is deeper
					if relPath != "." && !strings.HasPrefix(targetSlash, relPath+"/") {
						return filepath.SkipDir
					}
				} else {
					return nil
				}
			}
		}

		// Ignores filtering
		for _, ignore := range s.Options.Ignores {
			matched, _ := filepath.Match(ignore, d.Name())
			if !matched {
				matched, _ = filepath.Match(ignore, relPath)
			}
			if matched {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %w", path, err)
		}

		// Max size filtering
		if s.Options.MaxSize > 0 && info.Size() > s.Options.MaxSize {
			return nil
		}

		hash, err := s.HashFileFastPath(path, relPath, info)
		if err != nil {
			return fmt.Errorf("failed to process file %s: %w", relPath, err)
		}

		// ETag will be populated/preserved during the reconciliation phase.
		currentFiles[relPath] = state.FileInfo{
			LocalXXHash3: hash,
			Size:         info.Size(),
			ModTime:      info.ModTime(),
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan local directory: %w", err)
	}

	return currentFiles, nil
}

// HashFileFastPath implements the "I/O Saver" rule.
func (s *Scanner) HashFileFastPath(absPath, relPath string, info fs.FileInfo) (string, error) {
	// The Fast Path: Check if we have a cache hit based on OS metadata
	if s.State != nil && s.State.Files != nil {
		if cached, exists := s.State.Files[relPath]; exists {
			// Compare modtime and size. If they exactly match, file is unchanged.
			if cached.Size == info.Size() && cached.ModTime.Equal(info.ModTime()) {
				if cached.LocalXXHash3 != "" {
					return cached.LocalXXHash3, nil
				}
			}
		}
	}

	// The Slow Path: File was modified or is new. Compute the true xxhash3 hash.
	f, err := os.Open(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer f.Close()

	hasher := xxh3.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", fmt.Errorf("failed to compute hash for %s: %w", absPath, err)
	}

	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes), nil
}
