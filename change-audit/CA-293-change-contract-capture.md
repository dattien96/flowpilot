# CA-293 — Change Contract Capture (Task-184, CP-43 P-1)

## Scope

Implemented [Task-184](../requirements/08-Task/done/Task-184-Change-Contract-Capture.md) (P-1 of [CP-43](../requirements/07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md)): every code-mutating turn now has a stored **Change Contract** — the feature/intent/files the AI declared (or, absent a declaration, inferred from its diff) — so a future rule (Task-185) can flag edits that land outside it.

## Changes

- New module `apps/local-runner/internal/changecontract/`: `contract.go` (`Contract` struct + NDJSON `Store` at `.flowpilot/contracts/contracts.ndjson`, mutex-guarded, last-wins by `(run_id, step_id)`, mirrors `local_file_session_store.go`'s idiom), `parse.go` (`ParseDeclaration` — extracts a `[Change Contract]` block, tolerant of missing/partial lines, `ok=false` when absent), `infer.go` (`InferFromDiff` — buckets non-doc changed files by top-level dir via `flowgate.IsDocOrAuditFile`).
- `gate_hook.go`: new `captureChangeContract` function, called from `runFlowGate` right after the diff/`suggestedFeatureKeys` are observed. **Design deviation from Task-184's original wording:** the doc specified a "`contract.declare` resolver slot" — that predates CP-44's `ContextSourceRegistry` and doesn't map onto it (a `ContextSource.Fetch` fetches content to inject; it can't parse the AI's own already-generated message). Implemented as a direct post-`finishTurn` gate-hook call instead — same non-fatal, always-capture behavior, just timed after the turn (when the AI's `FinalMessage` actually exists to parse) rather than before it.
- `context-discipline` skill (`internal/skillpack/flow-pack/common/context-discipline/SKILL.md`): new rule 6 instructing the AI to emit a `[Change Contract]` block before editing.
- `skillpack.PackVersion` bumped 5→6, and all 16 other skill files' own `version:` field bumped in lockstep — `fileMatchesVersion` compares every installed file against the single pack-wide constant, so a partial bump would have left every other skill perpetually "stale" on re-bind.

## Verification

- `go build ./...` clean; `go vet ./internal/changecontract/... ./internal/runner/... ./internal/skillpack/...` clean.
- `go test ./internal/changecontract/...` — 15 passed (parse: full/fenced/partial/absent/case-insensitive; store: round-trip, last-wins, reopen-from-disk, file path; infer: bucketing, doc-exclusion, empty diff).
- `go test ./internal/skillpack/...` — 13 passed (was failing 2/13 before the lockstep version bump — `TestInstall_SkipsExistingSameVersionFiles`/`TestStatus_CurrentAfterInstall` — fixed by bumping all 17 skill files together, not just `context-discipline`).
- `go test ./internal/runner/... -count=1` — 1386 passed, 16 failed (all pre-existing environment-dependent flakes — Codex CLI resume, account-home, skills-merge, auth-workspace — identical to this session's established baseline), 18 skipped. Zero new failures.
- Confirmed by inspection (not code change needed): `contracts.ndjson` stays local-only — `contextsync/local.go` uses an explicit allowlist of subdirs/files that does not include the new `contracts/` directory.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-184
change_type: feature
summary: every code-mutating turn now captures a Change Contract (declared from a [Change Contract] block in the AI's message, or inferred from its diff) into a local-only contracts.ndjson store, laying the groundwork for Task-185's scope-drift detection; also fixes the context-discipline skill pack version bump to apply lockstep across all 17 skills instead of just one
# --->8---
