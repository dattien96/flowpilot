# BUG-279: Flow Mode Validate-Retry Ignores Implement Node's `reinvoke` Lifecycle

## Metadata

- Document ID: `BUG-279`
- Title: `Flow Mode Validate-Retry Ignores Implement Node's reinvoke Lifecycle`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [CP-41-RAG-Harness-Flow-Mode.md](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md)
- Child Documents: `none`
- Related Documents: [BUG-278-Flow-Mode-Coding-Step-Agent-Autonomously-Commits-And-Writes-Audit-Notes.md](BUG-278-Flow-Mode-Coding-Step-Agent-Autonomously-Commits-And-Writes-Audit-Notes.md), [BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md](../done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md)
- Replaces: `none`
- Tags: `flow-mode`, `rag-harness`, `agent-flow-engine`, `retry-loop`, `lifecycle`, `reinvoke`

## AI Quick View

### Summary

- The RAG Harness flow's `implement` node declares `lifecycle: reinvoke` (`rag-harness.yaml:38`), meaning a Testing-step retry back-edge into `implement` should reuse the existing Coding-step child run/provider session rather than starting a brand new one.
- Live evidence from a Scenario 5 (validate fail → retry → pass) manual test shows the two Coding-step invocations got **two different `provider_session_id` values** (`thread-10628` for the first attempt, `thread-10674` for the retry) — a brand new Claude session was spawned for the retry instead of the same session being reinvoked.
- Code inspection confirms the root cause: the back-edge `"retrying"` handler in `flow_validate_audit_dispatch.go` calls `s.spawnChildRun(...)` unconditionally (line ~219) and never checks the target node's lifecycle or calls `reinvokeExistingFlowChild`/`flowNodeReusesChild` — unlike the forward-edge auto-advance path in `flow_executor.go` (~line 789), which correctly checks `flowNodeReusesChild(node)` and calls `s.reinvokeExistingFlowChild(...)` first.
- Likely contributes to the severity of [BUG-278](BUG-278-Flow-Mode-Coding-Step-Agent-Autonomously-Commits-And-Writes-Audit-Notes.md): because every retry is a fresh, memory-less session, the coder cannot recall what it already did (e.g. that it already registered a feature key or wrote a CA note in a prior attempt) and re-derives "proper" repo-convention behavior from scratch each time, compounding autonomous git/file actions.

### Current Ask

- ~~Make the Testing-step retry back-edge (`validate --continue--> implement`) honor the target node's `lifecycle: reinvoke` declaration the same way the forward-edge auto-advance path already does.~~ **Done.**

### Key Decisions

- `V-1` Fixed and verified 2026-07-13 (see §7/§8 below). Filed per explicit owner decision (2026-07-13: "mark thành BUG mới lát chúng ta sẽ fix"), then fixed the same day.

### Constraints

- Must not change behavior for flows/nodes that declare `lifecycle: spawn` (or no lifecycle / default) — those should keep spawning a new child on every back-edge retry, same as today.
- Fix should reuse the existing `flowNodeReusesChild`/`reinvokeExistingFlowChild` helpers already proven correct for the forward-edge path, rather than writing a new parallel mechanism.
- Must preserve `AdvanceRetryState`'s existing retry-count/cap semantics (BUG-243) — this bug is only about *which child run/session* handles the reinvoked turn, not the retry-limit bookkeeping itself.

### Open Questions

- Does `reinvokeExistingFlowChild`/`reinvokeMatchingFlowChild` need any retry-specific adjustment (e.g. passing `ComposeRetryPrompt`'s output instead of a fresh prompt) to slot cleanly into the `"retrying"` branch, or is it a drop-in replacement for the `spawnChildRun` call at line ~219?
- Should this fix be verified against the review-loop's own back-edge reinvoke path (`maybeReinvokeCoderForContinue`, referenced in `flow_executor.go`'s `reinvokeMatchingFlowChild` comment as the earlier BUG-Rnd2 fix) to confirm no duplicate logic drift between the two back-edge consumers?

### Source Refs

- `D:\working\gate-sandbox` — `run-10622` (parent validate/retry run), `run-10627` (first `implement` child, `provider_session_id: thread-10628`), `run-10673` (retry `implement` child, `provider_session_id: thread-10674`) — confirmed via `sessions.ndjson`.
- `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml:36-41` (`implement` node: `lifecycle: reinvoke`).
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:198-234` (the `"retrying"` case — unconditional `spawnChildRun`, no lifecycle check).
- `apps/local-runner/internal/runner/flow_executor.go:783-802` (forward-edge auto-advance path — correct reference implementation: `flowNodeReusesChild(node)` gate before `reinvokeExistingFlowChild`).
- `apps/local-runner/internal/runner/flow_executor.go:915-946` (`reinvokeExistingFlowChild`/`reinvokeMatchingFlowChild` — the reusable helper the fix should call into).

## 1. Issue Summary

The RAG Harness `implement` node is declared with `lifecycle: reinvoke`, but when the Testing (`validate`) step fails and the flow follows the `validate --continue--> implement` back-edge to retry, the runner spawns a completely new Coding-step child run with a new provider session every time, instead of reinvoking the existing one. This defeats the purpose of `reinvoke` (continuity of the agent's own working memory/session across a retry) and was confirmed live during CP-41 Scenario 5 manual re-verification.

## 2. Parent Links

- impacted coding plan: [CP-41-RAG-Harness-Flow-Mode.md](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md) (Scenario 5 retry loop, `P-5`)
- impacted tech design: `none identified yet`
- impacted system spec: `none identified yet`

## 3. Environment and Reproduction

- environment: Desktop app + local-runner, Flow Mode, RAG Harness flow (built-in), project `D:\working\gate-sandbox`, Claude as the Coding-step provider.
- reproduction steps:
  1. Configure the `validate` node's test command to fail on the first call and pass on the second (e.g. a marker-file script).
  2. Run the RAG Harness flow with a prompt that resolves a known feature key.
  3. Let `implement` → `validate` (fails) → back-edge retry → `implement` (again) → `validate` (passes) run to completion.
  4. Inspect `.flowpilot/chats/sessions.ndjson` for the two `implement`-labeled child runs' `provider_session_id` values.
- frequency: observed consistently in the one Scenario 5 retry test performed; the code path is unconditional (not timing-dependent), so it is expected to reproduce every time a back-edge retry fires.

## 4. Expected vs Actual

- expected: the retry re-enters the *same* Coding-step child run/provider session that handled the first `implement` attempt, per `lifecycle: reinvoke`.
- actual: a brand new child run and a brand new Claude provider session/thread are spawned for the retry (`run-10627`/`thread-10628` → `run-10673`/`thread-10674`), identical in effect to `lifecycle: spawn`.

## 5. Impact

- users affected: anyone whose Flow Mode run hits a Testing-step retry on a node declared `lifecycle: reinvoke`.
- workflows affected: RAG Harness (and any other flow reusing the same back-edge retry dispatch in `flow_validate_audit_dispatch.go`).
- severity: medium — functionally the retry loop still completes correctly (retry count, prompt composition, and pass/fail detection all work per BUG-243), but the *lifecycle contract* is silently violated, and it is suspected to be a contributing factor to BUG-278's repeated autonomous coder behavior across retries (no session memory of its own prior actions).

## 6. Root Cause

- hypothesis: n/a — confirmed directly by code inspection, not just hypothesized.
- confirmed cause: `flow_validate_audit_dispatch.go`'s `"retrying"` case (around line 219) calls `s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{...})` unconditionally, without first checking `flowNodeReusesChild(targetNode)` and calling `s.reinvokeExistingFlowChild(parentRunID, targetNode.ID, prompt)` — the exact pattern the forward-edge auto-advance path in `flow_executor.go` (~line 789) already uses correctly for the same lifecycle field.
- evidence: `sessions.ndjson` entries for `run-10627` (`provider_session_id: "thread-10628"`) and `run-10673` (`provider_session_id: "thread-10674"`) — two distinct sessions for what should have been one reinvoked session, per `rag-harness.yaml`'s `implement: lifecycle: reinvoke`.

## 7. Fix Strategy

- `F-1` **Implemented.** In `flow_validate_audit_dispatch.go`'s `"retrying"` case, added a check for `flowNodeReusesChild(targetNode)` before the existing `spawnChildRun` call — when true, calls `s.reinvokeExistingFlowChild(parentRunID, targetNode.ID, prompt)` first (mirroring the forward-edge path in `flow_executor.go`), returning immediately on success; falls through to the pre-existing `spawnChildRun` path unchanged when the node isn't `lifecycle: reinvoke` or no matching non-terminal child exists.
- `F-2` Not needed as a separate change: `ComposeRetryPrompt`'s output is passed as-is to `reinvokeExistingFlowChild` (same `prompt` variable used for the `spawnChildRun` fallback), and `reinvokeMatchingFlowChild` delivers it via `scheduleChildTurn` exactly as the review-loop's own back-edge reinvoke already does — no new prompt-threading code was required.

## 8. Validation

- `V-1` **Done (unit test, not a live re-run).** Added `TestTryAdvanceFlowFromNodeValidateRetryReinvokesLifecycleReinvokeTarget` (`flow_validate_audit_dispatch_test.go`): spawns an initial `implement` child (`lifecycle: reinvoke`), waits for it to complete, then drives a failing `validate` → retry cycle and asserts (a) `countChildrenWithLabel(svc, parent.RunID, "implement")` is unchanged before/after the retry (no new child spawned) and (b) the same child received a second turn (prompt count reaches 2). A live gate-sandbox re-run with real `sessions.ndjson` provider-session-id comparison (matching the original `run-10627`/`run-10673` evidence) was not performed this pass — the unit test exercises the exact code path via the same public entrypoint (`tryAdvanceFlowFromNode`) the live run went through.
- `V-2` **Done** — see `V-1`; same test.
- `V-3` **Covered by pre-existing tests**, unchanged by this fix: `TestTryAdvanceFlowFromNodeValidateFailingCommandRetriesCoder` and `TestTryAdvanceFlowFromNodeValidateMaxRetriesEscalates` both use `flowFixtureEdgesNodes()`'s `implement` node with no `Lifecycle` set (defaults to spawn behavior) and still pass unmodified, confirming no regression for non-`reinvoke` nodes.
- Full suite: `go build ./...` clean; `go test ./internal/runner/... -count=1`: 1386 passed, 16 failed (all pre-existing environment-dependent flakes — Codex CLI resume, account-home, skills-merge, auth-workspace, same categories as prior sessions' baseline), 18 skipped — zero new failures.

## 9. Regression Guard

- tests: `TestTryAdvanceFlowFromNodeValidateRetryReinvokesLifecycleReinvokeTarget` (new) guards this specific regression going forward.
- alerts: none.
- audit checks: none yet.

## 10. Follow-Up Document Updates

- upstream docs that must change: `CP-41-RAG-Harness-Flow-Mode.md` Scenario 5's evidence note should get a follow-up line once a live gate-sandbox re-run confirms matching `provider_session_id` values end-to-end (not yet done — see `V-1`).
- notes left unchanged on purpose: `BUG-243`'s own scope (retry-count/cap bookkeeping, pass/fail/env-error classification) is confirmed still correct and unaffected — this bug is purely about which child run/session handles the reinvoked turn.
