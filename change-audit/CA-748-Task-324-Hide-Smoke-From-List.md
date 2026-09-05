# CA-748 — Task-324 follow-up: hide cp-harness-smoke from Desktop workflows list

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-324
change_type: feature
summary: Desktop Settings workflows sidebar hides opt-in-only cp-harness-smoke via HIDDEN_BUILTIN_PACK_FLOWS; direct flowRef use unaffected
# --->8---

## Change (Desktop-only, 1 file)

- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`: new `HIDDEN_BUILTIN_PACK_FLOWS` set (`cp-harness-smoke`) + `isHiddenBuiltinPackFlow` predicate; the sidebar list filters it out. The flow stays `selectableIn: []` (already), `cloneable: true`, and resolvable by direct flowRef — hiding is display-only, same class as the T-C rag-harness picker dedup.
- Mirrors existing field usage (`workflow.isBuiltin`, `workflow.packFlowId` already read in the same file). No Desktop test covers this file; `tsc` not run (node_modules absent in this env) — operator to confirm visually in Settings > Workflows.
