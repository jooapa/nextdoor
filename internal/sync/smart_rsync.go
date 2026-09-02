package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jooapa/nextdoor/internal/config"
	"github.com/jooapa/nextdoor/internal/local"
	"github.com/jooapa/nextdoor/internal/nextcloud"
	"github.com/jooapa/nextdoor/internal/state"
	"github.com/studio-b12/gowebdav"
)

func executeSmartRsync(client *gowebdav.Client, currentState *state.State, finalPlan []FilePlan, opts ExecutionOptions, cfg *config.RsyncConfig) error {
	var pushPaths []string
	var pullPaths []string
	var otherPlan []FilePlan

	for _, action := range finalPlan {
		if action.Action == ActionPush {
			pushPaths = append(pushPaths, action.RelPath)
		} else if action.Action == ActionPull || action.Action == ActionConflict {
			if action.Action == ActionConflict {
				// We don't support resolving conflicts easily with rsync --files-from unless we map names.
				// Fallback to WebDAV for conflicts, or just use WebDAV for everything else.
				otherPlan = append(otherPlan, action)
			} else {
				pullPaths = append(pullPaths, action.RelPath)
			}
		} else {
			otherPlan = append(otherPlan, action)
		}
	}

	remoteBase := strings.TrimRight(cfg.DataDir, "/") + "/" + strings.TrimLeft(currentState.RemoteTarget, "/")
	remoteDest := fmt.Sprintf("%s@%s:%s/", cfg.User, cfg.Host, remoteBase)

	sshCmd := "ssh"
	if cfg.Port != 0 && cfg.Port != 22 {
		sshCmd += fmt.Sprintf(" -p %d", cfg.Port)
	}
	if cfg.KeyPath != "" {
		sshCmd += fmt.Sprintf(" -i %s", cfg.KeyPath)
	}

	rsyncBin := "rsync"
	if cfg.RsyncPath != "" {
		rsyncBin = cfg.RsyncPath
	}

	if len(pushPaths) > 0 {
		fmt.Printf("Smart Rsync: Pushing %d files...\n", len(pushPaths))
		if err := runRsyncFilesFrom(pushPaths, rsyncBin, cfg, sshCmd, "./", remoteDest, true); err != nil {
			return err
		}

		if cfg.OccCmd != "" {
			fmt.Println("Running Nextcloud occ files:scan via SSH...")
			occArgs := strings.Fields(cfg.OccCmd)
			sshArgs := []string{"-t", cfg.User + "@" + cfg.Host}
			if cfg.Port != 0 && cfg.Port != 22 {
				sshArgs = append([]string{"-t", "-p", fmt.Sprintf("%d", cfg.Port)}, sshArgs[1:]...)
			}
			if cfg.KeyPath != "" {
				sshArgs = append([]string{"-t", "-i", cfg.KeyPath}, sshArgs[1:]...)
			}
			sshArgs = append(sshArgs, occArgs...)
			occExec := exec.Command("ssh", sshArgs...)
			occExec.Stdout = os.Stdout
			occExec.Stderr = os.Stderr
			occExec.Stdin = os.Stdin
			if err := occExec.Run(); err != nil {
				fmt.Printf("Warning: occ command failed: %v\n", err)
			}
		}

		fmt.Println("Smart Rsync: Updating remote ETags...")
		remoteFiles, err := nextcloud.FetchDirectoryTree(client, currentState.RemoteTarget)
		if err == nil {
			for _, p := range pushPaths {
				if rf, ok := remoteFiles[p]; ok {
					if localF, exists := currentState.Files[p]; exists {
						localF.RemoteETag = rf.ETag
						currentState.Files[p] = localF
					} else {
						// We need local hash. We can just wait for next scan, or do it now.
						// The local scan was already done before Reconcile.
						// We should have it in localInfo, but we don't have localFiles map here.
						// We will just let the next run handle it, or better, we pass it.
						// Actually, ExecutePlan already received currentState which has it if it was unmodified,
						// but for NEW pushes, it's missing.
						info, err := os.Stat(filepath.Join(opts.BaseDir, p))
						if err == nil {
							scanner := local.NewScanner(opts.BaseDir, currentState, local.ScannerOptions{})
							hash, _ := scanner.HashFileFastPath(filepath.Join(opts.BaseDir, p), p, info)
							currentState.Files[p] = state.FileInfo{LocalXXHash3: hash, Size: info.Size(), ModTime: info.ModTime(), RemoteETag: rf.ETag}
						}
					}
				}
			}
			state.Save(opts.BaseDir, currentState)
		}
	}

	if len(pullPaths) > 0 {
		fmt.Printf("Smart Rsync: Pulling %d files...\n", len(pullPaths))
		if err := runRsyncFilesFrom(pullPaths, rsyncBin, cfg, sshCmd, remoteDest, "./", false); err != nil {
			return err
		}

		fmt.Println("Smart Rsync: Updating local state...")
		remoteFiles, err := nextcloud.FetchDirectoryTree(client, currentState.RemoteTarget)
		if err == nil {
			for _, p := range pullPaths {
				if rf, ok := remoteFiles[p]; ok {
					info, err := os.Stat(filepath.Join(opts.BaseDir, p))
					if err == nil {
						scanner := local.NewScanner(opts.BaseDir, currentState, local.ScannerOptions{})
						hash, _ := scanner.HashFileFastPath(filepath.Join(opts.BaseDir, p), p, info)
						currentState.Files[p] = state.FileInfo{LocalXXHash3: hash, Size: info.Size(), ModTime: info.ModTime(), RemoteETag: rf.ETag}
					}
				}
			}
			state.Save(opts.BaseDir, currentState)
		}
	}

	if len(otherPlan) > 0 {
		fmt.Println("Smart Rsync: Handing over conflicts and deletions to WebDAV engine...")
		// We call the normal execution loop for deletes and conflicts
		// But we need to fake an ExecutePlan call or just run the loop.
		// I will just copy the normal loop for otherPlan.
		return ExecutePlanNormal(client, currentState, otherPlan, opts)
	}

	return nil
}

func runRsyncFilesFrom(files []string, rsyncBin string, cfg *config.RsyncConfig, sshCmd, src, dest string, isPush bool) error {
	f, err := os.CreateTemp("", "nextdoor-rsync-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	for _, p := range files {
		f.WriteString(p + "\n")
	}
	f.Close()

	args := []string{"-avz", "--progress"}
	if cfg.RemoteRsyncPath != "" {
		args = append(args, "--rsync-path="+cfg.RemoteRsyncPath)
	}
	if len(cfg.RsyncArgs) > 0 {
		args = append(args, cfg.RsyncArgs...)
	}
	args = append(args, "-e", sshCmd, "--files-from="+f.Name(), src, dest)

	fmt.Printf("Executing: %s %s\n", rsyncBin, strings.Join(args, " "))
	cmd := exec.Command(rsyncBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
