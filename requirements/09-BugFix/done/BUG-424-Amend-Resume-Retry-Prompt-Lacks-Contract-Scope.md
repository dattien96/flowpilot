# BUG-424: Amend-resume retry prompt does not re-inject the widened frozen contract scope

## Metadata

- Document ID: `BUG-424`
- Title: `Post-amend implement re-spawn prompt is a generic retry — no change.contract / Scope block tells the writer which paths the amendment allowed`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-55-Test-Steps](../../07-Coding-Plan/done/CP-55-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp55/RESULT.md` (BUG-LIVE-2)
- Feature Keys: `frozen-contract`, `amend`, `flow-resume`, `coder-prompt`

## AI Quick View

### Summary

- After a scope-drift block is resolved via `POST /agent-loop/amend`, the re-spawned implement child's prompt is `[flow-engine] Retrying failed delegate after user Continue.` + generic implement instructions — no `### change.contract` / `Scope (do not edit outside)` block.
- The gate still enforces the amended contract correctly (the turn passed), but the writer is never told which new paths the amendment allowed — a coder needing the amended path may keep avoiding it.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

### Symptom

Post-amend implement child `run-3822`'s prompt lacks the contract scope block that the original spawn `run-3811` carried (v1 scope list `calc.go, calc_divide_fastpath_test.go` / v3 unioned `[calc_multiply_test.go, scope_drift_probe.go]`).

### Expected

The retry prompt re-injects the current (amended) frozen contract scope — same `### change.contract` block as the original writer spawn — so the writer knows the newly-permitted paths.

### Actual

Generic retry text only. Enforcement is intact (gate evaluated the v3 contract and the turn passed), so this is a prompt-completeness gap, not an enforcement gap.

### Impact

A coder that was blocked for writing `X` and whose block was resolved by amending `X` into scope is never informed `X` is now allowed — it may avoid the path again, producing an incomplete fix or another drift loop.

## Reproduction

1. Bug-harness run; during `implement`, inject/have the writer produce a file outside `declared_paths` → `flow gate block: flow scope drift` → parked `blocked/escalate`.
2. `POST /agent-loop/amend {"paths":["<file>"]}` → new contract version minted (supersedes chain intact), flow resumes.
3. Inspect the re-spawned implement child's prompt (`chats/run-<child>-turns.ndjson`) → no `### change.contract` block (contrast with the original spawn's prompt).

## Root cause

- `apps/local-runner/internal/runner/interactive_service.go:2054` — `resumeFlowWithFeedback` → coder re-spawn path builds a bare retry prompt (`[flow-engine] Retrying failed delegate after user Continue.` + generic instructions) and does not re-render the `change.contract` scope block that the original writer spawn path includes.

## Evidence

- `~/fp-beds/lt-evidence/cp55/RESULT.md` — §"Bugs found" BUG-LIVE-2: `run-3193/chats/run-3822-turns.ndjson` (retry prompt) vs `run-3193/chats/run-3811-turns.ndjson` (original spawn with v1 scope list).
- Context: run-3193 grok bug-harness; drift block on `scope_drift_probe.go`; amend minted v3 `eb175217` (supersedes v2 `2ce0320a`, declared union `[calc_multiply_test.go, scope_drift_probe.go]`); flow then completed `flow_run_complete_done`.

## Severity

- `low` — enforcement intact; prompt completeness gap only.

## Completion Notes (implemented 2026-09-23, CA-921)

- Root cause: `resumeFlowWithFeedback` delegate-respawn prompts carried the user's amended-paths feedback but not the frozen change-contract scope block — the retried writer never learned the declared paths.
- Fix: all three retry prompts (reinvoke failed child, fresh respawn, missing change-audit writer retry) go through `appendChangeContractIfAnyWithSecret` so the `flowpilot-cc:` scope block is injected.
- Files: `internal/runner/interactive_service.go`.
- Tests: `TestBug424_ResumeRetryPromptCarriesContractScope` — asserts the contract marker block (not just the filename in feedback). Baseline-red verified.
