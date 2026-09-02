# Architecture

There is a `directory.json` file in the initialized `.nextdoor/` folder that contains the state of all the files in the folder. This state is compared with the one that is on the Nextcloud server, and the files are synced accordingly.

To ensure efficient lookups and robust state tracking during syncing, the schema utilizes a map (object) where relative file paths act as the keys. It also explicitly tracks the file hash to avoid relying solely on potentially fragile file modification times.

### `directory.json` Schema

```json
{
  "schemaVersion": 1,
  "lastSyncTime": "2024-06-01T12:00:00Z",
  "files": {
    "file1.txt": {
      "size": 1234,
      "modified": "2024-06-01T12:34:56Z",
      "sha256": "abc123def456"
    },
    "documents/notes.txt": {
      "size": 5678,
      "modified": "2024-06-02T12:34:56Z",
      "sha256": "def456abc123"
    }
  }
}
```