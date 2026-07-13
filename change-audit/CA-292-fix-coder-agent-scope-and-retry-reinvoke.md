# CA-292 — Coder Agent Stays In Scope; Validate-Retry Reinvokes Instead Of Respawning

## Scope

Implemented [BUG-278](../requirements/09-BugFix/done/BUG-278-Flow-Mode-Coding-Step-Agent-Autonomously-Commits-And-Writes-Audit-Notes.md) and [BUG-279](../requirements/09-BugFix/done/BUG-279-Flow-Mode-Validate-Retry-Ignores-Implement-Node-Reinvoke-Lifecycle.md), both found during live manual re-verification of [CP-41](../requirements/07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md)'s remaining Manual E2E scenarios in `D:\working\gate-sandbox`.

- BUG-278: the RAG Harness Coding-step agent (`agents/coder.md`) was observed running real `git commit`s on its own, and reverting/deleting files entirely outside its own task's scope (a test's validation config, backup files, and — most seriously — three unrelated pre-existing `change-audit/CA-*.md` files from an earlier session) — none of this gated by any approval step. Writing its own change-audit note was *not* part of the bug (SD-20 `r-ca` already requires that in any mode).
- BUG-279: the validate-retry back-edge (`validate --continue--> implement`) always spawned a brand-new Coding-step child run/provider session, ignoring the `implement` node's declared `lifecycle: reinvoke` — unlike the forward-edge auto-advance path, which already honored it. Confirmed via `sessions.ndjson`: two different `provider_session_id`s across one retry cycle.

## Changes

- `agents/coder.md`: added an explicit scope guard — the coder may still write its own `change-audit/*.md` note (unaffected), but must not run `git commit` itself (that stays the Audit step's job, gated on approval) and must not touch/revert/delete any file outside its own task, including anything under `.flowpilot/` that "looks wrong." Also forbids editing the validation command/config to force a failing check to pass instead of fixing production code.
- `flow_validate_audit_dispatch.go`: the `"retrying"` case now checks `flowNodeReusesChild(targetNode)` and calls `s.reinvokeExistingFlowChild(...)` first (mirroring `flow_executor.go`'s forward-edge path), falling back to the existing `spawnChildRun` only when the node isn't `lifecycle: reinvoke` or no matching non-terminal child exists.
- Tests: `TestCoderAgentPromptForbidsUnapprovedCommitsAndOutOfScopeFileChanges` (`pack_test.go`) and `TestTryAdvanceFlowFromNodeValidateRetryReinvokesLifecycleReinvokeTarget` (`flow_validate_audit_dispatch_test.go`).

## Verification

- `go build ./...` clean; `go vet ./internal/runner/... ./internal/agentpack/...` clean.
- `go test ./internal/runner/... ./internal/agentpack/...` — 1401 passed, 17 failed (all pre-existing environment-dependent flakes — Codex CLI resume, account-home, skills-merge, auth-workspace, plus one confirmed-flaky `TestProjectRunHistoryFiltersRunsByProject`), 18 skipped — zero new failures.
- Live re-verified in `D:\working\gate-sandbox`: BUG-278 (`run-4286`/`run-4291` — CA note written, no commit, no out-of-scope file touched); BUG-279 folded into re-running CP-41 Scenario 6 end-to-end (`run-4505` — 3 real validate failures, clean `retryAttempt: 1→2→3`, `failed_validation_max_retries` escalation, no coder self-heal of the deliberately-broken validation config).
- CP-41 closed (`inprogress/` → `done/`) as a result of this pass — all remaining Manual E2E scenarios (5, 6, 7, 9, 10) and DOD-7 are now checked off with live evidence.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-278
change_type: bugfix
summary: the Flow Mode Coding-step agent no longer commits or touches out-of-scope files on its own (still writes its own change-audit note, per r-ca), and the validate-retry back-edge now reuses the existing child session per the target node's lifecycle:reinvoke declaration instead of always spawning a new one — closes out CP-41's remaining manual E2E verification
# --->8---
