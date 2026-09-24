# CA-961 — Worktree conflict card + wire-shape conflict detection

## Summary

Task-436. A `worktree_merge` apply_patch conflict previously surfaced only as
a bare error toast — the card stayed on the three mode buttons and the user
had no path to "fix the conflict, then retry".

The inbox card now flips to a `worktree_conflict` state on a 409 conflict:
it lists `conflictPaths`, shows `patchArtifactRef`, and offers
Cancel / Retry (retry resends the same mode, no confirm flag). Confirm and
conflict states are mutually exclusive on the attention item.

**Wire-shape fix folded in**: `handleWorktreeResolve` writes the evidence
map bare for conflict/confirm 409s — the body has `conflict:true`, not an
`error.code` field. The initial detection keyed on
`err.code === "worktree_merge_conflict"`, which never fires on the real wire
(code falls back to `"http_error"`). Detection now keys off
`details.conflict === true` (code check kept for compatibility), and the
card message prefers `details.reason` over the `Conflict` statusText.

## Changes

- `src/state/attentionQueue.ts` — `WorktreeConflict` type +
  `worktreeConflicts` map + `setWorktreeConflict`/`clearWorktreeConflict`;
  mutually exclusive with `worktreeConfirm`; `evict` clears both.
- `src/state/store.ts` — conflict branch in `submitAttentionDecision`
  (detects `details.conflict === true || code === worktree_merge_conflict`),
  `dismissWorktreeConflict`.
- `src/components/DecisionControls.tsx` — `worktree_conflict` model + card
  render (message, ≤5 conflict paths + overflow count, patch ref,
  Cancel / Retry <mode>).
- `src/components/AttentionInbox.tsx` — wires `dismissWorktreeConflict`.

## Tests (additive)

`src/state/worktreeMergeConfirm.test.ts`:
- `worktree conflict 409 flips card to conflict state`
- `worktree conflict retry resends same mode`
- `dismissWorktreeConflict restores mode buttons`
- `confirm and conflict states are mutually exclusive`
- `worktree conflict detected on wire shape (no code field)` — guards the
  wire-shape fix

Provider-agnostic: operates on git/worktree state, no provider adapter path.
