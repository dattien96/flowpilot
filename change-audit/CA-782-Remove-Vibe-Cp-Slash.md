# CA-782 — Remove standalone `/vibe-cp` slash command

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-326
change_type: refactor
summary: Drop /vibe-cp; CP entry is only /flow vibe-cp-ingest plus the @ file picker (CA-781)
# --->8---

## Why

Operator: `/vibe-cp` is redundant now that vibe-cp-ingest is a `/flow` row.

## Change

- Remove `case "/vibe-cp"` and the slash catalog row
- `/vibe` autodetect of a CP path still works
- `/flow vibe-cp-ingest` + file picker unchanged (CA-781)

## Tests

`ca782_remove_vibe_cp_slash_test.go`. Task-321/326 slash tests retargeted: `/vibe-cp` is unknown; hyphen prefix does not list it.

## Will not undo

CA-781 /flow list + file picker. RejectNonCP on `/flow vibe-cp-ingest README.md`.
