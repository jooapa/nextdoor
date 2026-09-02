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
	Config         string `arg:"--config" help:"Path to the configuration file"`
	Verbose        bool   `arg:"--verbose,-v" help:"Enable verbose output"`
	IncludeHidden  bool   `arg:"--include-hidden" help:"Include hidden OS files"`
	FollowSymlinks bool   `arg:"--follow-symlinks" help:"Follow symlinks"`

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
		return local.Init(args.Init.Path)
	case args.Login != nil:
		fmt.Println("Login prompt would appear here.")
		return nil
	}

	configPath := args.Config
	if configPath == "" {
		exePath, _ := os.Executable()
		configPath = filepath.Join(filepath.Dir(exePath), "config.toml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	client := nextcloud.NewClient(cfg)

	switch {
	case args.Status != nil:
		fmt.Println("Status check not yet implemented.")
		return nil
	case args.ListRoot != nil:
		return nextcloud.ListRootFiles(client)
	}

	// For Push, Pull, Sync
	baseDir := "."
	currentState, err := state.Load(baseDir)
	if err != nil {
		return fmt.Errorf("failed to load local state: %w", err)
	}

	var execOpts sync.ExecutionOptions
	execOpts.BaseDir = baseDir

	if args.Push != nil {
		execOpts.Command = "push"
		execOpts.NoDelete = args.Push.NoDelete
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
	} else if args.Sync != nil {
		execOpts.Command = "sync"
		execOpts.DryRun = args.Sync.DryRun
		execOpts.Strategy = args.Sync.Strategy
		execOpts.NoDelete = args.Sync.NoDelete
	}

	if currentState.RemoteTarget == "" {
		return fmt.Errorf("no remote target configured")
	}
	execOpts.RemoteTarget = currentState.RemoteTarget

	fmt.Println("Scanning local directory...")
	scanner := local.NewScanner(baseDir, currentState)
	localFiles, err := scanner.Scan()
	if err != nil {
		return err
	}

	fmt.Println("Fetching remote state...")
	remoteFiles, err := nextcloud.FetchDirectoryTree(client, currentState.RemoteTarget)
	if err != nil {
		if !strings.Contains(err.Error(), "404") {
			return err
		}
		fmt.Printf("[Warning] Remote tree not found, assuming empty: %v\n", err)
		if remoteFiles == nil {
			remoteFiles = make(map[string]nextcloud.RemoteFile)
		}
	}

	fmt.Println("Reconciling state...")
	plan := sync.Reconcile(currentState, localFiles, remoteFiles)

	fmt.Println("Executing plan...")
	err = sync.ExecutePlan(client, currentState, plan, execOpts)
	if err != nil {
		return err
	}
	
	fmt.Println("Sync completed successfully.")
	return nil
}
