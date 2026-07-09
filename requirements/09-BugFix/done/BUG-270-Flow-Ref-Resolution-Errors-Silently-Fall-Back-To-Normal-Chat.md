# BUG-270: Flow-Ref Resolution Errors Silently Fall Back To Normal Chat

## Metadata

- Document ID: `BUG-270`
- Title: `Flow-Ref Resolution Errors Silently Fall Back To Normal Chat`
- Phase: `bugfix`
- Status: `done` — fixed and verified 2026-07-09, found live re-testing BUG-269's own fix against the desktop app.
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md)
- Child Documents: `none`
- Related Documents: [BUG-269: CP-45 Artifact-Bound Context Sources Bypass Unknown-Source-Id Validation](../../09-BugFix/done/BUG-269-CP45-Artifact-Bound-Context-Sources-Bypass-Unknown-Source-Validation.md) (the fix whose live re-test surfaced this bug), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (the coding plan `resolveWorkflowFlowRef`/BUG-174 belongs to; BUG-174 itself predates this project's per-bug doc convention and has no standalone file), [BUG-261: Chat Mode Explicit FlowRef Resolve Failure Silently Suppresses Hub Turn Forever](../../09-BugFix/done/BUG-261-Chat-Mode-Explicit-FlowRef-Resolve-Failure-Silently-Suppresses-Hub-Turn-Forever.md) (sibling bug on the adjacent explicit-chat-flowRef path, same "silent bail" family)
- Replaces: `none`
- Tags: `agent-flow-engine, flow-mode, error-surfacing, regression`

## AI Quick View

### Summary

- `resolveWorkflowFlowRef` (the Flow-Mode workflow-picker bridge, BUG-174) treats every resolution error identically: `workflowID` isn't a flow at all, the store is unavailable, AND a real flow definition that failed CP-44/CP-45 validation (e.g. BUG-269's unknown context-source id) all bail the exact same silent way, falling through to a normal chat turn with zero indication anything was wrong.
- Verified live: after fixing BUG-269 (which makes `ValidateFlowArtifactBindings` correctly return an error for a bad `context_artifact.v1` config), re-running the same flow through "Run Flow" did not fail visibly at all — it silently ran as a normal chat turn instead, which is a *worse* user experience than BUG-269's own pre-fix behavior (at least that ran the actual flow, just with a buried warning).
- CP-44 §11.3's own intent ("Resolve/load flow fail ngay" — fail immediately, visibly) can only be satisfied end to end if a genuine validation failure is distinguished from "this workflowID legitimately isn't a flow" and surfaced to the user, not swallowed by the same safe-bail contract that correctly protects the many legitimate non-flow cases.

### Current Ask

- Distinguish "workflowID resolved to nothing" (correct to bail silently) from "workflowID resolved to an actual flow definition that then failed validation" (must surface to the user) in the Flow-Mode workflow-picker launch path, without changing `resolveWorkflowFlowRef`'s existing `(string, bool)` contract that every other caller/test already depends on.

### Key Decisions

- `V-1` Introduce `ErrFlowDefinitionInvalid` (`flow_definition_resolver.go`) wrapping only the 6 validation-failure return sites in `ResolveFlowRef`/`ResolveBuiltin` (agentpack/context-source/artifact-binding checks, both the stored-row and mirrored-row branches) — every other error path in those functions ("not found", store/lookup failure) stays a plain `fmt.Errorf`, so `errors.As` cleanly separates the two categories.
- `V-2` Keep `resolveWorkflowFlowRef`'s signature `(string, bool)` unchanged — many existing tests call it directly expecting exactly two return values. Instead, stash the distinguished error on the run (`interactiveRun.pendingFlowRefInvalidErr`) as a one-shot value a new `takePendingFlowRefInvalidErr` helper reads and clears.
- `V-3` Surface at the HTTP boundary (`handleStartTurn`, the endpoint the desktop's "Run Flow" launch actually calls): when `resolveWorkflowFlowRef` returns `ok=false` and a pending invalid-definition error exists, write `422 invalid_flow_definition` instead of falling through to `startTurn`'s normal-chat path.

### Constraints

- Must not change behavior for the many legitimate "not a flow" cases (plain admin workflows, no store configured, workflowID simply not resolving) — those still bail silently, exactly as before.
- Must not affect the sibling explicit chat/flowRef path (`explicitFlowRefResolves`) — that already has its own BUG-261 fix and clears `flowRef` rather than erroring, a deliberately different contract for a different entry point (a corrupted flow picked deliberately vs. one auto-resolved from a workflow selection).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/flow_definition_resolver.go` (`ErrFlowDefinitionInvalid`, the 6 wrapped return sites).
- `apps/local-runner/internal/runner/flow_executor.go` (`resolveWorkflowFlowRef`'s new `errors.As` branch; `takePendingFlowRefInvalidErr`).
- `apps/local-runner/internal/runner/interactive_service.go` (`interactiveRun.pendingFlowRefInvalidErr` field).
- `apps/local-runner/internal/runner/interactive_handlers.go` (`handleStartTurn`'s new surfacing branch).
- Live run evidence (pre-fix): `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`, run `run-5090` (`prompt-turn-5092.txt` — a plain chat-shaped prompt with no `[FlowPilot flow context package]`/`[FlowPilot sub-agent — ...]` markers, and zero matching entries anywhere under `.flowpilot/logs/features/agent-flow-engine/`, confirming the flow executor never engaged at all for that run).

## 1. Issue Summary

A Flow-Mode "Run Flow" launch whose selected workflow resolves to a real, stored flow definition that fails CP-44/CP-45 validation does not fail visibly — it silently runs as an ordinary chat turn instead, with no error, no flow timeline, and no indication to the user that anything went wrong.

## 2. Parent Links

- impacted coding plan: `CP-44` (its own E2E test matrix §11.3's "fail rõ ràng" expectation is only satisfiable end to end with this fix)
- impacted tech design: none identified
- impacted system spec: none identified

## 3. Environment and Reproduction

- environment: any Flow-Mode workflow-picker ("Run Flow") launch whose selected workflow's stored definition fails `agentpack.ValidateFlowDefinition`, `ValidateFlowContextSources`, or `ValidateFlowArtifactBindings` — e.g. BUG-269's reproduction (an unknown source id inside a bound `context_artifact.v1` instance's config).
- reproduction steps:
  1. Set up a flow whose stored definition fails one of the three validations above (see BUG-269 for the exact SQL).
  2. Launch it via "Run Flow".
  3. Observe: the turn completes normally as plain chat — no flow timeline, no error, the prompt carries none of the Flow Context Package/sub-agent markers a real flow launch would.
- frequency: deterministic — any validation failure on a workflow-picker launch, not an edge case or a race.

## 4. Expected vs Actual

- expected: the launch fails with a clear, user-visible error identifying the problem (per CP-44's own "fail rõ ràng" E2E expectation), the same way an explicit chat flowRef failing validation is expected to be handled distinctly from "silently proceed."
- actual: the launch silently fell through to a normal chat turn — indistinguishable, from the user's side, from correctly *not* selecting a flow at all.

## 5. Impact

- users affected: anyone whose selected workflow has a validation problem — which, after BUG-269's fix, now includes every case BUG-269 itself was written to catch.
- workflows affected: Flow-Mode "Run Flow" launches; makes CP-44's own fail-fast validation guarantee invisible to the person who most needs to see it (the one configuring the flow).
- severity: medium — no crash, no data loss, but it directly undermines the user-facing half of BUG-269's fix and CP-44's own explicit design intent for §11.3.

## 6. Root Cause

- hypothesis: `resolveWorkflowFlowRef`'s doc comment explicitly designed it as a uniform "safe bail" for every failure mode, written when the only realistic failure was "this workflowID simply isn't a flow" (a plain admin workflow). Nobody revisited that contract when CP-44/CP-45 later added validations capable of failing for a real, selected flow — a categorically different situation the original safe-bail design never anticipated.
- confirmed cause: reproduced live (see Source Refs) — `run-5090` shows a plain-chat-shaped prompt and zero flow-diagnostic-log entries for a workflow whose stored definition, after BUG-269's fix, genuinely fails validation. Confirmed with a new unit test (`TestResolveWorkflowFlowRefSurfacesArtifactBindingValidationError`) that failed before this fix (no way to distinguish the two bail reasons existed) and passes after.
- evidence: see Source Refs.

## 7. Fix Strategy

- `F-1` Added `ErrFlowDefinitionInvalid` (`flow_definition_resolver.go`) and wrapped all 6 validation-failure returns in `ResolveFlowRef`/`ResolveBuiltin` with it, leaving every other error path (not-found, lookup failure) as plain errors.
- `F-2` `resolveWorkflowFlowRef` now does `errors.As` on `ResolveFlowRef`'s error: on a plain error, bails exactly as before (unchanged behavior, unchanged signature); on `*ErrFlowDefinitionInvalid`, additionally stashes the error onto the run's new `pendingFlowRefInvalidErr` field before still returning `("", false)`.
- `F-3` Added `takePendingFlowRefInvalidErr` — a one-shot read-and-clear so a stale error from an earlier turn is never re-surfaced for a later one.
- `F-4` `handleStartTurn` now checks `takePendingFlowRefInvalidErr` right after `resolveWorkflowFlowRef` returns `ok=false`; if set, writes `422 invalid_flow_definition` with the underlying validation message instead of falling through to `startTurn`.

## 8. Validation

- `V-1` **New unit test** — done: `TestResolveWorkflowFlowRefSurfacesArtifactBindingValidationError` (`flow_executor_test.go`) drives a real `resolveWorkflowFlowRef` call against a stored flow definition with BUG-269's own bad-source-id shape, asserts `ok=false`, asserts the stashed error names the offending source id, and asserts a second read clears to nil (one-shot).
- `V-2` **Existing contract tests unaffected** — done: `TestCustomUserOwnedFlowResolvesSpawnsEntryAndAdvancesEdge` (a valid custom flow resolving successfully through the same function) still passes unchanged, confirming the happy path and signature are untouched.
- `V-3` **Full suite** — `go build ./...`, `go vet ./...` clean; `go test ./internal/...` shows no new failures beyond the same pre-existing, already-confirmed-unrelated environment-dependent flakes noted in BUG-268/BUG-269 (Codex/Claude CLI resume, skills-merge, live provider tests).
- `V-4` **Live re-verification** — pending the maintainer's own re-run of the same reproduction (rebuild the runner, re-run "Run Flow" against the still-misconfigured flow from BUG-269's testing) to confirm the desktop app now shows a visible error instead of silently completing as chat; flagged here rather than claimed.

## 9. Regression Guard

- tests: `flow_executor_test.go` (+1 new test).
- alerts: none.
- audit checks: this note is the closing record.

## 10. Follow-Up Document Updates

- upstream docs: none required — CP-44 §11.3's own VERIFIED annotation (once `V-4` completes) should reference both BUG-269 and BUG-270 together, since neither alone satisfies the full "fail rõ ràng" expectation.
- notes left unchanged on purpose: `explicitFlowRefResolves`/BUG-261's own clear-and-fall-through contract for the explicit chat flowRef path is intentionally different and was not touched — see Constraints.
