# CA-786 — vibe-ingest writes CP then joins vibe-cp-ingest

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: feature
summary: After SS lock, vibe-ingest writes one CP (cp_writer) and starts vibe-cp-ingest; coding stays task_slicer → vibe-sprint
# --->8---

## Why

Operator freeze: two entries, one tail. Idea→SS→CP vs start-at-CP. No parallel sprint_slicer coding loop. Coding = existing vibe-cp steps.

## Change

- `vibe-ingest.yaml`: `ss_lock --done--> cp_writer` (plan-cp-from-SS), drop `sprint_slicer`
- `onVibeCpNodeDone(cp_writer)` → `startResolvedFlow(vibe-cp-ingest)` and re-await `cp_lock`
- `sprint_slicer` Go handler kept so old tests stay green

## Tests

New `ca786_ingest_chains_cp_test.go`, `ca786_vibe_ingest_cp_writer_test.go`. Old Task-326/CA-783 slicer tests untouched.

## Providers

Agnostic: `maybeStartVibeCpIngest` / `onVibeCpNodeDone` take no `providerKey`.

## Will not undo

CA-777/780 lock park. CA-783 task_slicer→vibe-sprint. CA-785 freeze through TDD.
