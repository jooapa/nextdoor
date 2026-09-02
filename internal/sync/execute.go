package sync

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	

	"github.com/jooapa/nextdoor/internal/nextcloud"
	"github.com/jooapa/nextdoor/internal/local"
	"github.com/jooapa/nextdoor/internal/state"
	"github.com/studio-b12/gowebdav"
)

type ExecutionOptions struct {
	BaseDir      string
	RemoteTarget string
	DryRun       bool
	NoDelete     bool
	Strategy     string // "force-local" or "force-remote"
	Command      string // "push", "pull", or "sync"
}

func ExecutePlan(client *gowebdav.Client, currentState *state.State, plan []FilePlan, opts ExecutionOptions) error {
	if opts.DryRun {
		fmt.Println("DRY RUN: No files will be changed.")
		return nil
	}

	for _, action := range plan {
		// Filter actions based on command
		if opts.Command == "push" {
			if action.Action == ActionPull || action.Action == ActionLocalDelete {
				return fmt.Errorf("remote contains changes you do not have locally. Please run 'nextdoor sync' or 'nextdoor pull' first")
			}
			if action.Action == ActionConflict {
				if opts.Strategy == "force-local" {
					action.Action = ActionPush
				} else {
					return fmt.Errorf("conflict detected on %s. Please run 'nextdoor sync' to resolve", action.RelPath)
				}
			}
		} else if opts.Command == "pull" {
			if action.Action == ActionPush || action.Action == ActionRemoteDelete {
				continue // Skip pushing local changes in pull command
			}
			if action.Action == ActionConflict {
				if opts.Strategy == "force-remote" {
					action.Action = ActionPull
				} else {
					// Default pull conflict resolution is to pull into a conflicted copy
					err := executePullConflict(client, currentState, action, opts)
					if err != nil {
						return err
					}
					continue
				}
			}
		} else if opts.Command == "sync" {
			if action.Action == ActionConflict {
				if opts.Strategy == "force-local" {
					action.Action = ActionPush
				} else if opts.Strategy == "force-remote" {
					action.Action = ActionPull
				} else {
					// Create conflicted copy
					err := executePullConflict(client, currentState, action, opts)
					if err != nil {
						return err
					}
					continue
				}
			}
		}

		// Execute the finalized action
		var err error
		switch action.Action {
		case ActionPush:
			err = executePush(client, currentState, action, opts)
		case ActionPull:
			err = executePull(client, currentState, action, opts)
		case ActionRemoteDelete:
			if !opts.NoDelete {
				err = executeRemoteDelete(client, currentState, action, opts)
			} else {
				fmt.Printf(" -> [SKIP REMOTE DELETE] %s (--no-delete active)\n", action.RelPath)
			}
		case ActionLocalDelete:
			err = executeLocalDelete(currentState, action, opts)
		}

		if err != nil {
			return fmt.Errorf("error processing %s: %w", action.RelPath, err)
		}
	}

	return nil
}

func executePush(client *gowebdav.Client, currentState *state.State, action FilePlan, opts ExecutionOptions) error {
	fmt.Printf(" -> [PUSHING] %s\n", action.RelPath)
	localPath := filepath.Join(opts.BaseDir, action.RelPath)
	remotePath := path.Join(opts.RemoteTarget, filepath.ToSlash(action.RelPath))

	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Ensure remote directory exists
	dir := path.Dir(remotePath)
	if err := client.MkdirAll(dir, 0755); err != nil {
		// Ignore errors on MkdirAll as it might exist
	}

	if err := nextcloud.AtomicUpload(client, remotePath, f); err != nil {
		return err
	}

	// Update state
	if currentState.Files == nil {
		currentState.Files = make(map[string]state.FileInfo)
	}
	
	// Fetch new remote ETag
	info, err := client.Stat(remotePath)
	var etag string
	if err == nil {
		if wdFile, ok := info.(nextcloud.FileInfo); ok {
			etag = wdFile.ETag()
		}
	}

	currentState.Files[action.RelPath] = state.FileInfo{
		LocalXXHash3: action.LocalInfo.LocalXXHash3,
		Size:         action.LocalInfo.Size,
		ModTime:      action.LocalInfo.ModTime,
		RemoteETag:   etag,
	}

	return state.Save(opts.BaseDir, currentState)
}

func executePull(client *gowebdav.Client, currentState *state.State, action FilePlan, opts ExecutionOptions) error {
	fmt.Printf(" -> [PULLING] %s\n", action.RelPath)
	return performPull(client, currentState, action.RelPath, action.RelPath, action.RemoteInfo.ETag, opts)
}

func executePullConflict(client *gowebdav.Client, currentState *state.State, action FilePlan, opts ExecutionOptions) error {
	conflictName := GenerateConflictFilename(action.RelPath)
	fmt.Printf(" -> [CONFLICT] Pulling remote to %s\n", conflictName)
	return performPull(client, currentState, action.RelPath, conflictName, action.RemoteInfo.ETag, opts)
}

func performPull(client *gowebdav.Client, currentState *state.State, remoteRelPath, localRelPath, remoteETag string, opts ExecutionOptions) error {
	localPath := filepath.Join(opts.BaseDir, localRelPath)
	remotePath := path.Join(opts.RemoteTarget, filepath.ToSlash(remoteRelPath))

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}

	partPath := localPath + ".part"
	f, err := os.Create(partPath)
	if err != nil {
		return err
	}

	reader, err := client.ReadStream(remotePath)
	if err != nil {
		f.Close()
		os.Remove(partPath)
		return err
	}
	
	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		reader.Close()
		os.Remove(partPath)
		return err
	}
	reader.Close()
	f.Close()

	if err := os.Rename(partPath, localPath); err != nil {
		return err
	}

	// Update state
	if currentState.Files == nil {
		currentState.Files = make(map[string]state.FileInfo)
	}

	// Calculate new local hash
	info, err := os.Stat(localPath)
	if err == nil {
		scanner := local.NewScanner(opts.BaseDir, currentState)
		hash, _ := scanner.HashFileFastPath(localPath, localRelPath, info) // note: requires exported func or bypass. Wait, HashFileFastPath is unexported!
		currentState.Files[localRelPath] = state.FileInfo{LocalXXHash3: hash, Size: info.Size(), ModTime: info.ModTime(), RemoteETag: remoteETag}
		return state.Save(opts.BaseDir, currentState)
	}
	
	return nil
}

func executeRemoteDelete(client *gowebdav.Client, currentState *state.State, action FilePlan, opts ExecutionOptions) error {
	fmt.Printf(" -> [REMOTE DELETE] %s\n", action.RelPath)
	remotePath := path.Join(opts.RemoteTarget, filepath.ToSlash(action.RelPath))
	
	err := client.Remove(remotePath)
	if err != nil && !strings.Contains(err.Error(), "404") {
		return err
	}

	delete(currentState.Files, action.RelPath)
	return state.Save(opts.BaseDir, currentState)
}

func executeLocalDelete(currentState *state.State, action FilePlan, opts ExecutionOptions) error {
	fmt.Printf(" -> [LOCAL DELETE] %s\n", action.RelPath)
	localPath := filepath.Join(opts.BaseDir, action.RelPath)
	
	err := os.Remove(localPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	delete(currentState.Files, action.RelPath)
	return state.Save(opts.BaseDir, currentState)
}
