# CA-999 — BUG-500: SupabaseWorkflowStore gains gate-history readers

## What changed

`apps/local-runner/internal/runner/supabase_workflow_store.go`:

- `ListApprovalsByRun` (ApprovalHistoryReader) — PostgREST select on
  `workflow_provider_approvals` filtered by `workflow_run_id`; non-2xx and
  decode failures return errors (fail-closed).
- `ListQuestionsByRun` (QuestionHistoryReader) — same pattern on
  `workflow_provider_questions`.
- New row types `dbApprovalRow` / `dbQuestionRow` decode the **migration
  schema** (`selected_decision`, `request_payload_json` → command/cwd/
  reason/policy/resolved_choices, `requested_at` → CreatedAt,
  `options_json`/`selected_choice_json`) and tolerate flat-column drifted
  deployments. Query values escaped.
- Compile-time `var _` interface assertions pin `SessionIndexReader`,
  `SessionHistoryReader`, `ApprovalHistoryReader`,
  `QuestionHistoryReader`, `ChatSessionReader` on both
  `SupabaseWorkflowStore` and `localFileSessionStore` — a dropped reader
  method now fails the build instead of silently degrading resume.
- Audit follow-up: write payloads name columns the migration schema lacks
  (`command`, `decision`, `options`, `choice`, …) and omit
  `provider_key` (NOT NULL on questions) — upserts would fail PostgREST
  validation on a real deployment. Latent: the store has no non-test
  wiring yet. Remap payloads or extend schema before wiring — tracked in
  the BUG-500 doc.

`apps/local-runner/internal/runner/interactive_service.go`:

- `persistProviderSession` logs write failures internally — the ~22
  discard sites now at least leave a diagnostic line (BUG-499 partial).

`apps/local-runner/internal/runner/dispatch_store_memory.go`:

- The earlier `persistErr` latch was reverted (it broke the designed
  retryable contract for commit-before-mutate paths). All mutate-first
  mutators were instead converted to disk-before-RAM — see CA-998.

## Invariant

A write-only store is not a store: gate history round-trips on every
backend, decoded against the real schema; and a mutation only claims
durability after the durable write actually landed.

## Tests

`bug500_supabase_gate_readers_test.go` — interface assertions, row decode
(migration + flat shapes), error propagation, query shape (mocked
httpRequestFn).
