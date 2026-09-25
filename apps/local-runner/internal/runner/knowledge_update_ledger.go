package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/knowledge"
)

// BUG-477: the audit-completion knowledge refresh used to be a naked
// `go UpdateAsync(...)` — a runner killed after the audit hook returned but
// before the worker wrote its sections lost the update forever, and the next
// boot saw a healthy index.json and stayed silent. The durable record is the
// source of truth, so the changed-path set is persisted BEFORE the hook
// acknowledges, replayed under the per-workspace lock, and only marked
// committed after the knowledge files are written. A corrupt intent file
// fails closed to a full rebuild rather than silently declaring the base
// fresh.

const (
	knowledgePendingFileName      = "pending-updates.json"
	knowledgePendingSchemaVersion = 1
	// knowledgeUpdateMaxPendingPaths bounds the on-disk dirty set: beyond it
	// the ledger folds to a single full-rebuild intent rather than growing
	// unboundedly.
	knowledgeUpdateMaxPendingPaths = 512
	// knowledgeUpdateMaxAttempts bounds one replay pass; leftover intents
	// stay pending for the next trigger or the next boot.
	knowledgeUpdateMaxAttempts = 3
)

// knowledgeUpdateRetryDelay backs off between replay attempts. A package var
// so tests can collapse it to zero.
var knowledgeUpdateRetryDelay = 2 * time.Second

type knowledgeUpdateIntent struct {
	ID    int64    `json:"id"`
	Paths []string `json:"paths,omitempty"`
	// Full marks a dirty-set overflow or corrupt-ledger tombstone: the only
	// safe replay is a full rebuild, never an incremental guess.
	Full bool   `json:"full,omitempty"`
	At   string `json:"at"`
}

type knowledgePendingUpdates struct {
	SchemaVersion int                      `json:"schemaVersion"`
	NextID        int64                    `json:"nextId"`
	Intents       []knowledgeUpdateIntent  `json:"intents"`
}

func knowledgePendingPath(workspace string) string {
	return filepath.Join(knowledge.KnowledgeDir(workspace), knowledgePendingFileName)
}

// appendKnowledgeUpdateIntent durably records changedPaths for replay.
// Serialized with replay by the caller's per-workspace lock; the write is
// atomic (temp + rename) so a kill mid-append never leaves a torn ledger.
func appendKnowledgeUpdateIntent(workspace string, paths []string) error {
	pending, err := loadKnowledgePending(workspace)
	if err != nil {
		// A corrupt ledger cannot be trusted to enumerate what is dirty —
		// fold to a full-rebuild tombstone rather than dropping intents.
		pending = &knowledgePendingUpdates{
			SchemaVersion: knowledgePendingSchemaVersion,
			NextID:        1,
		}
	}
	if pending.SchemaVersion == 0 {
		pending.SchemaVersion = knowledgePendingSchemaVersion
	}
	total := 0
	for _, in := range pending.Intents {
		total += len(in.Paths)
		if in.Full {
			total = knowledgeUpdateMaxPendingPaths
			break
		}
	}
	intent := knowledgeUpdateIntent{
		ID:    pending.NextID,
		Paths: paths,
		At:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	pending.NextID++
	pending.Intents = append(pending.Intents, intent)
	if total+len(paths) > knowledgeUpdateMaxPendingPaths {
		pending.Intents = []knowledgeUpdateIntent{{
			ID:   pending.NextID,
			Full: true,
			At:   time.Now().UTC().Format(time.RFC3339Nano),
		}}
		pending.NextID++
	}
	return writeKnowledgePending(workspace, pending)
}

// loadKnowledgePending reads the ledger; an absent file is an empty ledger,
// while a present-but-corrupt or schema-mismatched file is an error so the
// caller fails closed to a full rebuild (never silent freshness).
func loadKnowledgePending(workspace string) (*knowledgePendingUpdates, error) {
	data, err := os.ReadFile(knowledgePendingPath(workspace))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &knowledgePendingUpdates{
				SchemaVersion: knowledgePendingSchemaVersion,
				NextID:        1,
			}, nil
		}
		return nil, err
	}
	var pending knowledgePendingUpdates
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, err
	}
	if pending.SchemaVersion != knowledgePendingSchemaVersion {
		return nil, fmt.Errorf("knowledge pending schema %d, want %d", pending.SchemaVersion, knowledgePendingSchemaVersion)
	}
	if pending.NextID == 0 {
		pending.NextID = 1
	}
	return &pending, nil
}

func writeKnowledgePending(workspace string, pending *knowledgePendingUpdates) error {
	data, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return err
	}
	path := knowledgePendingPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pending-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// commitKnowledgeIntents drops the intents consumed by a successful replay.
// The ledger is re-read (not the stale in-memory copy) so intents appended
// while the update ran are preserved.
func commitKnowledgeIntents(workspace string, maxCommittedID int64) error {
	pending, err := loadKnowledgePending(workspace)
	if err != nil {
		return err
	}
	kept := pending.Intents[:0]
	for _, in := range pending.Intents {
		if in.ID > maxCommittedID {
			kept = append(kept, in)
		}
	}
	pending.Intents = kept
	return writeKnowledgePending(workspace, pending)
}

// clearKnowledgePending removes the ledger entirely — used after a full
// bootstrap WriteFull, which already covers every previously pending path.
func clearKnowledgePending(workspace string) {
	if err := os.Remove(knowledgePendingPath(workspace)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("[knowledge] clear pending updates failed workspace=%q: %v", workspace, err)
	}
}

// replayKnowledgeUpdates is the leased worker for pending intents: claim the
// per-workspace lock, coalesce the dirty set, run the incremental merge, and
// commit only after the knowledge files are written. Corrupt intent/index
// state fails closed to a full rebuild; a missing index stays pending for
// the bootstrap path (it owns mid-flow rebuild decisions). Bounded attempts;
// leftovers stay durable for the next trigger.
func replayKnowledgeUpdates(workspace string, redistill func(ctx context.Context) (*knowledge.KnowledgeBase, error)) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" || redistill == nil {
		return
	}
	mu, _ := knowledgeUpdateLocks.LoadOrStore(workspace, &sync.Mutex{})
	lock := mu.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	for attempt := 0; attempt < knowledgeUpdateMaxAttempts; attempt++ {
		if attempt > 0 && knowledgeUpdateRetryDelay > 0 {
			time.Sleep(knowledgeUpdateRetryDelay)
		}
		pending, err := loadKnowledgePending(workspace)
		if err != nil {
			// Corrupt ledger: the dirty set is unknowable — only a full
			// rebuild restores a trustworthy base.
			log.Printf("[knowledge] pending ledger corrupt workspace=%q: %v — full rebuild", workspace, err)
			if kb, rerr := redistill(context.Background()); rerr == nil {
				if werr := knowledge.WriteFull(workspace, kb); werr == nil {
					clearKnowledgePending(workspace)
					return
				}
			}
			continue
		}
		if len(pending.Intents) == 0 {
			return
		}
		if knowledge.Missing(workspace) {
			// Never full-rebuild mid-flow: the bootstrap path owns the
			// initial distill and clears pending intents on success.
			log.Printf("[knowledge] index missing with %d pending intents workspace=%q — left for bootstrap", len(pending.Intents), workspace)
			return
		}
		full := false
		var maxID int64
		paths := make([]string, 0, 64)
		for _, in := range pending.Intents {
			if in.ID > maxID {
				maxID = in.ID
			}
			if in.Full {
				full = true
			}
			paths = append(paths, in.Paths...)
		}
		var uerr error
		if full {
			var kb *knowledge.KnowledgeBase
			kb, uerr = redistill(context.Background())
			if uerr == nil {
				uerr = knowledge.WriteFull(workspace, kb)
			}
		} else {
			uerr = knowledge.IncrementalUpdate(workspace, paths, redistill)
		}
		if uerr != nil {
			log.Printf("[knowledge] pending replay attempt %d failed workspace=%q: %v", attempt+1, workspace, uerr)
			continue
		}
		if cerr := commitKnowledgeIntents(workspace, maxID); cerr != nil {
			log.Printf("[knowledge] commit pending intents failed workspace=%q: %v", workspace, cerr)
		}
		return
	}
}
