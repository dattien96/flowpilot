# BUG-500 — SupabaseWorkflowStore lacks the reader interfaces BUG-488/491/495 now depend on → structural fail-open on that backend

## Status
todo — documented, not yet implemented (needs Supabase schema query design)

## Severity
High on Supabase backend; latent (file store is the default backend)

## Symptom

`SupabaseWorkflowStore` (`internal/runner/supabase_workflow_store.go`)
implements writes (`UpsertApproval`, `UpsertQuestion`,
`UpsertProviderSession`) and *some* reads (`ListAllProviderSessions`,
`ListProviderSessionsByProject`, `GetProviderSession`) but NOT:

- `ApprovalHistoryReader.ListApprovalsByRun`
- `QuestionHistoryReader.ListQuestionsByRun`
- `ChatSessionReader.ListProviderSessionsByChat`

Every consumer guards with `store.(X); ok` — a missing implementation is
indistinguishable from "no rows", so on the Supabase backend:

- **resume pending-gate reconstruction NEVER runs** — `pendingGateStates`
  and `childPendingGateNodeIDs` return empty on every resume, so a run
  durably waiting on an approval/question is promoted to not-waiting —
  structurally, not just under fault (the BUG-495 class made permanent).
- **`resolveChatIdentity` computes legSeq from memory only** — BUG-488's
  duplicate-legSeq hole remains fully open on Supabase.

The BUG-495/488 fixes propagate *errors*; they cannot help when the
interface itself is absent — the `ok` guard silently succeeds.

## Root cause

Reader interfaces were added incrementally against the file store; the
Supabase backend was never extended to match. `upsert`-only parity is a
write-only mirror — state round-trips into Supabase but never comes back.

## Proposed fix

Implement the three readers on `SupabaseWorkflowStore` against the
existing tables (`provider_approval_states`, `provider_question_states`,
`step_artifact_bindings`-adjacent session rows — verify actual schema
names in supabase migrations before coding). Each must return
`(rows, error)` and propagate query errors, mirroring the file store's
semantics.

Until then, the `ok`-guard silence remains: consider a boot-time warning
when the active store lacks a reader the runtime depends on, so the gap
is at least visible instead of fail-open.

## Tests

- Store-capability contract test: assert the configured store implements
  the reader set a given feature requires, or that the service logs the
  degraded capability once at boot.
- Reader behavior tests against the Supabase fake/emulator used by
  existing supabase_workflow_store tests.
