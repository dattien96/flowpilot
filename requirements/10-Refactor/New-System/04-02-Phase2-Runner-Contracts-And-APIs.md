# 04-02 — Phase 2: Runner Contracts, Persistence & Interactive APIs

> Part of the `04` coding plan. Shared types + `ProviderEvent` union live in
> `04-Detailed-Coding-Plan.md`. Implemented in **Go** (`apps/local-runner`).

## Goal

Define the runner-side contract and persistence, and expose the **interactive APIs
+ normalized event stream** so the Phase 1 desktop app can swap its
`MockRunnerClient` for a real `HttpWsRunnerClient` against the **same interface**.
A **fake provider adapter** lets all of this be built and tested before the Codex
adapter (Phase 3) exists.

> **Contract parity gate:** the real API must satisfy the *entire* `RunnerClient`
> interface that `04-01` locked — including `answerQuestion`, `interrupt`
> (Stop), `restartStack`/`shutdownStack`, and a cold-open run fetch. Anything the
> client method calls but the API doesn't expose breaks the "swap impl, renderer
> unchanged" promise.

## Scope

### Provider contract (Go)
- Implement the shared types from `04` as Go types: `ProviderEvent` (struct +
  `Type` discriminator + correlation base **+ monotonic per-run `seq`**), session
  input/result types, capability struct.
- `ProviderRegistry`: Codex (placeholder until P3), Claude/Gemini disabled
  placeholders returning `UnsupportedProviderRuntimeError`.
- **Fake adapter** that emits scripted `ProviderEvent`s (mirrors the Phase 1 mock
  fixtures) — the test backbone for persistence, replay, and the APIs.

### Catalog data source (P2 sequencing — resolves a hidden dependency)
- `projects` / `workflows` / `steps` ultimately come from Supabase, but **Go
  Supabase access doesn't land until P5**. For P2, serve the navigator from a
  **fake catalog mirroring the P1 fixtures** (or proxy the existing admin-web read
  path). State the choice explicitly — the navigator must work in P2 without the
  P5 port.

### Persistence (data model from `03`)
- `workflow_provider_sessions` (incl. `provider_account_id`, `provider_thread_id`),
  `workflow_provider_events`, `workflow_provider_approvals`,
  **`workflow_provider_questions`** (for the `user_question_required` path).
- Each event row carries a FlowPilot id **and a monotonic per-run `seq`**,
  run/step/session correlation, persisted **before/while** streaming, **replayable**.
- **Run/step status** persisted with the explicit state set the client renders
  (`idle | starting | running | waiting_approval | waiting_question | completed |
  failed | cancelled`).
- Approvals **and** questions persist a **resolved/expired** outcome
  (first-write-wins; survives reconnect).

### Run/turn lifecycle & streaming model (the load-bearing decision)
- **One persistent per-run event stream.** `POST .../turns` **starts** a turn and
  returns `{ turnId }` immediately; events flow on the per-run stream. The client's
  `sendTurn` async-iterable is that stream **filtered to the `turnId`**. (The POST
  itself does not stream a body.)
- **Resumption cursor.** Every event carries a monotonic per-run `seq`. The stream
  accepts `?afterSeq=N` and honors SSE `Last-Event-ID`, so a reconnecting client
  replays **only what it missed** — no gaps, no duplicates. (`occurredAt` alone is
  insufficient: timestamps can tie.)
- **Delta persistence.** `message_delta`s are **ephemeral** (coalesced, not
  individually durable). Replay reconstructs from persisted non-delta events + the
  last `message_completed`; in-flight-turn deltas are lost on reconnect — consistent
  with the resume semantics in `05`/`06` (T-15/T-25).
- **One turn per session at a time** (Codex threads are single-turn): a second
  `POST .../turns` while a turn is in flight → `409 turn_in_progress`.
- **Transport.** Reuse the existing SSE-style streaming approach (precedent:
  `/sessions/message/stream`) for the per-run stream; plain JSON for the rest.
  WebSocket is an optional later upgrade; the contract is transport-stable. Routes
  use Go 1.26 `http.ServeMux` method+wildcard patterns (`POST /client/...`,
  `{runId}`) — not `:runId`.

### Interactive + admin APIs (the contract the desktop locks in P1)

```text
# interactive (client) — fulfils the full 04-01 RunnerClient
GET  /client/projects
GET  /client/projects/{projectId}/workflows
GET  /client/workflows/{workflowId}/steps
POST /client/workflow-runs                    # body {projectId,workflowId,stepId,yoloMode?} -> RunHandle
GET  /client/workflow-runs/{runId}            # cold-open: status + handle + pending approval/question
POST /client/workflow-runs/{runId}/resume     # -> RunHandle (validates active account)
POST /client/workflow-runs/{runId}/turns      # body {stepId,prompt,selectedSkills?} -> {turnId}; events on stream
GET  /client/workflow-runs/{runId}/events/stream?afterSeq=N   # per-run normalized SSE stream
POST /client/workflow-runs/{runId}/interrupt  # cancel the in-flight turn (Stop)
POST /client/approvals/{approvalId}/decision  # idempotent, first-write-wins; body {decision}
POST /client/questions/{questionId}/answer    # idempotent, first-write-wins; body {choice: string|string[]}
GET  /client/workflow-runs/{runId}/artifacts
GET  /client/provider-skills?provider=codex
# system (already implemented; the desktop sidebar controls call these)
POST /system/restart
POST /system/shutdown
# admin
GET/PUT /admin/providers, /admin/mcp-proxy, /admin/approval-policy
GET     /admin/workflow-runs/{runId}/provider-sessions|events|approvals|questions
```

### Cross-cutting API rules
- **Idempotency.** `POST .../turns`, `/resume`, `/approvals/{id}/decision`,
  `/questions/{id}/answer` accept an `Idempotency-Key`; decision/answer are
  first-write-wins on the resource id, so client retries never double-submit.
- **Account-scope validation.** `resume` and `turns` validate the session's
  `provider_account_id` against the **currently active** account; mismatch →
  `409 provider_account_changed`. (Fully enforced in P6; the contract is declared
  here so clients handle it.)
- **Error envelope.** Non-2xx returns `{ error: { code, message, details? } }` with
  stable codes: `run_not_found`, `provider_unavailable`,
  `provider_account_changed`, `turn_in_progress`, `invalid_decision`,
  `question_expired`. Distinct from the event-level `turn_failed`.
- **Auth/origin (P2 stance).** Bind to **loopback only**; no auth token in P2
  (single-user local); allow the desktop + admin-web origins for SSE/WS. Token auth
  is a later hardening item (`04` Open Questions).

### Approval / question lifecycle (API + persistence; bridge logic in `04-04`)
- **Multi-window safe.** Several windows can watch one run. Decisions/answers are
  **idempotent + first-write-wins**: the first valid submit resolves it; later or
  duplicate submits return the resolved outcome, not an error.
- **Expiry.** Approvals/questions carry a timeout; on expiry the record is marked
  `expired` and the turn fails **recoverably** (so it is never blocked forever).
  Default documented; configurable.
- **Validation.** An invalid decision value, or an answer not among the offered
  options (when free-text is not allowed), → `400 invalid_decision`.

## Acceptance criteria

- Phase 1 desktop runs against the real APIs by swapping the `RunnerClient` impl
  (no renderer change), including **answer-question** and **interrupt**, using the
  fake adapter.
- A scripted turn streams normalized events (each with `seq`) to the client and is
  persisted.
- Reconnect with `afterSeq`/`Last-Event-ID` → replay rebuilds the timeline with
  **no gaps or duplicates**.
- A second window on the same run sees the same stream; the **first valid** approval
  / question submit wins.
- A concurrent `POST /turns` returns `409`; an interrupt cancels the in-flight turn.
- Disabled providers can't be selected; typed unsupported errors surface; an
  account-mismatch resume returns the typed error.

## Tests

- persistence round-trip for session / event / approval / **question** rows (+
  `seq`, status, resolved/expired).
- event stream emits normalized DTOs; **ordering by `seq`** preserved.
- replay via `afterSeq` reconstructs the timeline with no gaps/dupes (`06` T-16, T-32).
- registry exposes Codex (pending) + Claude/Gemini placeholders.
- answer-question + interrupt round-trips (`06` T-29/T-30 client legs, T-31).
- idempotent/first-write-wins decision (`06` T-33); concurrent-turn 409 (T-34);
  approval/question expiry (T-35); account-mismatch typed error (T-36).

## Definition of Done (checklist)

- [ ] Go provider types + `ProviderEvent` (incl. monotonic per-run `seq`) + capability struct.
- [ ] Provider registry: Codex (placeholder until P3) + Claude/Gemini disabled.
- [ ] Fake adapter emits scripted events mirroring the P1 fixtures.
- [ ] **Catalog source** for projects/workflows/steps defined for P2 (fake catalog or existing read path); navigator works without the P5 Supabase port.
- [ ] `workflow_provider_sessions|events|approvals` + **`questions`** persistence (incl. account + thread columns, `seq`, run/step status, resolved/expired).
- [ ] Interactive + admin endpoints incl. **single-run GET**, **answer-question**, **interrupt**; `/system/*` referenced for sidebar controls; event stream emits normalized DTOs.
- [ ] Streaming model: per-run stream; `POST /turns` returns `{turnId}`; one-turn-per-session → `409`; deltas ephemeral.
- [ ] Reconnect cursor: `afterSeq`/`Last-Event-ID` replay with **no gaps/dupes** (T-16, T-32).
- [ ] Decisions/answers **idempotent + first-write-wins**; multi-window safe (T-33).
- [ ] Approval/question **expiry** → recoverable fail (T-35); invalid decision → `400` (validation).
- [ ] **Account-scope validation** on resume/turn → typed error (T-36); standard **error envelope**; loopback bind + CORS/origin stance.
- [ ] **YOLO** accepted on run start (and/or per turn) per the SSOT (`04-04`).
- [ ] Phase 1 desktop runs against real APIs by swapping `RunnerClient` (renderer unchanged).
- [ ] Tests: persistence round-trip, stream ordering by `seq`, replay, registry, answer/interrupt, idempotent decision, concurrent-turn 409, expiry, account-mismatch.
- [ ] **Review gate:** human + AI review this checklist after the phase; all ticked or deferred with a reason.
