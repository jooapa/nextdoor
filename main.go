package main

import (
	"fmt"
	"os"

	"github.com/alexflint/go-arg"
	"github.com/jooapa/nextdoor/internal/config"
	"github.com/jooapa/nextdoor/internal/nextcloud"
)

// ListRootCmd represents the 'list_root' subcommand
type ListRootCmd struct {
	// Add any specific flags for list_root here in the future
}

// Args defines the command-line interface structure for go-arg
var args struct {
	Config   string       `arg:"--config" default:"config.toml" help:"Path to the configuration file"`
	ListRoot *ListRootCmd `arg:"subcommand:lr" help:"Connects to Nextcloud and lists all files in the root directory"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse the arguments using go-arg
	p := arg.MustParse(&args)

	// Ensure a subcommand was actually provided
	if p.Subcommand() == nil {
		p.WriteHelp(os.Stdout)
		return fmt.Errorf("a command is required")
	}

	// Load configuration using the provided flag path
	cfg, err := config.Load(args.Config)
	if err != nil {
		return err
	}

	// Initialize the WebDAV client
	client := nextcloud.NewClient(cfg)

	// Route the command based on which struct was populated
	switch {
	case args.ListRoot != nil:
		return nextcloud.ListRootFiles(client)
	default:
		return fmt.Errorf("internal error: unhandled subcommand")
	}
}
