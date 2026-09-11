# CA-773 — Dev `/flow` hides vibe catalog clones

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-326
change_type: bugfix
summary: Filter catalog workflows whose name/id looks like vibe-* from the TUI /flow picker in working_mode=dev
# --->8---

## Why

Live M2: `/vibe off` then `/flow` listed UUID catalog rows `Vibe Cp Ingest` / `Vibe Owner Debate` / `Vibe Sprint`. `flowCatalogForWorkingMode` filtered builtins but passed `m.flowWorkflows` unfiltered.

## Change

- `LooksLikeVibeFlow` (space→hyphen, then vibe family)
- Dev picker drops matching catalog rows. Vibe picker still returns no catalog.

## Tests

New: `looks_like_vibe_test.go`, `task326_dev_hides_vibe_catalog_test.go`. Old `TestTUIFlowSuggest_DevOmitsVibe` untouched.

## Will not undo

CA-772 Tab on/off. Task-326 FlowPickerOptions five harness.
