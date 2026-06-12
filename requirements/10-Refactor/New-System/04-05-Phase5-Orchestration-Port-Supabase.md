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

- [ ] `deriveStepPromptBase`, session sync, send-with-retry, finalize (summary/RAG/GDrive) ported to Go.
- [ ] Go Supabase access for run/step/history (least-privilege / RLS-scoped key; trust model documented).
- [ ] Edge-function reconciliation: `workflow-engine-*` subsumed by the runner or reduced to thin triggers — orchestration not split.
- [ ] Concurrent runs safe: per-run locking; idempotent + ordered state/finalize writes; partial-failure retryable (T-42).
- [ ] Navigator catalog (projects/workflows/steps) backed by real Go Supabase reads (replaces the P2 fake catalog).
- [ ] Admin Web no longer orchestrates server-side; it is a thin client of the runner.
- [ ] Golden parity: old TS path vs new Go path produce identical artifacts/RAG/audit.
- [ ] Desktop and web runs go through the same Go-runner path (T-20).
- [ ] Finalize runs in Go after `turn_completed` (T-14); audit timeline visible in Admin Web (T-18).
- [ ] Workflow steps can emit `user_question_required` directly (deterministic question path, T-30).
- [ ] **Review gate:** human + AI review this checklist after the phase.
