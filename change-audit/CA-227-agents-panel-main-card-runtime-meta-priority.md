# CA-227: Agents Panel Main Card Runtime-Meta Priority

## Summary

Fixed the Agents panel's `main` orchestrator card showing a stale/wrong provider and model (e.g. `CODEX gpt-5.4-mini`) for an active Flow-mode run whose actual resolved posture was different (e.g. `CLAUDE claude-haiku`, from the workflow's `model_override`), while every spawned sub-agent card already showed the correct posture. Root cause was a priority-order bug in `AgentsPanel.tsx`: the pre-run catalog preview (`selectedWorkflow?.model || project?.model`) outranked the run's actual resolved posture (`workflowStepRuntimeMeta`) instead of the reverse. Extracted the priority chain into a pure, exported `resolveMainAgentDisplay` function with direct unit-test coverage. Also filed `BUG-228` to separately track the related, pre-existing, intentionally-out-of-scope limitation that a non-entry flow step's own `step_definitions.model` has no effect mid-flow (every step in one run shares the run's single resolved model) — not fixed here, only documented.

## What Changed

- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx`:
  - Extracted the `mainProvider`/`mainModel` priority computation into a new exported pure function `resolveMainAgentDisplay(input)`.
  - Reordered priority from `resolvedProvider/resolvedModel (pre-run preview) → runtimeMeta* → selected* → default` to `runtimeMeta* (actual run posture) → resolvedProvider/resolvedModel (pre-run preview) → selected* (last chat-controller pick) → "codex"/"" default`.
- `apps/desktop-flowpilot/src/components/AgentsPanel.test.ts`:
  - Added three regression tests for `resolveMainAgentDisplay`: runtime meta wins over a conflicting pre-run preview; the pre-run preview still shows before any run starts (runtime meta empty); and the last-resort fallback chain (last chat selection, then `"codex"`/`""`) when nothing else resolves.
- `requirements/09-BugFix/done/BUG-227-Flow-Mode-Main-Card-Shows-Pre-Run-Catalog-Model-Instead-Of-Resolved-Model.md`: new bug doc recording the fix.
- `requirements/09-BugFix/todo/BUG-228-Step-Level-Model-Not-Reresolved-Mid-Flow.md`: new bug doc recording the separate, deliberately-unfixed known limitation (step model ignored mid-flow), with two candidate future fix directions left for a product decision.

## Verification

- `go build ./...` in `apps/local-runner` — passes.
- `go test ./internal/runner/...` — 1053 passed, 16 failed (pre-existing, unrelated: Windows path mismatches, mocked Codex CLI resume, skills-merge ordering — confirmed identical failure set before this change), 14 skipped.
- `go test ./internal/runner -run 'TestE2EReviewLoop|TestStartTurnWithFlowRefEmitsSyntheticTurnCompletedForHubHandoff|TestAutoReinvokeHub|TestReviewOutcomeToolOfferedOnlyOnHubSynthesisTurn|TestSubmitFlowControlRejectsCohortMemberButAllowsHub|TestCodexAdapterSubmitReviewOutcome|TestE2EParallelCodingCohortReinvokesHub'` — 22 passed (uncommitted CA-226 synthesis-hang fix regression suite, unaffected by this change).
- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `npx tsx --test src/components/AgentsPanel.test.ts src/state/store.test.ts` — 66 passed, 2 failed; confirmed via `git stash` that both failures (`selectProject resets the active chat run when switching projects` — missing `localStorage` in the Node test runner; `sendPrompt aborts an open-ended history replay stream before sending` — timing-sensitive) pre-date this change and are unrelated to `AgentsPanel.tsx`/`resolveMainAgentDisplay`.
- `npm run build` in `apps/desktop-flowpilot` — passes.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-183
change_type: bugfix
summary: Fix Agents panel main card to prefer the run's actual resolved provider/model over the pre-run catalog preview
# --->8---
