package runner

import (
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/flowgate"
)

// BUG-357: per-run file_artifact OUTPUT instance recording.
//
// Bindings tell a flow writer WHERE to write and the gate verifies the files
// exist — but nothing ever recorded WHAT was written per run, so the
// artifacts panel/API only ever showed finalizer rows or the fake catalog.
// This file closes that gap: on flow-child completion the runner snapshots
// the node's required bound OUTPUT paths, keeps only files actually present
// on disk (same existence semantics as the gate), and stores one record per
// file keyed by the parent (workflow) run. handleListArtifacts merges these
// records ahead of the finalizer rows.
//
// Deliberately local/in-memory (same lifetime model as the finalizer store):
// Supabase artifact_runs sync is a separate cut-over (see finalizer.go:128).

// runArtifactKindFile is the Artifact.Kind for recorded bound OUTPUT files.
const runArtifactKindFile = "file_artifact"

// runArtifactStore is an in-memory per-run artifact record store keyed by
// run ID. It is safe for concurrent use. Records upsert by Artifact.ID so a
// re-settled child completion cannot duplicate rows.
type runArtifactStore struct {
	mu    sync.Mutex
	byRun map[string][]Artifact
}

func newRunArtifactStore() *runArtifactStore {
	return &runArtifactStore{byRun: map[string][]Artifact{}}
}

// record upserts arts under runID (matching IDs are replaced in place,
// preserving first-seen order).
func (st *runArtifactStore) record(runID string, arts []Artifact) {
	if st == nil || strings.TrimSpace(runID) == "" || len(arts) == 0 {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	cur := st.byRun[runID]
	idx := make(map[string]int, len(cur)+len(arts))
	for i, a := range cur {
		idx[a.ID] = i
	}
	for _, a := range arts {
		if strings.TrimSpace(a.RunID) == "" {
			a.RunID = runID
		}
		if j, ok := idx[a.ID]; ok {
			cur[j] = a
			continue
		}
		idx[a.ID] = len(cur)
		cur = append(cur, a)
	}
	st.byRun[runID] = cur
}

// forRun returns a copy of the records for runID. False when none recorded.
func (st *runArtifactStore) forRun(runID string) ([]Artifact, bool) {
	if st == nil || strings.TrimSpace(runID) == "" {
		return nil, false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	cur := st.byRun[runID]
	if len(cur) == 0 {
		return nil, false
	}
	return append([]Artifact(nil), cur...), true
}

// buildFlowChildArtifactRecords shapes one Artifact per written bound OUTPUT
// path (pure — unit-testable). IDs are deterministic
// (run:artifact:node:N) so re-recording upserts instead of duplicating.
func buildFlowChildArtifactRecords(runID, nodeID string, relPaths []string, now time.Time) []Artifact {
	created := now.UTC().Format(time.RFC3339Nano)
	var out []Artifact
	seen := make(map[string]bool, len(relPaths))
	for _, p := range relPaths {
		rel := strings.TrimSpace(p)
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		name := path.Base(rel)
		if name == "" || name == "." || name == "/" {
			name = rel
		}
		out = append(out, Artifact{
			ID:        fmt.Sprintf("%s:artifact:%s:%d", runID, nodeID, len(out)),
			RunID:     runID,
			Kind:      runArtifactKindFile,
			Name:      name,
			Path:      rel,
			NodeID:    nodeID,
			Preview:   rel,
			CreatedAt: created,
		})
	}
	return out
}

// snapshotFlowChildArtifactPathsLocked copies the routing keys + required
// bound OUTPUT paths (plain + structured, deduped) for a completed flow
// child. Caller must hold s.mu — it uses the Locked node lookup because the
// locking wrapper would deadlock here.
func (s *InteractiveService) snapshotFlowChildArtifactPathsLocked(rs *interactiveRun) (runID, nodeID, workspaceCwd string, paths []string) {
	if s == nil || rs == nil {
		return "", "", "", nil
	}
	runID = strings.TrimSpace(rs.parentRunID)
	if runID == "" {
		runID = strings.TrimSpace(rs.id)
	}
	nodeID = strings.TrimSpace(rs.label)
	if nodeID == "" {
		nodeID = strings.TrimSpace(rs.stepID)
	}
	workspaceCwd = strings.TrimSpace(rs.workspaceCwd)
	node, ok := flowNodeForRunLocked(s, rs)
	if !ok {
		return runID, nodeID, workspaceCwd, nil
	}
	seen := make(map[string]bool)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}
	for _, p := range requiredFileArtifactOutputPaths(node) {
		add(p)
	}
	for _, st := range requiredStructuredFileArtifactOutputs(node) {
		add(st.Path)
	}
	return runID, nodeID, workspaceCwd, paths
}

// recordFlowChildArtifacts keeps only bound OUTPUT paths actually present on
// disk (gate-identical existence semantics via
// flowgate.MissingRequiredFileArtifactOutputs — workspace-escape-safe) and
// stores one record per file. Sync so tests can call it directly; the settle
// hook dispatches it via goroutine.
func (s *InteractiveService) recordFlowChildArtifacts(runID, nodeID, workspaceCwd string, requiredPaths []string) {
	if s == nil || s.runArtifacts == nil {
		return
	}
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(workspaceCwd) == "" || len(requiredPaths) == 0 {
		return
	}
	missing := make(map[string]bool)
	for _, m := range flowgate.MissingRequiredFileArtifactOutputs(workspaceCwd, requiredPaths) {
		missing[strings.TrimSpace(m)] = true
	}
	var present []string
	for _, p := range requiredPaths {
		if t := strings.TrimSpace(p); t != "" && !missing[t] {
			present = append(present, t)
		}
	}
	if len(present) == 0 {
		return
	}
	s.runArtifacts.record(runID, buildFlowChildArtifactRecords(runID, nodeID, present, time.Now()))
}
