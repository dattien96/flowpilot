# CA-1052 — desktop wires CP-89 chat-then-forward (flowArm pending + forwardFlow)

## What changed

The runner implemented the CP-89 launch latch (`flowArm`: `immediate` /
`pending` / `started`) and the `forwardFlow` turn flag in Task-451/452/453,
but no client sent them — TUI arms via a local `/vibe` two-run flow and the
desktop had no fields at all. This wires the desktop:

- `src/types/contract.ts`: `StartRunInput.flowArm?: "pending" | "chat_then_forward"`,
  `TurnInput.forwardFlow?: boolean`, `RunHandle.flowRef`/`flowArm`,
  `RunHistoryItem.flowArm`.
- `src/client/HttpWsRunnerClient.ts`: `sendTurn` body carries `forwardFlow`
  (`startRun` already POSTs the whole input).
- `src/state/store.ts`:
  - vibe-mode first send creates the run with `flowArm:"pending"` +
    `sourceDocId` pin (the forward-time ingest fence validates off the pin,
    no repaste) instead of letting the first turn launch the flow;
  - `pendingFlowArm?: { flowRef }` state — set on armed create, restored in
    `openHistoryRun` from the durable latch (handle `flowArm` first, history
    row as fallback), cleared by `resetRun`;
  - `forwardArmedFlow(forwardText?)` action — the only path that sends
    `forwardFlow:true`, reusing the same turn stream + retry seams as
    `sendPrompt` (module-level `TRANSIENT_SEND_*` / `CONN_SEND_*` constants
    hoisted for sharing); restores the armed affordance if the forward is
    rejected (the runner rolls the latch back to pending);
  - `mintedRunHistoryRow` carries `flowArm` so locally minted history rows
    keep the armed signal.
- `src/components/ChatInput.tsx` + `styles.css`: armed strip above the
  composer — "Flow armed: <ref> — chat freely…" + a Start flow button that
  sends the composer text as the forward prompt (the runner folds it plus
  the settled transcript into the flow's entry leg). Disabled while busy.
- `src/client/MockRunnerClient.ts`: mirrors the latch — persists
  `flowArm`/`flowRef` on run create, echoes on resume/history, flips
  pending→started on a `forwardFlow` turn.

## Bug fix riding along

- `src/state/timelineReducer.ts`: `closeAssistant()` now flips the open
  assistant bubble to `finalized:true`. Providers that end a turn without a
  plain `message_completed` (e.g. deltas then `turn_completed`, or the
  placeholder-completed path) left the bubble unfinalized, so the streaming
  caret (▌) kept blinking at the end of every response — including cached
  snapshots of past chats.

## Invariant

Client wiring only — the runner's durable latch, forward path, rollback, and
idempotency are untouched. A pending run's ordinary chat turns carry no
`forwardFlow` and cannot launch the flow; the composer text the user types
before pressing Start flow rides as the forward prompt.

## Tests

- `state/store.flowArmForward.test.ts` (new, 6/6): armed-pending create,
  plain second turn, forward on the same run, busy/no-arm no-op, history
  restore of `pending` vs `started`.
- `state/timelineReducer.test.ts` (+2): `turn_completed` and `tool_started`
  finalize a streaming bubble that never saw `message_completed`.
- `npx tsc --noEmit` clean.
