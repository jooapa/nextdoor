# Nextdoor Sync Architecture

This document defines the airtight, conflict-resistant architecture for the Nextdoor two-way sync engine. 

## 1. Core Concept: The Common Ancestor
To safely perform two-way synchronization, the engine must distinguish between a *newly created* file and a *deleted* file. It does this by maintaining a local state file at `.nextdoor/directory.json`. 

This file acts as the **Common Ancestor** (the "last known synchronized state"). Every sync run compares the *current* local disk and the *current* remote Nextcloud server against this base state.

---

## 2. The Hybrid Tracking System
To achieve maximum performance without downloading files unnecessarily, the engine uses two different tracking mechanisms:

1. **Local Tracking (`xxhash3` + Fast Path):** 
   Scanning the local disk computes an ultra-fast `xxhash3` hash to verify local file integrity. To prevent I/O bottlenecks, the engine uses a "Fast Path": if the OS `modtime` and `size` exactly match the state file, the file is assumed unchanged, and hashing is skipped entirely.
2. **Remote Tracking (`ETag`):** 
   Nextcloud natively calculates an `ETag` (a unique string) for every file. A single WebDAV `PROPFIND` request fetches all remote ETags instantly. If the remote ETag changes, the remote file has changed.

---

## 3. The Sync Workflow (Reconciliation)

When `nextdoor sync` is executed, it runs in three distinct phases:

### Phase 1: Local Discovery
Iterate through the local filesystem.
* Compare `modtime` and `size` to `directory.json`.
* If different, calculate the `xxhash3` hash.
* **Result:** Generate a list of Local Changes (Added, Modified, Deleted).

### Phase 2: Remote Discovery
Send a WebDAV `PROPFIND` request to Nextcloud to get the directory tree.
* Compare the fetched `ETag` for each file against `directory.json`.
* **Result:** Generate a list of Remote Changes (Added, Modified, Deleted).

### Phase 3: Reconciliation (The Logic Matrix)
The engine cross-references the changes and executes actions based on these rules:

| Local State | Remote State | Action Taken |
| :--- | :--- | :--- |
| Modified / Added | Unchanged | **PUSH:** Upload to remote. Update state. |
| Unchanged | Modified / Added | **PULL:** Download from remote. Update state. |
| Deleted | Unchanged | **REMOTE DELETE:** Remove from Nextcloud. |
| Unchanged | Deleted | **LOCAL DELETE:** Remove from local disk. |
| Modified | Modified | **CONFLICT:** Do not overwrite. Download remote as `filename (conflicted copy).ext`. |

---

## 4. "Airtight" Safety Guarantees

To prevent data corruption, network issues, and power failures from destroying data, the engine enforces these safety rules:

* **Atomic Transfers (`.part` files):** Files are never uploaded or downloaded directly to their final destination. They are transferred as `.filename.part`. Only when the transfer reaches 100% does the engine send an atomic `MOVE` (rename) command. This prevents corrupted, half-transferred files from generating new ETags.
* **Atomic State Saves:** If power is lost while writing `directory.json`, the engine gets "amnesia". To prevent this, the engine writes the new state to `directory.json.tmp`. Once writing is fully complete, it uses Go's `os.Rename()` to instantly overwrite the old state file.
* **Directory ETag Short-Circuiting:** Nextcloud bubbles up ETags. If a file deep in `/Photos/Summer` changes, the ETag of the `/Photos` folder also changes. The engine can check the Root folder ETag first—if it hasn't changed since the last sync, Phase 2 (Remote Discovery) can be skipped entirely.

---

## 5. Schema Blueprint

Below is the structure of the `.nextdoor/directory.json` file ensuring this architecture works:

```json
{
  "schemaVersion": 2,
  "remoteTarget": "/Photos/Summer",
  "lastSyncTime": "2024-06-01T12:00:00Z",
  "files": {
    "budget.xlsx": {
      "local_xxhash3": "993bc928f52df...",
      "remote_etag": "\"abc123def45\"",
      "size": 4096,
      "modtime": "2024-06-01T10:30:00Z"
    },
    "photos/summer.jpg": {
      "local_xxhash3": "772bc839d41ea...",
      "remote_etag": "\"xyz987qwe65\"",
      "size": 2048000,
      "modtime": "2024-05-20T14:15:00Z"
    }
  }
}
```