# CA-1053 — CP-89 desktop review fixes: late-attach, reattach latch, chat-scoped forward context, caret tail

## What changed

Review of CA-1052 surfaced six defects; this entry fixes all of them across
runner + desktop.

### Runner (`apps/local-runner/internal/runner`)

- `forward_prompt.go`: the forward entry prompt now reads the **chat-scoped**
  durable transcript (`forwardChatTranscriptTurns` → `ReadChatRecords` on the
  chatID) instead of the current leg's `rs.events`. A reattach or provider
  switch mints a leg whose event log starts empty; leg-scoped reads silently
  dropped the whole pre-switch discussion. System prompts and the trailing
  unanswered turn (incl. this forward's own `turn_started`) are excluded.
  `buildForwardPromptPackage` takes the pre-read turns and falls back to the
  leg-scoped settled turns when the transcript store is empty/unavailable.
- `interactive_service.go`: the transcript read runs in a **lock gap** —
  chat-store I/O never happens under `s.mu` (SD26-S-2). `turnInFlight` stays
  set across the gap; on relock the forward revalidates run identity,
  terminal state, and re-runs the latch gate + fences before packing.
  `forwardPinnedFlow` now accepts the turn-level `flowRef` as the user's
  FINAL flow choice (overrides/completes the provisional create-time pin)
  and admits a **late arm+forward**: an unpinned, never-driven `immediate`
  chat run whose forward turn carries an explicit `flowRef`.
  `commitPendingFlowStartLocked` stamps `started` directly for the late-arm
  path (`setFlowArmStartedLocked` intentionally stays pending-only) and
  prepared-relaunch replay treats any non-`started` latch as uncommitted.
- `interactive_handlers.go`: `createRun` adopts the newest prior durable
  leg's armed state (`flowArm`, `chatFlowRef`, `sourceDocID`, `workflowID`,
  `workingMode`) for chat reattach legs — a pending pin survives the leg
  hop, and an already-started chat neutralizes a client-sent re-arm
  (a stamped pin on an immediate arm would auto-launch at turnCount==0).
  Scan failure fails closed (`chat_identity_unprovable`). `RunHandle` and
  `runHistoryItem` now echo `sourceDocId` so reopened armed chats can
  re-render the CP path.
- `provider_event.go`: `RunHandle.SourceDocID` field (additive).

### Desktop (`apps/desktop-flowpilot/src`)

- `state/store.ts`:
  - `pendingFlowArm` gains `sourceDocId`; the detached-reattach `startRun`
    re-declares `flowRef` + `flowArm:"pending"` + `sourceDocId` (guarded —
    `flow_arm_requires_flow` rejects an arm with no pin), and applies the
    new leg's echoed latch (`pending` keeps the banner, `started` clears it
    and sets `flowStarted`, absent echo keeps prior local state);
  - `flowStarted` state — set on forward success, restored from
    handle/history `flowArm === "started"` in `openHistoryRun` and
    `switchChatProvider`, cleared in `resetRun` / legacy switch;
  - `forwardArmedFlow(text, {flowRef, sourceDocId})` — sends the final flow
    choice + optional CP path on the forward turn, carries a stable
    `idempotencyKey` (post-commit retry replays the minted turnId instead of
    422ing on `flow_already_started`), guards every late write behind
    `_streamRunSeq` + `shouldApplyRunEvent` (a rejection landing after a chat
    switch no longer fails the focused run), and restores the composed text
    into the drafts map on rejection;
  - `restoreRunSnapshot` finalizes cached assistant bubbles and clears
    `_streamingAssistantId` — a `finalized:false` bubble in a cached
    snapshot was a stale caret that survived every reopen (live replay still
    re-opens the bubble id via `message_delta`).
- `state/timelineReducer.ts`: `message_completed` no longer dereferences
  `e.text` unconditionally — a textless completion (pure end-of-turn
  signal) crashed the reducer (`timeline_terminal_parity`) and no longer
  pushes an empty bubble.
- `components/ChatInput.tsx` + `styles.css`: the armed strip is now a flow
  picker (`vibe-ingest` / `vibe-cp-ingest`, suffix-normalized against
  pack-prefixed refs) + a CP-doc path input (required for `vibe-cp-ingest`)
  + Start flow. It renders for armed chats and — when nothing is armed and
  the chat is in vibe mode — as the late-attach "Start a flow" affordance.
  It hides once `flowStarted` is true. A drafts→composer subscription
  repopulates text/skills/attachments written back by a failed send or
  forward (previously the draft only reappeared on lane switch).
- `client/MockRunnerClient.ts`: mirrors the new gate — turn-level `flowRef`
  override, late attach, latch echo on switch handles, reattach adoption.
- `types/contract.ts`: `RunHandle.sourceDocId`, `RunHistoryItem.sourceDocId`.

## Invariants kept

- `immediate` remains the wire default; pending is opt-in. Forward is still
  the only path that flips the latch, still durable-first, still fenced.
- Chat-store I/O happens only outside `s.mu`; the gap revalidates before
  any commit.
- A second forward after commit still 422s `flow_already_started`; a rejected
  forward leaves the run byte-identical (retryable).

## Tests

- `cp89_desktop_fixes_test.go` (new, 5 tests): turn-level ref overrides the
  pin; late attach on an unpinned chat flips started + stamps ref; late
  attach still requires an explicit ref; pinned-immediate forward stays
  rejected; forward reads the chat-scoped transcript across legs (diag
  `turns_included`); transcript filter drops system + unsettled tail.
- `store.flowArmForward.test.ts` (+5, now 11): reattach re-declares the arm,
  started echo clears it, late attach, rejected forward restores arm +
  draft, stale result can't corrupt the focused chat.
- `timeline_terminal_parity` + `timelineReducer` suites pass.
- `npx tsc --noEmit` + `vite build` clean. `go test ./internal/runner` — the
  CP-89/forward tests pass; the remaining suite failures reproduce
  identically on the parent commit (pre-existing gate/env debt, out of
  scope).
