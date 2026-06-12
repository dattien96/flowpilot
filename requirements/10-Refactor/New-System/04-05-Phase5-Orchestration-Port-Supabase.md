# 04-05 — Phase 5: Orchestration Port (TS→Go) + Supabase + Admin Web Thin

> Part of the `04` coding plan. This is the **single largest workstream**: it makes
> the Go runner the single backend.

## Goal

Port the workflow orchestration that lives in the Admin Web **server** tier today
(`workflow-start-runtime.ts`, server-side: `node:fs`/`node:crypto`) into the **Go
runner**, and give the runner direct Supabase access. After this, Web UI and Desktop
Client are both thin clients of the same runner.

## What the runner absorbs (TS → Go)

- workflow run/step **state machine & sequencing**
- **base-prompt build** from step definitions (`deriveStepPromptBase`,
  result-summary prompt) — feeds the Phase 3 runner-side assembly + Phase 4 turn
- **send-with-retry** + provider-session sync
  (`syncWorkflowRunSessionProviderSessionId`, `sendMessageWithRetry`)
- **finalize logic** the Phase 4 hook calls: summary, Supabase RAG, Google-Drive
  write audit (`finalizeWorkflowRunSessions`, `createLocalWorkflowOutputArtifactSnapshot`,
  `createGoogleDriveWriteAuditArtifacts`)
- **Supabase reads/writes** for run/step/history (Go client / REST — precedent:
  artifact cloud sync, `supabase_config.go`)
- **navigator catalog** (`projects`/`workflows`/`steps`): P2 served these from a
  fake catalog / existing read path (`04-02`); P5 replaces that with **real Go
  Supabase reads** so the desktop navigator is backed by live data
- **workflow-driven questions:** the ported state machine can emit
  `user_question_required` directly at defined steps (the deterministic question
  path from `04-04`) — required confirmations/branches that must not depend on the
  model calling `ask_user`
- **Supabase edge functions** (`workflow-engine-start-run`,
  `workflow-engine-submit-step-approval`, `workflow-engine-toggle-yolo-mode`):
  orchestration **also** lives here today, not only in `workflow-start-runtime.ts`.
  The port must reconcile them — decide which are **subsumed** by the runner
  (anything on the run path) vs **kept** as thin Supabase-side triggers that call the
  runner. Orchestration must not stay split across the runner *and* edge functions.

## Admin Web becomes thin

- Drop server-side orchestration from Admin Web routes; they call the runner.
- Admin Web keeps: provider settings, MCP/proxy settings, approval-policy config,
  workflow config, run history, **read-only** provider event/session/approval audit,
  artifact browser, desktop-client setup page.
- Run detail gains a read-only provider-runtime timeline (session started, turn
  events grouped, approvals, finalize, errors). Live approval happens in the
  interactive client, not Admin Web.

## Concurrency & consistency

- The single backend now serves **many concurrent runs/workspaces** — the state
  machine needs **per-run locking**; no global serialization.
- **Idempotent, ordered writes:** run/step state + finalize writes are idempotent;
  a partial failure (e.g. Supabase write ok, RAG fails) is retryable without
  duplicating or corrupting state.
- **Supabase credentials:** the runner needs DB access — prefer a **least-privilege
  / RLS-scoped** key over a broad service-role key on a local machine; document the
  trust model.

## Migration strategy (de-risk)

- Re-implement the exported `workflow-start-runtime.ts` functions in Go.
- **Keep the admin-web path working until the Go port reaches parity** (the
  `ExecutePrompt` fallback, W8, helps).
- **Golden comparison:** a run via the old TS path and the new Go path must produce
  identical artifacts/RAG/audit before cut-over — diff a **fixture run** through both
  paths, normalizing volatile fields (ids, timestamps).

## Open sub-questions

- Go Supabase access: official client vs hand-rolled REST/RPC; least-privilege vs
  service-role auth in the runner.
- Exactly which admin-web server responsibilities remain TS (pure config CRUD) vs
  move to Go (anything on the run path).
- Edge-function reconciliation: which `workflow-engine-*` functions are subsumed by
  the runner vs kept as thin triggers that call it.

## Acceptance

- A run started from the Desktop Client and one from the Web UI go through the
  **same Go-runner** path and produce identical artifacts/RAG/audit.
- Admin Web no longer runs orchestration server-side; it is a thin client.
- The runner reads/writes Supabase directly for run/step/history.

## Tests

- Go ports of prompt build / finalize match TS outputs (golden).
- T-14 finalizer (artifact/summary/RAG) runs after `turn_completed` in Go.
- T-18 Admin Web shows provider event/session/approval audit for a run.
- T-20 existing workflow control, artifacts, summaries, RAG unchanged end-to-end.
- T-42 two concurrent runs (different workspaces) stay isolated; per-run state is not
  corrupted; finalize writes are idempotent under retry.

## Definition of Done (checklist)

> Implemented in `apps/local-runner/internal/runner/`
> (`workflow_state_machine.go`, `workflow_prompt.go`, `workflow_store.go`,
> `workflow_orchestrator.go`, `supabase_workflow_store.go`, + the
> `AskWorkflowQuestion` path on `InteractiveService`) with golden/unit tests
> (`workflow_state_machine_test.go`, `workflow_prompt_test.go`,
> `workflow_orchestrator_test.go`, `phase5_test.go`). `go vet` clean; all Phase 5
> tests pass; no regressions vs the HEAD baseline.
>
> **Scope note — this is the single largest workstream and is split here into the
> portable, testable core (done now) vs. the parts that need live infrastructure
> (deferred).** Deferred because they require a live Supabase instance + credentials,
> a live provider, and a TS/Next.js Admin Web refactor that can't be validated from
> the Go side: end-to-end Supabase reads/writes, the navigator catalog on live data,
> Admin Web going thin, edge-function reconciliation, golden parity TS-vs-Go on a
> fixture run, send-with-retry + session recovery (rides the deferred P3 registry
> swap), and the live finalize summary/RAG/GDrive pipeline. The pure-logic ports and
> the orchestration core are in and tested; the live wiring lands with the registry
> swap + a real Supabase (06 Part D).

- [~] `deriveStepPromptBase` + prompt builders + the **state machine** + the **orchestrator** ported to Go (golden-tested). _Session sync, send-with-retry, and the finalize summary/RAG/GDrive body are deferred to the live provider + Supabase wiring (the P4 finalizer hook + local snapshot already exist)._
- [~] Go Supabase access for run/step/log via PostgREST (`SupabaseWorkflowStore`); apikey/bearer auth, request shaping tested over a mocked transport; trust model documented (key from the OS secret store; deployment chooses service-role vs RLS-scoped). _End-to-end reads/writes against a real Supabase are deferred._
- [ ] Edge-function reconciliation (`workflow-engine-*` subsumed vs thin triggers) — _deferred: needs a Supabase/Deno deploy; the Go orchestrator + PostgREST store are the subsuming path._
- [x] Concurrent runs safe: **per-run locking** (distinct runs never serialize), idempotent state writes, partial-failure retryable (T-42).
- [x] Navigator catalog read-path over Supabase PostgREST (`SupabaseCatalogStore`: projects/workflows/steps), request shaping tested over a mocked transport (`TestSupabaseCatalogStoreShaping`). _The live service still defaults to the P2 fake catalog until pointed at a real Supabase (swap is a constructor change)._
- [ ] Admin Web no longer orchestrates server-side — _deferred: TS/Next.js refactor, validated separately from the Go runner._
- [ ] Golden parity old-TS vs new-Go on a fixture run — _deferred: needs both paths runnable (live infra); the pure-logic ports are golden-tested against the TS source shape in the meantime._
- [ ] Desktop and web runs go through the same Go-runner path (T-20) — _deferred with the live cut-over._
- [x] Finalize hook runs in Go after `turn_completed` (T-14): the hook + idempotent snapshot now shape final-response + diff + **summary + RAG-document** artifacts (`buildFinalizeSummary`/`buildRagDocument`, `TestFinalizeLocalSnapshotShapesSummaryAndRag`). _The live writes (Supabase artifact_runs + RAG embeddings, GDrive audit) + the Admin Web audit timeline (T-18) attach at cut-over._
- [x] Workflow steps can emit `user_question_required` directly — deterministic question path via `AskWorkflowQuestion`, same card + resume as the model-driven path (T-30).
- [x] **Review gate:** AI review complete this pass; _human sign-off pending._
