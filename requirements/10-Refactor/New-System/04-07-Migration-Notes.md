# 04-07 — Migration Notes

> What changed between the old Admin-Web-orchestrated system and the refactored
> Codex-app-server runner, and how to cut over safely. Companion to
> `04-07-Operator-Docs.md`.

## 1. The `="approve"` hack is retired

- **Old:** the Google Drive proxy MCP server pinned
  `default_tools_approval_mode = "approve"` unconditionally, decoupling approval
  behavior from YOLO.
- **New:** approval mode is **YOLO-derived** via the SSOT resolver
  (`googleDriveProxyMcpApprovalMode` → `resolveYoloPosture`): `approve` when gating
  is disabled (YOLO=true), `on-request` otherwise (the request flows through the
  FlowPilot approval bridge / policy engine). The proxy still gates internally via
  its `--yolo-mode` arg.
- **Action:** none for operators; behavior is now consistent with the run's YOLO
  posture. If you previously relied on the proxy always auto-approving, that now
  only happens under YOLO=true.

## 2. Orchestration moved from Admin Web → the runner

- **Old:** the workflow run/step state machine, prompt build, send-with-retry, and
  finalize lived in the Admin Web server tier (`workflow-start-runtime.ts`) and in
  Supabase edge functions (`workflow-engine-*`).
- **New:** ported into the Go runner — the state machine (`PlanWorkflowProgress`),
  prompt builders (`DeriveStepPromptBase` / `BuildWorkflowStepPrompt` /
  result-summary), the orchestrator (per-run locking, idempotent writes), and a
  PostgREST `WorkflowStore`. Admin Web becomes a thin client; the desktop and web
  paths go through the **same** runner.
- **Edge functions — reconciliation decision (04-05):** the three
  `workflow-engine-*` functions are reconciled as follows so orchestration is **not
  split** across the runner and edge functions:
  - `workflow-engine-start-run` → **subsumed** by the runner (`POST
    /client/workflow-runs` + the Go orchestrator `Progress`). The edge function is
    reduced to a thin auth/trigger shim that forwards to the runner, or removed once
    all clients call the runner directly.
  - `workflow-engine-submit-step-approval` → **subsumed** (`POST
    /client/approvals/{id}/decision` + the approval bridge / policy engine).
  - `workflow-engine-toggle-yolo-mode` → **subsumed** (YOLO is per-run/turn on the
    runner via the SSOT resolver; the toggle becomes a thin DB-write trigger if a
    Supabase-side toggle UI is retained).
  The Go orchestrator + PostgREST store are the single subsuming path. The remaining
  work is the **Deno-side reduction + redeploy** of these functions to thin triggers
  (a deploy step, tracked with the live-Supabase cut-over).
- **Cut-over de-risking:** keep the Admin-Web path working until the Go port reaches
  parity; validate with a **golden comparison** — run a fixture through both paths
  and diff artifacts/RAG/audit (normalize ids/timestamps) before flipping clients.

## 3. `ExecutePrompt` fallback retirement

- The legacy one-shot `ExecutePrompt` path (`runner.go`) remains as the fallback
  while the app-server adapter is validated against a real Codex build.
- **Retirement criteria:** retire `ExecutePrompt` once (a) the Codex app-server
  adapter is the live registry default (the deferred registry swap, 06 Part D),
  (b) golden parity holds for prompt build + finalize, and (c) reconnect/replay +
  approval/question + interrupt are verified end-to-end on the live backend.

## 4. Status of the refactor (what is live vs deferred)

**Implemented + tested (over fakes / in-memory / mocked transport):**
the normalized `ProviderEvent` contract + interactive/admin APIs (P2), the async
Codex dispatcher + adapter + event mapper (P3), YOLO SSOT + approval policy +
finalizer hook + deterministic questions (P4/P5), the workflow state machine +
prompt builders + orchestrator + PostgREST store shaping (P5), multi-workspace cwd +
account-switch scoping + real desktop IdeBridge (P6), and runner-side provider
capability enforcement (P7).

**Deferred to a live Codex build + live Supabase (06 Part D):**
the app-server **process** spawn/recreate (`ensureCodexAppServer`), flipping the
registry from the fake adapter to the real `codexAdapter`, runner-side skill/MCP
prompt injection + `ask_user` tool registration, end-to-end Supabase reads/writes +
the navigator catalog on live data, Admin Web going thin + edge-function
reconciliation, the live finalize summary/RAG/GDrive body, golden parity on a
fixture run, the OLD `runner.go` `r.workspace` audit, real Claude/Gemini adapters,
and signed desktop installers (needs CI runners + signing secrets).
