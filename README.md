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

## High Performance: Smart Rsync Mode

By default, `nextdoor` uses the official Nextcloud WebDAV API to transfer files. While WebDAV is smart, it is restricted by PHP and database overhead, making massive data transfers or large directory syncs incredibly slow.

To bypass this bottleneck, `nextdoor` features a **Smart Rsync Mode**. When enabled, the sync engine will:
1. Use WebDAV to quickly scan the remote server and calculate a 3-way merge conflict map.
2. Generate a highly specific list of modified files.
3. Use a native `rsync` SSH connection to transfer *only* those specific files at raw network speeds.
4. Execute `occ files:scan` on the remote server via SSH to notify Nextcloud's database of the new files.
5. Re-fetch the new ETags via WebDAV and lock them into the local `.nextdoor` state database.

### How to Enable Rsync Mode
Rsync mode is configured via your `config.toml`.

```toml
[rsync]
enabled = true
smart_mode = true
host = "nextcloud.example.com"
port = 22
user = "your_ssh_username"
key_path = "~/.ssh/id_rsa"
data_dir = "/var/www/nextcloud/data/your_username/files"
remote_rsync_path = "sudo rsync"
occ_cmd = "sudo /usr/bin/chown -R 33:33 /var/www/nextcloud/data/your_username/files && sudo /usr/bin/docker exec -u www-data nextcloud-app php occ files:scan your_username"
```

*You can also trigger Rsync mode temporarily on any command by appending the `--rsync` flag (e.g., `nextdoor push --rsync`).*

### Handling Nextcloud Permissions & Passwords
Nextcloud strictly enforces its file permissions (usually `www-data` or UID `33`). If you connect via SSH as a standard user, you will get "Permission Denied" errors when trying to read or write files directly to the Nextcloud data directory.

To bypass this seamlessly:
1. **Force Rsync to run as Root:** Add `remote_rsync_path = "sudo rsync"` to your config.
2. **Setup SSH Keys:** Run `ssh-keygen -t ed25519` and `ssh-copy-id user@host` on your local machine so `nextdoor` can SSH into the server without a password prompt.
3. **Setup Passwordless Sudo:** On your remote server, run `sudo visudo` and add the following line to the bottom of the file so `nextdoor` can run `rsync` and `chown` without a password prompt:
   ```text
   your_username ALL=(ALL) NOPASSWD: /usr/bin/rsync, /usr/bin/chown, /usr/bin/docker
   ```

### Smart Mode vs. Dumb Mode
* `smart_mode = true`: Safely protects against conflicts. Selectively transfers only modified files using `rsync --files-from`. Deletions are routed safely through the WebDAV engine so they end up in the Nextcloud Trashbin.
* `smart_mode = false` ("Dumb Mirror"): Bypasses the 3-way merge brain entirely. Executes a raw `rsync --delete` which blindly forces the remote directory to look exactly like the local directory, permanently wiping remote changes or conflicts.
* `--concurrency <N>` / `-c`: Overrides the auto-scaler and forces exactly N concurrent network workers (e.g., `-c 25`).
* `--rsync`: Forces direct SSH/Rsync transfer mode (requires `config.toml` setup).
