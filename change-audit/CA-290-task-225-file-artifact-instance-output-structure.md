# CA-290 — Task-225 File Artifact Instance Output Structure

Implements per-instance optional `structure` on `file_artifact.v1` OUTPUT:

1. **Config** — optional `structure: { kind: markdown_sections, sections: [...] }` only (no `format` field). Paths-only instances remain existence-only.
2. **Prompt** — `appendRequiredOutputArtifactPrompt` lists required paths always; section template only when instance has structure (Why bullets when `"Why"` is listed).
3. **Gate** — sibling rule `r-artifact-output-structure` after Task-223 existence; flexible ATX section match; child gate via `runChildArtifactOutputGate`.
4. **Schema** — migration updates `file_artifact.v1` `config_schema`; best-effort attaches structure to existing coder-summary-like instances.

## Files Changed

- `apps/local-runner/internal/flowgate/{rules,evaluate,enforce,flowgate_test}.go`
- `apps/local-runner/internal/runner/{artifact_type_registry,artifact_type_registry_test,gate_hook}.go`
- `supabase/migrations/20260712215515_file_artifact_optional_structure.sql`
- requirements Task-225 / SD-23 / CP-45 residual cross-links

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-225
change_type: feature
summary: per-instance file_artifact structure prompt and section gate
# --->8---
