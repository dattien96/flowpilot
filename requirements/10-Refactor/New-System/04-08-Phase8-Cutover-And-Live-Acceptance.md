# 04-08 — Phase 8: Cut-over Wiring & Live Acceptance (the pending backlog)

> Part of the `04` coding plan. This consolidates **every item still open** after
> Phases 1–7 (the `[~]`/`[ ]` rows in `04-05`, `04-07`, and the `06` checklist) into
> one ordered, actionable plan. Phases 1–7 built and unit-tested the runner +
> desktop against fakes / in-memory state / mocked transports; this phase does the
> **live wiring + cut-over** and the **acceptance runs** that need real
> infrastructure (a `codex` binary, a live Supabase, CI signing secrets).

## How to read this

- **Part A — Implement first.** FlowPilot code/refactor work with **no hard external
  blocker** — it can be written and (mostly) tested in-repo. Do these before the
  acceptance runs.
- **Part B — Live acceptance.** Verification that **requires real infrastructure**
  not present in the dev environment. Each item names exactly what unblocks it.
- Every task lists the **source DOD row** it closes, the **files**, and **deps**.

The runner-side building blocks all exist already (`apps/local-runner`): the async
dispatcher + Codex adapter + mapper, `ensureCodexAppServer` + registry swap
(`FLOWPILOT_CODEX_APPSERVER`), the YOLO SSOT + approval policy + finalizer,
`WorkflowOrchestrator` + `WorkflowStore`, `SupabaseWorkflowStore` +
`SupabaseCatalogStore`, `sendTurnWithRetry`, account scoping, and the desktop
`HttpWsRunnerClient` + real `IdeBridge`. Part A is about **connecting** them to live
state and the existing Admin Web; Part B is about **proving** them against real
providers/data.

---

## Part A — Implement first (FlowPilot code / refactor)

### A1 — Wire the orchestrator + Supabase stores into the live run path
The `InteractiveService` currently uses the **fake catalog** + **in-memory** run/
step/event/approval/question state. Make these pluggable and back them with the live
stores behind a config flag (mirroring `FLOWPILOT_CODEX_APPSERVER`).

- Inject `CatalogStore` into `InteractiveService` (default fake; `SupabaseCatalogStore`
  when Supabase is configured) so the navigator reads live projects/workflows/steps.
- Drive run progression through `WorkflowOrchestrator.Progress` + `WorkflowStore`
  (load steps → plan → apply transitions) instead of the ad-hoc per-turn flow.
- Resolve per-step provider/model/skills/required-MCPs/YOLO from the loaded step
  definitions (port `resolvePlannedStepExecution` / `resolveBuiltInStepExecution`).
- _Closes:_ `04-05` navigator/orchestration live rows; `06` Part B catalog + "real
  Go Supabase reads".
- _Files:_ `interactive_service.go`, `interactive_handlers.go`, `interactive_catalog.go`,
  a new `live_service_wiring.go`; `cmd/cli/root.go` (construct with stores when configured).
- _Test:_ store-backed service over the **fake** `WorkflowStore`/`CatalogStore`
  (in-repo); live DB is Part B (B2).

### A2 — Persist run/step/session/question/event state to Supabase
Replace the in-memory maps with durable writes via `SupabaseWorkflowStore` (+ new
methods) so state survives runner restart and is auditable.

- Persist `workflow_runs` / `workflow_run_steps` / `workflow_run_logs` transitions
  (already shaped in `SupabaseWorkflowStore`).
- Add `workflow_provider_sessions` (run→thread mapping, `provider_account_id`,
  `working_directory`, status) and `workflow_provider_approvals` /
  `workflow_provider_questions` persistence (records + resolved/expired).
- Persist the normalized event log + finalizer artifacts (`artifact_runs`).
- Idempotent + ordered writes (keys per run/step/turn) — the concurrency contract
  from `04-05`.
- _Closes:_ `06` "(workspace, threadId) persistence", "question/approval persistence
  (DB)", "every event persisted (DB)".
- _Files:_ `supabase_workflow_store.go` (new methods), `interactive_service.go`,
  `finalizer.go` (live write hook).
- _Test:_ request shaping over a mocked transport (in-repo); live round-trip is B2.

### A3 — Make Admin Web a thin client (drop server-side orchestration)
Rip the orchestration out of the Admin Web **server** tier; routes call the runner.

- Repoint `POST /api/workflow-engine/start-run`, `submit-step-approval`,
  `google-drive-write-approval` at the runner's interactive API
  (`/client/workflow-runs…`, `/client/approvals…`).
- Remove `runWorkflowStartRuntime` server-side orchestration usage; keep config CRUD,
  approval-policy config, workflow config, run history.
- Admin Web keeps **read-only** audit (provider event/session/approval timeline) over
  the runner's `/admin/*` endpoints.
- _Closes:_ `04-05` "Admin Web no longer orchestrates", "desktop+web same path"
  (T-20); `06` Part B "Admin Web" + Clients.
- _Files:_ `apps/admin-web/src/features/workflow-engine/*`, `app/api/workflow-engine/*`.
- _Caution:_ Admin Web's Next.js is **non-standard** (`apps/admin-web/AGENTS.md`) —
  read its docs first; this must be done where it can be built + run.

### A4 — Admin Web read-only provider-runtime audit timeline (T-18)
Render the runner's audit (session started, turn events grouped, approvals, finalize,
errors) read-only in the run-detail view. Live approval stays in the interactive
client.
- _Closes:_ T-18; `06` Part B "Admin Web audit", Part C T-18.
- _Files:_ `apps/admin-web/src/features/…/run-detail`.
- _Dep:_ A3.

### A5 — Retire the `ExecutePrompt` fallback (cut-over)
Once A1–A2 reach parity: repoint the legacy chat path to the Codex-thread
`turn/start`, trace every `ExecutePrompt` caller (workflow engine + `root.go`), and
migrate them; keep `ExecutePrompt` only until criteria in
`04-07-Migration-Notes.md` §3 are met, then remove.
- _Closes:_ `06` Part B "Chat path migration" (W6) rows.
- _Dep:_ A1, A2, and B1 (live verification) before deletion.

### A6 — Reduce the Deno edge functions to thin triggers
Per the reconciliation decision (`04-07-Migration-Notes.md`): reduce
`workflow-engine-start-run` / `submit-step-approval` / `toggle-yolo-mode` to thin
shims that forward to the runner (or remove once clients call the runner directly).
- _Closes:_ `04-05` edge-function reconciliation (deploy side).
- _Files:_ `supabase/functions/workflow-engine-*`.
- _Note:_ needs a Supabase/Deno deploy to validate (overlaps Part B infra).

---

## Part B — Live acceptance (needs real infrastructure)

> These are **verification** steps, not missing code. Each requires infrastructure
> absent from the dev box. They are the `06` Part D sign-off gate.

### B1 — Run against a real `codex` app-server build
Set `FLOWPILOT_CODEX_APPSERVER=1` with `codex` on PATH and verify against the
**installed build**:
- `initialize` caps + graceful degrade of unsupported methods.
- `thread/list` by `cwd`, `thread/resume` across a runner restart (T-13, T-15).
- two threads/different cwd concurrently; mixed YOLO posture per turn (T-23).
- a dangerous **terminal** command: `permission_required` emitted and **deny truly
  blocks** under the Codex sandbox (T-22) — the core safety claim.
- _Unblocks:_ `06` Part D #4 third caveat; T-13/T-15/T-22/T-23.

### B2 — Live Supabase round-trip + golden parity
Point the runner at a real Supabase (least-privilege/RLS-scoped key):
- run/step/log/session/approval/question/artifact reads+writes succeed (A1/A2 live).
- **Golden parity:** run a fixture through the old TS path and the new Go path; diff
  artifacts/RAG/audit with volatile fields normalized (T-20).
- _Unblocks:_ `04-05` Supabase live rows; T-20.

### B3 — `ask_user` live model call + tool-result-on-expiry
With a live model + the MCP proxy intercepting the registered `ask_user` tool:
- the model calls `ask_user` → `user_question_required` → answer resumes (T-29).
- on expiry/interrupt while pending, the `ask_user` `tools/call` returns an **error
  tool result** so the model does not hang (T-40, the tool-result variant).
- _Unblocks:_ T-29, T-40.

### B4 — Signed desktop installers + auto-update
Run `.github/workflows/desktop-release.yml` with signing secrets present
(`CSC_*`/`APPLE_*`/`WIN_CSC_*`): produce signed Windows + macOS installers
(notarized), confirm launch smoke; stand up the auto-update server.
- _Unblocks:_ `04-07` signed-installer rows; `06` packaging acceptance.

---

## Acceptance / Definition of Done

### Part A — implement first
- [ ] A1 — orchestrator + `CatalogStore`/`WorkflowStore` wired into the live run path (store-backed, fake-store tested).
- [ ] A2 — run/step/session/approval/question/event/artifact state persisted to Supabase (idempotent, ordered; shaping tested).
- [ ] A3 — Admin Web thin: server orchestration removed, routes call the runner; config + history retained.
- [ ] A4 — Admin Web read-only provider-runtime audit timeline (T-18).
- [ ] A5 — `ExecutePrompt` callers traced + migrated; legacy chat repointed to `turn/start` (retire per criteria).
- [ ] A6 — Deno `workflow-engine-*` reduced to thin triggers / removed.

### Part B — live acceptance
- [ ] B1 — real `codex` build: init/caps, `thread/list`/`resume` (T-13/T-15), concurrent + mixed-YOLO threads (T-23), terminal **deny blocks** (T-22).
- [ ] B2 — live Supabase reads/writes; golden parity old-TS vs new-Go on a fixture run (T-20).
- [ ] B3 — `ask_user` live model call (T-29) + error tool-result on expiry (T-40).
- [ ] B4 — signed/notarized Windows + macOS installers via CI; auto-update wired.

### Sign-off
- [ ] All Part A done + in-repo tests green; all Part B verified against real infra.
- [ ] `06` Part D gate fully satisfied (every PP `[x]`, every T `[x]`, caveats met).
- [ ] Human + AI review.
