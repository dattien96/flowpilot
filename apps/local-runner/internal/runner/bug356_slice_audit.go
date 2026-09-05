package runner

// Docs-only audit verification (BUG-356): cp-harness slice-only flows declare
// no command.validate node by design, so no validation state can ever exist
// and audit would park blocked_validation_failed with no forward path
// (Retry re-runs the same empty state). For such flows, verify docs-only
// scope integrity + harness artifact presence instead of blocking forever.
// Flows WITH a validate node keep the existing fail-closed behavior untouched.

import (
	"path"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// sliceOutputsCheckCommand is the ValidationCommand stamped when the docs-only
// fast path verifies a slice-only flow (audit trail shows what verified).
const sliceOutputsCheckCommand = "slice-outputs-check (docs-only)"

// flowHasValidateNode reports whether the flow declares a command.validate
// behavior node. Only flows WITH one can ever produce validation state;
// slice-only flows (cp-harness, Task-306 T-1) have none by design.
func flowHasValidateNode(nodes []agentpack.FlowNode) bool {
	for _, n := range nodes {
		if strings.EqualFold(strings.TrimSpace(n.Behavior), "command.validate") {
			return true
		}
	}
	return false
}

// normalizeDocPath mirrors changecontract's scope normalization for the local
// prefix checks below (the Is* helpers normalize internally).
func normalizeDocPath(p string) string {
	np := strings.ReplaceAll(strings.TrimSpace(p), `\`, `/`)
	if np == "" {
		return ""
	}
	return path.Clean(np)
}

// isDocsScopePath reports whether a changed path is docs/bookkeeping scope:
// harness docs under requirements/, CA notes, or runner-owned bookkeeping
// (same exemption set as the frozen-scope drift gate, CA-427 Finding 2 —
// notably NOT flow-rules.json or the feature registry).
func isDocsScopePath(p string) bool {
	np := normalizeDocPath(p)
	if np == "" {
		return false
	}
	if strings.HasPrefix(np, "requirements/") {
		return true
	}
	return changecontract.IsChangeAuditPath(np) ||
		changecontract.IsFrozenStoreBookkeepingPath(np) ||
		changecontract.IsPendingCanonicalStoreBookkeepingPath(np) ||
		changecontract.IsRunnerLedgerBookkeepingPath(np) ||
		changecontract.IsCanonicalHeadStorePath(np) ||
		changecontract.IsLegacyContractsStorePath(np) ||
		changecontract.IsToolOwnedScaffoldPath(np)
}

// isHarnessArtifactDoc reports whether a path is a harness plan artifact doc:
// CP docs or Task docs in their todo folders (the slice-only deliverables).
func isHarnessArtifactDoc(p string) bool {
	np := normalizeDocPath(p)
	if !(strings.HasPrefix(np, "requirements/07-Coding-Plan/todo/") ||
		strings.HasPrefix(np, "requirements/08-Task/todo/")) {
		return false
	}
	base := path.Base(np)
	if !strings.HasSuffix(base, ".md") {
		return false
	}
	return strings.HasPrefix(base, "CP-") || strings.HasPrefix(base, "Task-")
}

// verifySliceOnlyOutputs checks a docs-only aggregate diff for audit: every
// changed path must be docs/bookkeeping scope, and at least one harness
// artifact doc (CP/Task) must be present. Pure over paths — no I/O.
func verifySliceOnlyOutputs(changedFiles []string) bool {
	found := false
	for _, f := range changedFiles {
		if !isDocsScopePath(f) {
			return false
		}
		if isHarnessArtifactDoc(f) {
			found = true
		}
	}
	return found
}
