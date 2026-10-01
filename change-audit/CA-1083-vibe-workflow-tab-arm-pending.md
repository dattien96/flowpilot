# CA-1083: vibe-mode Workflow-tab picks arm pending (CP-89) + real CP dropdown

Date: 2026-10-01
Refs: user-reported — "vibe-tasks requires a
requirements/07-Coding-Plan/**/CP-*.md source document (HTTP 422 /
invalid_cp_source)" fired on the FIRST chat message, and "cp89 … khi chọn 1
workflow, thì tôi hoàn toàn có quyền chat normal với AI trước".

## Root cause

Two desktop gaps made CP-89 chat-then-forward unreachable from the Vibe +
Workflow-tab entry:

1. `sendPrompt` dispatched `workflow_step_auto` sends straight to
   `startRun({workflowId})` — an **immediate** launch. In vibe mode that meant
   the very first typed message hit the runner's flow fences. For
   `vibe-cp-ingest`/`vibe-tasks` the fence demands a CP source the picker
   path never collected → `422 invalid_cp_source` on message one.
2. `ChatInput` only rendered the armed/Start-flow bar under `isChatMode`, and
   CA-1070 disables the Chat tab in vibe mode — so even after arming, the
   forward affordance was invisible. `needsSourceDoc` also covered only
   `vibe-cp-ingest` (not `vibe-tasks`) and the CP source was a free-text
   field the user had to retype.

Runner-side contract was already correct and needed no change: a
`flowArm=pending` run's turns stay plain (the turn-1 CP fence is
`FlowArmImmediate`-only), and `runFirstTurnFences` validates the CP source
at **forward** time — `isVibeCpSourcedFlowID` already covers `vibe-tasks`.

## Fix

Desktop only; no runner change.

- `store.ts` `sendPrompt`: a vibe-mode Workflow-tab send whose picked
  workflow resolves (via `packFlowId`) to a vibe user-selectable flow now
  creates a `chatMode:"normal_chat"` run with `flowRef` + `flowArm:"pending"`
  — identical to the `detectVibeEntry` chat arm — instead of a `workflowId`
  launch. Gated on `userFlowSelectableForMode("vibe", bare)` so a stale
  non-vibe `selectedWorkflowId` keeps the fail-closed immediate path (the
  runner's create-time mode gate still rejects it). History row mints as
  `runKind:"chat"` with `flowArm:"pending"` so reopening restores the armed
  bar. `turnStepId` uses the run's synthetic chat step while an arm is
  pending. The armed create also stamps the picked flow's resolved model
  (workflow override → project default → composer pick) as `model` — the
  armed run IS that flow's run, so both the pre-forward chat leg and
  post-forward children without a step pin inherit its configured model
  (live run-2198 stamped gpt-5.5/codex while the flow was configured
  devin/swe-2-high, dying on `no connected local account` for codex).
- `ChatInput.tsx`: the armed bar renders whenever `!flowStarted` and an arm
  or vibe mode is present — no longer `isChatMode`-only. `needsSourceDoc`
  now matches `vibe-cp-ingest` **and** `vibe-tasks`. The free-text CP field
  is replaced by a real dropdown populated from
  `client.listWorkspaceFiles(cwd, "07-coding-plan")` filtered through
  `isCodingPlanCPPath` (free-text fallback when the workspace has no CP
  docs). The selected path is sent as `sourceDocId` on the forward turn.

## Tests

`store.flowArmForward.test.ts` +4 (all green, existing 11 unchanged):

- vibe Workflow-tab pick of vibe-tasks → armed pending chat run, no
  `workflowId`, turn 1 has no `forwardFlow`, `model` = the workflow's
  configured model (not the composer pick)
- same for vibe-cp-ingest (no workflow model → project default wins)
- non-vibe flow pick in vibe mode → stays on the fail-closed `workflowId`
  path
- dev-mode Workflow-tab launch unchanged (immediate `workflowId`)

Phase-1: 735 tests / 723 pass / 12 fail — the identical pre-existing
baseline set (jira scoping, history replay, style tokens, …); no new
failures. `tsc --noEmit` clean.

## Live verification

Probe runner (fresh build) on :4318 against the real remote:

- `POST /client/workflow-runs` `{workingMode:"vibe", chatMode:"normal_chat",
  flowRef:"vibe-tasks", flowArm:"pending"}` (no source) → `run-2181`
  created, `flowArm:"pending"`, `runKind:"chat"` — no fence at create.
- `POST …/run-2181/turns` `{forwardFlow:true, flowRef:"vibe-tasks"}` with no
  `sourceDocId` → `422 invalid_cp_source` fail-closed, and the run-history
  row still reads `flowArm:"pending"` — latch survives a rejected forward,
  retry-safe.
- `POST /client/workflow-runs` `{workflowId:"vibe-tasks", workingMode:"vibe"}`
  → `run-2215` created with `providerKey:"devin"` — the remote's current
  config (workflow override + project default + cp_reader/task_plan_reader
  all `devin/swe-2-high`) resolves correctly; run-2198's `gpt-5.5/codex`
  stamp predated the user's Settings save.
- Probe runs deleted; probe runner stopped.
