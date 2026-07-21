# BUG-298: Chat-mode (Review Loop) run stuck "running" forever — dispatch ledger never recognizes chat mode needs gate settlement

## Metadata

- Document ID: `BUG-298`
- Title: `A chat-mode run using the built-in Review Loop orchestration gets permanently stuck at status="running" after its hub calls submit_review_outcome, because the dispatch ledger's SettleOwed computation only recognizes runMode "flow"/"workflow"/"" — never "chat" — so the live settle-drive machinery never re-evaluates the post-turn gate; only a server restart's boot-time recovery scan resolves it`
- Phase: `bugfix`
- Status: `fixed`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-20`
- Last Updated: `2026-07-20` (fix applied: F-1 in `dispatch_record.go`)
- Parent Documents: `-`
- Child Documents: `-`
- Related Documents: `Task-248 (introduced the dispatch record store this bug's gap lives in, commit 914dce7)`, `BUG-288 (fixed the sibling "Chat Mode Review Loop never flagged flowEngineDriven" gap the day before, commit b47d4c4 — same class of chat-mode-was-never-considered omission, different gate)`, `BUG-295, BUG-296 (same-session investigations that first traced the resumePendingFlowGate/dispatch-settle machinery this bug also lives in)`

- Replaces: `-`
- Tags: `agent-flow-engine, chat-mode, review-loop, dispatch-ledger, gate-settle, severity-high`

## AI Quick View

### Summary

- A chat-mode run (`run-12715`) using the built-in "Review Loop" orchestration reached `status="completed"` once, then its hub called `submit_review_outcome` a second time (after both reviewer children reported back) — and from that point on, `status` stayed stuck at `"running"` forever in the live session, only resolving to `"completed"` after a full server restart.
- Confirmed root cause (with real production evidence, not just code reading): `sessions.ndjson` shows `run-12715` flip to `status:"running", pending_flow_gate_settle:true` at `2026-07-20T07:01:06.32Z` (right after the hub's `submit_review_outcome` turn completes) and never recovers. The matching `dispatch.ndjson` record for that exact turn (`turn-12847`) shows `terminal_completed` with `settle_owed:false`.
- `requiresGateSettlement(runMode, stepID)` (`dispatch_record.go:537-543`) only returns `true` for `runMode` values `"flow"`, `"workflow"`, or `""` — it does not recognize `"chat"`. `newDispatchRecord` (`dispatch_live.go:116-120`) passes `rs.runKind` directly as `runMode`, with a fallback to `"workflow"` only when it is EMPTY — `"chat"` is not empty, so it passes straight through unrecognized.
- Meanwhile, `markPendingFlowGateSettleLocked` (which arms the SESSION-level `pendingFlowGateSettle` flag on every `EventTurnCompleted` for a flow-engine-driven run) is gated correctly on `rs.flowEngineDriven`, NOT on `runKind` — and a chat-mode Review Loop run IS flagged `flowEngineDriven=true` (fixed the day before, in `b47d4c4`/BUG-288). So the SESSION thinks a gate check is owed, but the DISPATCH LEDGER (which is what live settle-scheduling — `maybeScheduleSettleAfterTerminal` and the rest of the Task-251 settle-drive machinery — actually reads) thinks nothing is owed for this turn. Nothing ever schedules the `resumePendingFlowGate` call that would re-check the gate and flip `status` back to `Completed`.
- Only a server restart resolves it, because boot-time recovery (`drivePendingSettlesOnBoot`, `dispatch_live.go:539-579`) checks `rs.pendingFlowGateSettle` DIRECTLY from reconstructed session state — bypassing the dispatch ledger's `SettleOwed` entirely — and unconditionally calls `resumePendingFlowGate` for any such run. This is a different, more correct trigger condition than the live path uses, which is exactly why restart "fixes" what the live session cannot.
- Confirmed NOT a regression against previously-working behavior: `requiresGateSettlement` (`dispatch_record.go`) was introduced whole-cloth by commit `914dce7` (Task-248, "durable dispatch record store", 2026-07-17) — 12 hours AFTER `b47d4c4` (2026-07-16) had just taught `flowEngineDriven` to recognize chat-mode Review Loop runs. Task-248 built its OWN, PARALLEL "does this turn need gate settlement" check without accounting for the chat-mode case `b47d4c4` had just legitimized the day before — an original design gap at introduction, not something that broke later.
- Confirmed provider-agnostic: `requiresGateSettlement`/`newDispatchRecord` take no `providerKey` parameter and never branch on it — the gap depends only on `rs.runKind`, so it reproduces identically regardless of which provider (Claude, Codex, Grok, Gemini) runs the hub or its children.

### Current Ask

- None — `F-1` implemented and additive-tested across Claude/Codex/Grok. See "7. Fix Strategy" and "8. Validation".

### Key Decisions

- `V-1` `requiresGateSettlement` recognizes `runMode == "chat"` alongside `"flow"`/`"workflow"`/`""`, so the dispatch ledger's `SettleOwed` agrees with the session-level `flowEngineDriven` gate for chat-mode Review Loop runs.

### Constraints

- The fix touches `requiresGateSettlement`, a function whose `SettleOwed` value is written ONCE per turn at dispatch-record creation time (`newDispatchRecord`) and is durable (persisted to `dispatch.ndjson`) — the fix only affects NEW turns created after it lands; a run already stuck in this state (like the real `run-12715` on disk) still needs its existing restart-recovery path (`drivePendingSettlesOnBoot`) to resolve, which is unaffected and remains the correct safety net for any turn dispatched before the fix.
- additive-tests-only: any future fix must add new dedicated tests only; no existing dispatch-record/settle test may be edited or weakened.
- `-race` cannot currently be run on this machine (no gcc/CGO).
- GitNexus impact analysis unavailable on this machine (`%1 is not a valid Win32 application`) — performed localized tracing instead (see Root Cause evidence), including direct cross-reference against the real `sessions.ndjson`/`dispatch.ndjson` records for the actual affected run.

### Open Questions

- None identified — root cause is confirmed with both code evidence and matching real production data (same run, same turn, same timestamp) for every step of the causal chain.

### Source Refs

- `run-12715` (main, chat-mode Review Loop), children `run-12720` (coder, completed), `run-12766` (reviewer, completed), project `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`.
- `.flowpilot/chats/sessions.ndjson`: `run-12715` reaches `status:"completed"` at `2026-07-20T07:01:02.85Z` ("Both reviewers independently verified the fix..."), then flips to `status:"running", pending_flow_gate_settle:true` at `2026-07-20T07:01:06.32Z` ("Review outcome submitted: **approved**...") and never recovers in the live log.
- `.flowpilot/chats/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/dispatch.ndjson`: `run-12715` turn `turn-12847` — `prepared` (07:00:50.25), `send_claimed`, `send_started`, `terminal_completed` (07:01:06.32) with `settle_owed:false` throughout.
- User report: "Chạy xong hết r mà vẫn show icon loading" (History sidebar spinner stuck even though everything actually finished) / "Sau khi restart server thì show ok" (restarting the server resolves it).
- Desktop screenshot showing "Completed run-12766" in the toolbar (the currently-focused REVIEWER child's own status, correctly completed) while the History sidebar's top-level chat entry for MAIN still shows a spinner.

## 1. Issue Summary

A chat-mode run using the built-in "Review Loop" orchestration completes its actual work (coder fixes the bug, both reviewers approve, the hub submits the review outcome) but its own top-level run status never transitions to `Completed` in the live session — the History sidebar shows a permanent loading spinner for that chat even though nothing further is happening or will happen. Only restarting the server resolves it.

## 2. Parent Links

- impacted coding plan: `CP-51-PhaseAB-Timeline-And-Verification-Log`
- impacted tech design: durable dispatch ledger (`dispatch_record.go`, `dispatch_live.go`, Task-248), post-turn flow-gate settlement (`markPendingFlowGateSettleLocked`, `resumePendingFlowGate`, Task-251 settle-drive machinery — `dispatch_settle.go`, `dispatch_settle_wire.go`)
- impacted system spec: chat-mode Review Loop built-in orchestration (run-status/completion contract)

## 3. Environment and Reproduction

- environment: local runner `127.0.0.1:4318`, desktop FlowPilot, chat-mode "Review Loop" built-in orchestration, any provider.
- reproduction steps (as observed on real data):
  1. Start a chat-mode run with the built-in "Review Loop" orchestration (`in.FlowRef` set, `ChatMode: "normal_chat"`).
  2. Let the hub spawn a coder child; let it complete.
  3. Let the hub spawn reviewer children (a cohort); let all of them complete and report back to the hub (cohort join → hub reinvoke).
  4. The hub's own synthesis turn calls `submit_review_outcome` (approved). Observe: `status` flips to `"running"` and never returns to `"completed"` in the live session, no matter how long you wait.
  5. Restart the runner. On reconnect, `status` correctly shows `"completed"`.
- frequency: deterministic — every chat-mode Review Loop run whose hub reaches a second (or any subsequent) `flowEngineDriven`-gated turn completion hits this, since `SettleOwed` is `false` for every turn of every chat-mode run, unconditionally.

## 4. Expected vs Actual

- expected: once the hub's review-outcome turn completes and the post-turn gate passes, the run's status transitions to `Completed` live, matching what already happened correctly the first time (`07:01:02.85Z`) before this specific turn reopened it.
- actual: the run is left in `status:"running"` with `pending_flow_gate_settle:true` set but nothing in the live system ever acts on that flag — an indefinite, unrecoverable-without-restart hang from the user's perspective, despite the underlying work being fully done.

## 5. Impact

- users affected: anyone using the built-in chat-mode Review Loop orchestration whose flow reaches more than one `flowEngineDriven`-gated turn completion on the hub (i.e., essentially every successful Review Loop run that reaches a review-outcome submission after an earlier completion checkpoint).
- workflows affected: chat-mode Review Loop specifically; formal `WorkflowID`-launched flows are unaffected (their `runKind` is `"workflow"`, which `requiresGateSettlement` already recognizes).
- severity: high — the user cannot tell, from the UI, whether the run is genuinely still doing something or is permanently done; the only recovery is a full server restart, which is disruptive and non-obvious.

## 6. Root Cause

- confirmed cause: `requiresGateSettlement(runMode, stepID)` (`dispatch_record.go:537-543`) is:
  ```go
  func requiresGateSettlement(runMode, stepID string) bool {
      if strings.TrimSpace(stepID) == "" { return false }
      m := strings.ToLower(strings.TrimSpace(runMode))
      return m == "flow" || m == "workflow" || m == ""
  }
  ```
  `"chat"` matches none of these, so it returns `false`.
- `newDispatchRecord` (`dispatch_live.go:112-134`) computes `runMode := rs.runKind`, with `if runMode == "" { runMode = "workflow" }` as the ONLY normalization — since a chat-mode run's `runKind` is literally `"chat"` (non-empty), this fallback never triggers, and `SettleOwed: requiresGateSettlement(runMode, env.StepID)` is persisted as `false` for every dispatch record of every chat-mode run's every turn.
- Independently, `markPendingFlowGateSettleLocked` (`interactive_service.go:3876-3905`, called from `EventTurnCompleted` handling at `interactive_service.go:4228-4236`) arms the SESSION-level `rs.pendingFlowGateSettle = true` flag whenever a turn completes for a `flowEngineDriven` run — gated on `rs.flowEngineDriven`, a COMPLETELY SEPARATE flag from `runKind`/`SettleOwed`. A chat-mode Review Loop run IS `flowEngineDriven=true` (set inline at `interactive_service.go:6754-6776`, per the "BUG-NOTE (Chat Mode Review Loop)" comment there, fixed by commit `b47d4c4`/BUG-288 the day before Task-248 landed).
- These two gates — `flowEngineDriven` (arms the session flag) and `runKind`-derived `SettleOwed` (drives whether the dispatch ledger schedules a live re-check) — are meant to represent the SAME underlying question ("does this run need post-turn gate evaluation before it can be marked Completed") but are computed from DIFFERENT, uncoordinated signals. For a chat-mode Review Loop run, the first says yes, the second says no.
- The dispatch-ledger's own settle-scheduling (`maybeScheduleSettleAfterTerminal`, `dispatch_settle_wire.go:147-165`) explicitly SKIPS scheduling when it has nothing owed (`!rec.SettleOwed`, checked implicitly via `dispatchStore.Get` returning `SettleOwed:false` and the function's early-return conditions) — so for a chat-mode run, the LIVE post-turn `resumePendingFlowGate` call that would re-evaluate the gate and flip `rs.status` back to `Completed` is simply never scheduled. The run sits with `pendingFlowGateSettle=true` and `status="running"` indefinitely.
- Only `drivePendingSettlesOnBoot` (`dispatch_live.go:539-579`, run once at server startup) resolves it, because it reads `rs.pendingFlowGateSettle` DIRECTLY off the reconstructed session state (line 565-566: `pendingGate := rs.pendingFlowGateSettle`) and calls `resumePendingFlowGate` unconditionally when true — completely bypassing the dispatch ledger's (wrong) `SettleOwed` value. This is a structurally DIFFERENT and more correct trigger condition than the live path uses, which is exactly why a restart "fixes" what the live session cannot.
- Confirmed NOT a regression: `requiresGateSettlement` has never included `"chat"` since its introduction (`914dce7`, 2026-07-17, Task-248) — there is no prior working state to regress from. It is an original design gap: Task-248 built dispatch-ledger settle-tracking as a parallel mechanism to the pre-existing `flowEngineDriven`/`pendingFlowGateSettle` session-level gate, and never accounted for the chat-mode case that `b47d4c4` (BUG-288) had, just the day before, taught the OTHER gate (`flowEngineDriven`) to recognize.
- Confirmed provider-agnostic: neither `requiresGateSettlement` nor `newDispatchRecord` take a `providerKey` parameter or branch on one anywhere — grepped and confirmed. The gap depends only on `rs.runKind` (`"chat"` vs `"flow"`/`"workflow"`), so it reproduces identically for a Claude, Codex, Grok, or Gemini hub/children under chat-mode Review Loop.
- evidence: `dispatch_record.go:537-543` (`requiresGateSettlement`); `dispatch_live.go:112-134` (`newDispatchRecord`, `runMode := rs.runKind`); `interactive_service.go:3876-3905` (`markPendingFlowGateSettleLocked`, gated on `flowEngineDriven`); `interactive_service.go:4228-4236` (`EventTurnCompleted` calling it); `interactive_service.go:6754-6776` (chat-mode `flowEngineDriven=true`, `b47d4c4`/BUG-288); `dispatch_settle_wire.go:144-165` (`maybeScheduleSettleAfterTerminal`, the live scheduler that never fires when `SettleOwed=false`); `dispatch_live.go:539-579` (`drivePendingSettlesOnBoot`, the boot-only recovery path that bypasses `SettleOwed`); `git show --stat 914dce7` / `git log -L` confirming `requiresGateSettlement`'s introduction date and that it never included `"chat"`; direct cross-reference of `sessions.ndjson` + `dispatch.ndjson` for the real `run-12715`/`turn-12847` showing the exact moment and mechanism of the stall.

## 7. Fix Strategy (APPLIED)

- `F-1` (applied) — in `requiresGateSettlement` (`dispatch_record.go:537-543`), add `"chat"` to the recognized `runMode` set: `return m == "flow" || m == "workflow" || m == "chat" || m == ""`. This is the minimal, precise fix: it makes the dispatch ledger's `SettleOwed` computation agree with the session-level `flowEngineDriven` gate for chat-mode Review Loop runs, so `maybeScheduleSettleAfterTerminal` (and the rest of Task-251's settle-drive machinery) will schedule the live `resumePendingFlowGate` re-check exactly when the session already believes one is owed — closing the gap between the two mechanisms without touching either gate's own logic.
- Considered and rejected: changing `newDispatchRecord`'s fallback (`if runMode == "" { runMode = "workflow" }`) to also normalize `"chat"` — functionally equivalent for THIS call site, but `requiresGateSettlement` is a general-purpose predicate that could be called elsewhere with a raw `runKind`; fixing the predicate itself is the more correct, single point of truth.
- Considered and rejected: gating `requiresGateSettlement` on `rs.flowEngineDriven` directly instead of `runMode` — would require plumbing the `flowEngineDriven` bool through to `newDispatchRecord`/`requiresGateSettlement` (currently a pure string/string function with no `*interactiveRun` access), a larger signature change for no additional precision (a chat-mode run only reaches this code path when it IS `flowEngineDriven`, since only flow-engine-driven runs use hub/cohort/gate machinery at all).
- additive-tests-only: existing dispatch-record/settle tests (`dispatch_record_test.go`, `dispatch_settle_barrier_test.go`, `gate_checkpoint_outage_test.go`, `run2383_live_gate_settle_test.go`, etc.) were left unmodified. New coverage added in `bug298_chat_mode_gate_settlement_test.go` (see Validation).
- Provider matrix: since neither the bug nor the fix branches on `providerKey`, one table test (parameterized by `runMode`) plus one `newDispatchRecord`-level test parameterized by `ProviderKey` (Claude, Codex, Grok) fully covers all three — no per-provider divergence exists to test for.

## 8. Validation

- New additive test file `apps/local-runner/internal/runner/bug298_chat_mode_gate_settlement_test.go`, 3 top-level tests (6 sub-tests total), all pass:
  - `TestBug298RequiresGateSettlementRecognizesChatMode` — the fix itself: `requiresGateSettlement("chat", "some-step")` now returns `true`.
  - `TestBug298RequiresGateSettlementNonRegression` — table test confirming `"flow"`/`"workflow"`/`""` are unchanged, and an empty `stepID` still returns `false` regardless of `runMode`.
  - `TestBug298NewDispatchRecordSetsSettleOwedForChatMode` — end-to-end through the real `newDispatchRecord` constructor with `runKind="chat"`, parameterized by `ProviderKey` (Claude, Codex, Grok as sub-tests): confirms `SettleOwed:true` is produced identically for all three.
- `go test ./internal/runner -run TestBug298 -count=1`: 6 passed (3 top-level + 3 provider sub-tests).
- Regression battery `go test ./internal/runner -run 'TestDispatch|TestSettle|TestGate|Test.*Settle|TestRun1264|TestRun2383|TestBug288|TestBug289|TestBug298' -count=2`: 294 passed, run twice — fully deterministic.
- No existing test calls `requiresGateSettlement` directly (grepped, confirmed) — so no prior test could have relied on the old (incorrect) `"chat"` → `false` behavior; nothing was weakened.
- `-race` not run: this machine lacks gcc/CGO.
- GitNexus impact analysis unavailable (`%1 is not a valid Win32 application`); localized tracing performed instead (see Root Cause evidence) — enumerated both gates (`flowEngineDriven`, `requiresGateSettlement`) and every settle-scheduling call site before changing the single shared predicate.

## 9. Regression Guard

- tests: added, all pass (see Validation). The three composed checks land exactly as planned, plus the existing full dispatch/settle test suite (294 tests, covering Codex and Grok flow/chat dispatch scenarios) continues to pass unmodified.
- alerts: consider a runtime warning/log line when `rs.pendingFlowGateSettle` has been `true` for longer than some threshold with no scheduled settle-drive activity — would have surfaced this class of stuck-run defect immediately in logs instead of requiring cross-referencing two separate NDJSON files after the fact. Not implemented in this fix (out of scope; noted as a future observability improvement).
- audit checks: CA note `CA-375` (feature_key `agent-flow-engine`) accompanies this fix.

## 10. Follow-Up Document Updates

- upstream docs that must change: none identified — Task-248's own documentation (if any exists beyond code comments) should note that `requiresGateSettlement` must stay in sync with `flowEngineDriven`'s own recognized run shapes going forward, to prevent a similar gap the next time a new run-shape is taught to one gate but not the other.
- notes left unchanged on purpose: none — this document's root cause is fully confirmed, no deliberately-unresolved threads remain.
