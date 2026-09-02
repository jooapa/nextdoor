package rsync

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jooapa/nextdoor/internal/config"
	"github.com/jooapa/nextdoor/internal/state"
)

func Run(cfg *config.RsyncConfig, currentState *state.State, command string, target string, noDelete bool, ignores []string) error {
	if cfg.Host == "" || cfg.User == "" || cfg.DataDir == "" {
		return fmt.Errorf("rsync configuration is missing required fields (host, user, data_dir)")
	}

	if currentState.RemoteTarget == "" {
		return fmt.Errorf("no remote target configured in state")
	}

	rsyncBin := "rsync"
	if cfg.RsyncPath != "" {
		rsyncBin = cfg.RsyncPath
	}

	// Build the SSH command
	sshCmd := "ssh"
	if cfg.Port != 0 && cfg.Port != 22 {
		sshCmd += fmt.Sprintf(" -p %d", cfg.Port)
	}
	if cfg.KeyPath != "" {
		sshCmd += fmt.Sprintf(" -i %s", cfg.KeyPath)
	}

	args := []string{"-avz", "--progress"}
	if cfg.RemoteRsyncPath != "" {
		args = append(args, "--rsync-path="+cfg.RemoteRsyncPath)
	}
	
	if !noDelete {
		args = append(args, "--delete")
	}

	// Always protect the local state directory from being pushed or deleted
	args = append(args, fmt.Sprintf("--exclude=%s", state.StateDir))
	for _, ign := range ignores {
		args = append(args, fmt.Sprintf("--exclude=%s", ign))
	}

	if len(cfg.RsyncArgs) > 0 {
		args = append(args, cfg.RsyncArgs...)
	}

	args = append(args, "-e", sshCmd)

	localPath := "."
	if target != "" {
		localPath = target
	}

	localSrc := localPath
	if localPath == "." {
		localSrc = "./"
	} else {
		info, err := os.Stat(localPath)
		if err == nil && info.IsDir() {
			if !strings.HasSuffix(localSrc, "/") {
				localSrc += "/"
			}
		}
	}

	remoteBase := strings.TrimRight(cfg.DataDir, "/") + "/" + strings.TrimLeft(currentState.RemoteTarget, "/")
	remoteDest := fmt.Sprintf("%s@%s:%s", cfg.User, cfg.Host, remoteBase)

	if target != "" {
		remoteDest = fmt.Sprintf("%s@%s:%s", cfg.User, cfg.Host, strings.TrimRight(remoteBase, "/")+"/"+target)
		// If target is a directory, ensure remoteDest ends with /
		info, err := os.Stat(localPath)
		if err == nil && info.IsDir() {
			if !strings.HasSuffix(remoteDest, "/") {
				remoteDest += "/"
			}
		}
	} else {
		if !strings.HasSuffix(remoteDest, "/") {
			remoteDest += "/"
		}
	}

	switch command {
	case "push":
		args = append(args, localSrc, remoteDest)
	case "pull":
		args = append(args, remoteDest, localSrc)
	case "sync":
		return fmt.Errorf("rsync does not natively support two-way sync. Please use 'nextdoor push' or 'nextdoor pull'")
	default:
		return fmt.Errorf("unsupported command for rsync: %s", command)
	}

	fmt.Printf("Executing: %s %s\n", rsyncBin, strings.Join(args, " "))
	cmd := exec.Command(rsyncBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync failed: %w", err)
	}

	// Run occ files:scan if configured
	if command == "push" && cfg.OccCmd != "" {
		fmt.Println("Running Nextcloud occ files:scan via SSH...")
		occArgs := strings.Fields(cfg.OccCmd)

		// Add -t to force pseudo-terminal allocation so sudo can prompt for passwords
		sshArgs := []string{"-t", cfg.User + "@" + cfg.Host}
		if cfg.Port != 0 && cfg.Port != 22 {
			sshArgs = append([]string{"-t", "-p", fmt.Sprintf("%d", cfg.Port)}, sshArgs[1:]...)
		}
		if cfg.KeyPath != "" {
			sshArgs = append([]string{"-t", "-i", cfg.KeyPath}, sshArgs[1:]...)
		}

		sshArgs = append(sshArgs, occArgs...)

		fmt.Printf("Executing: ssh %s\n", strings.Join(sshArgs, " "))
		occExec := exec.Command("ssh", sshArgs...)
		occExec.Stdout = os.Stdout
		occExec.Stderr = os.Stderr
		occExec.Stdin = os.Stdin
		if err := occExec.Run(); err != nil {
			fmt.Printf("Warning: occ command failed: %v\n", err)
		}
	}

	return nil
}
