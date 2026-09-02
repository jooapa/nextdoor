package sync

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jooapa/nextdoor/internal/nextcloud"
	"github.com/jooapa/nextdoor/internal/state"
)

type Action string

const (
	ActionPush         Action = "PUSH"
	ActionPull         Action = "PULL"
	ActionRemoteDelete Action = "REMOTE_DELETE"
	ActionLocalDelete  Action = "LOCAL_DELETE"
	ActionConflict     Action = "CONFLICT"
	ActionNone         Action = "NONE"
)

type ChangeState string

const (
	StateUnchanged ChangeState = "UNCHANGED"
	StateAdded     ChangeState = "ADDED"
	StateModified  ChangeState = "MODIFIED"
	StateDeleted   ChangeState = "DELETED"
)

// FilePlan represents a single synchronization decision for a specific file.
type FilePlan struct {
	RelPath      string
	Action       Action
	LocalState   ChangeState
	RemoteState  ChangeState
	LocalInfo    *state.FileInfo
	RemoteInfo   *nextcloud.RemoteFile
}

// Reconcile acts as the 3-Way Merge Brain, cross-referencing Local and Remote against Base.
func Reconcile(baseState *state.State, localFiles map[string]state.FileInfo, remoteFiles map[string]nextcloud.RemoteFile) []FilePlan {
	var plan []FilePlan

	// Collect a unique union of all file paths across all three states
	allFiles := make(map[string]bool)
	if baseState != nil && baseState.Files != nil {
		for p := range baseState.Files {
			allFiles[p] = true
		}
	}
	for p := range localFiles {
		allFiles[p] = true
	}
	for p := range remoteFiles {
		allFiles[p] = true
	}

	for p := range allFiles {
		base, hasBase := state.FileInfo{}, false
		if baseState != nil && baseState.Files != nil {
			base, hasBase = baseState.Files[p]
		}
		
		localF, hasLocal := localFiles[p]
		remoteF, hasRemote := remoteFiles[p]

		localState := determineState(hasBase, hasLocal, base.LocalXXHash3, localF.LocalXXHash3)
		remoteState := determineState(hasBase, hasRemote, base.RemoteETag, remoteF.ETag)

		action := determineAction(localState, remoteState)

		if action != ActionNone {
			filePlan := FilePlan{
				RelPath:     p,
				Action:      action,
				LocalState:  localState,
				RemoteState: remoteState,
			}
			
			// Defensively copy values to pointers
			if hasLocal {
				localCopy := localF
				filePlan.LocalInfo = &localCopy
			}
			if hasRemote {
				remoteCopy := remoteF
				filePlan.RemoteInfo = &remoteCopy
			}
			
			plan = append(plan, filePlan)
		}
	}

	return plan
}

func determineState(hasBase, hasCurrent bool, baseHash, currentHash string) ChangeState {
	if !hasBase && hasCurrent {
		return StateAdded
	}
	if hasBase && !hasCurrent {
		return StateDeleted
	}
	if hasBase && hasCurrent {
		if baseHash != currentHash {
			return StateModified
		}
		return StateUnchanged
	}
	// Edge case: !hasBase && !hasCurrent (should never occur given map union)
	return StateUnchanged
}

func determineAction(localState, remoteState ChangeState) Action {
	// The Logic Matrix implementation
	if (localState == StateModified || localState == StateAdded) && remoteState == StateUnchanged {
		return ActionPush
	}
	if localState == StateUnchanged && (remoteState == StateModified || remoteState == StateAdded) {
		return ActionPull
	}
	if localState == StateDeleted && remoteState == StateUnchanged {
		return ActionRemoteDelete
	}
	if localState == StateUnchanged && remoteState == StateDeleted {
		return ActionLocalDelete
	}

	// Any simultaneous changes on both sides are a conflict, unless they both agree to delete.
	if (localState != StateUnchanged) && (remoteState != StateUnchanged) {
		if localState == StateDeleted && remoteState == StateDeleted {
			return ActionNone
		}
		return ActionConflict
	}

	return ActionNone
}

// GenerateConflictFilename takes a normal filename and safely renames it to 
// prevent overwriting during a collision. (e.g. "budget.xlsx" -> "budget (conflicted copy).xlsx")
func GenerateConflictFilename(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return fmt.Sprintf("%s (conflicted copy)%s", base, ext)
}
