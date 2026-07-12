# BUG-275: Hub Synthesis Prompt Duplicates Joined Notes And Misresolves Feature History

## Metadata

- Document ID: `BUG-275`
- Title: `Hub Synthesis Prompt Duplicates Joined Notes And Misresolves Feature History`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex`
- Created: `2026-07-11`
- Last Updated: `2026-07-11`
- Parent Documents: [CP-45](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md)
- Child Documents: `None`
- Related Documents: [CA-288](../../../change-audit/CA-288-cp45-e2e-edge-dirty-file-path-only-hub-dedupe.md), [Task-224](../../08-Task/done/Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md)
- Replaces: `None`
- Tags: `agent-flow-engine, hub, feature-history, done`

## AI Quick View

### Summary

- Hub synthesis embedded cohort joined note twice (pendingAgentContext + embed).
- Flow-engine synthesis text self-resolved feature history (calc-core → calc-format → sandbox-meta).
- Fixed: remove one pending copy when embedding; `isFlowEnginePrompt` (CA-288).

### Current Ask

- Closed for duplicate/drift. Further “who gets history bulk” policy is Task-224 / BUG-277.

### Key Decisions

- `V-1` Join note once in synthesis prompt.
- `V-2` Flow-engine prompts are system for feature resolve.

### Constraints

- Do not drop join note on in-flight race paths.

### Open Questions

- None for this bug.

### Source Refs

- `run-41047` pre-fix; `run-309` post-fix; CA-288.

## 1. Issue Summary

Hub synthesis prompts duplicated joined notes and misresolved feature history from internal flow-engine text.

## 2. Parent Links

- impacted coding plan: `CP-45`
- impacted tech design: `SD-17`
- impacted system spec: none specific

## 3. Environment and Reproduction

- environment: flow review cohort join → hub reinvoke
- reproduction: inspect hub `prompt-turn-*.txt` after join
- frequency: deterministic pre-fix

## 4. Expected vs Actual

- expected: one joined note; feature stays on originating key
- actual: duplicate notes; feature drift

## 5. Impact

- Wrong history on hub synthesis; noisy prompts; user confusion main↔child.

## 6. Root Cause

- confirmed: note both in `pendingAgentContext` and embedded prompt; flow-engine text not classified as system for resolve.

## 7. Fix Strategy

- `F-1` Dedupe pending when embedding; `isFlowEnginePrompt` (CA-288).

## 8. Validation

- `V-1` Unit tests for dedupe + feature inherit; live `run-309` shape.

## 9. Regression Guard

- tests: `TestAutoReinvokeHubWithNoteDedupes…`, `TestResolveInjectionFeatureFlowEngine…`
- audit: CA-288

## 10. Follow-Up Document Updates

- Task-224 for broader history placement policy.
