# RemoteAccess Folder Type — Implementation Plan

> **Goal:** Add a RemoteAccess folder type to Syncthing — see remote files without syncing them locally, upload files on demand.
> **Architecture:** New folder type wrapping `*folder` with no-op pull. Protocol declares as SendReceive so remote side sends index + accepts uploads. Local behavior differs: never downloads files, uploads staged via temp directory.
> **Tech Stack:** Go (existing Syncthing codebase), Protobuf (minor enum addition)

---

## Task 1: Add FolderTypeRemoteAccess to config and protocol

### Step 1: Add protobuf enum value

**File:** `proto/bep/bep.proto`

Add `FOLDER_TYPE_REMOTE_ACCESS = 4` to the FolderType enum:

```proto
enum FolderType {
    FOLDER_TYPE_SEND_RECEIVE = 0;
    FOLDER_TYPE_SEND_ONLY = 1;
    FOLDER_TYPE_RECEIVE_ONLY = 2;
    FOLDER_TYPE_RECEIVE_ENCRYPTED = 3;
    FOLDER_TYPE_REMOTE_ACCESS = 4;
}
```

### Step 2: Regenerate protobuf Go code

```bash
cd g:/SyncShareBox
go generate ./proto/...
```

This generates `internal/gen/bep/bep.pb.go` with the new enum value.

### Step 3: Add Go protocol constant

**File:** `lib/protocol/bep_clusterconfig.go` — add after line 30:

```go
FolderTypeRemoteAccess = FolderType(bep.FolderType_FOLDER_TYPE_REMOTE_ACCESS)
```

### Step 4: Add config constant

**File:** `lib/config/foldertype.go` — add after line 17:

```go
FolderTypeRemoteAccess = FolderType(protocol.FolderTypeRemoteAccess)
```

And update `String()`, `MarshalText()`, `UnmarshalText()` methods to handle the new type.

### Step 5: Verify compilation

```bash
go build ./...
```

Expected: compiles successfully.

---

## Task 2: Create remoteAccessFolder implementation

**Files:**
- Create: `lib/model/folder_remoteaccess.go`

### Step 1: Create the file

```go
package model

import (
    "context"
    "os"
    "path/filepath"

    "github.com/syncthing/syncthing/lib/config"
    "github.com/syncthing/syncthing/lib/events"
    "github.com/syncthing/syncthing/lib/ignore"
    "github.com/syncthing/syncthing/lib/protocol"
    "github.com/syncthing/syncthing/lib/semaphore"
    "github.com/syncthing/syncthing/lib/versioner"
)

func init() {
    folderFactories[config.FolderTypeRemoteAccess] = newRemoteAccessFolder
}

type remoteAccessFolder struct {
    *folder
    stagingDir string // temp directory for staged upload files
}

func newRemoteAccessFolder(model *model, ignores *ignore.Matcher, cfg config.FolderConfiguration, _ versioner.Versioner, evLogger events.Logger, ioLimiter *semaphore.Semaphore) service {
    // Create a staging directory for uploads
    stagingDir := filepath.Join(cfg.Path, ".staging")
    os.MkdirAll(stagingDir, 0700)

    f := &remoteAccessFolder{
        folder:     newFolder(model, ignores, cfg, evLogger, ioLimiter, nil),
        stagingDir: stagingDir,
    }
    f.puller = f
    return f
}

// PullErrors returns empty — we never pull, so no pull errors.
func (*remoteAccessFolder) PullErrors() []FileError {
    return nil
}

// pull is a no-op: we receive the remote index (so we can see the file list)
// but we never download any files.
func (f *remoteAccessFolder) pull(ctx context.Context) (bool, error) {
    // RemoteAccess does not pull files — it only browses the remote index.
    // The index is already in the database from index exchange.
    return true, nil
}

// Override is not supported for RemoteAccess folders.
func (f *remoteAccessFolder) Override() {
    // no-op
}
```

### Step 2: Verify compilation

```bash
go build ./lib/model/...
```

Expected: compiles successfully.

---

## Task 3: Skip health checks for RemoteAccess folders

**File:** `lib/model/folder.go`

### Step 1: Modify getHealthErrorWithoutIgnores

In `getHealthErrorWithoutIgnores()` (around line 365), skip `CheckPath()` for RemoteAccess type:

```go
func (f *folder) getHealthErrorWithoutIgnores() error {
    // RemoteAccess folders don't require a valid local path — they
    // only browse remote content and stage uploads temporarily.
    if f.Type == config.FolderTypeRemoteAccess {
        return nil
    }

    if err := f.CheckPath(); err != nil {
        return err
    }
    // ... rest unchanged
```

### Step 2: Verify compilation

```bash
go build ./lib/model/...
```

---

## Task 4: Map protocol type in ClusterConfig

**File:** `lib/model/model.go`

### Step 1: In generateClusterConfigRLocked

In `generateClusterConfigRLocked()` (around line 2606), after creating `protocolFolder`, set the folder type. For RemoteAccess folders, map to SendReceive so the remote side:
- Sends us index updates (we can browse)
- Accepts our index updates (we can upload)

```go
protocolFolder := protocol.Folder{
    ID:    folderCfg.ID,
    Label: folderCfg.Label,
}

// RemoteAccess declares as SendReceive to the remote side so
// we receive index updates and can send uploads.
if folderCfg.Type == config.FolderTypeRemoteAccess {
    protocolFolder.Type = protocol.FolderTypeSendReceive
}
```

### Step 2: Verify compilation

```bash
go build ./lib/model/...
```

---

## Task 5: Add upload support to RemoteAccess folder

**File:** `lib/model/folder_remoteaccess.go`

### Step 1: Add UploadFile method

Add a method that stages a file, triggers a scan, and lets Syncthing's existing sending machinery handle the rest:

```go
import (
    "fmt"
    "io"
    "os"
    "path/filepath"
)

// StageFile copies a file into the staging directory and triggers a scan
// so Syncthing's existing send machinery picks it up and sends it to peers.
// After sync completes, the staging file should be cleaned up.
func (f *remoteAccessFolder) StageFile(name string, reader io.Reader) (string, error) {
    stagingPath := filepath.Join(f.stagingDir, name)

    // Ensure parent directory exists
    if dir := filepath.Dir(stagingPath); dir != "." {
        if err := os.MkdirAll(dir, 0700); err != nil {
            return "", fmt.Errorf("creating staging dir: %w", err)
        }
    }

    // Write the file to staging
    dst, err := os.Create(stagingPath)
    if err != nil {
        return "", fmt.Errorf("creating staging file: %w", err)
    }
    defer dst.Close()

    if _, err := io.Copy(dst, reader); err != nil {
        return "", fmt.Errorf("writing staging file: %w", err)
    }

    // Trigger a scan of the staging directory
    f.ScheduleScan()

    return stagingPath, nil
}
```

### Step 2: Add cleanup after sync

After the file is successfully sent to all peers, clean up the staging file:

```go
// CleanupStaging removes successfully synced files from the staging directory.
func (f *remoteAccessFolder) CleanupStaging() error {
    entries, err := os.ReadDir(f.stagingDir)
    if err != nil {
        return err
    }
    for _, entry := range entries {
        os.Remove(filepath.Join(f.stagingDir, entry.Name()))
    }
    return nil
}
```

### Step 3: Verify compilation

```bash
go build ./lib/model/...
```

---

## Task 6: GUI — Remote directory panel

**File:** `gui/default/index.html` (existing GUI)

### Step 1: Add remote folder browser UI

Add a new section to the folder view that shows remote file listing for RemoteAccess folders. This uses existing Syncthing REST API endpoints:

- `GET /rest/db/file?folder=<folderID>` — list files from index (works even without local files since index is in DB)
- `GET /rest/db/browse?folder=<folderID>` — browse directory structure

### Step 2: Add upload button to remote directory view

A file input / drag-drop zone that:
1. Takes the selected file
2. Posts to a new or existing endpoint that calls `StageFile`
3. Shows upload progress via existing Syncthing events

### Step 3: Frontend changes summary

Minimal changes to `gui/default/`:
- Add a "RemoteAccess" folder section in the folder list
- Show remote file tree (read from DB index)
- Add upload UI element

---

## Task 7: Integration test

### Step 1: Manual test scenario

1. Configure folder on PC1 as `ReceiveOnly` — this is the "receiver"
2. Configure same folder on Phone/PC2 as `RemoteAccess` — this is the "sender/browser"
3. Verify: Phone sees file list from PC1 without downloading any files
4. Upload a file from Phone to PC1 — verify it appears in PC1's folder
5. Verify: Phone's disk usage does NOT grow (no files synced locally)

### Step 2: Verify existing tests still pass

```bash
go test ./lib/model/... -count=1
go test ./lib/config/... -count=1
```

---

## Summary

| Component | Files Changed | LOC (est.) |
|-----------|--------------|------------|
| Protobuf | `proto/bep/bep.proto` | +1 line |
| Protocol consts | `lib/protocol/bep_clusterconfig.go` | +2 lines |
| Config | `lib/config/foldertype.go` | +10 lines |
| Folder type | `lib/model/folder_remoteaccess.go` (new) | ~80 lines |
| Health check | `lib/model/folder.go` | +4 lines |
| ClusterConfig | `lib/model/model.go` | +4 lines |
| GUI | `gui/default/` | ~50 lines |
| **Total** | | **~150 lines** |
