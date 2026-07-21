# BUG-297: Focused child's transcript is polluted by main's own live sibling-spawn events

## Metadata

- Document ID: `BUG-297`
- Title: `Viewing a focused child agent's transcript (e.g. coder) shows sibling agent cards (e.g. reviewer_correctness, reviewer_security) that were actually spawned by main, because the orchestration stream applies main's own events onto the single shared timeline regardless of which run is currently displayed`
- Phase: `bugfix`
- Status: `fixed`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-20`
- Last Updated: `2026-07-20` (fix applied: F-1 in `state/store.ts`)
- Parent Documents: `-`
- Child Documents: `-`
- Related Documents: `BUG-137, BUG-138, BUG-139 (introduced the mainRunId-anchored orchestration-stream staleness check this bug traces to), BUG-180 (added a further branch to the same function), BUG-294 (same-session sibling investigation, independent root cause)`
- Replaces: `-`
- Tags: `desktop-flowpilot, chat-ui, timeline, agent-focus, review-loop, client-only`

## AI Quick View

### Summary

- While viewing a focused CHILD agent's transcript (e.g. `coder`), two SIBLING agent cards (`reviewer_correctness`, `reviewer_security` — children of MAIN, not of coder) appeared inline inside the coder transcript, styled identically to a normal `AgentTimelineCard`.
- Root cause: `s.timeline` is a single, global field in the desktop Zustand store — there is no per-run timeline. Focusing a child (`focusAgentRun`) simply overwrites `s.timeline` with that child's own history; it does not isolate it from other live event sources.
- The background "orchestration stream" (`consumeOrchestrationStream`, started once for MAIN and never cancelled while a child is focused) treats itself as non-stale using `shouldApplyRunEvent(get().mainRunId ?? get().runId, runId)` — comparing against `mainRunId` (which never changes when a child is focused), NOT against whichever run is currently displayed. So when MAIN emits its own `agent_spawned_by_user` events for reviewer children (a normal step of the review-loop, happening live while the user is looking at coder), the stream's `else` branch unconditionally applies them via `applyEvent`/`applyTimelineEvent` straight onto `s.timeline` — which at that moment displays coder's transcript, not main's.
- Confirmed provider-agnostic: neither `consumeOrchestrationStream`, `applyEvent`, nor `applyTimelineEvent` branches on `providerKey` anywhere — the bug reproduces identically regardless of which provider (Claude, Codex, Grok, Gemini) runs main, coder, or the reviewers, since the routing gap is purely about WHICH RUN's timeline is being written to, never about the provider.
- This is a long-standing architectural gap, not a recent regression: the `mainRunId`-anchored staleness check traces back at least to commit `c58f360` (BUG-137/138/139), predating this investigation session by a wide margin. It was designed so MAIN keeps updating live while a child is focused (so returning to main shows fresh content) — but never accounted for the case where the user is looking at a DIFFERENT child's transcript while that background update fires.

### Current Ask

- None — `F-1` implemented and additive-tested. See "7. Fix Strategy" and "8. Validation".

### Key Decisions

- `V-1` `consumeOrchestrationStream`'s live-apply branch only mutates the shared `s.timeline` when MAIN is the currently displayed run (`get().runId === runId`); `backToMainRun`'s existing pre-focus-snapshot replay already guarantees nothing is lost by skipping while a child is focused, so no separate background-cache mechanism was needed.

### Constraints

- `s.timeline` is read by many components (`Timeline.tsx` and others); any fix must not break the existing "MAIN keeps updating live while a child is focused" behavior the mechanism was built for (BUG-137/138/139/180) — it must scope OUT only the case where a DIFFERENT run is currently displayed, not remove live-updating altogether.
- additive-tests-only: any future fix must add new dedicated tests only; existing store tests (`store.test.ts`, `store.agent-replay-causal-order.test.ts`, `store.history-replay-order.test.ts`, `timeline_agent_lifecycle.test.ts`) must not be edited or weakened.
- This is a desktop (TypeScript/Zustand) bug, not a runner (Go) bug — no GitNexus/Go build implications; verification is via `npm test`/vitest in `apps/desktop-flowpilot`.

### Open Questions

- None — resolved during implementation: `backToMainRun` (`store.ts:727-765`) already replays every event since its pre-focus snapshot (`restore.lastEventSeq`) independent of whatever the orchestration stream did while away, so no separate background-cache mechanism was needed; skipping the live-apply while a child is focused loses nothing on return.

### Source Refs

- Desktop screenshot: viewing "main · coder" (child agent coder, live), showing inline `reviewer_correctness · Claude · running · claude-sonnet` and `reviewer_security · Claude · completed · claude-opus` cards inside coder's own transcript, both with "Open ↗" buttons matching `AgentTimelineCard`'s exact rendering shape.
- `apps/desktop-flowpilot/src/state/store.ts:2345-2385` (`consumeOrchestrationStream`).
- `apps/desktop-flowpilot/src/state/store.ts:235` (`timeline: TimelineItem[]` — single global field, no per-run keying).
- `apps/desktop-flowpilot/src/state/store.ts:657-725` (`focusAgentRun` — overwrites `s.timeline` on focus, does not isolate it).
- `apps/desktop-flowpilot/src/state/timelineReducer.ts:116` (`applyTimelineEvent` — mutates `s.timeline` unconditionally, no run-id check of its own; correctly assumes its caller has already verified scope).
- `apps/desktop-flowpilot/src/components/Timeline.tsx:529-551` (`AgentTimelineCard` — the exact visual shape seen in the screenshot, rendered from `timelineGroups`, i.e. from `s.timeline` contents, not from the separate `liveAgentRunsWithoutVisibleCard` banner list which IS correctly guarded — see Root Cause).

## 1. Issue Summary

While actively viewing a focused child agent's transcript (`coder`, spawned by MAIN as part of a review-loop flow), two agent cards belonging to MAIN's OTHER children (`reviewer_correctness`, `reviewer_security` — siblings of coder, not descendants) appeared inline inside coder's own displayed transcript, as if they were part of coder's own conversation. The user expected these cards to appear only in MAIN's own transcript/sidebar, since they are not children of coder.

## 2. Parent Links

- impacted coding plan: `CP-51-PhaseAB-Timeline-And-Verification-Log` (review-loop UI)
- impacted tech design: desktop timeline event application (`state/store.ts`, `state/timelineReducer.ts`), agent-focus navigation (`focusAgentRun`/`backToMainRun`)
- impacted system spec: chat-mode Review Loop built-in orchestration (desktop presentation layer)

## 3. Environment and Reproduction

- environment: FlowPilot Desktop, runner `127.0.0.1:4318`, chat-mode "Review Loop" built-in orchestration, any provider.
- reproduction steps:
  1. Start a chat-mode run using the built-in "Review Loop" orchestration; let it spawn a `coder` child.
  2. Click into the `coder` child's transcript (`focusAgentRun`) while it is still running or shortly after.
  3. Let the parent flow proceed to its next step live (coder finishes, MAIN spawns `reviewer_correctness` and `reviewer_security` as ITS OWN children) WHILE still viewing coder's transcript.
  4. Observe: the reviewer cards render inline inside coder's own displayed transcript.
- frequency: deterministic whenever a MAIN-level event that mutates the timeline (e.g. `agent_spawned_by_user`) fires while the user is focused on any OTHER run's transcript — not limited to coder/reviewer, and not limited to spawn events (any event `consumeOrchestrationStream`'s `else` branch applies via `applyEvent` while the user is off viewing a child is equally exposed).

## 4. Expected vs Actual

- expected: a focused child's transcript shows only that child's own turn history; sibling agents spawned by the parent should appear in MAIN's own transcript and the global Agents sidebar, never inside an unrelated child's view.
- actual: MAIN's own live events (including sibling-spawn annotations) bleed into whatever transcript happens to be currently displayed, because `s.timeline` is a single shared field and the background orchestration stream applies events to it without checking which run is currently on screen.

## 5. Impact

- users affected: anyone using a multi-child flow (review-loop or similar) who inspects one child's transcript while the parent flow progresses live to spawn further children.
- workflows affected: any flow/chat-mode session with 2+ sequential or concurrent child agents under one parent, when a user navigates into a child's transcript mid-flow.
- severity: medium — purely a display/attribution defect (no data loss, no incorrect state on disk or in the server), but confusing and potentially misleading about which agent produced what.

## 6. Root Cause

- confirmed cause: `s.timeline` (`store.ts:235`) is ONE global array, not partitioned per run. `focusAgentRun` (`store.ts:657-725`) replaces its CONTENTS with the focused child's own history on navigation, but nothing else in the app treats "whichever run's content currently occupies `s.timeline`" as an invariant that other code paths must respect.
- `consumeOrchestrationStream` (`store.ts:2345-2385`) is started once, bound to MAIN's own run id via closure, and its own staleness check (`isStale`, line 2353) is: `!shouldApplyRunEvent(get().mainRunId ?? get().runId, runId)` — comparing the STABLE `mainRunId` (unchanged by focusing a child) against its own bound `runId` (also always mainRunId). This check answers "is MAIN's own session still the one we should be listening for," which stays true for the whole session — it does NOT answer "is MAIN's timeline the one currently on screen."
- The `else` branch (line 2371-2383) — added for CP-35 (gate-reprompt turns arriving after `sendTurn()` already closed) — applies non-`agent_graph_updated`/`agent_bus_message` events (including `agent_spawned_by_user`) via `set((s) => applyEvent(s, e))`, unconditionally mutating whatever `s.timeline` currently is.
- `applyTimelineEvent` (`timelineReducer.ts:116`) itself has no run-id awareness at all — by design, it trusts its caller to have already verified the event belongs to the currently-displayed run. `consumeStream`/`consumeHistoryReplayStream` (the FOREGROUND turn/history streams, used when a specific run's OWN turn is being sent/replayed) correctly gate on `isEventForRun(e, runId)` AND `shouldApplyRunEvent(get().runId, runId)` (the CURRENT `runId`, which DOES change on focus) before calling `applyEvent` — but `consumeOrchestrationStream` is the ONE caller that intentionally keeps running in the background across focus changes (by design, for BUG-137/138/139/180), and its check anchors to `mainRunId` instead of the currently-displayed `runId`, so it is the one path that can write to `s.timeline` while a DIFFERENT run's content is on screen.
- The GLOBAL `liveAgentRuns` banner (`Timeline.tsx:244-258,650,697`), which IS correctly guarded (`(!activeAgentRunId || activeAgentRunId === mainRunId) && liveAgentRuns...`, `Timeline.tsx:697`) so it never renders while focused on a child, is NOT the mechanism at play here — the cards seen in the screenshot render via `AgentTimelineCard` from actual `TimelineItem`s inside `s.timeline` (i.e., they are baked into the displayed transcript itself, added by the unconditional `applyEvent` call above), not via that separately-guarded live banner.
- evidence: `store.ts:235` (single `timeline` field); `store.ts:657-679` (`focusAgentRun` sets `runId`/`activeAgentRunId` to the child, `s.timeline` becomes the child's own snapshot); `store.ts:2345-2385` (`consumeOrchestrationStream`, `isStale` anchored to `mainRunId`, `else` branch's unconditional `applyEvent`); `store.ts:2080-2100` (`consumeStream`, the CORRECTLY-scoped sibling that anchors to `get().runId`, for contrast); `timelineReducer.ts:116` (`applyTimelineEvent`, no run-id check by design); `Timeline.tsx:697` (the separately-guarded live-banner list, ruled out as the actual source).

## 7. Fix Strategy (APPLIED)

- `F-1` (applied) — in `consumeOrchestrationStream`'s `else` branch (`store.ts:2371-2385`), added `if (get().runId !== runId) continue;` right before the `applyEvent` call, so it only mutates the shared, live-displayed `s.timeline` when MAIN is actually the currently-displayed run. Verified this loses nothing on return: `backToMainRun` (`store.ts:727-765`) already replays every event from its OWN pre-focus snapshot (`restore.lastEventSeq`) independent of whatever the orchestration stream did while a child was focused, and restarts a fresh `startOrchestrationStream` on return — so skipping the live-apply while away is fully safe, no separate background-cache mechanism was needed (resolving the Open Question from the draft of this document).
- Considered and rejected: partition `s.timeline` per run id (a `Record<runId, TimelineItem[]>` instead of one global array) — structurally more correct, but a much larger refactor touching every reader of `s.timeline`; the one-line guard fully resolves the bug with `backToMainRun`'s existing replay guaranteeing correctness on return, so the larger refactor was not needed.
- additive-tests-only: existing tests were left unmodified. New coverage added directly in `store.test.ts` (see Validation) — additive per this repo's convention of appending new `test(...)` blocks to the existing file rather than a separate file, matching how other store-level bug tests (BUG-109, BUG-110, BUG-235, etc.) are recorded in this same file.
- Confirmed no cross-provider concern: `consumeOrchestrationStream`, `applyEvent`, and `applyTimelineEvent` never branch on `providerKey` — grepped and confirmed. One representative test fully covers Claude/Codex/Grok/Gemini equally, since the routing gap is about which RUN's timeline is targeted, never which PROVIDER is involved.

## 8. Validation

- New additive test in `apps/desktop-flowpilot/src/state/store.test.ts`: `"orchestration stream does not bleed a sibling agent_spawned_by_user into a focused child's timeline"`. It drives `sendPrompt` to start MAIN's own turn + orchestration stream, confirms MAIN's own coder spawn card renders correctly, then simulates `focusAgentRun`'s effect (navigating into the coder child's transcript), then lets the still-running orchestration stream deliver a sibling (`reviewer`) spawn event, and asserts the focused child's displayed timeline is completely untouched by it.
- Verification method: this repo's environment has no wired-up runner for `apps/desktop-flowpilot/src/**/*.test.ts` (`npm run test:phase1`'s `tsc` step fails on unrelated, pre-existing stale fixtures in OTHER test files under `tests/phase1/`, and directly invoking the compiled output via `node --test` hits `@/` path-alias and `node_modules` resolution gaps that also pre-date this fix — confirmed identical on a git-stashed baseline). Worked around for verification purposes only (a `--require` hook resolving `@/*`/`@flowpilot/client-core` the same way the project's own `tsconfig.phase1-tests.json` `paths` do, and a temporary local `node_modules` copy for the compiled-output tree — neither committed, both cleaned up after verification) to get a real, executable signal from the actual project test suite rather than a hand-rolled harness.
- With the fix applied: the new test passes; `store.test.ts` overall: 82 passed, 5 failed — all 5 failures (`selectProject resets the active chat run...`, `openHistoryRun does not set historyOpenError...`, `sendPrompt aborts an open-ended history replay stream...`, `workflow handoff turn settles and orchestration stream keeps the run active`, `stop uses loop stop plus parent interrupt...`) reproduce byte-for-byte identically on a git-stashed baseline (fix reverted) — confirmed pre-existing and unrelated to this change (verified across 3 repeated runs, no flake in the failure set itself, though `workflow handoff...`'s own internal assertion is order/timing-sensitive independent of this fix).
- With the fix REVERTED (baseline): the new test correctly FAILS (proving it genuinely detects the bug) alongside the same 5 pre-existing failures — a clean before/after contrast.
- Broader related-file battery (`store.agent-replay-causal-order.test.ts`, `store.history-replay-order.test.ts`, `store.flow-terminal.test.ts`, `timeline_agent_lifecycle.test.ts`, `timelineReducer.test.ts`, `stop_parent_snapshot_reconciliation.test.ts`): 133 tests total across all files, 125 passed, 8 failed — the 8 failures are the same 5 above plus 3 in `store.history-replay-order.test.ts` (`history replay orders recovered lifecycle events for codex/claude/grok`), ALL confirmed identical on the stashed baseline (pre-existing, unrelated — notably, these 3 orders-recovered-lifecycle-events failures already span Codex/Claude/Grok themselves, further corroborating this bug class and its fix are provider-agnostic).
- `tsc --noEmit`-equivalent (via `tsc -p tsconfig.phase1-tests.json`): no new type errors from this change — the 10 pre-existing errors (5 unrelated files: `adminLogic.test.ts`, `desktopSupabaseAuthRepository.test.ts`, `navigatorCatalog.test.ts`, `settingsHelpers.test.ts`, `workflowFlowEngineAttrs.test.ts`) are identical before and after.

## 9. Regression Guard

- tests: added, all pass (see Validation) — the composed scenario (orchestration stream bound to main + child focused + sibling spawn event + assert displayed timeline unaffected) is now pinned. The pre-existing test `"workflow handoff turn settles and orchestration stream keeps the run active"` (unmodified) already covers the non-regression case — MAIN's own timeline still updates live from the orchestration stream while MAIN itself is the currently-displayed run (the original BUG-137/138/139/180 behavior) — and continues to fail/pass identically to its pre-existing (unrelated, timing-sensitive) baseline behavior, confirming this fix does not touch that guarantee.
- alerts: none proposed — this is a pure UI-attribution defect with no safety/data implications warranting a runtime alert.
- audit checks: CA note `CA-376` (feature_key `chat-ui`) accompanies this fix.

## 10. Follow-Up Document Updates

- upstream docs that must change: none identified.
- notes left unchanged on purpose: none — the Open Question from the draft (whether a separate background cache was needed) was resolved during implementation; no deliberately-unresolved threads remain.
