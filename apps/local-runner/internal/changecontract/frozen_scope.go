// CP-55 P-4: comparing a Flow writer's actual output against its frozen
// preflight contract, and amending that contract (a new, superseding,
// higher-versioned record) when a retry genuinely needs a wider declared
// scope. Deliberately separate from scope.go's ScopeDiff, which operates on
// the legacy declared-or-inferred Contract — a Flow's agent.code writer is
// governed by FrozenContractRecord instead (Task-264/265), never by that
// post-hoc mechanism.
package changecontract

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// frozenStoreContractsFile/frozenStoreEventsFile must match the exact
// filenames FrozenStore itself writes (preflight.go's NewFrozenStore) —
// FrozenStoreBookkeepingPaths is the single source of truth both sides use.
const (
	frozenStoreContractsFile = "frozen_contracts.ndjson"
	frozenStoreEventsFile    = "frozen_contract_events.ndjson"
)

// FrozenStoreBookkeepingPaths returns the exact repo-relative paths (forward
// slash) that FrozenStore writes into the workspace it is rooted at. These —
// and only these — are excluded from a Flow writer's scope-drift comparison:
// freezing/amending a contract is FlowPilot's own bookkeeping, not something
// the coder wrote.
//
// A prior version of this exclusion used flowgate.IsDocOrAuditFile, which
// exempts the entire .flowpilot/** tree and every *.md file — Claude-agent
// review (CA-427 Finding 2) correctly flagged this as a real security hole:
// under that rule a writer could silently rewrite
// .flowpilot/settings/flow-rules.json (disabling the very gate judging it)
// or forge frozen_contracts.ndjson directly, with zero drift ever detected.
// Excluding only these two exact files closes that hole while still not
// false-positive-blocking on the freeze's own unavoidable write.
func FrozenStoreBookkeepingPaths() []string {
	return []string{
		path.Join(".flowpilot", "contracts", frozenStoreContractsFile),
		path.Join(".flowpilot", "contracts", frozenStoreEventsFile),
	}
}

// IsFrozenStoreBookkeepingPath reports whether p is one of
// FrozenStoreBookkeepingPaths (after the same normalization
// FrozenContractScopeDrift applies to every path it compares).
func IsFrozenStoreBookkeepingPath(p string) bool {
	np := normalizeScopePath(p)
	for _, b := range FrozenStoreBookkeepingPaths() {
		if np == b {
			return true
		}
	}
	return false
}

// RunnerLedgerBookkeepingPaths returns the exact repo-relative paths
// (forward slash) that FlowPilot's runner-internal ledger (changeledger /
// contextsync / chat_summary) writes into the workspace it is rooted at.
// These exact bookkeeping files are excluded from a Flow writer's scope-drift
// comparison so runner ledger writes during turns do not false-positive as
// scope drift (BUG-327).
func RunnerLedgerBookkeepingPaths() []string {
	return []string{
		path.Join(".flowpilot", "manifest.json"),
		path.Join(".flowpilot", "ledger", "chat_summary.ndjson"),
		path.Join(".flowpilot", "ledger", "feature_history.ndjson"),
	}
}

// IsRunnerLedgerBookkeepingPath reports whether p is one of
// RunnerLedgerBookkeepingPaths (after the same normalization
// FrozenContractScopeDrift applies to every path it compares).
func IsRunnerLedgerBookkeepingPath(p string) bool {
	np := normalizeScopePath(p)
	for _, b := range RunnerLedgerBookkeepingPaths() {
		if np == b {
			return true
		}
	}
	return false
}

// IsChangeAuditPath reports whether p is a change audit note under change-audit/*.md
// which coding agents are explicitly allowed to create per BUG-278 without triggering
// code scope drift.
func IsChangeAuditPath(p string) bool {
	np := normalizeScopePath(p)
	return strings.HasPrefix(np, "change-audit/") && strings.HasSuffix(np, ".md")
}

// PendingCanonicalStoreBookkeepingPaths returns the exact repo-relative paths
// PendingCanonicalStore (pending_head.go, CP-55 P-5) writes into the
// workspace it is rooted at — the same idiom as FrozenStoreBookkeepingPaths,
// for the same reason: a Flow coder's own gate pass staging a pending
// Canonical Head update is FlowPilot's own bookkeeping, not something the
// coder wrote, and must never itself register as scope drift on that same
// writer's NEXT gate pass (CP-55 P-8 finding — this exemption did not exist
// when PendingCanonicalStore was added in P-5, since no coder had gone
// through more than one gate pass against a frozen contract until P-8's
// migrated flows made that a live path).
func PendingCanonicalStoreBookkeepingPaths() []string {
	return []string{
		path.Join(".flowpilot", pendingCanonicalStoreDir, pendingCanonicalStoreRecordsFile),
		path.Join(".flowpilot", pendingCanonicalStoreDir, pendingCanonicalStoreEventsFile),
	}
}

// IsPendingCanonicalStoreBookkeepingPath reports whether p is one of
// PendingCanonicalStoreBookkeepingPaths (after the same normalization
// FrozenContractScopeDrift applies to every path it compares).
func IsPendingCanonicalStoreBookkeepingPath(p string) bool {
	np := normalizeScopePath(p)
	for _, b := range PendingCanonicalStoreBookkeepingPaths() {
		if np == b {
			return true
		}
	}
	return false
}

// FrozenContractScopeDrift returns the paths in writtenPaths that are not
// among rec.DeclaredPaths — what a Flow writer touched beyond what its
// frozen contract authorized. Both sides are forward-slash normalized,
// path.Clean'd and trimmed before comparison (matching NormalizeDeclaredCodePaths'
// own normalization, so a written path reported in a different but
// equivalent form — "./src/calc.go" vs "src/calc.go" — does not
// false-positive as drift); blank entries in writtenPaths are ignored. A nil
// result means the writer stayed entirely within its declared scope.
// Deduplicated and sorted for a deterministic, reproducible report.
func FrozenContractScopeDrift(rec FrozenContractRecord, writtenPaths []string) []string {
	declared := make(map[string]bool, len(rec.DeclaredPaths))
	for _, p := range rec.DeclaredPaths {
		declared[normalizeScopePath(p)] = true
	}
	seen := make(map[string]bool, len(writtenPaths))
	var drift []string
	for _, p := range writtenPaths {
		np := normalizeScopePath(p)
		if np == "" || seen[np] || declared[np] {
			continue
		}
		seen[np] = true
		drift = append(drift, np)
	}
	sort.Strings(drift)
	return drift
}

func normalizeScopePath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, `/`)
	p = filepath.ToSlash(p)
	if p == "" {
		return ""
	}
	return path.Clean(p)
}

func normalizeScopePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if np := normalizeScopePath(p); np != "" {
			out = append(out, np)
		}
	}
	return out
}

// AmendFrozenContract supersedes existing with a new, higher-versioned frozen
// record whose DeclaredPaths is the normalized union of existing.DeclaredPaths
// and additionalPaths. If that union contributes nothing existing did not
// already declare, no amendment is made — existing is returned unchanged, so
// a plain retry that does not need wider scope (e.g. retrying after an
// unrelated test failure) never mints a pointless new version (CP-55 P-4,
// TestRetryWithoutNewPathsReusesFrozenVersion).
//
// Every non-blank entry in additionalPaths must itself be a concrete code
// target (per IsConcreteCodeTarget) — an extension-less file like "Makefile"
// or a doc/glob/flag-like entry is rejected with an explicit error rather
// than silently dropped. Fixed after Claude-agent review (CA-427 Finding 5):
// the original version silently discarded any such path during
// normalization, so a caller asking to widen scope to include a legitimately
// out-of-scope but non-"concrete" file got back existing unchanged with a
// nil error — indistinguishable from "you didn't need to widen," which
// would have looped a scope-drift retry forever with no actionable signal.
//
// On a genuine widen, the new version is saved BEFORE existing is marked
// ContractStatusSuperseded (fixed after CA-427 Finding 4: the original order
// meant a crash/error between the two calls left existing superseded with no
// active successor, permanently blocking the step — GetFrozenForStep already
// returns the highest-VERSION active record, so saving the successor first
// means even a crash between the two calls still resolves correctly).
func AmendFrozenContract(store *FrozenStore, workspace string, existing FrozenContractRecord, additionalPaths []string, now time.Time) (FrozenContractRecord, error) {
	for _, p := range additionalPaths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		if !IsConcreteCodeTarget(normalizeScopePath(trimmed)) {
			return FrozenContractRecord{}, fmt.Errorf("changecontract: amendment path %q is not a concrete code target and cannot widen scope", trimmed)
		}
	}

	existingNormalized := normalizeScopePaths(existing.DeclaredPaths)
	union := append([]string(nil), existing.DeclaredPaths...)
	union = append(union, additionalPaths...)
	normalized, err := NormalizeDeclaredCodePaths(workspace, union)
	if err != nil {
		return FrozenContractRecord{}, err
	}
	if sameStringSet(normalized, existingNormalized) {
		return existing, nil
	}

	draft := PreflightContractDraft{
		FeatureKey:    existing.FeatureKey,
		Intent:        existing.Intent,
		DeclaredPaths: normalized,
		SourceDocID:   existing.SourceDocID,
	}
	amended, err := FreezeContract(workspace, existing.RunID, existing.PlannerStepID, existing.CoderStepID, draft, existing.BaseSHA, existing.BaselineWorktree, existing.ContractID, existing.Version+1, now)
	if err != nil {
		return FrozenContractRecord{}, err
	}
	if err := store.SaveFrozen(amended); err != nil {
		return FrozenContractRecord{}, err
	}
	if err := store.AppendStatus(existing.ContractID, ContractStatusSuperseded, "amended: scope widened", now); err != nil {
		return FrozenContractRecord{}, err
	}
	return amended, nil
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}
