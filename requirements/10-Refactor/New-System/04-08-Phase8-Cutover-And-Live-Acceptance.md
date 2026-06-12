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

## What you can test TODAY (before any 04-08 work)

There are **two independent layers** between you and a "real" run:

| Layer | Mock/fake default | Real |
|-------|-------------------|------|
| **Transport** (desktop ↔ runner) | `MockRunnerClient` (offline JSON fixtures) | `HttpWsRunnerClient` (live HTTP + SSE) |
| **Provider** (runner ↔ AI) | fake scripted adapter (scenarios) | real Codex adapter |

`just dev` now sets `VITE_RUNNER_URL` so the desktop runs on the **real transport**
(Layer 1 = real). The **provider** still defaults to the **fake adapter** (Layer 2 =
fake) until `FLOWPILOT_CODEX_APPSERVER=1` + a real `codex` (B1).

**What is real and testable right now (`just dev`, fake provider):**
- Live HTTP + SSE streaming, message deltas rendering incrementally.
- Real approval round-trip (`permission_required` → decision → resume) and the
  structured question card (`user_question_required` → answer → resume), both over
  the actual runner bridge — idempotent, first-write-wins, expiry.
- Reconnect/replay: drop the stream and rebuild the timeline from `afterSeq`.
- One-turn-per-session `409`, interrupt → `cancelled`, account-scope `409`.
- The YOLO posture + approval policy auto-decisions, and the finalizer artifacts
  (final/diff/summary/RAG) surfaced on `GET …/artifacts`.
- Admin audit endpoints (`/admin/providers|events|approvals|questions|…`).

**Driving the fake content** — the turn carries a `scenario` field; pick one per
turn (or via the desktop dev scenario switcher):
`normal` · `approval-required` · `question-required` · `tool-heavy` ·
`file-changes` · `failed`. Example against the runner directly:
```bash
# start a run, then a turn with a scenario:
curl -s localhost:4317/client/workflow-runs -d '{"projectId":"proj-web","workflowId":"wf-feature","stepId":"step-plan"}'
curl -s localhost:4317/client/workflow-runs/<runId>/turns -d '{"stepId":"step-plan","prompt":"hi","scenario":"approval-required"}'
curl -N localhost:4317/client/workflow-runs/<runId>/events/stream   # watch the SSE
```

**What is NOT real yet (needs 04-08):**
- The streamed **content** is scripted, not a real model (Layer 2 → B1).
- State is **in-memory** (lost on runner restart); no Supabase persistence (A1/A2).
- The navigator catalog is the **fake** projects/workflows/steps (A1).
- **Admin Web** still orchestrates server-side; it is not yet a thin client (A3).

Use this to validate the **UX + contract + event/approval/question/reconnect
plumbing** end to end now; the model content + persistence come with 04-08.

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

### B1 — Run against a real `codex` build (detailed code guide)

> **Reality check first.** The runner **already** spawns codex today, but via the
> **`codex mcp-server`** surface (MCP `tools/call`, tool name `codex`/`codex-reply`),
> not the `codex app-server` thread API that `04-03` assumed. See
> `resolveBinaryAndArgs` (`sessions.go:405`) → `"codex", ["mcp-server", …]`,
> transport `codex_mcp`, and the call site (`sessions.go:727`):
> `tools/call {name:"codex"|"codex-reply", arguments:{prompt, threadId?, model?}}`.
> So the **proven** protocol on your machine is `mcp-server`; the `app-server` method
> names in `codex_appserver.go` / `codex_event_mapper.go` are **assumptions** and must
> be verified or the adapter pointed at `mcp-server` instead.

**Step 0 — discover what your installed codex supports.** Run and record:
```bash
codex --version
codex --help                 # list subcommands — is "app-server" present? "mcp-server"?
codex app-server --help      # if it exists: flags, transport (stdio?), schema hints
codex mcp-server --help      # the proven surface (the old system used this)
```
Decision:
- **`app-server` exists** → reconcile its protocol (Step 1) and use the existing
  `codexAdapter` (Steps 2–3).
- **`app-server` absent / unstable** → use `mcp-server` instead (Step 4): write a
  `codexMcpAdapter` against the proven `tools/call` path, behind the same
  `ProviderRuntimeAdapter` interface — zero changes above the adapter.

**Step 1 — capture the real `app-server` wire (if present).** Drive it by hand and
record the actual JSON so we map real names, not guesses:
```bash
codex app-server --listen stdio://     # or whatever Step 0 showed
# paste an initialize request, then observe the response + notifications:
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"flowpilot","version":"1.0"}}}
```
Record: the **`initialize` result** (`protocolVersion`, `capabilities` keys), the
exact **method names** for start/turn (is it `thread/start`+`turn/start`? `newThread`?
`sendUserTurn`?), the **notification** names (`turn.delta`? `item.delta`?
`codex/event`?), the **approval request** method, and where `threadId`/`turnId`/
`cwd` live in params. (Codex builds have shipped several shapes — only your build is
authoritative.)

**Step 2 — reconcile the assumed names to reality.** The assumptions are localized;
update only these:
- `codex_appserver.go`: `codexInitializeParams`, `codexThreadStartParams`
  (`cwd`/`sandbox`/`approvalMode`/`mcpServers` placement), `codexThreadResume/List/
  ReadParams`, `codexTurnStartParams`, `codexInterruptParams`, and
  `codexThreadIDFromParams` (the notification key holding the thread id).
- `codex_event_mapper.go`: `mapCodexNotification` — the `switch` on method names
  (`turn.delta`→`message_delta`, `command.completed`→`tool_completed`+exit status,
  `file.changed`→`file_changed`, `turn.completed`→`turn_completed`, …). Fix the
  left-hand method strings + the params keys to match Step 1.
- `codex_adapter.go`: `handleInbound` — the approval request's params keys
  (`command`/`cwd`/`reason`) and the **reply shape** (`{decision:"approve"|"deny"}`
  vs whatever the build expects). `codexAskUserMcpServer` — the `mcpServers` entry
  shape Codex accepts for a custom tool (verify against `codex mcp` config docs).
- The mapper/adapter are table-/pipe-tested (`codex_event_mapper_test.go`,
  `codex_appserver_test.go`); **update the fake-server scripts in those tests to the
  real names** so the unit tests lock the reconciled protocol.

**Step 3 — run it live + iterate.**
```bash
# codex on PATH; turn the live path on:
FLOWPILOT_CODEX_APPSERVER=1 FLOWPILOT_CODEX_BIN=codex \
  go run ./cmd/flowpilot runner serve --port 4317
# then drive a real turn from the desktop (just dev) or curl /turns (no scenario field)
```
The runner logs `ensureCodexAppServer` start + the `initialize` result. Verify, in
order: `initialize` succeeds → a simple `turn/start` streams `message_delta` →
`turn_completed`; then `file_changed`, command-exec exit status, `thread/list` by
`cwd`, `thread/resume` after a runner restart (T-13/T-15), two threads/cwd
concurrently + mixed YOLO (T-23), and a dangerous terminal command where
`permission_required` is emitted and **deny truly blocks** under the sandbox (T-22).

**Step 4 — fallback adapter on `mcp-server` (if `app-server` isn't viable).** Add
`codex_mcp_adapter.go` implementing `ProviderRuntimeAdapter` over the proven path:
spawn `codex mcp-server` (reuse `resolveBinaryAndArgs`), send `tools/call`
`{name:"codex", arguments:{prompt}}` (first turn) / `{name:"codex-reply",
arguments:{threadId,…}}` (resume), and translate the streamed `tools/call`
notifications through `mapCodexNotification`. Register it in `ProviderRegistryFor`
instead of the app-server adapter. This reuses the dispatcher's framing but targets
the surface your build is known to speak. _(Note: `mcp-server` exposes fewer native
approval/thread affordances than `app-server` — that trade-off is the reason `02`/`05`
preferred `app-server`; pick per what Step 0 reveals.)_

- _Unblocks:_ `06` Part D #4 third caveat; T-01 (live), T-13/T-15/T-22/T-23.
- _Files:_ `codex_appserver.go`, `codex_event_mapper.go`, `codex_adapter.go`,
  `codex_appserver_process.go`, (+ `codex_mcp_adapter.go` for Step 4); their tests.

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
- [~] A1 — **catalog wired**: `InteractiveService` depends on `CatalogStore`; `CatalogStoreFor(runner)` selects `SupabaseCatalogStore` when Supabase is configured, else fake; handlers pass ctx + surface `catalog_unavailable` (tested). _Driving run progression through `WorkflowOrchestrator`/`WorkflowStore` (vs the per-turn flow) is the remaining half of A1, lands with A2._
- [ ] A2 — run/step/session/approval/question/event/artifact state persisted to Supabase (idempotent, ordered; shaping tested).
- [ ] A3 — Admin Web thin: server orchestration removed, routes call the runner; config + history retained.
- [ ] A4 — Admin Web read-only provider-runtime audit timeline (T-18).
- [ ] A5 — `ExecutePrompt` callers traced + migrated; legacy chat repointed to `turn/start` (retire per criteria).
- [ ] A6 — Deno `workflow-engine-*` reduced to thin triggers / removed.

### Part B — live acceptance
- [ ] B1 — real `codex` build: (Step 0) discover `app-server` vs `mcp-server`; (Steps 1–2) capture the real wire + reconcile method/param names in `codex_appserver.go`/`codex_event_mapper.go`/`codex_adapter.go` + their tests; (Step 3) init/caps, stream, `thread/list`/`resume` (T-13/T-15), concurrent + mixed-YOLO threads (T-23), terminal **deny blocks** (T-22); or (Step 4) the `codex_mcp_adapter.go` fallback on the proven `mcp-server` surface.
- [ ] B2 — live Supabase reads/writes; golden parity old-TS vs new-Go on a fixture run (T-20).
- [ ] B3 — `ask_user` live model call (T-29) + error tool-result on expiry (T-40).
- [ ] B4 — signed/notarized Windows + macOS installers via CI; auto-update wired.

### Sign-off
- [ ] All Part A done + in-repo tests green; all Part B verified against real infra.
- [ ] `06` Part D gate fully satisfied (every PP `[x]`, every T `[x]`, caveats met).
- [ ] Human + AI review.
