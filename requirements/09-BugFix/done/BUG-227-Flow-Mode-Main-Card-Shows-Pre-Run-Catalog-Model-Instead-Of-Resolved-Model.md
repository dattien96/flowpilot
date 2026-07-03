# BUG-227: Flow Mode Main Card Shows Pre-Run Catalog Model Instead Of Resolved Model

## Metadata

- Document ID: `BUG-227`
- Title: `Flow Mode Main Card Shows Pre-Run Catalog Model Instead Of Resolved Model`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [Task-183: User-Owned Model/Provider In Flow Mode](../../08-Task/todo/Task-183-User-Owned-Model-Provider-Resolution-Across-Chat-And-Flow.md), [BUG-165: Implement Step > Flow > Project > Default Model Resolution](./BUG-165-Implement-Step-Flow-Project-Default-Model-Resolution.md), [BUG-171: Flow Run Provider Not Reconciled With Resolved Model](./BUG-171-Flow-Run-Provider-Not-Reconciled-With-Resolved-Model.md)
- Child Documents: `none`
- Related Documents: [BUG-228: Step-Level Model Override Not Re-Resolved Mid-Flow](./BUG-228-Step-Level-Model-Not-Reresolved-Mid-Flow.md), [CA-226: Hub Synthesis Fallback Escalation](../../../change-audit/CA-226-hub-synthesis-fallback-escalation.md)
- Replaces: `none`
- Tags: `agent-flow-engine, desktop, agents-panel, model-resolution, display-bug`

## AI Quick View

### Summary

- User ran the built-in "Review Loop" workflow (`workflows.model_override = claude-haiku`). Every spawned sub-agent (coder, reviewer_correctness, reviewer_security) correctly showed `CLAUDE claude-haiku` in the Agents panel, but the `main` orchestrator card showed `CODEX gpt-5.4-mini` — a provider/model that was never actually running.
- Root cause is a display-only bug in `AgentsPanel.tsx`: the main card's provider/model gave a pre-run **catalog preview** (`selectedWorkflow?.model || project?.model`, meant only to render a greyed/tooltip'd state before Run is clicked per Task-183 T-7) priority over `workflowStepRuntimeMeta` (the run's actual resolved posture, populated once the run starts). When the selected workflow's own `.model` lookup missed and fell through to the **project's** default model (a Codex model, unrelated to this specific workflow's override), that stale preview outranked the correct, already-fetched runtime value.
- Backend model resolution itself was already correct (`interactive_handlers.go` `createRun`, verified via `providerKeyFromModel` provider derivation, BUG-171) — nothing server-side needed to change.

### Current Ask

- Once a run has started, the main card must show the run's actual resolved provider/model (`workflowStepRuntimeMeta`), not the pre-run catalog guess.

### Key Decisions

- `F-1` Extracted the main card's provider/model priority chain into a pure, exported function `resolveMainAgentDisplay` (`AgentsPanel.tsx`) so the ordering has a direct unit-test surface.
- `F-2` Reordered priority to `runtimeMeta* → resolved* (pre-run preview) → selected* (last chat-controller pick) → "codex"/"" default`. The pre-run preview still wins before a run exists (`runtimeMeta*` is empty then, per Task-183 T-6/T-7), but is superseded the instant the real run posture is available.

### Constraints

- Do not change backend model resolution — it was already correct per BUG-165/BUG-171; this is a desktop-only display fix.
- Do not change the pre-run preview behavior (Task-183 T-6/T-7: no model shown until a flow is picked, then greyed/tooltip'd entries when unresolved) — the preview must still be shown before a run starts.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx:51-90` (`resolvedModel`/`resolvedProvider` pre-run preview vs. `workflowStepRuntimeMeta` runtime posture, `resolveMainAgentDisplay`)
- `apps/desktop-flowpilot/src/state/store.ts:496-522` (`refreshWorkflowStepRuntime` — the fetch that populates `workflowStepRuntimeMeta` from the run's actual posture, documented in `contract.ts` as "the RUN's own posture")
- `apps/desktop-flowpilot/src/types/contract.ts:188-198` (`WorkflowStepsRuntimeSnapshot.provider/model`)
- `apps/local-runner/internal/runner/interactive_handlers.go:664-686` (`createRun` provider derivation — confirmed already correct, not touched)

## 1. Issue Summary

The Agents panel's `main` orchestrator card displayed a stale/wrong provider and model (`CODEX gpt-5.4-mini`) for a Flow-mode run whose actual, correctly-resolved posture was `CLAUDE claude-haiku` — the same posture every spawned sub-agent card already displayed correctly.

## 2. Parent Links

- impacted coding plan: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- impacted tech design: `none` (display-only; no SD-06 resolution-rule change)
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, Flow Mode, a built-in workflow with a non-default `model_override` (e.g. "Review Loop" → Claude Haiku) while the project's own default model differs (e.g. a Codex model) or `selectedWorkflow` catalog data has not resolved by first render.
- reproduction steps: select the "Review Loop" workflow in Flow Mode and start a run; observe the Agents panel while the run is active.
- frequency: deterministic whenever the pre-run preview (`selectedWorkflow?.model || project?.model`) resolves to a value that differs from the workflow's actual `model_override`.

## 4. Expected vs Actual

- expected: the `main` card shows the run's actual resolved provider/model once the run has started, matching every sub-agent card spawned under it.
- actual (pre-fix): the `main` card showed a provider/model derived from a pre-run catalog guess, which could disagree with the real running posture and never updated to the correct value even after the run's runtime meta was successfully fetched.

## 5. Impact

- users affected: anyone running a built-in or user flow whose resolved model differs from the project's own default model.
- workflows affected: Flow Mode `main` card display only — spawned agents, actual execution, and provider selection at run time were never affected (execution-correct, display-wrong).
- severity: low-to-medium — confusing/misleading UI during a live run, no functional or safety impact.

## 6. Root Cause

- confirmed cause: `AgentsPanel.tsx`'s `mainProvider`/`mainModel` computation gave the pre-run catalog preview (`resolvedProvider`/`resolvedModel`) priority over the authoritative post-run `workflowStepRuntimeMeta` (`runtimeMetaProvider`/`runtimeMetaModel`), instead of the reverse.
- evidence: see Source Refs; confirmed the run's actual backend-resolved `providerKey`/`modelName` matches what every sub-agent card already displayed, and matches `workflowStepRuntimeMeta` once fetched — only the `main` card's own priority order was backwards.

## 7. Fix Strategy

- `F-1`..`F-2` as described in Key Decisions.

## 8. Validation

- `V-1` New unit tests in `AgentsPanel.test.ts` (`resolveMainAgentDisplay: runtime meta wins over the pre-run catalog preview`, `...falls back to the pre-run catalog preview before any run has started`, `...falls back to the last chat selection, then codex, when nothing else resolves`) — all pass.
- `V-2` Full existing regression suite re-run after the change (Go `internal/runner`, desktop `store.test.ts`/`AgentsPanel.test.ts`, typecheck, build) — see `change-audit/CA-227-*.md` for the exact commands and results.
- `V-3` Not executed: a live re-check against a running desktop app + real Supabase instance — no backend/live environment available in this session; verified via targeted unit tests against the extracted pure function instead.

## 9. Regression Guard

- tests: the three new `resolveMainAgentDisplay` unit tests lock in the priority order.
- alerts: none.
- audit checks: recorded in `change-audit/CA-227-agents-panel-main-card-runtime-meta-priority.md`.

## 10. Follow-Up Document Updates

- upstream docs updated as part of this fix: none required (display-only bug, no SS-05/SD-06 resolution-rule change).
- notes left unchanged on purpose: the related, separate limitation that a step's own `step_definitions.model` has no effect mid-flow (every step in one run shares the run's single resolved model) is tracked independently in `BUG-228` — not fixed here.
