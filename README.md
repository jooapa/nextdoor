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

* `--config <path>`: Specifies a custom `config.toml` file to use instead of the default.
* `--verbose` / `-v`: Prints detailed, real-time logging of what the engine is doing.
* `--include-hidden`: Include OS hidden files (e.g., `.DS_Store`, `desktop.ini`, `.swp`) which are skipped by default.
* `--follow-symlinks`: Tells the engine to follow and sync the targets of symlinks instead of ignoring them.

---

## Advanced & "Exotic" Scenarios

### 1. The First Push (Git-style)
If you initialized a local folder without specifying a remote target, you must define where the files should go on your very first push. The target folder on Nextcloud **must be empty or non-existent**.
```sh
nextdoor push --set-remote /New_Project_Folder
```
*If the folder has existing files, the CLI will reject the push to protect your data.*

### 2. Initialize from an existing Remote Folder (The "Clone" Scenario)
If you already have a folder full of data on Nextcloud and want to pull it into an empty local folder, use `--remote`:
```sh
nextdoor init --remote /Photos/Summer ./local_summer
```

### 3. The "Dry Run" (Safe Mode)
You have 10,000 files and want to see what the sync *would* do without actually risking modifying or deleting anything.
```sh
nextdoor sync --dry-run
```

### 4. Conflict Resolution (Force Overrides)
You modified a document locally, but someone else modified the same document on Nextcloud. `nextdoor sync` will block to prevent data loss. You must tell it who wins:
```sh
nextdoor sync --strategy=force-local   # My local changes overwrite the server
nextdoor sync --strategy=force-remote  # The server overwrites my local changes
```

### 5. Selective Syncing & Single-File Targeting
You only want to upload one specific folder right now without waiting for the whole repository to scan:
```sh
nextdoor push Documents/Taxes/
```

### 6. Protection from Disasters
You accidentally deleted your local folder and are terrified that running `sync` will delete the files on the server too. Use the `no-delete` flag for a purely additive push:
```sh
nextdoor push --no-delete
```