# CA-232: Flow Step Posture Resolved At Seed Time

## Summary

Follow-up to `CA-231`. The user's next live screenshot showed the SAME stale `claude-haiku` for the reviewer steps — but this time for rows still in `PENDING` status. `CA-231`'s `stampFlowNodePosture` only patches a node's posture once it actually spawns (RUNNING); a `PENDING` row — which is what most steps show for most of a run's lifetime, since each waits its turn — never had `Provider`/`Model` set at all and fell back to the run's baseline. Fixed by resolving every row's posture at seed time: extracted `resolveFlowNodeProviderModel` out of `stampFlowNodePosture` and reused it inside `flowStepRowsFromNodes` (now a method on `InteractiveService`), so every step row — `PENDING`, `RUNNING`, or `DONE` — carries its correct eventual posture from the moment the flow starts, not only once that specific node has been spawned.

**Critical finding during verification**: this change introduced a real deadlock. `flowStepRowsFromNodes` now calls `resolveFlowNodeProviderModel`, which acquires `s.mu`. `reconstructRun` (`interactive_resume.go`, the resume-from-restart path) called `reseedFlowStepRuntimeForResume` — which reaches `flowStepRowsFromNodes` — **while still holding `s.mu.Lock()`**. `InteractiveService.mu` is a plain, non-reentrant `sync.Mutex`, so any resumed flow-engine-driven run (a real production scenario: local-runner restarts with an in-flight flow run) hung forever. The first full-suite regression run after this change confirmed it concretely: the suite that normally completes ~1058 tests instead stalled at ~472 and was killed by `go test`'s default 10-minute timeout, dumping dozens of goroutines parked on `chan receive` for 9 minutes. Fixed by moving `s.mu.Unlock()` earlier in `reconstructRun`, before the reseed call, instead of after it.

## What Changed

- `apps/local-runner/internal/runner/flow_executor.go`:
  - Extracted `resolveFlowNodeProviderModel(ctx, parentRunID, node) (provider, model string)` out of `stampFlowNodePosture` (same resolution logic, now reusable).
  - `stampFlowNodePosture` now just calls `resolveFlowNodeProviderModel` + `setFlowStepPosture`.
- `apps/local-runner/internal/runner/flow_step_runtime.go`:
  - `flowStepRowsFromNodes` converted from a free function to a method `(s *InteractiveService) flowStepRowsFromNodes(ctx, parentRunID, nodes, status, ts)`, calling `resolveFlowNodeProviderModel` per node and setting `Provider`/`Model` on each seeded row.
  - Updated `reseedFlowStepRuntime`/`reseedFlowStepRuntimeForResume` to call the new method form.
- `apps/local-runner/internal/runner/interactive_resume.go` (**deadlock fix**):
  - Moved `s.mu.Unlock()` to immediately after `s.runs[rs.id] = rs`, before the chat-step-seed / `reseedFlowStepRuntimeForResume` branch — both branches only need `s.runs[rs.id]` to already be visible, not the lock held throughout.
- `apps/local-runner/internal/runner/workflow_step_runtime_test.go`:
  - Updated `TestWorkflowStepsRuntimeReflectsPerNodeModelOverride` to assert the reviewer rows' posture is correct immediately after `startResolvedFlow` returns (at whatever status they happen to be in — the fake adapter completes synchronously, so asserting a specific `PENDING` status at that exact instant is itself racy and unrelated to the production behavior under test), and again once the wait-loop confirms the cohort has left `PENDING`.

## Verification

- `go build ./...` and `go vet ./internal/runner/...` in `apps/local-runner` — both clean.
- Deadlock confirmed and fixed: before the `interactive_resume.go` fix, `go test ./internal/runner/...` reliably stalled at ~472/492 tests and hit the 10-minute default timeout (reproduced 3x). After the fix, reliably completes in ~90-100s at 1058 passed, 15 failed (identical pre-existing, environment-specific failure set already established as this session's baseline), 14 skipped — reproduced 4x with `-timeout 120s`/`180s` to positively confirm no hang, not just a lucky fast run.
- `TestReconstructRunRestoresTrackedFlowTopology`/`TestReconstructRunPreservesUpdatedAt` (the tests that exercise the exact `reconstructRun` deadlock path) — both pass.
- `TestWorkflowStepsRuntimeReflectsPerNodeModelOverride`, `TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel`, `TestStartResolvedFlow*`, `TestCoderCompletionAutoSpawnsReviewerCohort` re-run together 15x (`-count=15`) — 0 failures. The test's earlier draft (asserting a specific `PENDING` status at the first checkpoint) had flaked ~1/3 of the time before being corrected to check posture regardless of status — unrelated to the deadlock, a separate test-only timing assumption.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-183
change_type: bugfix
summary: Resolve each flow step's posture at seed time (not just once spawned) so PENDING rows show their correct eventual model/provider from the start
# --->8---
