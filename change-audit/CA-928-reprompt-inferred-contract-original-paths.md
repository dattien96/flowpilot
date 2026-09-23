---
id: CA-928
title: Reprompt-turn inferred contracts now derive from the failing turn's carried code paths (BUG-425)
type: BugFix
feature: change-contract
date: 2026-09-23
status: done
---

## Context

BUG-425 (BUG-LIVE-CP43-001): when a turn failed `r-ca`/`r-contract` and was
reprompted, the committed inferred contract was built from the **reprompt
turn's own incremental diff** — which contains only the remediation delta,
usually just the `change-audit/*.md` note. `startTurn` re-captures
`turnStartGitHead` on every turn, so the reprompt turn's `diff` legitimately
holds no code paths. `InferFromDiff` skips doc/audit files, producing rows
like `{"feature_key":"claude","intent":"","confidence":"inferred"}` with no
`declared_paths` — useless for scope gating and poisonous to the feature
catalog (the bogus `claude` feature with whole-repo globs later hijacked
canonical-head injection, BUG-417).

## Root cause

- Root gate (`runFlowGateAtEpoch`) never populated nor consulted
  `pendingGateCodePaths` — the durable per-run carrier the child gate had
  used since BUG-288 #8 to remember a gate-failing coding turn's paths.
- `prepareChangeContract` + `suggestFeatureKeys` consumed only the current
  turn's `diff`, so inference on the reprompt turn saw an empty code set.
- Child gate had the same inference gap: `pendingPaths` forced the
  `isCodingChild` re-check but was never merged into the diff fed to
  `prepareChangeContract`, and the stash *replaced* the carrier so chained
  reprompts lost earlier paths.

## Change

`internal/runner/gate_hook.go` only:

- new `mergeCarriedPathsIntoDiff(diff, carried)` — returns a copy of the
  turn diff with absent carried paths appended as `ChangedFile{Status:"M"}`;
  slash-normalizes, trims, dedupes. Used **only** for contract inference /
  scope (`prepareChangeContract`) — `tr.GitDiff` stays strictly turn-scoped
  so `r-tests`/`r-reg`/`HasCodeChanges` still evaluate only the current
  turn's delta.
- Root gate: reads `rs.pendingGateCodePaths` under `s.mu`; unions it into
  `changedPaths` (feature-key suggestion + `tr.ChangedPaths` telemetry) and
  merges it into the diff passed to `prepareChangeContract`.
- Root reprompt branch: stashes `tr.WrittenPaths` (fallback `changedPaths`)
  onto `pendingGateCodePaths` via `appendUniqueStrings`, so the queued
  reprompt turn inherits the failing turn's real paths.
- Root clean-pass and warn/approve paths: clear `pendingGateCodePaths`
  alongside the `repromptAttempts` reset so carried paths cannot leak into
  an unrelated later turn.
- Child gate: same merge for `changedPaths`/`suggestFeatureKeys`/
  `prepareChangeContract` and `tr.ChangedPaths`; the reprompt stash now
  **unions** instead of replacing, preserving original paths across chained
  reprompts.

Durability: `pendingGateCodePaths` was already serialized
(`local_file_session_store.go`) and rehydrated on resume
(`interactive_resume.go`), so the carry survives runner restart and
pending-reprompt replay with no schema change.

## Tests

`internal/runner/bug425_gate_reprompt_contract_paths_test.go` (additive):

- `TestBug425MergeCarriedPathsIntoDiff` — merge semantics: dedupe,
  blank/whitespace skip, empty-carry no-op.
- `TestBug425GateRepromptCarriesFailingTurnCodePaths` — enforce-mode gate:
  a coding turn with no audit note resolves to `reprompt` and stashes
  `src/calc.go` onto `pendingGateCodePaths`.
- `TestBug425RepromptGateInfersFromCarriedCodePaths` — the live scenario:
  seeded carry + reprompt turn whose diff holds only
  `change-audit/CA-999-bug425.md` → committed contract has
  `declared_paths` covering `src` and `feature_key="calc-core"` resolved
  from the carried path via catalog glob + FEATURE-KEYS.md registry;
  carry cleared after the pass.

## Provider parity

Provider-agnostic by construction: the change sits in the shared
post-finalize gate (`runFlowGateAtEpoch` / `runChildArtifactOutputGateAtEpoch`),
consuming `fin.ChangedFiles`/`diff` — both produced by provider-neutral
worktree observation. The Devin `WrittenPaths`-empty case is covered: the
stash falls back to `changedPathsFromDiff(diff)`.

## Known baseline failures

Unrelated pre-existing set (16 runner failures identical on baseline +
documented flakes); see cluster notes in CP-Test-Progress-Tracking.md.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: BUG-425
change_type: bugfix
summary: Reprompt-turn inferred contract commits original-turn paths via durable pendingGateCodePaths carry
# --->8---
