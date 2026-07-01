# CA-186: Validate Manifest and Flow-File Metadata Agree (BUG-NOTE-CP42 #22)

## Scope

Verified and fixed a P3 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: `manifest.yaml` and each flow YAML both independently declare `selectableIn`/`chatSubModes`/`cloneable`/`editable`/`chatBaseline`, with nothing checking the two stay in sync.

## The bug

`ManifestFlow` (manifest entries) and `FlowDefinition.Builtin` (each flow file's own `builtin:` block) both carry the same five fields. The runtime picker/mirror path (`chat_builtin_orchestration.go`) only ever reads `def.Builtin.*` — the flow file's copy — never the manifest's. `ValidateManifestFS` validates the manifest's own internal consistency (e.g. `selectableIn` enum values) but never cross-checks it against the flow file. A future manifest-only edit to one of these fields (e.g. adding `"flow"` to a manifest entry's `selectableIn` without updating the flow file's `builtin.selectableIn`) would load successfully and silently have no effect on actual behavior — misleading anyone reading the manifest about the pack's real behavior.

## Fix

Added `validateManifestFlowMatchesDefinition(flow ManifestFlow, def FlowDefinition) error` (`pack.go`), called from `LoadPackFS` immediately after each flow file is loaded (where both the manifest entry and the parsed `FlowDefinition` are available). Compares `editable`/`chatBaseline`/`cloneable` by exact boolean equality, and `selectableIn`/`chatSubModes` via a new `stringSetEqual` helper (order-independent set comparison, since these are logically unordered sets of enum values) — failing the pack load with a specific error naming which field disagrees.

Confirmed the real embedded pack's manifest and both flow files already agree on all five fields for both `review-loop` and `rag-harness` — no drift exists today; this only guards against future drift.

## Verification

- New test `TestLoadPackFSRejectsManifestFlowMetadataDrift` (`pack_test.go`): a fixture where the manifest declares `selectableIn: [chat]` but the flow file declares `builtin.selectableIn: [flow]`, asserting `LoadPackFS` rejects it with an error mentioning `selectableIn`.
- Full `internal/agentpack` suite (14 tests) passes against the real embedded pack, confirming no drift exists in the shipped content.
- Full `internal/runner` suite: 1012 passed, 15 pre-existing/environmental failures (unchanged).
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: add validateManifestFlowMatchesDefinition to LoadPackFS so a manifest entry's selectableIn/chatSubModes/cloneable/editable/chatBaseline must agree with the flow file's own builtin.* declaration, instead of silently diverging with no runtime effect
# --->8---
