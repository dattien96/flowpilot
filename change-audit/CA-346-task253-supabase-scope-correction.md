# CA-346: Task-253 closed — Supabase wiring correctly recognized as moot (dead backend)

## Scope

User correction mid-session: CP-51/Task-258 pivoted BOTH dispatch AND session storage away from Supabase to local NDJSON + Drive sync. Confirmed by repo-wide grep: `SupabaseWorkflowStore` has zero production constructor calls anywhere. The earlier audit's "not wired into prod Supabase load path" finding for Task-253 is real as a code fact but has no live impact — pursuing it further would harden a path nothing exercises.

## Changes

- `apps/local-runner/internal/runner/supabase_workflow_store.go` — added a minimal `dispatchStore DispatchStore` field + `SetDispatchStore` setter (mirrors `InteractiveService.SetDispatchStore`) for forward-compatibility, but did NOT thread it through `providerSessionFromDBRow`'s 3 call sites — that would be speculative work against a dead path.
- Task-253 doc: status `done`, moved to `done/`. Completion notes document the scope correction explicitly so a future reader doesn't rediscover the same dead end.
- CP-51 ledger row `I5` flipped to ✅ — the fail-closed validation logic (decode → version → presence → semantics, no partial mutation) is real and unit-tested (`TestApplySessionRuntime_*`, `TestOpenRepair_CreateIfAbsent_OneCommit`); it just isn't reachable via a live Supabase HTTP path because none exists in this deployment.
- Noted for the record: the active backend (`localFileSessionStore`) has a structurally different corruption story (flat NDJSON, no nested blob to version, corrupt line skipped on load) — not a gap for this task, which is explicitly scoped to the Supabase jsonb-blob concern.

## Verification

- `go build ./...`, `go vet ./internal/runner` clean.
- Existing Task-253 unit tests still pass (6/6, unaffected by the scaffold addition).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: docs
summary: Close Task-253 — recognize Supabase wiring as moot (dead backend, confirmed zero production constructors); fail-closed validation logic itself is done and tested
# --->8---
