# CA-932 — Inbox per-kind controls, preview & triage (CP-84 Task-431)

## Summary

AttentionInbox rendered approve/deny for everything or Open-only. Now each
item carries the originating `DecisionPayload` (populated from the mux lane)
and renders per-kind controls:

- `DecisionControls.tsx` — pure `decisionControlModel` descriptor + renderer:
  gate → radio (Fix the code / Suggest requirement change) + custom text +
  Open; ss_lock → Approve spec / Reject + AI Quick View preview; worktree_merge
  → apply_patch / keep_branch / discard; quota → "Switch to X" routed to the
  existing account-switch confirm; approval/question-with-options → choice
  chips. `null` model → caller renders Open-only (unchanged rule).
- `DecisionPreview` — expandable ▸ details (regressed tests, quickView,
  conflictPaths, branch) for heavy kinds; collapsed by default (T-2).
- Triage: kind + project filter selects (local component state, show-all
  default), batch "Approve all" for eligible kinds only (approval +
  single-select question), sequential submits with per-item failure reporting.
- `submitAttentionDecision(runId, decision, choice, customText?)` — ID-scoped
  routing to existing endpoints (`submitApproval`, `answerQuestion`,
  `submitGateDecision`, `confirmSSLock`, `resolveWorktreeMerge`,
  pendingAccountSwitch); never touches `get().runId`. Failure → `runToast`
  + history refresh; item retained (T-4, covers 409/stale).
- `attentionQueue.markActing/clearActing/isActing` — shared in-flight guard
  for all decision kinds.
- New client surface: `resolveWorktreeMerge`, `confirmSSLock` (+ mock stubs).
- T-5: notification click deep-link — `notification:show` carries runId;
  click → `openRunAtAttention` via the Electron bridge.

## Verified

- 9 new tests green (`inboxDecisions.test.ts`) covering all §7 signatures.
- Provider parity: controls render from `decision` payload only — zero
  provider branches.

## Files

- `components/DecisionControls.tsx` (new), `components/AttentionInbox.tsx`,
  `components/RunToast.tsx`, `app/runToastGrouping.ts`, `state/store.ts`,
  `state/attentionQueue.ts`, `types/contract.ts`, `client/*`, `styles.css`,
  `electron/main.ts`, `electron/preload.ts`, `types/flowpilotBridge.d.ts`

# ---8<--- flowpilot:change-ledger
feature_key: attention-queue
source_doc_id: CP-84
change_type: feature
summary: per-kind inbox decision controls, expandable previews, kind/project triage filters, eligible-only sequential batch approve, ID-scoped submitAttentionDecision
# --->8---
