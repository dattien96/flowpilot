# CA-778 — vibe-ingest must not review leftover production code

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: SS ingest/validate prompts forbid coding and dirty-tree package conflicts; leftover snake/ is out of scope until vibe-sprint
# --->8---

## Why

Live run-212833: dirty `snake/` from a prior chat run made `ss_validator` `REQUEST CHANGES` for "Go package conflict main vs snake". User: dirty code is a **pass case** for ingest; coding starts at sprint coder, not intake.

## Change

- `prompts/vibe-ss-ingest.md` + `vibe-ss-validate.md`
- `vibe-ingest.yaml` wires them on ingest_reader, ss_converter, ss_validator
- `vibe-intake.md` restates no leftover-code fixes

## Tests

`task326_vibe_ingest_ss_prompt_test.go`

## Will not undo

CA-777 ss_lock park.
