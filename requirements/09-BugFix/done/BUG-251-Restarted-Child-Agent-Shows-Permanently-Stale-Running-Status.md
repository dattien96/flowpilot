# BUG-251: Restarted Child Agent Shows Permanently Stale "Running" Status

## Metadata

- Document ID: `BUG-251`
- Title: `Restarted Child Agent Shows Permanently Stale "Running" Status`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-07`
- Last Updated: `2026-07-07`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 5), [BUG-248: Stop Leaves Main Run Permanently Stuck Running](./BUG-248-Stop-Leaves-Main-Run-Permanently-Stuck-Running.md) (same symptom class, different trigger)
- Child Documents: `none`
- Related Documents: [BUG-250: Restarted Flow Hub Run Permanently Unresumable — Placeholder Session](./BUG-250-Restarted-Flow-Hub-Run-Permanently-Unresumable-Placeholder-Session.md) (found in the same live-testing pass, immediately after BUG-250 was fixed), [CA-249: Normalize Stale Child Agent Status In Restart Disk Fallback](../../../change-audit/CA-249-normalize-stale-child-status-after-restart.md)
- Replaces: `none`
- Tags: `agent-flow-engine, resume, restart, run-status, regression, agents-panel`

## AI Quick View

### Summary

- Found immediately after fixing BUG-250, in the same CP-36 Scenario 5 live-testing pass: once the hub chat could be reopened again post-restart, the Agents panel showed both reviewer children as `running` — even though the underlying `claude` CLI subprocesses for those reviewers had unquestionably been killed along with the whole runner process (confirmed via a live process listing: no `flowpilot.exe runner serve` process at all, and no orphaned `claude`/`codex` CLI process for the review-loop's working directory).
- Root cause: `listAgentRunSummaries`'s disk-fallback branch (used when a child is in neither the live in-memory run map nor the orchestrator's in-memory historical cache — exactly the state right after a restart) read `session.Status`/`session.AgentStatus` straight from the persisted store with **no normalization** at all. A sibling code path, `reconstructRun`, already has and correctly applies `normalizeResumedStatus` (rewrites `running`/`starting`/`waiting_*` → `cancelled` for anything rebuilt from disk, since nothing is actually executing it in this process instance) — this fallback branch simply never called it.
- Confirmed live: no `flowpilot.exe` runner process and no orphaned provider CLI process existed on the machine at all when the panel still showed "running" for both reviewer children.

### Current Ask

- Apply the same restart-time status normalization already used for a run's own top-level status (`reconstructRun`/`normalizeResumedStatus`) to the disk-fallback branch of the children list, so a restarted child agent reads as `cancelled`, not a permanently stale `running`.

### Key Decisions

- `V-1` Reuse `normalizeResumedStatus` verbatim rather than inventing a second normalization rule — it already encodes exactly the right semantic ("running/starting/waiting_* read from disk with nothing live behind them → cancelled") and is already proven correct by `reconstructRun`'s existing use.
- `V-2` Apply the same function to both `Status` (a `RunStatus`) and `AgentStatus` (a plain string that happens to reuse the same constant values for the in-flight states) via a cast, rather than adding a parallel string-based rule.
- `V-3` Scoped to exactly the values `normalizeResumedStatus` already recognizes (`running`, `starting`, `waiting_approval`, `waiting_question`). Other `AgentStatus`-only values (`spawned`, `waiting_dependency`) are left untouched — they are arguably also stale after a restart, but normalizing them is a broader behavior change than this bug's reported symptom and is left as a candidate follow-up, not silently bundled in here.

### Constraints

- Scoped to `listAgentRunSummaries`'s `SessionIndexReader` disk-fallback loop; the live in-memory branch (`liveIDs`) and the orchestrator's historical-cache branch are untouched — they already reflect real in-process state.
- Does not change `reconstructRun`, `normalizeResumedStatus`, or any other caller of it.

### Open Questions

- Should `AgentStatus` values `spawned`/`waiting_dependency` also normalize to `cancelled` after a restart? Left open — not part of this bug's reported symptom (both observed stale statuses were `running`).

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` `listAgentRunSummaries` (~line 2582, the fixed disk-fallback branch ~line 2619), `normalizeResumedStatus` (`interactive_resume.go:339`, the pre-existing function reused here), `reconstructRun` (`interactive_resume.go:348`, the sibling call site that already applied it correctly).
- Owner-provided evidence: live screenshots of the Agents panel showing both reviewer children as "running" post-restart-and-reopen, cross-checked against a live Windows process listing (`Get-CimInstance Win32_Process`) showing zero `flowpilot.exe runner serve` processes and zero orphaned `claude`/`codex` CLI processes for the review-loop's working directory.

## 1. Issue Summary

Immediately after BUG-250 made it possible to reopen a restarted flow hub's chat again, the owner observed the Agents panel still showing both reviewer children as `running`, even though the runner process (and everything it had spawned) had been killed and restarted. This was the very next symptom found in the same CP-36 Scenario 5 live-testing pass.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 5 — found as a follow-on to BUG-250 during the same restart-mid-loop repro.
- same symptom class as [BUG-248](./BUG-248-Stop-Leaves-Main-Run-Permanently-Stuck-Running.md) (a run/child stuck showing "running" after the thing driving it has actually stopped), but a materially different trigger and code path: BUG-248 was a synchronous-vs-async race in the explicit Stop button's handler (`stopAgentLoop`); this bug is a missing normalization step in the restart/rebuild-from-disk read path (`listAgentRunSummaries`), which BUG-248's fix never touched.

## 3. Environment and Reproduction

- environment: Desktop app + local-runner against a real project, Review Loop flow, Claude provider.
- reproduction steps:
  1. Start a Review Loop; let reviewers spawn and start running.
  2. Kill and restart the local-runner process (as in BUG-250's repro).
  3. Reopen the hub chat (now possible after the BUG-250 fix).
  4. Look at the Agents panel's "Sub-agents in this session" list.
- frequency: deterministic for any child agent whose run was mid-turn (`running`/`starting`/`waiting_*`) when the process was killed.

## 4. Expected vs Actual

- expected: a child agent whose backing process was killed by the restart reads as `cancelled` (or otherwise clearly non-live), matching how the top-level hub run's own status is already normalized on the same kind of restart.
- actual: the child's badge stayed `running` indefinitely, with no path to ever correct itself, even though the underlying CLI process was confirmed dead.

## 5. Impact

- users affected: anyone restarting the runner while any flow (Review Loop or a custom flow) has spawned child agents that are actively running.
- workflows affected: the Agents panel / Board view's live-status display; does not affect the flow engine's own actual execution or eventual reinvoke.
- severity: medium — misleading UI state (a permanently "running" agent that will never finish or be actionable) rather than data loss or execution incorrectness.

## 6. Root Cause

- confirmed cause: `listAgentRunSummaries` (`interactive_service.go:2582`) builds a child's `AgentRunSummary` from three sources in order: the live in-memory map, the orchestrator's in-memory historical cache, then (for anything still unseen) a raw read via `SessionIndexReader.ListAllProviderSessions` straight from the persisted store. Right after a restart, the first two are empty for every child (nothing has been reopened or touched yet), so every child comes from the third, disk-backed branch — which assigned `Status: session.Status` and `AgentStatus: session.AgentStatus` verbatim, with no normalization. `reconstructRun` (used when the run itself is directly resumed) already calls `normalizeResumedStatus` for exactly this reason; `listAgentRunSummaries`'s disk-fallback branch simply never called it.
- evidence: a live Windows process listing at the time of the report showed zero `flowpilot.exe runner serve` processes and zero orphaned `claude`/`codex` CLI processes for the review-loop's working directory — conclusively ruling out "the reviewer process is genuinely still alive" as an alternative explanation.

## 7. Fix Strategy

- `F-1` In `listAgentRunSummaries`'s disk-fallback branch, wrap `session.Status` in `normalizeResumedStatus(...)` before assigning it to `AgentRunSummary.Status`.
- `F-2` Apply the same function to `session.AgentStatus` via `string(normalizeResumedStatus(RunStatus(session.AgentStatus)))`, since `AgentStatus` reuses the identical string constants for the in-flight states this bug is about.

## 8. Validation

- `V-1` `go build ./...` — clean. `go vet ./internal/runner/` — clean.
- `V-2` New test `TestListAgentRunSummariesNormalizesStaleRunningStatusAfterRestart`: seeds a parent + a `running` child directly into a fake `WorkflowStore` (nothing in `s.runs`, a fresh `agentOrchestrator` — exactly the post-restart state), calls `listAgentRunSummaries`, asserts both `Status` and `AgentStatus` come back `cancelled`. Confirmed the test actually catches the bug: reverting the two `normalizeResumedStatus` calls back to the raw field reproduces the failure (`Status = "running", want "cancelled"`); restoring the fix passes it again.
- `V-3` `go test ./internal/runner/` (full package, `-count=1`): 15 pre-existing environment-specific failures plus the already-confirmed-flaky `TestProjectRunHistoryFiltersRunsByProject` (passes in isolation and on a clean rerun; established as unrelated pre-existing test-order flakiness during BUG-250's validation in this same session) — no new regressions.
- `V-4` Not performed: a live restart-and-reopen round trip against the owner's actual project. The rebuilt local-runner binary (`bin/flowpilot.exe`) was recompiled with this fix; the owner should retry the same Scenario 5 sequence to confirm the reviewer children now show `cancelled` instead of `running` after reopening.

## 9. Regression Guard

- tests: `TestListAgentRunSummariesNormalizesStaleRunningStatusAfterRestart` (`interactive_service_test.go`).
- audit checks: `gitnexus_detect_changes()` was not run — GitNexus MCP tools were unavailable in this thread; proceeded via direct code inspection per the `add-new-bug` skill's fallback instruction.

## 10. Follow-Up Document Updates

- upstream docs that must change: [CP-36 Scenario 5](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) gets a note recording this second finding from the same live pass.
- notes left unchanged on purpose: the open question above (whether `spawned`/`waiting_dependency` should also normalize) is deliberately left as a candidate follow-up, not expanded into this fix's scope.
