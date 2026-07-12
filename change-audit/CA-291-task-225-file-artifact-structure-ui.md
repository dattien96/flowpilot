# CA-291 — Task-225 File Artifact Structure UI

Adds user-facing config for per-instance `file_artifact.v1` output structure (Task-225 follow-up):

- Settings → Flow → Artifacts → file instance form
- Checkbox: **Require markdown sections (OUTPUT template + gate)**
- Textarea: section titles (one per line)
- Button: **Use coding memo (What / Why / Baseline)**
- Save normalizes empty structure → default sections; strips stale `format`
- Helpers + unit tests: `fileArtifactConfig.ts`

## Files Changed

- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- `apps/desktop-flowpilot/src/components/settings/fileArtifactConfig.ts`
- `apps/desktop-flowpilot/src/components/settings/fileArtifactConfig.test.ts`
- Task-225 completion notes

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-225
change_type: feature
summary: Artifacts UI to edit file_artifact structure sections
# --->8---
