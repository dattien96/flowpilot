# CA-1079: binding seed drops rows whose artifact_instance is absent

Date: 2026-10-01
Refs: CP-58 Task-307 (harness plan artifact instances), CA-966/BUG-469
(tdd_signatures mapping added), live boot log:
`builtin artifact binding heal failed … 23503 … artifact_instance_id
(00000000-0000-0000-0000-000000000005) is not present in table
"artifact_instances"`.

## Problem

`builtinHarnessArtifactInstanceIDs` maps slot `tdd_signatures` → fixed UUID
`…0005`, added by CA-966 — but **no migration ever seeded that
`artifact_instances` row** (20260831090000 covers `…0002`–`…0004` only).
On every boot `SeedBuiltinHarnessArtifactBindings` for `vibe-sprint` (whose
sole binding is `tdd_signatures`) upserts a row FK'd to `…0005` → remote
rejects the whole batch `409 23503` → the flow's seed fails loudly, and
any flow sharing the batch would lose its valid bindings too.

## Fix

Degrade-soft, same class as CA-1075 (PGRST204 unapplied migration):

- `postStepArtifactBindingRows` — shared by both builtin seeders — posts
  the batch and, only on `status 409` + body `23503` + details naming a
  concrete `artifact_instance_id`, drops exactly the rows bound to the
  missing instance and retries (bounded: each round removes ≥1 row).
  All other statuses/transport errors propagate unchanged.
- New migration `20261001090000_add_tdd_signatures_artifact_instance.sql`
  actually seeds `…0005` so a properly migrated remote persists the
  binding — the runtime drop is the fallback, not the fix, for data.

Healthy remotes see zero extra requests (lazy retry on failure only), so
`TestBUG469_AllBuiltinBindingsSeedHealsUnchangedMirror`'s call-count
contract holds untouched.

## Files

- `internal/runner/builtin_artifact_bindings.go` — hoisted shared
  `builtinArtifactBindingRow`, added `postStepArtifactBindingRows` +
  `artifactInstanceFKMiss`, both seeders route through it.
- `internal/runner/ca1079_missing_artifact_instance_test.go` — 3 cases:
  sibling rows survive a missing instance (2 POSTs, retry minus the bad
  row), all-missing degrades to no-op nil, non-23503 failures propagate.
- `supabase/migrations/20261001090000_add_tdd_signatures_artifact_instance.sql`
  — the missing `…0005` seed.

## Verification

- Red first: tests failed with the verbatim live 23503 body.
- Green after; `TestBUG469` (per-flow POST count) and the bindings suite
  pass unchanged; `go vet` clean.
- Live boot on :4318 with the real remote: logged the degrade warn
  (`dropping 1 binding row(s) and retrying`) and **no `heal failed`** —
  previously the 23503 surfaced every boot.
