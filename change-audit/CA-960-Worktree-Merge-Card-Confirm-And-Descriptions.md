# CA-960 — Worktree merge card: per-action descriptions + requiresConfirm flow

## Summary

Task-435. The worktree merge inbox card rendered three bare buttons with no
consequence text, and had no way to answer the runner's `409 requiresConfirm`
gate (CA-958 keep_branch; SD-27 Q-2 discard) — the submit just `fail`ed and
the user was stuck.

Two changes:

1. **Descriptions** — `WORKTREE_MODES` (new exported const) gives each mode a
   one-line consequence; the worktree card renders it beside each button.
   Destructive modes state their loss explicitly (keep_branch: "Only
   committed work survives"; discard: "all changes are thrown away").
2. **Confirm flow** — `RunnerApiError.details` now carries the full response
   body (409 top-level fields). `submitAttentionDecision` intercepts a
   worktree_merge 409 with `requiresConfirm:true`, stores a
   `WorktreeConfirm` on the attention item (queue-level projection, like
   `acting`/`evicted`), and returns true — the card re-renders as a
   Cancel/Proceed step listing the files at stake. Proceed resends
   `resolveWorktreeMerge(runId, mode, confirm:true)`; Cancel clears the
   confirm and restores the mode buttons. `evict` clears the confirm.

Non-confirm 409s (`worktree_merge_conflict`) still surface as errors —
fail-closed preserved.

## Files

- `src/client/HttpWsRunnerClient.ts` — `RunnerApiError.details` (5th ctor
  arg, optional; `parse` passes the response body)
- `src/state/attentionQueue.ts` — `WorktreeConfirm` type, `worktreeConfirms`
  map, `setWorktreeConfirm`/`clearWorktreeConfirm`, overlay at recompute,
  cleared in `evict`
- `src/state/store.ts` — `submitAttentionDecision` gains `confirm` param;
  worktree_merge catch intercepts 409-requiresConfirm;
  `dismissWorktreeConfirm` action
- `src/components/DecisionControls.tsx` — `ControlChoice.description`,
  `WORKTREE_MODES`, `worktree_confirm` model + render, `onDismiss` prop,
  `onAct` gains `confirm` arg
- `src/components/AttentionInbox.tsx` — wires `confirm` through `onAct`,
  `onDismiss` → `dismissWorktreeConfirm`
- `src/client/MockRunnerClient.ts` — `resolveWorktreeMergeError` test hook
- `src/styles.css` — `.inbox-act-group--worktree`, `.inbox-worktree-mode`,
  `.inbox-act-desc`, `.inbox-act-group--confirm`
- `src/state/worktreeMergeConfirm.test.ts` — new, 5 tests

## Verified

- `npm run typecheck` clean.
- `worktreeMergeConfirm.test.js` — 5/5 pass (descriptions, 409→confirm
  state, confirm resend + evict, dismiss restores buttons, non-confirm 409
  still fails).
- `inboxDecisions.test.js` — 9/9 still green (additive-only).
- Full desktop suite: 537/547 pass; the 10 failures reproduce identically
  on the pre-change baseline (verified via stash) — `localStorage is not
  defined` DOM-env gaps and unrelated ordering assertions.

## Provider parity

Provider-agnostic — decision-payload handling at the desktop inbox layer;
the runner's 409 contract is identical for all providers.
