# CA-764 — vibe-sprint v2 context + validate + audit

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-323
change_type: feature
summary: Rewrite vibe-sprint to v2 auto-only topology (context, validate, audit) with single synthesis→coder loop
# --->8---

## Change

Task-323 / CP-60 P-7.

- `flows/vibe-sprint.yaml` v2: `plan → freeze → context → tdd → coder → validate → synthesis → audit`.
- Single `continue/back` `synthesis → coder`. `acceptance_nodes: [validate, synthesis, audit]`.
- No plan-review loop, no reviewer cohort, no Dev cards. `selectableIn: []` unchanged.

## Tests

- New `task323_vibe_sprint_v2_test.go` (node order, behaviors, tdd→coder, one back-edge, safety topology, no harness review nodes).
- Removed P-1 `TestPack_VibeSprintStillV1` (same-session pin superseded).

## Provider impact

**Case 1 agnostic.** Pack YAML only; no `providerKey`.

## Will not undo

CA-763 working_mode gate. CA-755 / CP-58 / CP-61.
