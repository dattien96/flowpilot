# CA-1084: armed bar shows on flow pick; Start flow works with no chat run

Date: 2026-10-01
Refs: user-reported — "Khi mo new chat - chọn flow. Toi bat buoc phai chat
thi UI attach flow moi show. ban phai show ngay lap tuc. Toi la user, toi
muon chat thi chat, muon trigger flow ngay lap tuc thi cung dc"

## Root cause

CA-1083 armed the vibe Workflow-tab pick correctly, but two gates still
forced a chat-first flow:

1. The armed bar rendered only when `runId` existed — the pending arm was
   minted by `sendPrompt`, so nothing showed until the first message sent.
2. `forwardArmedFlow` early-returned when `!runId`, so pressing "Start flow"
   on a fresh session was a no-op even if the user wanted to launch
   immediately.

## Fix

Desktop only; no runner change.

- `selectWorkflow`: in vibe mode with no run, a picked flow pre-arms
  `pendingFlowArm` immediately (resolved via `packFlowId`, gated on
  `userFlowSelectableForMode("vibe", …)`). A live run's latch is never
  rewritten (`!runId` guard); picking a non-vibe/empty value clears the
  pre-arm.
- `setWorkingMode`: mode switches with no run discard the pre-arm.
- `forwardArmedFlow`: when no run exists, mints the armed pending run
  (`chatMode:"normal_chat"` + `flowRef` + `flowArm:"pending"` + flow's
  resolved model — same chain as the CA-1083 sendPrompt arm — plus the
  picked `sourceDocId`), commits `runId`/`activeStepId`, then sends the
  forward turn in the same gesture. One turn total — the typed text seeds
  the flow's entry leg instead of spending a provider chat turn.
- `sendPrompt`: honors a pre-run arm (`pendingFlowArm`) even when
  `launchMode` didn't derive one from `selectedWorkflowId` (bar-only pick);
  the launch-target guard no longer rejects an armed send with no workflow/
  step selected.
- `ChatInput.tsx`: bar gate drops `runId` (shows whenever an arm exists or
  vibe mode is on and the flow hasn't started); the bar's flow select
  mirrors pre-run picks via `armPendingFlow`; the CP dropdown/input mirrors
  picks via `setPendingFlowArmSourceDoc` so a pre-run selection rides the
  armed create as `sourceDocId` and survives the run-mint re-seed;
  `canSend` accepts a pending arm without a tab selection.

## Tests

`store.flowArmForward.test.ts` +6 (all green; existing 15 unchanged):

- `selectWorkflow` in vibe pre-arms before any run
- pre-arm never rewrites a live run's latch on re-pick
- pre-armed Start flow mints the armed run (pending + flowRef + flow model
  + sourceDocId) and sends exactly one `forwardFlow` turn on it
- a CP picked pre-run rides the armed create and survives the re-seed
- a bar-only pick arms the first chat send
- dev-mode switch with no run discards the pre-arm

Phase-1: 741 tests / 729 pass / 12 fail — identical pre-existing baseline
set. `tsc --noEmit` clean.
