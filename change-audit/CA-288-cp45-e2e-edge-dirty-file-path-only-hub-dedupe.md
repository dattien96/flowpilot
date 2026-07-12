# CA-288 — CP-45 E2E residuals: edge dirty, file path-only INPUT, hub join dedupe

Closes live E2E residuals from gate-sandbox testing of CP-45 / Task-223:

1. **BUG-274** — `normalizeWorkflowSnapshot` omitted `edges` and policy fields so Save stayed disabled on edges-only edits.
2. **BUG-276** — `file_artifact` INPUT no longer pastes full file body into prompts; path mentions only (agent reads via tools). Context package content push unchanged.
3. **BUG-275** — Hub synthesis no longer double-embeds cohort joined notes; flow-engine prompts treated as system for feature-history resolution (stop calc-core→calc-format→sandbox-meta drift).

## Files Changed

- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` — dirty snapshot + file_artifact INPUT copy
- `apps/local-runner/internal/runner/artifact_type_registry.go` — path-only INPUT inject
- `apps/local-runner/internal/runner/feature_history.go` — `isFlowEnginePrompt`
- `apps/local-runner/internal/runner/interactive_service.go` — pending note dedupe on hub reinvoke
- Tests updated/added under runner package

## Residual Notes

- Desktop visual main/child card mixing (if any remains) is verification-only; primary fix was prompt composition.
- SD-23/Task-202 prose may still mention excerpt inject historically; product rule is path-only for file INPUT.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-45
change_type: bugfix
summary: edge dirty fingerprint; file_artifact path-only INPUT; hub join note dedupe + feature guard
# --->8---
