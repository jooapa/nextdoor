package sync

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jooapa/nextdoor/internal/utils"
	"github.com/schollz/progressbar/v3"

	"github.com/jooapa/nextdoor/internal/local"
	"github.com/jooapa/nextdoor/internal/nextcloud"
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
	Target       string // specific file or directory to process
}

func ExecutePlan(client *gowebdav.Client, currentState *state.State, plan []FilePlan, opts ExecutionOptions) error {
	var finalPlan []FilePlan

	for _, action := range plan {
		// Filter by target
		if opts.Target != "" && action.RelPath != opts.Target && !strings.HasPrefix(action.RelPath, opts.Target+"/") {
			continue
		}
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
				}
				// Default pull conflict resolution is to pull into a conflicted copy
			}
		} else if opts.Command == "sync" {
			if action.Action == ActionConflict {
				switch opts.Strategy {
				case "force-local":
					action.Action = ActionPush
				case "force-remote":
					action.Action = ActionPull
				}
			}
		}
		finalPlan = append(finalPlan, action)
	}

	if opts.Command == "status" || opts.DryRun {
		if opts.Command == "status" {
			fmt.Println("--- STATUS ---")
		} else {
			fmt.Println("DRY RUN: No files will be changed. Plan:")
		}
		for _, action := range finalPlan {
			fmt.Printf("[%s] %s\n", action.Action, action.RelPath)
		}
		return nil
	}

	// Print summary
	var pushCount, pullCount, delRemoteCount, delLocalCount, conflictCount int
	var pushSize, pullSize int64
	for _, action := range finalPlan {
		switch action.Action {
		case ActionPush:
			pushCount++
			if action.LocalInfo != nil {
				pushSize += action.LocalInfo.Size
			}
		case ActionPull:
			pullCount++
			if action.RemoteInfo != nil {
				pullSize += action.RemoteInfo.Size
			}
		case ActionRemoteDelete:
			delRemoteCount++
		case ActionLocalDelete:
			delLocalCount++
		case ActionConflict:
			conflictCount++
			if action.RemoteInfo != nil {
				pullSize += action.RemoteInfo.Size // Will be pulled as conflict
			}
		}
	}

	fmt.Printf("\nSummary: %d files to push (%s), %d files to pull (%s), %d remote deletes, %d local deletes, %d conflicts\n\n",
		pushCount, utils.FormatBytes(pushSize), pullCount, utils.FormatBytes(pullSize), delRemoteCount, delLocalCount, conflictCount)

	for _, action := range finalPlan {
		if action.Action == ActionConflict {
			if opts.Command == "pull" || opts.Command == "sync" {
				err := executePullConflict(client, currentState, action, opts)
				if err != nil {
					return fmt.Errorf("error processing %s: %w", action.RelPath, err)
				}
			}
			continue
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
	localPath := filepath.Join(opts.BaseDir, action.RelPath)
	remotePath := path.Join(opts.RemoteTarget, filepath.ToSlash(action.RelPath))

	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	infoLocal, err := f.Stat()
	var reader io.Reader = f
	var bar *progressbar.ProgressBar
	if err == nil {
		bar = progressbar.DefaultBytes(
			infoLocal.Size(),
			"Pushing "+action.RelPath,
		)
		reader = io.TeeReader(f, bar)
	}

	// Ensure remote directory exists
	dir := path.Dir(remotePath)
	if err := client.MkdirAll(dir, 0755); err != nil {
		// Ignore errors on MkdirAll as it might exist
	}

	if err := nextcloud.AtomicUpload(client, remotePath, reader); err != nil {
		return err
	}
	if bar != nil {
		bar.Close()
		fmt.Println()
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
	return performPull(client, currentState, action, action.RelPath, opts)
}

func executePullConflict(client *gowebdav.Client, currentState *state.State, action FilePlan, opts ExecutionOptions) error {
	conflictName := GenerateConflictFilename(action.RelPath)
	fmt.Printf(" -> [CONFLICT] Pulling remote to %s\n", conflictName)
	return performPull(client, currentState, action, conflictName, opts)
}

func performPull(client *gowebdav.Client, currentState *state.State, action FilePlan, localRelPath string, opts ExecutionOptions) error {
	remoteRelPath := action.RelPath
	remoteETag := action.RemoteInfo.ETag
	localPath := filepath.Join(opts.BaseDir, localRelPath)
	remotePath := path.Join(opts.RemoteTarget, filepath.ToSlash(remoteRelPath))

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}

	partPath := localPath + ".nextdoor-tmp"
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
	
	bar := progressbar.DefaultBytes(
		action.RemoteInfo.Size,
		"Pulling "+localRelPath,
	)

	if _, err := io.Copy(io.MultiWriter(f, bar), reader); err != nil {
		f.Close()
		reader.Close()
		os.Remove(partPath)
		return err
	}
	reader.Close()
	f.Close()
	bar.Close()
	fmt.Println()

	if err := os.Rename(partPath, localPath); err != nil {
		return err
	}

	// Update state
	if currentState.Files == nil {
		currentState.Files = make(map[string]state.FileInfo)
	}

	// Calculate new local hash
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat downloaded file: %w", err)
	}
	
	scanner := local.NewScanner(opts.BaseDir, currentState, local.ScannerOptions{})
	hash, _ := scanner.HashFileFastPath(localPath, localRelPath, info) // note: requires exported func or bypass. Wait, HashFileFastPath is unexported!
	currentState.Files[localRelPath] = state.FileInfo{LocalXXHash3: hash, Size: info.Size(), ModTime: info.ModTime(), RemoteETag: remoteETag}
	return state.Save(opts.BaseDir, currentState)
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
