package runner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// CA-1094 (bounded stubs): the Contract-First scaffold may CREATE declared
// production paths as whitelist stubs, but it must never modify a file that
// already existed at contract.freeze. Live run-3362: the tdd leg rewrote
// settings.gradle.kts / libs.versions.toml — committed before freeze —
// because its frozen record carried the full declared scope with no
// existing-vs-new split. The freeze now records pre-existing declared paths
// as ReadOnlyPaths on the SCAFFOLD's record only (the coder fills the
// bodies, so the same files stay writable for it).

// scaffoldPreExistingReadOnlyPaths returns the declared paths that already
// exist on disk at freeze time — the read-only surface for a scaffold
// writer. Test files are never classified here: a pre-existing test file
// stays writable for the scaffold (it may extend an existing suite); the
// post-pass LockScaffoldArtifacts owns the test-file lock for the coder.
// Directories are skipped — a declared dir that exists must not lock new
// stub files created underneath it.
func scaffoldPreExistingReadOnlyPaths(workspace string, declared []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range declared {
		rel := normalizeReproduceLockCandidate(workspace, p)
		if rel == "" || seen[rel] || flowgate.IsTestFile(rel) {
			continue
		}
		info, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(rel)))
		if err != nil || info.IsDir() {
			continue
		}
		seen[rel] = true
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

// scaffoldBoundedReadOnly returns the freeze-time read-only set for a
// writer node: declared paths that pre-existed, but only when the node is a
// scaffold-behavior writer. An agent.code writer gets nil — it is the leg
// that fills the stub bodies and implements the pre-existing files.
func scaffoldBoundedReadOnly(workspace string, node agentpack.FlowNode, declared []string) []string {
	if !IsScaffoldBehavior(node.Behavior) {
		return nil
	}
	return scaffoldPreExistingReadOnlyPaths(workspace, declared)
}

// decideScaffoldPreExistingLock is the bridge-level enforcement of the
// bounded-stub lock: while the scaffold child runs, a write/edit or a
// mutating shell command aimed at a path frozen read-only for its step is
// silent-denied — same provider-neutral choke point as
// decideReproduceTestLock, likewise BEFORE YOLO. Reads fall through
// unhandled (the scaffold must read the files it integrates with).
func (s *InteractiveService) decideScaffoldPreExistingLock(rs *interactiveRun, details ApprovalDetails) (decision, reason string, handled bool) {
	if s == nil || rs == nil {
		return "", "", false
	}
	parentID := strings.TrimSpace(rs.parentRunID)
	if parentID == "" {
		return "", "", false
	}
	nodeID := strings.TrimSpace(rs.label)
	if nodeID == "" {
		nodeID = strings.TrimSpace(rs.stepID)
	}
	if nodeID == "" {
		return "", "", false
	}
	// Only scaffold-behavior children carry this lock — the coder sibling's
	// own record has no freeze-time ReadOnlyPaths for these files anyway,
	// but keying on behavior keeps the enforcement leg explicit even when a
	// topology reuses step ids.
	scaffold := false
	for _, n := range s.activeFlowNodesFor(parentID) {
		if n.ID == nodeID && IsScaffoldBehavior(n.Behavior) {
			scaffold = true
			break
		}
	}
	if !scaffold {
		return "", "", false
	}
	cwd := strings.TrimSpace(rs.workspaceCwd)
	if cwd == "" {
		return "", "", false
	}
	store, err := changecontract.NewFrozenStore(cwd)
	if err != nil {
		return "", "", false
	}
	rec, ok, err := store.GetFrozenForStep(parentID, nodeID)
	if err != nil || !ok || len(rec.ReadOnlyPaths) == 0 {
		return "", "", false
	}

	switch details.Kind {
	case "exec":
		if isReadOnlyCommand(details.Command) {
			return "", "", false
		}
		if reproduceLockCommandTargetsLockedPath(details.Command, rec) {
			return "deny", "scaffold_preexisting_locked", true
		}
	case "file":
		if isReadOnlyToolName(details.Reason) {
			return "", "", false
		}
		candidate := normalizeReproduceLockCandidate(cwd, details.Command)
		if candidate == "" {
			return "", "", false
		}
		if changecontract.IsReadOnlyLockedPathUnder(rec, candidate, cwd) {
			return "deny", "scaffold_preexisting_locked", true
		}
	}
	return "", "", false
}
