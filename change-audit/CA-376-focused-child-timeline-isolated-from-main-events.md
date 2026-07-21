# CA-376: Focused child transcript no longer shows sibling agents spawned by main

## Summary

Fixed BUG-297: while viewing a focused child agent's transcript (e.g. `coder`), sibling agent cards spawned by MAIN (e.g. `reviewer_correctness`, `reviewer_security`, children of main, not of coder) appeared inline inside coder's own displayed transcript. Root cause: `s.timeline` is a single, global field in the desktop store, not partitioned per run. The background "orchestration stream" (`consumeOrchestrationStream`), which stays bound to MAIN for the whole session and is never cancelled while a child is focused, applied MAIN's own live events (e.g. `agent_spawned_by_user` for a sibling) to `s.timeline` unconditionally — its own staleness check compared against the stable `mainRunId`, never against whichever run is actually displayed.

Fix: the stream's live-apply branch now skips mutating the shared timeline unless MAIN is the currently displayed run (`get().runId === runId`). Verified this loses nothing on return: `backToMainRun` already replays every event since its own pre-focus snapshot independent of what the orchestration stream did while away, so no separate background-cache mechanism was needed.

## Verification

- New additive test in `store.test.ts`: `"orchestration stream does not bleed a sibling agent_spawned_by_user into a focused child's timeline"` — passes with the fix, and correctly fails on a stashed (reverted) baseline, confirming it genuinely detects the bug.
- This environment has no wired-up runner for `apps/desktop-flowpilot/src/**/*.test.ts` (`npm run test:phase1`'s compile step fails on unrelated, pre-existing stale fixtures under `tests/phase1/`, and direct `node --test` execution hits `@/` path-alias / `node_modules` resolution gaps predating this fix). Worked around with a temporary, uncommitted `--require` hook resolving the same aliases `tsconfig.phase1-tests.json` defines, to get a real signal from the actual project test suite.
- `store.test.ts` overall with the fix: 82 passed, 5 failed — all 5 reproduce byte-for-byte identically on the stashed baseline (pre-existing, unrelated, confirmed across 3 repeated runs).
- Broader related-file battery (store/timeline tests, 133 tests total): 125 passed, 8 failed — the same 5 plus 3 in `store.history-replay-order.test.ts` (codex/claude/grok lifecycle-ordering), all confirmed identical on the stashed baseline.
- `tsc -p tsconfig.phase1-tests.json`: no new type errors from this change (10 pre-existing errors in 5 unrelated files, identical before/after).
- Confirmed provider-agnostic: `consumeOrchestrationStream`/`applyEvent`/`applyTimelineEvent` never branch on `providerKey`.

## Files

- `apps/desktop-flowpilot/src/state/store.ts`: `consumeOrchestrationStream`'s live-apply branch gated on `get().runId === runId`.
- `apps/desktop-flowpilot/src/state/store.test.ts`: additive regression test.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-297
change_type: bugfix
summary: A focused child agent's transcript no longer shows sibling agents main spawned live while the user was looking elsewhere; the background orchestration stream now only writes to the shared timeline when main itself is the displayed run.
# --->8---
