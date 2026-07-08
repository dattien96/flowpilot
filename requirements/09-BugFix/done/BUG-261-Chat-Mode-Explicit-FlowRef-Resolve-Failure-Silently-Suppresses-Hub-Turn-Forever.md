# BUG-261: Chat-Mode Explicit FlowRef Resolve Failure Silently Suppresses Hub Turn Forever

## Metadata

- Document ID: `BUG-261`
- Title: `Chat-Mode Explicit FlowRef Resolve Failure Silently Suppresses Hub Turn Forever`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 11), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (hub-first-turn suppression / `startResolvedFlow` design)
- Child Documents: `none`
- Related Documents: [BUG-249: Corrupted Builtin Mirror Recreate Duplicates Row And Drops Overrides](./BUG-249-Corrupted-Builtin-Mirror-Recreate-Duplicates-Row-And-Drops-Overrides.md) (same class of built-in-mirror-corruption trigger, found via CP-36 Scenario 14), [BUG-250: Restarted Flow Hub Run Permanently Unresumable Placeholder Session](./BUG-250-Restarted-Flow-Hub-Run-Permanently-Unresumable-Placeholder-Session.md) (same "hub turn suppressed with nothing to reinvoke it" symptom family, different trigger), [CA-259: Fall Back To Normal Turn When Explicit FlowRef Resolve Fails](../../../change-audit/CA-259-fallback-to-normal-turn-when-explicit-flowref-resolve-fails.md)
- Replaces: `none`
- Tags: `agent-flow-engine, chat-mode, flow-executor, silent-failure, regression, data-integrity`

## AI Quick View

### Summary

- Found live while diagnosing a Chat Mode run (`run-10560`, project `D:\working\gate-sandbox`) that the user reported as stuck: they sent a normal message with **Bug** sub-mode + **Review Loop** selected, the run showed `completed` almost instantly, and no agent ever appeared — no coder spawn, no reply text at all.
- Root cause: `handleStartTurn`/`startTurn` (`interactive_handlers.go`, `interactive_service.go`) unconditionally suppresses the hub's own provider turn (`flowStartOnly=true`) the instant an explicit chat `flowRef` is present, then kicks off `startResolvedFlow`'s actual resolve **asynchronously**. If that async resolve fails (e.g. a corrupted stored flow definition), `flow_executor.go` only logs the error and returns — nothing is ever spawned, so nothing ever reinvokes the hub, and the run is stuck at `completed` forever with an empty reply and no error surfaced anywhere.
- This directly contradicts `startResolvedFlow`'s own doc comment: *"A flow-start failure must never break the run's own first turn, which proceeds to its own provider normally."* That promise was never actually implemented for the synchronous caller — the hub's turn was already short-circuited before the async resolve outcome was known.
- The sibling Flow-Mode workflow-picker path (`resolveWorkflowFlowRef`) already resolves synchronously before deciding to attach a `flowRef`, so it never hits this gap — only the explicit Chat-Mode `flowRef` path (Bug sub-mode's built-in orchestration picker) was affected.
- The specific corrupted-data trigger observed in `run-10560` (the built-in `review-loop` mirror's `synthesis` node `dependsOn` a nonexistent `claude-review-fake-model` node) is a separate, already-known class of issue (manual tampering with the shared built-in row instead of a clone — see BUG-249) and is **not** fixed by this bug; only the silent-failure code gap is in scope here.

### Current Ask

- When an explicit chat `flowRef` fails to resolve to a valid flow definition, the run must fall through to a normal chat turn (real provider call, real reply) instead of getting permanently stuck at `completed` with no reply and no error.

### Key Decisions

- `V-1` Resolve the explicit chat `flowRef` **synchronously**, in `handleStartTurn`, before it ever reaches `startTurn` — mirroring the exact pattern `resolveWorkflowFlowRef` already uses for the Flow-Mode workflow-picker path — rather than trying to retroactively un-suppress a turn whose synthetic "completed" response the client may already have received.
- `V-2` On resolve failure, clear `flowRef` to `""` before calling `startTurn`, so `startTurn`'s existing `if flowRef := strings.TrimSpace(in.FlowRef); flowRef != ""` branch never fires and `flowStartOnly`/`flowEngineDriven` are never wrongly latched for a flow that cannot actually run.
- `V-3` Leave `startResolvedFlow`'s own async resolve-failure branch (`flow_executor.go`) unchanged — it still logs-only, but is now unreachable via a bad `flowRef` from the normal Chat-Mode entry point, since the new synchronous pre-check filters it out first. It remains a defense-in-depth log line if some other path ever calls `startResolvedFlow` directly with an unresolvable ref.

### Constraints

- Scoped to the explicit chat `flowRef` path in `handleStartTurn`; the Flow-Mode workflow-picker path (`resolveWorkflowFlowRef`) and `startResolvedFlow`'s own internal logic are unchanged.
- Does not fix the underlying corrupted built-in `review-loop` mirror row that triggered the live repro — that is a Supabase data-integrity issue (same class as BUG-249), out of scope for this code fix.
- Does not add a user-visible warning/toast when a flowRef silently falls back to normal chat (e.g. "Review Loop could not start") — the run now at least produces a real reply instead of hanging, but the user still has no explicit signal that their orchestration choice didn't take effect. Left as a possible follow-up (see Follow-Up Document Updates).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_handlers.go` `handleStartTurn` (~line 279-299, new `explicitFlowRefResolves` call), new `explicitFlowRefResolves` helper referenced from here.
- `apps/local-runner/internal/runner/flow_executor.go` `explicitFlowRefResolves` (~line 217, new), `resolveWorkflowFlowRef` (~line 190, the pre-existing sibling pattern this mirrors), `startResolvedFlow` (~line 13-45, the async path whose failure comment was never actually honored for the chat path).
- `apps/local-runner/internal/runner/interactive_service.go` `startTurn` (~line 3268-3302, the `flowStartOnly`/`flowEngineDriven` latch and its `flowStartOnly` short-circuit at ~line 3353-3374).
- Live evidence: `%AppData%\FlowPilot\logs\runner.log` line for `run-10560`: `[flow-executor] resolve flowRef "flowpilot-core-flow-pack/review-loop" for run "run-10560" failed: flow definition resolver: stored definition for "flowpilot-core-flow-pack/review-loop" failed validation: flow "review-loop" node "synthesis" dependsOn references missing node "claude-review-fake-model"`; `.flowpilot/chats/sessions.ndjson` records for `run-10560` (status `idle`→`running`→`completed` within ~200ms, `provider_session_id` stuck at placeholder `"thread-10561"`); `.flowpilot/chats/run-10560-turns.ndjson` (only the user's prompt line, no assistant reply).
- New regression test: `TestChatModeExplicitFlowRefResolveFailureFallsBackToNormalTurn` (`flow_executor_test.go`).

## 1. Issue Summary

A user testing CP-36 Scenario 11 (Chat Mode's built-in Review Loop picker) sent a normal message with the Bug sub-mode's **Review Loop** orchestration selected. The run reported `completed` almost instantly, with no reply text and no agent ever appearing on the Agents panel. Diagnosis traced this to a stale/corrupted stored flow definition causing the flow's resolve step to fail — but the deeper, more general defect is that the code path had **no fallback at all** for that failure: the hub's own turn had already been unconditionally suppressed before the async resolve even ran, so a resolve failure left the run permanently stuck with no reply and no visible error, despite `startResolvedFlow`'s own doc comment promising otherwise.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 11 ("CP-42 Built-in Orchestration Picker: Deep Technical Verification") — found while preparing that scenario's Chat-Mode parity checks.
- impacted design precedent: CP-42's hub-first-turn suppression design (`startResolvedFlow`'s "the hub's first provider turn is suppressed; it will be reinvoked only after the flow reaches a hub.inline node") is correct and unchanged by this fix — this bug is about the fallback path CP-42's own comment already promised for when that suppression's premise (a flow actually starts) turns out to be false.

## 3. Environment and Reproduction

- environment: Desktop app + local-runner against a real project (`D:\working\gate-sandbox`), Chat Mode Bug sub-mode → Built-in orchestration → Review Loop, Claude provider (`claude-haiku`).
- reproduction steps:
  1. Corrupt the stored built-in `review-loop` mirror definition so it fails `agentpack.ValidateFlowDefinition` (e.g. add a `dependsOn` reference to a nonexistent node id on any node — the live incident had `synthesis` depending on a nonexistent `claude-review-fake-model`).
  2. In Chat Mode, click the **Bug** tab, select **Review Loop** in the Built-in orchestration picker, and send the first message.
  3. Observe: the run reports `completed` almost immediately, with no assistant reply and no child agent ever spawned.
- frequency: deterministic for any explicit chat `flowRef` whose stored definition fails to resolve, regardless of how the definition became invalid.

## 4. Expected vs Actual

- expected (per `startResolvedFlow`'s own doc comment): "A flow-start failure must never break the run's own first turn, which proceeds to its own provider normally regardless of whether the flow's entry node could be spawned."
- actual: the run's first turn was already marked `flowStartOnly=true` synchronously, before the async `startResolvedFlow` goroutine's resolve outcome was known. On resolve failure, the goroutine only logged and returned; the hub's own provider call (`runTurn`) was never invoked, so the run permanently settled at `completed` with an empty `EventTurnCompleted` and no assistant reply.

## 5. Impact

- users affected: anyone using Chat Mode's Bug sub-mode built-in orchestration picker (Review Loop) whose stored flow definition cannot resolve for any reason (data corruption, a bad clone, a future flow with a genuine authoring error).
- workflows affected: the explicit chat `flowRef` path only (`handleStartTurn` → `startTurn`'s `in.FlowRef != ""` branch). The Flow-Mode workflow-picker path (`resolveWorkflowFlowRef`) was already immune, since it resolves synchronously before ever attaching a `flowRef`.
- severity: high — the run becomes permanently stuck with no reply, no error, and no recovery path other than starting a brand-new chat; the user has no way to tell why nothing happened.

## 6. Root Cause

- confirmed cause: `startTurn` (`interactive_service.go:3297-3301`) sets `flowStartOnly = true` and `rs.flowEngineDriven = true` synchronously the moment `in.FlowRef` is non-empty, then launches `go s.startResolvedFlow(...)` asynchronously. `flowStartOnly` (`interactive_service.go:3353-3374`) unconditionally skips the real `s.runTurn(...)` call for this turn, instead emitting a synthetic `EventTurnCompleted{FinalMessage: ""}` and returning — the assumption being that the flow's spawned entry node will do the real work and eventually reinvoke the hub. If `startResolvedFlow`'s `resolver.ResolveFlowRef` call (`flow_executor.go:37`) fails, the function logs the error and returns (`flow_executor.go:39-45`) without spawning anything — so nothing ever reinvokes the hub, and the synchronous short-circuit above is never corrected.
- evidence: live `run-10560` — `runner.log` shows the resolve failure at `09:35:08`; `sessions.ndjson` shows `idle`→`running`→`completed` within ~200ms with `provider_session_id` stuck at the synthetic placeholder `"thread-10561"` (the same placeholder pattern BUG-250 documented as specific to a suppressed, never-reinvoked hub turn); `run-10560-turns.ndjson` contains only the user's prompt, no assistant reply. Reproduced deterministically in a unit test (`TestChatModeExplicitFlowRefResolveFailureFallsBackToNormalTurn`) by seeding a `fakeFlowDefinitionStore` with a `review-loop` mirror whose `synthesis` node `dependsOn` a nonexistent node id, then driving the real HTTP `handleStartTurn` route — before the fix, this produced the same empty synthetic `EventTurnCompleted` pattern; the resolve failure itself was independently confirmed via a direct `ResolveFlowRef` call in the test fixture.

## 7. Fix Strategy

- `F-1` Added `(*InteractiveService).explicitFlowRefResolves(ctx, flowRef) bool` (`flow_executor.go`), which synchronously runs `NewFlowDefinitionResolver(store).ResolveFlowRef(ctx, flowRef)` and reports whether it succeeded — mirroring `resolveWorkflowFlowRef`'s existing resolve-before-attach pattern for the sibling Flow-Mode path.
- `F-2` `handleStartTurn` (`interactive_handlers.go`) now calls `explicitFlowRefResolves` whenever the client already sent a non-empty `body.FlowRef` (the explicit chat path). On failure, it logs `[chat-flow-ref] flowRef %q for run %q failed to resolve; falling back to a normal chat turn` and clears `body.FlowRef = ""` before calling `startTurn`, so `startTurn`'s `in.FlowRef != ""` branch never fires and the turn proceeds through its normal `s.runTurn(...)` call exactly like a plain chat message.
- `F-3` No changes to `startResolvedFlow`'s own async resolve-failure branch, `startTurn`'s `flowStartOnly` short-circuit, or the Flow-Mode workflow-picker path — the fix is a single new pre-check at the one call site that lacked it.

## 8. Validation

- `V-1` `go build ./...` — clean. `go vet ./internal/runner/` — clean.
- `V-2` New test `TestChatModeExplicitFlowRefResolveFailureFallsBackToNormalTurn` (`flow_executor_test.go`): seeds a `fakeFlowDefinitionStore` with a `review-loop` mirror corrupted the same way as the live incident, drives the real HTTP `handleStartTurn`/`startTurn` route with that `flowRef`, and asserts (a) no coder child ever spawns, (b) the run is never flagged `flowEngineDriven`, (c) the turn produces a real `EventMessageCompleted` with non-empty text, and (d) the terminal `EventTurnCompleted` carries a non-empty `FinalMessage` — the exact opposite of the pre-fix empty-synthetic-completion pattern. Passes.
- `V-3` Confirmed no regression to the existing successful-resolve path: `TestChatModeHandleStartTurnMarksRunFlowEngineDriven`, `TestStartTurnWithFlowRefEmitsSyntheticTurnCompletedForHubHandoff`, and `TestStartTurnWithFlowRefSpawnsEntryNodeAsynchronously` all still pass unchanged — a `flowRef` that resolves successfully still gets the synthetic-completion + async-spawn handoff exactly as before.
- `V-4` `go test ./internal/runner/... -count=1` (full package): 1145 passed, 15 failed, 14 skipped. The 15 failures are pre-existing and environment-specific (missing real Codex CLI binary, Windows-specific home-dir/path assertions, provider-home skill-precedence tests needing real files on this machine, Google Drive path parsing) — the same category of baseline failures BUG-250's own validation section documented; none touch `interactive_handlers.go`, `flow_executor.go`, or `interactive_service.go`'s flow-start logic.
- `V-5` GitNexus MCP tools were not available in this thread (confirmed via `ToolSearch`); proceeded via direct code inspection of `startTurn`, `handleStartTurn`, `startResolvedFlow`, and `resolveWorkflowFlowRef` per the `add-new-bug` skill's fallback instruction, cross-checking every call site of the touched functions (`grep` confirmed `explicitFlowRefResolves` and `resolveWorkflowFlowRef` are each called from exactly one place).
- `V-6` Not performed: a live re-run of the original repro (corrupt the real Supabase `review-loop` mirror row, send a Chat Mode Review Loop message, confirm a normal reply now arrives) — the corrupted mirror row from the live incident was left untouched (fixing it is out of scope, see Constraints); the unit test reproduces the same failure shape against an isolated fake store instead.

## 9. Regression Guard

- tests: `TestChatModeExplicitFlowRefResolveFailureFallsBackToNormalTurn` (`flow_executor_test.go`).
- audit checks: `gitnexus_detect_changes()` was not run — GitNexus MCP tools were unavailable in this thread; see `V-5`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none required — CP-36 Scenario 11 already documents the Chat-Mode-vs-Flow-Mode parity gap that led to this finding; no new upstream behavior or acceptance criteria changed (the fix restores already-documented intended behavior).
- notes left unchanged on purpose: the corrupted `review-loop` mirror row that triggered the live repro is left as-is (out of scope, see Constraints — BUG-249's established repair path applies if someone wants to clean it up). Surfacing a user-visible "orchestration could not start" notice when a flowRef silently falls back to normal chat is a reasonable future UX improvement but was intentionally left out of this fix's scope, which targets only the "stuck forever with no reply" defect.
