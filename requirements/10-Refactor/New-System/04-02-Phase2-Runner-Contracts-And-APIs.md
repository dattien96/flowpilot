# 04-02 — Phase 2: Runner Contracts, Persistence & Interactive APIs

> Part of the `04` coding plan. Shared types + `ProviderEvent` union live in
> `04-Detailed-Coding-Plan.md`. Implemented in **Go** (`apps/local-runner`).

## Goal

Define the runner-side contract and persistence, and expose the **interactive APIs
+ normalized event stream** so the Phase 1 desktop app can swap its
`MockRunnerClient` for a real `HttpWsRunnerClient` against the **same interface**.
A **fake provider adapter** lets all of this be built and tested before the Codex
adapter (Phase 3) exists.

## Scope

### Provider contract (Go)
- Implement the shared types from `04` as Go types: `ProviderEvent` (struct +
  `Type` discriminator + correlation base), session input/result types,
  capability struct.
- `ProviderRegistry`: Codex (placeholder until P3), Claude/Gemini disabled
  placeholders returning `UnsupportedProviderRuntimeError`.
- **Fake adapter** that emits scripted `ProviderEvent`s (mirrors the Phase 1 mock
  fixtures) — the test backbone for persistence, replay, and the APIs.

### Persistence (data model from `03`)
- `workflow_provider_sessions` (incl. `provider_account_id`, `provider_thread_id`),
  `workflow_provider_events`, `workflow_provider_approvals`.
- Event persistence: every normalized event gets a FlowPilot id, run/step/session
  correlation, persisted **before/while** streaming, **replayable**.

### Interactive + admin APIs (the contract the desktop locks in P1)
- Interactive (client): list projects/workflows/steps, start/resume run, **send
  turn (streaming)**, submit approval, list artifacts, list skills, event stream.
- Admin: providers, provider-capabilities, mcp-proxy, approval-policy, workflow
  runs, provider sessions/events/approvals (audit).
- Transport: HTTP + WebSocket (or local socket) — contract stable regardless of
  transport. Event stream emits **normalized FlowPilot events**, not raw provider
  events.
- **Event replay API** so a reconnecting client rebuilds the timeline from
  persisted events.

```text
# interactive
GET  /client/projects
GET  /client/projects/:projectId/workflows
GET  /client/workflows/:workflowId/steps
POST /client/workflow-runs
POST /client/workflow-runs/:runId/resume
POST /client/workflow-runs/:runId/turns
GET  /client/workflow-runs/:runId/events/stream
POST /client/approvals/:approvalId/decision
GET  /client/workflow-runs/:runId/artifacts
GET  /client/provider-skills?provider=codex
# admin
GET/PUT /admin/providers, /admin/mcp-proxy, /admin/approval-policy
GET     /admin/workflow-runs/:runId/provider-sessions|events|approvals
```

## Acceptance criteria

- Phase 1 desktop app runs against the real APIs by swapping `RunnerClient` impl
  (no renderer change), using the **fake adapter**.
- A scripted turn from the fake adapter streams normalized events to the client and
  is persisted.
- Reconnect → event replay rebuilds the run timeline.
- Disabled providers cannot be selected; typed unsupported errors surface.

## Tests

- persistence round-trip for session/event/approval rows.
- event stream emits normalized DTOs; ordering preserved.
- replay reconstructs the timeline (covers `06` T-16).
- registry exposes Codex (pending) + Claude/Gemini placeholders.

## Definition of Done (checklist)

- [ ] Go provider types + `ProviderEvent` + capability struct per the shared contract.
- [ ] Provider registry: Codex (placeholder until P3) + Claude/Gemini disabled.
- [ ] Fake adapter emits scripted events mirroring the P1 fixtures.
- [ ] `workflow_provider_sessions|events|approvals` persistence (incl. account + thread columns).
- [ ] Interactive + admin endpoints implemented; event stream emits normalized DTOs.
- [ ] Event replay API rebuilds a run timeline (T-16).
- [ ] Phase 1 desktop runs against real APIs by swapping `RunnerClient` (renderer unchanged).
- [ ] Tests: persistence round-trip, stream ordering, replay, registry placeholders.
- [ ] **Review gate:** human + AI review this checklist after the phase; all ticked or deferred with a reason.
