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
// contextsync / chat_summary / gate-metrics) writes into the workspace it is
// rooted at. These exact bookkeeping files are excluded from a Flow writer's
// scope-drift comparison so runner ledger writes during turns do not
// false-positive as scope drift (BUG-327). CA-649: gate-metrics.ndjson is
// appended by the gate itself on EVERY gate pass, so it is always dirty at the
// moment the coder's own FrozenContractScopeDrift check runs — without the
// exemption the gate parks itself on its own observability file.
func RunnerLedgerBookkeepingPaths() []string {
	return []string{
		path.Join(".flowpilot", "manifest.json"),
		path.Join(".flowpilot", "ledger", "chat_summary.ndjson"),
		path.Join(".flowpilot", "ledger", "feature_history.ndjson"),
		path.Join(".flowpilot", "gate-metrics.ndjson"),
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

// IsChangeAuditPath reports whether p is a change audit note — flat, direct
// children of change-audit/ named CA-*.md — which coding agents are explicitly
// allowed to create per BUG-278 without triggering code scope drift.
// Deliberately NOT every .md under change-audit/: FEATURE-KEYS.md (the feature
// registry) and any nested sub-directory note are still fully subject to scope
// enforcement — BUG-278 allowed a coder to write its own CA note, not to
// mutate the registry or unrelated notes.
func IsChangeAuditPath(p string) bool {
	np := normalizeScopePath(p)
	if !strings.HasPrefix(np, "change-audit/") {
		return false
	}
	rest := np[len("change-audit/"):]
	if rest == "" || strings.Contains(rest, "/") {
		return false
	}
	base := path.Base(rest)
	return strings.HasPrefix(base, "CA-") && strings.HasSuffix(base, ".md")
}

// IsMarkdownDocPath reports whether p is a markdown/docs path the frozen
// coder-gate must ignore (BUG-370, live run-678326): any *.md (FEATURE-KEYS.md,
// tdd-signatures.md, SS/CP/Task notes) plus anything under requirements/.
// Deliberately NOT the rest of .flowpilot/** — CA-427 Finding 2:
// .flowpilot/settings/flow-rules.json must still count as drift.
func IsMarkdownDocPath(p string) bool {
	np := normalizeScopePath(p)
	if np == "" {
		return false
	}
	if strings.HasSuffix(strings.ToLower(np), ".md") {
		return true
	}
	return strings.HasPrefix(np, "requirements/")
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

// IsCanonicalHeadStorePath reports whether p is a Canonical Head file written
// by FlowPilot's own head store (.flowpilot/canonical/<feature_key>.json):
// exactly one level under .flowpilot/canonical/ with a .json suffix.
// Runner-owned bookkeeping (SaveHead on gate passes), never something a flow
// writer authors — so a frozen writer's own gate pass must never attribute it
// to the writer as scope drift. Deliberately narrow per CA-427 Finding 2: only
// this exact one-level *.json shape is exempt — nested paths, non-.json
// files, and every other .flowpilot/** path (notably
// .flowpilot/settings/flow-rules.json) stay fully subject to enforcement.
func IsCanonicalHeadStorePath(p string) bool {
	np := normalizeScopePath(p)
	dir, file := path.Split(np)
	if dir != ".flowpilot/canonical/" {
		return false
	}
	if file == "" || strings.Contains(file, "/") {
		return false
	}
	return strings.HasSuffix(strings.ToLower(file), ".json")
}

// LegacyContractsStorePath is the exact legacy change-contract store file
// (Store, contract.go) FlowPilot itself writes on gate passes for non-frozen
// writers. A frozen writer's gate diff can still observe it (written by an
// earlier turn's commitChangeContract into the same workspace), and it is
// runner-owned bookkeeping — never the current writer's drift.
func LegacyContractsStorePath() string {
	return path.Join(".flowpilot", "contracts", "contracts.ndjson")
}

// IsLegacyContractsStorePath reports whether p is exactly
// LegacyContractsStorePath (after normalization). Only that one file — a
// sibling like forged.ndjson still drifts (CA-427 Finding 2).
func IsLegacyContractsStorePath(p string) bool {
	return normalizeScopePath(p) == LegacyContractsStorePath()
}

// ToolOwnedScaffoldPaths returns the repo-relative path prefixes and exact
// root files owned by tool/skill-pack installers (CA-645/CA-648), NOT by the
// flow writer: `.claude/**`, `.agents/**`, `.grok/**` agent skill dirs plus
// the root `AGENTS.md`/`CLAUDE.md`/`.gitignore` scaffold. Skillpack install
// and desktop skill sync write these mid-flow (run-151954: 9 files at
// 06:58:50, 17s after flow start), so both the freeze planner-mutation guard
// and the coder's frozen-scope drift gate must never attribute them to the
// writer. Deliberately NOT `.flowpilot/**` or `.gitnexus/**` — those stay
// subject to the same per-path rules CA-427/CA-640 already established.
func ToolOwnedScaffoldPaths() []string {
	return []string{
		".claude", ".agents", ".grok",
		"AGENTS.md", "CLAUDE.md", ".gitignore",
	}
}

// IsToolOwnedScaffoldPath reports whether p is a tool/skill-pack owned
// scaffold path — one of ToolOwnedScaffoldPaths (path prefix) — after the
// same normalization FrozenContractScopeDrift applies to every path it
// compares, so `.claude/x`, `./claude/x`, `.\\claude\\x` and `claude/x` all
// resolve to the same normalized form.
func IsToolOwnedScaffoldPath(p string) bool {
	np := normalizeScopePath(p)
	if np == "" {
		return false
	}
	for _, s := range ToolOwnedScaffoldPaths() {
		ns := normalizeScopePath(s)
		if np == ns || strings.HasPrefix(np, ns+"/") {
			return true
		}
	}
	return false
}

// FrozenContractScopeDrift returns the paths in writtenPaths that are not
// among rec.DeclaredPaths or rec.AllowedExtraPaths — what a Flow writer
// touched beyond what its frozen contract authorized. Both sides are
// forward-slash normalized,
// path.Clean'd and trimmed before comparison (matching NormalizeDeclaredCodePaths'
// own normalization, so a written path reported in a different but
// equivalent form — "./src/calc.go" vs "src/calc.go" — does not
// false-positive as drift); blank entries in writtenPaths are ignored. A nil
// result means the writer stayed entirely within its declared scope.
// Deduplicated and sorted for a deterministic, reproducible report.
func FrozenContractScopeDrift(rec FrozenContractRecord, writtenPaths []string) []string {
	declared := make(map[string]bool, len(rec.DeclaredPaths)+len(rec.AllowedExtraPaths))
	for _, p := range rec.DeclaredPaths {
		declared[normalizeScopePath(p)] = true
	}
	for _, p := range rec.AllowedExtraPaths {
		if np := normalizeScopePath(p); np != "" {
			declared[np] = true
		}
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
	var concrete []string
	for _, p := range additionalPaths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		if !IsConcreteCodeTarget(normalizeScopePath(trimmed)) {
			return FrozenContractRecord{}, fmt.Errorf("changecontract: amendment path %q is not a concrete code target and cannot widen scope", trimmed)
		}
		concrete = append(concrete, trimmed)
	}
	return amendFrozenContractUnion(store, workspace, existing, concrete, nil, now)
}

// AmendFrozenContractForAllow is the Task-309 Allow path: widen frozen scope
// to match files the coder actually wrote. Concrete code targets go into
// DeclaredPaths (same as AmendFrozenContract). Specific doc/audit files the
// gate reported as drift (change-audit/FEATURE-KEYS.md, other *.md) go into
// AllowedExtraPaths so retrieval-locus stays code-only while the next gate
// pass does not re-park. Globs, flags, and extension-less buckets still
// return the CA-427 Finding 5 explicit error — they cannot be a git-diff
// written file the operator is Allowing.
func AmendFrozenContractForAllow(store *FrozenStore, workspace string, existing FrozenContractRecord, additionalPaths []string, now time.Time) (FrozenContractRecord, error) {
	var concrete, extras []string
	for _, p := range additionalPaths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		np := normalizeScopePath(trimmed)
		if IsConcreteCodeTarget(np) {
			concrete = append(concrete, trimmed)
			continue
		}
		if IsUserAllowableDriftPath(np) {
			cleaned, err := normalizeAllowableExtraPath(workspace, trimmed)
			if err != nil {
				return FrozenContractRecord{}, err
			}
			if cleaned != "" {
				extras = append(extras, cleaned)
			}
			continue
		}
		return FrozenContractRecord{}, fmt.Errorf("changecontract: amendment path %q is not a concrete code target and cannot widen scope", trimmed)
	}
	return amendFrozenContractUnion(store, workspace, existing, concrete, extras, now)
}

func amendFrozenContractUnion(store *FrozenStore, workspace string, existing FrozenContractRecord, additionalConcrete, additionalExtras []string, now time.Time) (FrozenContractRecord, error) {
	existingNormalized := uniqueNormalizedPaths(existing.DeclaredPaths)
	newConcrete := []string(nil)
	if len(additionalConcrete) > 0 {
		added, err := NormalizeDeclaredCodePaths(workspace, additionalConcrete)
		if err != nil {
			return FrozenContractRecord{}, err
		}
		newConcrete = added
	}
	normalized := uniqueNormalizedPaths(append(append([]string(nil), existingNormalized...), newConcrete...))
	extraUnion := uniqueNormalizedPaths(append(append([]string(nil), existing.AllowedExtraPaths...), additionalExtras...))
	if sameStringSet(normalized, existingNormalized) && sameStringSet(extraUnion, uniqueNormalizedPaths(existing.AllowedExtraPaths)) {
		return existing, nil
	}

	draft := PreflightContractDraft{
		FeatureKey:    existing.FeatureKey,
		Intent:        existing.Intent,
		DeclaredPaths: normalized,
		SourceDocID:   existing.SourceDocID,
	}
	id := ComputeContractID(existing.RunID, existing.CoderStepID, existing.Version+1, draft, existing.BaseSHA, existing.BaselineWorktree)
	amended := FrozenContractRecord{
		ContractID:        id,
		Version:           existing.Version + 1,
		RunID:             existing.RunID,
		PlannerStepID:     existing.PlannerStepID,
		CoderStepID:       existing.CoderStepID,
		FeatureKey:        draft.FeatureKey,
		Intent:            draft.Intent,
		DeclaredPaths:     normalized,
		AllowedExtraPaths: extraUnion,
		SourceDocID:       draft.SourceDocID,
		BaseSHA:           existing.BaseSHA,
		BaselineWorktree:  existing.BaselineWorktree,
		Supersedes:        existing.ContractID,
		DeclaredAt:        now,
	}
	if err := store.SaveFrozen(amended); err != nil {
		return FrozenContractRecord{}, err
	}
	if err := store.AppendStatus(existing.ContractID, ContractStatusSuperseded, "amended: scope widened", now); err != nil {
		return FrozenContractRecord{}, err
	}
	return amended, nil
}

func uniqueNormalizedPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		np := normalizeScopePath(p)
		if np == "" || seen[np] {
			continue
		}
		seen[np] = true
		out = append(out, np)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeAllowableExtraPath(workspace, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	forward := strings.ReplaceAll(trimmed, `\`, `/`)
	native := filepath.Clean(filepath.FromSlash(forward))
	if filepath.IsAbs(native) {
		if strings.TrimSpace(workspace) == "" {
			return "", fmt.Errorf("changecontract: declared path %q is absolute but no workspace was given", raw)
		}
		absWorkspace, err := filepath.Abs(workspace)
		if err != nil {
			return "", fmt.Errorf("changecontract: resolve workspace %q: %w", workspace, err)
		}
		rel, err := filepath.Rel(absWorkspace, native)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("changecontract: amendment path %q escapes workspace %q", raw, workspace)
		}
		native = rel
	}
	p := path.Clean(filepath.ToSlash(native))
	if p == "" || p == "." {
		return "", nil
	}
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("changecontract: amendment path %q escapes workspace", raw)
	}
	return p, nil
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
