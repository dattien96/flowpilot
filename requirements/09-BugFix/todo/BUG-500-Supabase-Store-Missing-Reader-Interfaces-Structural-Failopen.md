# BUG-500 — SupabaseWorkflowStore lacked gate-history readers → structural fail-open on resume

## Status
fixed (code) — `ListApprovalsByRun` + `ListQuestionsByRun` implemented;
`ListProviderSessionsByChat` was already present in
`supabase_chat_transcript_store.go` (initial audit missed it).

## Severity
High on Supabase backend; latent (file store is the default backend)

## Symptom

`SupabaseWorkflowStore` implemented writes (`UpsertApproval`,
`UpsertQuestion`, `UpsertProviderSession`) but only *some* reads. Missing:

- `ApprovalHistoryReader.ListApprovalsByRun`
- `QuestionHistoryReader.ListQuestionsByRun`

(~~`ChatSessionReader.ListProviderSessionsByChat`~~ — already implemented
in `supabase_chat_transcript_store.go:150`; corrected during fix.)

Consumers guard with `store.(X); ok` — a missing implementation is
indistinguishable from "no rows", so on the Supabase backend pending-gate
reconstruction NEVER ran: `pendingGateStates` and
`childPendingGateNodeIDs` returned empty on every resume, promoting durably
waiting approval/question nodes — structurally, not just under fault.

## Fix (implemented)

- `ListApprovalsByRun` — PostgREST `workflow_provider_approvals` filtered
  by `workflow_run_id`, mapped to `ProviderApprovalState`.
- `ListQuestionsByRun` — `workflow_provider_questions` filtered by
  `workflow_run_id`, mapped to `ProviderQuestionState`.
- Both propagate non-2xx/decode errors (fail-closed, mirrors file store).
- Query params escaped via `neturl.QueryEscape`.
- Compile-time `var _ X = (*Store)(nil)` assertions now pin the full
  reader surface on both backends — a dropped reader method fails the
  build instead of silently degrading resume again.
- Row decoding targets the **migration schema**
  (`20260615120000_add_workflow_provider_tables.sql`):
  `selected_decision`, `request_payload_json` (carries command/cwd/
  reason/policy/resolved_choices), `requested_at` → `CreatedAt`,
  `options_json` / `selected_choice_json`. Flat `command`/`decision`/
  `options`/`choice` columns are also decoded for drifted deployments.

## Follow-up finding — write-side schema drift (tracked, not fixed)

While aligning the readers, audit found `UpsertApproval`/`UpsertQuestion`
POST payloads name columns the migration schema does **not** have
(`command`, `cwd`, `reason`, `decision`, `policy`, `options`, `choice`),
and omit columns the schema requires (`provider_key` on questions is NOT
NULL; the state struct has no provider field). On a real deployment the
upserts would fail PostgREST validation — the write path has likely never
persisted a gate row on Supabase. This is latent:
`NewSupabaseWorkflowStore` is only referenced from tests today; no prod
wiring exists. Before that backend is wired, payloads must be remapped to
the migration columns (`selected_decision`, `request_payload_json`,
`options_json`, `selected_choice_json`, `provider_key`) or the schema
extended — and `Revision` (Task-430 stale-frame identity) needs a column
or an acceptable zero-value contract.
