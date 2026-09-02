package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/jooapa/nextdoor/internal/config"
	"github.com/jooapa/nextdoor/internal/local"
	"github.com/jooapa/nextdoor/internal/nextcloud"
	"github.com/jooapa/nextdoor/internal/state"
	"github.com/jooapa/nextdoor/internal/sync"
)

// --- Subcommand Structures ---

type InitCmd struct {
	Path    string `arg:"positional" default:"." help:"Path to initialize the sync folder"`
	Remote  string `arg:"--remote" help:"Initialize by cloning an existing remote Nextcloud folder (e.g., /Photos)"`
	Rebuild bool   `arg:"--rebuild" help:"Force a complete rescan and hash recalculation of the local directory"`
}

type LoginCmd struct{}

type StatusCmd struct{}

type PullCmd struct {
	Target  string `arg:"positional" help:"Specific file or directory to pull (leave blank for everything)"`
	BwLimit int    `arg:"--bwlimit" help:"Bandwidth limit in KB/s"`
	MaxSize string `arg:"--max-size" help:"Skip files larger than this size (e.g., '1G', '500M')"`
}

type PushCmd struct {
	Target    string   `arg:"positional" help:"Specific file or directory to push (leave blank for everything)"`
	SetRemote string   `arg:"--set-remote" help:"Set the remote target folder for the very first push"`
	Ignore    []string `arg:"--ignore" help:"Ignore files matching these glob patterns (e.g., '*.tmp')"`
	BwLimit   int      `arg:"--bwlimit" help:"Bandwidth limit in KB/s"`
	MaxSize   string   `arg:"--max-size" help:"Skip files larger than this size (e.g., '1G', '500M')"`
	NoDelete  bool     `arg:"--no-delete" help:"Prevent deleting files on the remote server (append-only)"`
}

type SyncCmd struct {
	Target   string `arg:"positional" help:"Specific file or directory to sync (leave blank for everything)"`
	DryRun   bool   `arg:"--dry-run" help:"Preview what would happen without actually transferring any files"`
	Strategy string `arg:"--strategy" help:"Conflict resolution strategy: 'force-local' or 'force-remote'"`
	BwLimit  int    `arg:"--bwlimit" help:"Bandwidth limit in KB/s"`
	MaxSize  string `arg:"--max-size" help:"Skip files larger than this size (e.g., '1G', '500M')"`
	NoDelete bool   `arg:"--no-delete" help:"Prevent deleting files on the remote server (append-only)"`
}

type ListRootCmd struct{}

// --- Main Arguments Structure ---

var args struct {
	// Global Flags
	Config         string `arg:"--config" help:"Path to the configuration file (defaults to config.toml in the binary's directory)"`
	Verbose        bool   `arg:"--verbose,-v" help:"Enable verbose output for debugging and real-time progress"`
	IncludeHidden  bool   `arg:"--include-hidden" help:"Include hidden OS files (e.g., .DS_Store, .swp) which are ignored by default"`
	FollowSymlinks bool   `arg:"--follow-symlinks" help:"Follow symlinks instead of ignoring them (use with caution)"`

	// Commands
	Init     *InitCmd     `arg:"subcommand:init" help:"Initializes a new sync folder at the specified path"`
	Login    *LoginCmd    `arg:"subcommand:login" help:"Authenticate and configure WebDAV settings"`
	Status   *StatusCmd   `arg:"subcommand:status" help:"Compare local state against remote state"`
	Pull     *PullCmd     `arg:"subcommand:pull" help:"Pull newer files from Nextcloud to local"`
	Push     *PushCmd     `arg:"subcommand:push" help:"Push newer files from local to Nextcloud"`
	Sync     *SyncCmd     `arg:"subcommand:sync" help:"Perform a two-way sync (push and pull)"`
	ListRoot *ListRootCmd `arg:"subcommand:list_root" help:"Connects to Nextcloud and lists all files in the root directory"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	p := arg.MustParse(&args)

	if p.Subcommand() == nil {
		p.WriteHelp(os.Stdout)
		return fmt.Errorf("a command is required")
	}

	if args.Verbose {
		fmt.Println("[DEBUG] Verbose logging enabled.")
	}

	// 1. Handle commands that DO NOT require Nextcloud connection/config yet
	switch {
	case args.Init != nil:
		if args.Init.Remote != "" {
			fmt.Printf("Initializing from remote path: %s\n", args.Init.Remote)
		}
		if args.Init.Rebuild {
			fmt.Println("Rebuild flag detected: will recalculate all hashes.")
		}
		return local.Init(args.Init.Path)

	case args.Login != nil:
		fmt.Println("Login prompt would appear here.")
		return nil
	}

	// 2. Load configuration for commands that require a connection
	configPath := args.Config
	if configPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}
		configPath = filepath.Join(filepath.Dir(exePath), "config.toml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	client := nextcloud.NewClient(cfg)

	// 3. Route the remaining commands
	switch {
	case args.Status != nil:
		fmt.Println("Status check not yet implemented.")
		return nil

	case args.Pull != nil:
		if args.Pull.Target != "" {
			fmt.Printf("Pulling specific target: %s\n", args.Pull.Target)
		} else {
			fmt.Println("Pulling all files...")
		}
		return nil

	case args.Push != nil:
		baseDir := "."

		// 1. Load Local State
		currentState, err := state.Load(baseDir)
		if err != nil {
			return fmt.Errorf("failed to load local state: %w", err)
		}

		// 2. Handle --set-remote validation
		if args.Push.SetRemote != "" {
			fmt.Printf("Verifying remote target %s...\n", args.Push.SetRemote)

			// Constraint: Query WebDAV. If folder exists AND contains files, reject.
			infos, err := client.ReadDir(args.Push.SetRemote)
			if err == nil && len(infos) > 0 {
				return fmt.Errorf("remote target '%s' is not empty. To protect your data, use 'nextdoor init --remote %s' instead to clone it locally first", args.Push.SetRemote, args.Push.SetRemote)
			}

			currentState.RemoteTarget = args.Push.SetRemote
			if err := state.Save(baseDir, currentState); err != nil {
				return fmt.Errorf("failed to save new remote target to state: %w", err)
			}
			fmt.Println("Remote target configured successfully.")
		} else if currentState.RemoteTarget == "" {
			return fmt.Errorf("no remote target configured. Use --set-remote <target> on your first push")
		}

		fmt.Printf("Pushing to Nextcloud target: %s\n", currentState.RemoteTarget)

		// 3. Run Local Discovery
		fmt.Println("Scanning local directory...")
		scanner := local.NewScanner(baseDir, currentState)
		localFiles, err := scanner.Scan()
		if err != nil {
			return fmt.Errorf("local scan failed: %w", err)
		}

		// 4. Run Remote Discovery
		fmt.Println("Fetching remote state...")
		remoteFiles, err := nextcloud.FetchDirectoryTree(client, currentState.RemoteTarget)
		if err != nil {
			if !strings.Contains(err.Error(), "404") {
				return fmt.Errorf("failed to fetch remote tree: %w", err)
			}
			fmt.Printf("[Warning] Remote tree not found, assuming empty: %v\n", err)
			if remoteFiles == nil {
				remoteFiles = make(map[string]nextcloud.RemoteFile)
			}
		}

		// 5. Run Reconciliation Engine
		fmt.Println("Reconciling state...")
		plan := sync.Reconcile(currentState, localFiles, remoteFiles)

		// 6. Preview Plan
		pushCount := 0
		for _, action := range plan {
			if action.Action == sync.ActionPush {
				fmt.Printf(" -> [PUSH] %s\n", action.RelPath)
				pushCount++
			} else if action.Action == sync.ActionConflict {
				fmt.Printf(" -> [CONFLICT] %s\n", action.RelPath)
			}
		}

		if pushCount == 0 {
			fmt.Println("Everything is up-to-date. Nothing to push.")
		}

		return nil

	case args.Sync != nil:
		if args.Sync.DryRun {
			fmt.Println("Running sync in DRY-RUN mode. No files will be changed.")
		}
		if args.Sync.Target != "" {
			fmt.Printf("Syncing specific target: %s\n", args.Sync.Target)
		} else {
			fmt.Println("Syncing all files...")
		}
		return nil

	case args.ListRoot != nil:
		return nextcloud.ListRootFiles(client)

	default:
		return fmt.Errorf("internal error: unhandled subcommand")
	}
}
