# CA-1191 — focused member-run composer (send a turn to a child)

## Defect

During live sprint CP-03, every operator bypass for a stuck member was a
hand-rolled HTTP call — most importantly `POST /turns` against a parked
member run id to un-stick a child owing work with no gate card. Desktop
users had no equivalent: a focused child transcript was read-only
(`ChatInput` rendered a note + Stop only; `sendPrompt` hard-refused child
focus), and `@agent` mention routing dead-ended on the same guard for any
non-busy member. injectAgentFeedback only queues a bus message — it never
dispatches a turn, so it cannot un-park a member.

## Fix

- `store.sendAgentRunPrompt(prompt, attachments)` — posts `client.sendTurn`
  to `activeAgentRunId` (the focused member run) with the run's own
  `activeStepId`, prompt bubble + thinking state on the focused transcript,
  `consumeStream` folding the reply into view, draft restore + error
  surfaces mirroring `sendPrompt` (409 `flow_awaiting_user` → blocked/warn,
  not failed). Falls back to `sendPrompt` when focus is on the main run.
- `ChatInput`: the focused-member branch renders a real textarea + send
  button; `canSend` permits a focused-member send when the member's
  `agentStatus` is not mid-turn (`running`/`spawned`). `@member` mention
  routing targets non-main runs through `sendAgentRunPrompt` too.
- Admission stays server-side — a mid-turn or gate-locked member still
  answers 409; the desktop merely exposes the turn endpoint.

## Regression evidence

- `store.sendAgentRunPrompt.test.ts`: turn posts to the focused child id
  with stepId + idempotency key, prompt bubble lands on the focused
  transcript; non-child focus falls back to `sendPrompt`; 409
  flow_awaiting_user maps to blocked/warn.
- 57/57 phase1 tests green (sendAgentRunPrompt + flow-awaiting-user full
  matrix + flowAwaitingUserDrift + AgentsPanel).

## Files

- `apps/desktop-flowpilot/src/state/store.ts` — `sendAgentRunPrompt`
- `apps/desktop-flowpilot/src/components/ChatInput.tsx` — member composer
  branch, canSend gating, mention-routing target
- `apps/desktop-flowpilot/src/state/store.sendAgentRunPrompt.test.ts`
