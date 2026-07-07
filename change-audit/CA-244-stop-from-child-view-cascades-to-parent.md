# CA-244: Stop From Child-Focused View Cascades To Parent Loop

## Summary

Fixed a Stop-button gap: pressing Stop while viewing a focused child agent's read-only transcript only interrupted that child, leaving the parent run (and its agent loop) running until the user switched back to the main chat and pressed Stop a second time. `store.ts`'s `stop()` now drives the parent-loop-stop / parent-interrupt path regardless of which run is focused, with a best-effort secondary interrupt on the other run so both settle in one press. No backend change was needed — `Interrupt`/`stopAgentLoop` already cascade parent-to-children per CA-223; this closed the mirror-image gap on the child-focused Stop button that CA-223 didn't cover.

## What Changed

- `apps/desktop-flowpilot/src/state/store.ts`: `stop()` — removed the `!childFocused &&` guard from both the loop-stop branch and the `workflow_step_auto` branch so they fire regardless of which run is focused; added a best-effort secondary `client.interrupt()` call on the non-primary run (child when the parent branches fire, parent when the plain fallback fires) in each branch.
- `apps/desktop-flowpilot/src/state/store.test.ts`: replaced `"stop keeps child-focused stop on the child run"` (which encoded the bug) with `"stop from a child-focused view also stops the parent loop (BUG-247)"` and added `"stop from a child-focused view with an active tracked loop stops parent and child (BUG-247)"`.

## Verification

- `apps/desktop-flowpilot`: `tsc --noEmit` passes.
- `store.test.ts` full suite (76 tests) run via an isolated scoped-tsconfig + alias-resolution Node harness: 74 passed, 2 pre-existing environment-only failures (no DOM `localStorage`; one timing-sensitive test) — same failures confirmed present on the pre-fix baseline, no regressions.
- `go vet ./internal/runner/...` in `apps/local-runner` — no issues (no Go changes made; backend cascade already correct).

# ---8<--- flowpilot:change-ledger
feature_key: agent-spawn
source_doc_id: BUG-247
change_type: bugfix
summary: Make the child-focused Stop button also stop the parent run and its loop in one press, instead of requiring a second Stop from the main chat
# --->8---
