# BUG-373 — `chat --print` headless dies with 400 stepId is required

## Metadata

- Document ID: `BUG-373`
- Title: `Headless chat --print never sends stepId, startTurn rejects with 400`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-16`
- Last Updated: `2026-09-16`
- Parent Documents: `None` (CLI headless path, CP-56)
- Child Documents: `None`
- Related Documents: [CA-886](../../../change-audit/CA-886-BUG-373-Headless-StepId.md)
- Replaces: `None`
- Tags: `cli-tui, headless, chat-print, stepid`

## AI Quick View

### Summary

- `just chat-print "q" <project>` (or `chat --print`) always failed: runner boots, then `turn: runner API error 400 (invalid_request): stepId is required` (tui.log, live 2026-09-16).
- Cause: interactive turns send `resolveTurnStepID()` (runner-minted `chat-<runId>` from the StartRun handle); `runHeadless` built its `TurnInput` without `StepID` at all.
- Second block found live while verifying: offline headless sent no `projectId` either → every turn 502 `dispatch_prepare_failed` (projects API needs Supabase). Fixed in the same headless-only path with a stable local id.
- Fix (`internal/tui/app/app.go`, headless-only): seed `m.runHandle`/`m.stepID` from the fresh handle, send `StepID: m.resolveTurnStepID()`; synthesize `localProjectID(cwd)` (fnv64a) when no catalog project resolves.
- Live proof: `chat --print 'Reply with exactly: PRINT_FIX_OK' --provider grok --model grok-4.5` → prints `PRINT_FIX_OK`, RC=0.

### Current Ask

- `chat --print` works end-to-end (grok): boots runner, sends turn with stepId, prints final message, exits 0.

### Key Decisions

- `V-1` Reuse `resolveTurnStepID()` (same helper as interactive + Desktop parity) instead of inlining `handle.StepID` — resume-without-stepId falls back identically.
- `V-2` `localProjectID` is fnv64a of cwd: deterministic per workspace, no catalog, no new deps (`hash/fnv` already imported).
- `V-3` Interactive TUI untouched (only `runHeadless` changed); provider-agnostic (no providerKey branch on the path — grep 0 hits in the edited hunk).

### Constraints

- `feature_key: cli-tui` (dominant).
- R1: additive tests only — 3 new tests in `run_headless_stepid_test.go`; legacy suite untouched; 2 failing tests proven pre-existing at pristine HEAD (detached worktree, no stash).
- R2: provider-agnostic by construction (turn-envelope fix, verified live on grok per operator Grok-only constraint).

### Completion Notes

- result: fixed + verified. `TestRunHeadlessSendsRunnerMintedStepID`, `TestRunHeadlessResumeWithoutStepIDFallsBack`, `TestRunHeadlessSynthesizesLocalProjectID` green; full `tui/app` 1428 pass except the 2 pre-existing HEAD failures; `tui/client` full green; live `--print` prints `PRINT_FIX_OK` RC=0.
