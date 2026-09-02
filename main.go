package main

import (
	"fmt"
	"os"

	"github.com/alexflint/go-arg"
	"github.com/jooapa/nextdoor/internal/config"
	"github.com/jooapa/nextdoor/internal/local"
	"github.com/jooapa/nextdoor/internal/nextcloud"
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
	Target   string   `arg:"positional" help:"Specific file or directory to push (leave blank for everything)"`
	SetRemote string  `arg:"--set-remote" help:"Set the remote target folder for the very first push"`
	Ignore   []string `arg:"--ignore" help:"Ignore files matching these glob patterns (e.g., '*.tmp')"`
	BwLimit  int      `arg:"--bwlimit" help:"Bandwidth limit in KB/s"`
	MaxSize  string   `arg:"--max-size" help:"Skip files larger than this size (e.g., '1G', '500M')"`
	NoDelete bool     `arg:"--no-delete" help:"Prevent deleting files on the remote server (append-only)"`
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
	Config         string `arg:"--config" default:"config.toml" help:"Path to the configuration file"`
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
	cfg, err := config.Load(args.Config)
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
		if args.Push.Target != "" {
			fmt.Printf("Pushing specific target: %s\n", args.Push.Target)
		} else {
			fmt.Println("Pushing all files...")
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
