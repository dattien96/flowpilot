# CA-727 — bug-harness ships as Bug tier clone of rag-harness (Task-305 T-5 option a)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-305
change_type: feature
summary: flows/bug-harness.yaml added as byte-identical clone of rag-harness.yaml (only id/description/header differ, chatBaseline dropped so rag-harness keeps the Chat baseline role); manifest entry selectableIn flow + cloneable; inventory TestLoadBuiltinPack 9 to 10 with bug-harness in the want-list (planned CP-58 section P-5); new TestBugHarnessPackClone pins clone parity + picker contract + cap 3; Desktop HARNESS_LABELS Bug tier moved to bug-harness, rag-harness relabeled chat baseline
# --->8---

## Problem

- CP-58 DOD-3 requires a Bug tier entry in the `/flow` picker; Task-305 T-5
  left two options open (clone vs keep rag-harness). Operator chose (a)
  clone on 2026-09-04.

## Changes

- `apps/local-runner/internal/agentpack/flow-pack/flows/bug-harness.yaml`
  (new): clone of `rag-harness.yaml`, diff is header + `id` + `description`
  + dropped `chatBaseline: true` + trailing newline only.
- `flow-pack/manifest.yaml`: `bug-harness` entry (`selectableIn:[flow]`,
  `cloneable:true`, no `chatBaseline`).
- `internal/agentpack/bug_harness_pack_test.go` (new):
  `TestBugHarnessPackClone` — loads, validates, picker/cloneable/cap:3
  contract, node-by-node + edge + acceptance_nodes parity with rag-harness.
- `internal/agentpack/pack_test.go`: inventory 9 to 10 + `bug-harness`
  want-list (the exact edit CP-58 section P-5 prescribes for new harnesses).
- `apps/desktop-flowpilot/.../WorkflowsSettings.tsx`: `HARNESS_LABELS`
  Bug tier moved to `bug-harness`; `rag-harness` relabeled chat baseline.

## Verification

- `go test ./internal/agentpack/ -count=1`: full suite green (incl. new
  test + safety-topology all-builtin-flows guard covering bug-harness).
- Desktop `tsc --noEmit`: no errors in WorkflowsSettings.
- Live `/flow` picker visual + Supabase mirror row for bug-harness: pending
  operator checks (items 1 and 3, 2026-09-04 session).
