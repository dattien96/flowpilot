# 04-04 — Phase 4: Approval Bridge, YOLO SSOT, Interrupt & Finalizer

> Part of the `04` coding plan. Builds on the Phase 3 Codex adapter. New Go files:
> `yolo_resolver.go`, `codex_approval_bridge.go`.

## Goal

Make dangerous-command handling correct and FlowPilot-owned: structured approvals,
**YOLO as the single source of truth** driving both runner policy and Codex config,
interrupt/cancel, and the post-turn finalizer hook.

## YOLO resolver (SSOT) — `yolo_resolver.go`

One value configures both layers:

```text
type YoloPosture struct { CodexSandbox; CodexApprovalMode; RunnerAutoApprove }
resolveYoloPosture(true)  -> {"full-access",     "never",      true}
resolveYoloPosture(false) -> {"workspace-write", "on-request", false}
```

- **Input:** per-step YOLO already flows in (`PromptExecutionRequest.YoloMode`
  `types.go:469`; session-start has `ApprovalMode`/`AllowWrite` `types.go:511-512`).
  Resolve per run/step (Task-030/031), carry on the turn.
- **Applied at:** `CodexSandbox` → `codexThreadStartParams`; `CodexApprovalMode` →
  thread/turn params; `RunnerAutoApprove` → approval-bridge behavior.
- **Retire the hack:** stop pinning `default_tools_approval_mode = "approve"` in
  `ensureCodexGoogleDriveMcpConfig` (`google_drive_mcp_provider_config.go:275`);
  approval mode is now YOLO-derived per turn. Proxy MCP gate keeps keying on YOLO
  (CP-29 P-5/P-11).

## Approval bridge — `codex_approval_bridge.go`

On a `permission_required` notification (from the Phase 3 dispatcher):
1. create an approval record (`workflow_provider_approvals`);
2. emit `permission_required` via the stream;
3. **block the turn** on a per-`approvalId` decision channel held in a runner map
   (survives client reconnect) — **without blocking the shared read loop**.

`SubmitApprovalDecision(approvalId, decision)`: validate vs FlowPilot policy, send
`codexApprovalDecisionParams` back, unblock, persist outcome. Mirror the existing
`SubmitApprovalDecisionUseCase`
(`apps/admin-web/.../submit-approval-decision-usecase.ts`). Exposed to clients as
`POST /client/approvals/{approvalId}/decision` (`04-02`).

YOLO=true → Codex `never` approval mode means it does not ask; if a request still
arrives, auto-approve and record it.

**Robustness (contract in `04-02`, enforced here):**
- **Idempotent + first-write-wins:** multiple windows may watch one run; the first
  valid decision resolves it, later/duplicate submits return the resolved outcome.
- **Validation:** a decision outside the offered set → `400 invalid_decision`.
- **Expiry:** an unanswered approval times out → record `expired`, turn fails
  **recoverably** (never blocked forever); the pending inbound request is **answered
  with a deny/error** so the provider unblocks. Same policy applies to questions below.

**Boundary:** real safety = `permission_required` + Codex sandbox + FlowPilot policy
together. App-server is the gate channel, not enforcement alone.

### Approval policy engine (YOLO=false)

`permission_required` does not always mean "ask the human." An `ApprovalPolicyEngine`
(`03`) sits between the inbound approval request and the user and decides, per
command/tool:

- **auto-approve** known-safe operations (allowlist) → reply immediately, record;
- **auto-deny** known-dangerous operations (denylist) → reply deny, record;
- **ask** everything else → emit `permission_required`, show the card.

This is what makes "gate **some** actions" possible (Task-032) instead of
all-or-nothing. Policy is configured in Admin Web (`03`); the **default is ask**.
The proxy MCP gate keeps keying on YOLO (CP-29).

> Every auto-decision and the YOLO=true auto-approve **must reply to the inbound
> request** (the third dispatcher category in `04-03`), not merely record it — else
> Codex hangs waiting for a response.

## User-interaction bridge (approvals + questions)

The "ask the user a structured question" UX (confirm/options popup) reuses the
**same pause/resume machinery** as approvals. Generalize `codex_approval_bridge.go`
into a user-interaction bridge handling approvals and questions. A question can be
**originated two ways** — with very different guarantees. Both feed the same
`user_question_required` event and the same desktop card.

### Path 1 — `ask_user` MCP tool (model-driven, best-effort)

- **Not a built-in tool.** FlowPilot **registers** a custom MCP tool
  `ask_user(prompt, options[], multiSelect?)` on its MCP proxy. The provider learns
  it via `tools/list` at session start — the tool's **name + description + JSON
  schema** are injected into the model's context, and the **description** is what
  tells the model when to use it (e.g. "call this when you need a decision or
  clarification before continuing").
- When the **model chooses** to call it → proxy intercepts → create a question
  record → emit `user_question_required` → **block the turn** on a per-`questionId`
  channel (without blocking the shared read loop) → return the choice as the tool
  result → turn resumes.
- **Reliability: best-effort.** Because the *model* decides whether to call it, it
  may skip it and assume an answer. Raise the call rate by reinforcing usage in the
  injected prompt (reuse `preparePromptForRequiredMcps`,
  `mcp_prompt_instructions.go:221`) — but this is **never guaranteed**.
- Provider-neutral (Codex/Claude/Gemini) because it rides the MCP proxy.

### Path 2 — workflow-driven question (deterministic, guaranteed)

- When FlowPilot **must** ask at a defined point (not at the model's discretion), the
  **runner/orchestration emits `user_question_required` directly** as a workflow step
  — no model tool call involved. The card always appears.
- Use this for required confirmations/branches; use Path 1 for model-discretion
  clarifications. (The workflow state machine that emits these is ported to Go in
  `04-05`.)

### Shared resume + client decision

- **`AnswerQuestion(questionId, choice)`**: client submits the pick (single/multi);
  the bridge unblocks. Path 1 returns the choice as the tool result; Path 2 feeds the
  workflow step. Survives client reconnect (state in the runner). Exposed as
  `POST /client/questions/{questionId}/answer` (`04-02`); **idempotent +
  first-write-wins**, invalid choice → `400`, unanswered → **expires** (recoverable
  fail) — same policy as approvals. On expiry/interrupt while a Path-1 question is
  pending, the **`ask_user` `tools/call` is returned an error result** so the model
  does not hang waiting for the tool.
- **Native option:** if the installed Codex app-server exposes a native
  elicitation/user-input request, map it to the same `user_question_required` event.
  Verify availability; Path 1's MCP-tool path is the robust default.

### Mechanism guarantees (do not conflate)

| Mechanism | Who initiates | Guarantee |
|---|---|---|
| Approval (`permission_required`) | the runtime (Codex, gated by sandbox/approval) | **enforced** — model cannot skip |
| `ask_user` MCP tool | the model | **best-effort** — may not call it |
| Workflow-driven question | FlowPilot runner/step | **deterministic** — always asked |

This is the mechanism behind the desktop "options popup" in `04-01`.

## Interrupt / cancel

A user "stop" (client `POST /client/workflow-runs/{runId}/interrupt`, `04-02`)
cancels the in-flight turn via `codexInterruptParams`; mark the turn `cancelled`,
drain its event channel, free pending-request/approval entries. A `ctx` cancel on
the turn must also issue the interrupt so the shared app-server is not left
mid-turn.

## Finalizer hook

After `turn_completed`, run the finalizer (full artifact/summary/RAG/GDrive logic
is **ported in Phase 5**; here establish the hook + local diff snapshot + retry
state):
1. save final-response artifact, 2. changed-files/diff snapshot (runner has the
workspace on disk), 3–4. summary + Supabase RAG (Phase 5), 5. update step status,
6. persist event audit trail.
Finalizer failure must **not** erase the completed turn — mark finalize state
separately for retry. The finalizer must be **idempotent**: a retry must not
double-write artifacts/RAG/audit (key by run/step/turn id; upsert).

## Acceptance

- YOLO=false: dangerous command → `permission_required`; **deny blocks**, approve
  runs; decision persisted.
- YOLO=true: Codex full-access + never; no `permission_required`; run audited as
  gating-disabled.
- One resolved YOLO value drives both layers; no standalone `="approve"`.
- Interrupt cancels a turn cleanly.

## Tests

- T-08/T-09/T-22 approval mapping; deny blocks; approve runs.
- T-21/T-24/T-25 YOLO posture; gating-disabled audit; no standalone `="approve"`.
- T-27/T-28 finalizer-failure retry (turn stays completed); recoverable
  `turn_failed` re-send.
- T-29 `ask_user` (model-driven) emits `user_question_required` with options; the
  pick resumes the turn with the chosen value.
- T-30 a workflow-driven `user_question_required` (no model tool call) renders the
  same card and the answer resumes the step.
- T-31 interrupt cancels an in-flight turn cleanly (turn marked `cancelled`).
- T-33 decision/answer idempotent + first-write-wins (multi-window).
- T-35 unanswered approval/question expires → turn fails recoverably.
- T-39 approval policy: allowlisted command auto-approves, denylisted auto-denies,
  others show the card; each auto-decision replies to the inbound request.
- T-40 expiry/interrupt while pending replies deny/error to the inbound request and
  returns an error `ask_user` tool result (provider does not hang).
- T-41 finalizer is idempotent: a retried finalize does not double-write
  artifacts/RAG/audit.

## Definition of Done (checklist)

- [ ] `yolo_resolver.go`: one YOLO value → Codex sandbox/approval + runner policy; `="approve"` hack removed (T-25).
- [ ] YOLO=false: dangerous command → `permission_required`; **deny blocks**, approve runs (T-08/T-09/T-22).
- [ ] YOLO=true: full-access + never; no `permission_required`; run audited gating-disabled (T-21/T-24).
- [ ] Approval bridge: pause turn (per-`approvalId` channel), survive reconnect, decision round-trip.
- [ ] **Approval policy engine** (YOLO=false): allowlist auto-approve / denylist auto-deny / else ask; every auto-decision replies to the inbound request (T-39).
- [ ] Expiry/interrupt while pending replies deny/error and returns an error `ask_user` tool result — provider never hangs (T-40).
- [ ] **`ask_user` MCP tool (model-driven, best-effort)**: registered via `tools/list`, prompt-reinforced → `user_question_required` → options card → `AnswerQuestion` → turn resumes (T-29).
- [ ] **Workflow-driven question (deterministic)**: runner/step emits `user_question_required` directly → same card → resumes the step (T-30).
- [ ] Interrupt/cancel: stop cancels the turn cleanly via `codexInterruptParams` (T-31).
- [ ] Decisions/answers **idempotent + first-write-wins**; invalid → `400`; unanswered **expires** to a recoverable fail (T-33/T-35).
- [ ] Finalizer hook on `turn_completed`; failure retryable without erasing the turn (T-27/T-28); finalize is **idempotent** — retry never double-writes (T-41).
- [ ] **Review gate:** human + AI review this checklist after the phase.
