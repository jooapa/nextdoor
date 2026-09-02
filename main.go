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
	"github.com/jooapa/nextdoor/internal/rsync"
	"github.com/jooapa/nextdoor/internal/state"
	"github.com/jooapa/nextdoor/internal/sync"
	"github.com/jooapa/nextdoor/internal/utils"
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
	Config         string `arg:"--config" help:"Path to the configuration file"`
	Verbose        bool   `arg:"--verbose,-v" help:"Enable verbose output"`
	IncludeHidden  bool   `arg:"--include-hidden" help:"Include hidden OS files"`
	FollowSymlinks bool   `arg:"--follow-symlinks" help:"Follow symlinks"`
	Concurrency    int    `arg:"--concurrency,-c" default:"0" help:"Number of concurrent transfers (0 = auto)"`
	Rsync          bool   `arg:"--rsync" help:"Use direct SSH/Rsync instead of WebDAV (requires config.toml setup)"`

	Init     *InitCmd     `arg:"subcommand:init"`
	Login    *LoginCmd    `arg:"subcommand:login"`
	Status   *StatusCmd   `arg:"subcommand:status"`
	Pull     *PullCmd     `arg:"subcommand:pull"`
	Push     *PushCmd     `arg:"subcommand:push"`
	Sync     *SyncCmd     `arg:"subcommand:sync"`
	ListRoot *ListRootCmd `arg:"subcommand:list_root"`
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

	switch {
	case args.Init != nil:
		return local.Init(args.Init.Path, args.Init.Remote, args.Init.Rebuild)
	case args.Login != nil:
		fmt.Println("Login prompt would appear here.")
		return nil
	}

	configPath := args.Config
	if configPath == "" {
		localConfig := filepath.Join(".nextdoor", "config.toml")
		if _, err := os.Stat(localConfig); err == nil {
			configPath = localConfig
		} else {
			exePath, _ := os.Executable()
			configPath = filepath.Join(filepath.Dir(exePath), "config.toml")
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	client := nextcloud.NewClient(cfg)

	// For Push, Pull, Sync, Status
	baseDir := "."
	currentState, err := state.Load(baseDir)
	if err != nil {
		return fmt.Errorf("failed to load local state: %w", err)
	}

	var execOpts sync.ExecutionOptions
	execOpts.BaseDir = baseDir
	
	var scanOpts local.ScannerOptions
	scanOpts.IncludeHidden = args.IncludeHidden
	scanOpts.FollowSymlinks = args.FollowSymlinks

	if args.Status != nil {
		execOpts.Command = "status"
		execOpts.DryRun = true
	} else if args.Push != nil {
		execOpts.Command = "push"
		execOpts.NoDelete = args.Push.NoDelete
		execOpts.Target = args.Push.Target
		scanOpts.Target = args.Push.Target
		scanOpts.Ignores = args.Push.Ignore
		var err error
		scanOpts.MaxSize, err = utils.ParseSize(args.Push.MaxSize)
		if err != nil {
			return err
		}
		if args.Push.SetRemote != "" {
			infos, err := client.ReadDir(args.Push.SetRemote)
			if err == nil && len(infos) > 0 {
				return fmt.Errorf("remote target '%s' is not empty", args.Push.SetRemote)
			}
			currentState.RemoteTarget = args.Push.SetRemote
			if err := state.Save(baseDir, currentState); err != nil {
				return err
			}
		}
	} else if args.Pull != nil {
		execOpts.Command = "pull"
		execOpts.Target = args.Pull.Target
		scanOpts.Target = args.Pull.Target
		var err error
		scanOpts.MaxSize, err = utils.ParseSize(args.Pull.MaxSize)
		if err != nil {
			return err
		}
	} else if args.Sync != nil {
		execOpts.Command = "sync"
		execOpts.DryRun = args.Sync.DryRun
		execOpts.Strategy = args.Sync.Strategy
		execOpts.NoDelete = args.Sync.NoDelete
		execOpts.Target = args.Sync.Target
		scanOpts.Target = args.Sync.Target
		var err error
		scanOpts.MaxSize, err = utils.ParseSize(args.Sync.MaxSize)
		if err != nil {
			return err
		}
	} else if args.ListRoot != nil {
		return nextcloud.ListRootFiles(client)
	}

	if currentState.RemoteTarget == "" {
		return fmt.Errorf("no remote target configured")
	}
	execOpts.RemoteTarget = currentState.RemoteTarget

	if (cfg.Rsync.Enabled || args.Rsync) && !cfg.Rsync.SmartMode {
		if execOpts.Command == "status" {
			return fmt.Errorf("status command is not supported with dumb rsync")
		}
		if execOpts.Command == "sync" {
			return fmt.Errorf("two-way sync is not natively supported by dumb rsync. Please use 'nextdoor push' or 'nextdoor pull'")
		}
		
		fmt.Println("Bypassing WebDAV and using direct SSH/Rsync transfer (Dumb Mode)...")
		return rsync.Run(&cfg.Rsync, currentState, execOpts.Command, execOpts.Target, execOpts.NoDelete, scanOpts.Ignores)
	}

	if args.Verbose {
		fmt.Println("Scanning local directory...")
	}
	scanner := local.NewScanner(baseDir, currentState, scanOpts)
	localFiles, err := scanner.Scan()
	if err != nil {
		return err
	}

	if args.Verbose {
		fmt.Println("Fetching remote state...")
	}
	
	// ETag Short-Circuit Optimization
	var currentRootETag string
	rootInfo, err := client.Stat(currentState.RemoteTarget)
	if err == nil {
		if wdFile, ok := rootInfo.(nextcloud.FileInfo); ok {
			currentRootETag = wdFile.ETag()
		}
	}

	var remoteFiles map[string]nextcloud.RemoteFile
	
	if currentRootETag != "" && currentState.RemoteRootETag == currentRootETag && args.Status == nil && execOpts.Target == "" {
		if args.Verbose {
			fmt.Println("Remote root ETag unchanged. Skipping remote discovery phase.")
		}
		// Reconstruct remoteFiles from current state
		remoteFiles = make(map[string]nextcloud.RemoteFile)
		for p, f := range currentState.Files {
			if f.RemoteETag != "" {
				remoteFiles[p] = nextcloud.RemoteFile{
					Path: p,
					ETag: f.RemoteETag,
					Size: f.Size,
				}
			}
		}
	} else {
		remoteFiles, err = nextcloud.FetchDirectoryTree(client, currentState.RemoteTarget)
		if err != nil {
			if !strings.Contains(err.Error(), "404") {
				return err
			}
			fmt.Printf("[Warning] Remote tree not found, assuming empty: %v\n", err)
			if remoteFiles == nil {
				remoteFiles = make(map[string]nextcloud.RemoteFile)
			}
		}
		
		// Update the root ETag so we can save it later
		currentState.RemoteRootETag = currentRootETag
	}

	fmt.Println("Reconciling state...")
	plan := sync.Reconcile(currentState, localFiles, remoteFiles)

	fmt.Println("Executing plan...")
	err = sync.ExecutePlan(client, currentState, plan, execOpts, &cfg.Rsync)
	if err != nil {
		return err
	}
	
	if err := state.Save(baseDir, currentState); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	fmt.Println("Sync completed successfully.")
	return nil
}
