# *N*ext*D*oor

A fast, Git-like CLI tool for syncing local folders to a Nextcloud server.

## Command Reference

```sh
nextdoor init [path]                    # Initializes a new sync folder (defaults to current dir)
  --remote <path>                       # Initializes by cloning an existing remote Nextcloud folder
  --rebuild                             # Forces a complete rescan and hash recalculation

nextdoor login                          # Authenticates and configures WebDAV settings

nextdoor status                         # Compares local state against remote state without making changes

nextdoor pull [target]                  # Pulls newer files from Nextcloud to local
  --bwlimit <KB/s>                      # Throttle download speed
  --max-size <size>                     # Skip downloading files larger than size (e.g., '1G')

nextdoor push [target]                  # Pushes newer files from local to Nextcloud
  --set-remote <path>                   # Set the remote target folder for the very first push
  --ignore <pattern>                    # Ignores files matching glob patterns (e.g., '*.tmp')
  --bwlimit <KB/s>                      # Throttle upload speed
  --max-size <size>                     # Skip uploading files larger than size
  --no-delete                           # Prevent deleting files on the remote server (append-only)

nextdoor sync [target]                  # Performs a two-way sync (push and pull)
  --dry-run                             # Previews what would happen without transferring files
  --strategy <force-local|force-remote> # Conflict resolution strategy
  --bwlimit <KB/s>                      # Throttle transfer speed
  --max-size <size>                     # Skip transferring files larger than size
  --no-delete                           # Prevent deleting files on the remote server
```

### Global Flags
These flags can be appended to any command above to alter the global behavior of the sync engine:

* `--config <path>`: Specifies a custom `config.toml` file to use instead of the default. If omitted, checks for `.nextdoor/config.toml` before falling back to the executable directory.
* `--verbose` / `-v`: Prints detailed, real-time logging of what the engine is doing.
* `--include-hidden`: Include OS hidden files (e.g., `.DS_Store`, `desktop.ini`, `.swp`) which are skipped by default.
