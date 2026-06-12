# 04 - Detailed Coding Plan (Index)

This is the **index** for the coding plan. The detailed per-phase plans live in
`04-01` … `04-07`. This file holds the cross-phase **shared contract**, the **phase
map**, cross-cutting tests, resolved decisions, open questions, and risks.

## Goal

Implement the new FlowPilot system **mock-first**: stand up the desktop UX against
mock data, lock the client↔runner contract, then build the Go backend to fulfill it.
The implementation must preserve FlowPilot's core value: workflow control, prompt
optimization, artifacts, summaries, Supabase RAG, approval/proxy gates, MCP proxy
enforcement.

## Implementation Language & Authority

> **The backend is Go** (`apps/local-runner`, package `internal/runner`) and is the
> **single backend** — it owns all workflow logic and calls Supabase directly. The
> TypeScript interfaces below are **illustrative, language-neutral contracts** (and
> the literal contract for the TS clients). The **Go implementation in the phase
> files is authoritative**. The desktop client (`04-01`) and Admin Web are the only
> TypeScript surfaces.

## Phase Map (mock-first)

| Phase | File | Goal |
|---|---|---|
| 1 | [`04-01`](04-01-Phase1-Desktop-Mock-MVP.md) | Desktop App MVP (mock data) — clickable UX, contract-first |
| 2 | [`04-02`](04-02-Phase2-Runner-Contracts-And-APIs.md) | Runner contracts + persistence + interactive/admin APIs + event stream (swap mock→real) |
| 3 | [`04-03`](04-03-Phase3-Codex-Adapter-MVP.md) | Codex adapter MVP — shared app-server, async dispatcher, thread/turn, event mapping |
| 4 | [`04-04`](04-04-Phase4-Approval-Yolo-Finalizer.md) | Approval bridge + YOLO SSOT + interrupt + finalizer hook |
| 5 | [`04-05`](04-05-Phase5-Orchestration-Port-Supabase.md) | Orchestration port TS→Go + Supabase access + Admin Web thin |
| 6 | [`04-06`](04-06-Phase6-MultiWorkspace-Account-Hardening.md) | Multi-workspace + account-scoped app-server + hardening + desktop real-wiring |
| 7 | [`04-07`](04-07-Phase7-Providers-Capability-Packaging.md) | Claude/Gemini placeholders + capability UI + packaging/signing |

Dependencies: P1 defines the contract; P2 realizes it (fake adapter) so P1 can swap
mock→real; P3 replaces the fake adapter with Codex; P4 adds gating; P5 makes the
runner the single backend; P6 hardens; P7 ships. P1 can proceed in parallel with
P2/P3 because both sides meet at the shared contract below.

## Shared Contract (cross-phase)

Create these shared runtime types before UI/adapter work. Realized in Go in the
phase files; shown here as the language-neutral contract.

```ts
type ProviderKey = "codex" | "claude" | "gemini";

interface ProviderRuntimeAdapter {
  startSession(input: ProviderSessionStartInput): Promise<ProviderSession>;
  resumeSession(input: ProviderSessionResumeInput): Promise<ProviderSession>;
  sendTurn(input: ProviderTurnInput): AsyncIterable<ProviderEvent>;
  submitApproval(input: ProviderApprovalDecisionInput): Promise<void>;
  interrupt(input: ProviderInterruptInput): Promise<void>;
  closeSession(input: ProviderCloseInput): Promise<void>;
  listSkills?(input: ProviderListSkillsInput): Promise<ProviderSkill[]>;
}

interface ProviderSession {
  id: string;
  workflowRunId: string;
  workflowStepRunId?: string;
  providerKey: ProviderKey;
  providerSessionId: string;   // for Codex this equals providerThreadId
  providerThreadId?: string;
  status: ProviderSessionStatus;
}

interface ProviderTurnInput {
  sessionId: string;
  workflowRunId: string;
  workflowStepRunId: string;
  optimizedPrompt: string;
  workingDirectory: string;
  selectedSkill?: ProviderSkillSelection;
  modelName?: string;
  reasoningEffort?: string;
  requiredMcps?: string[];
  mcpAccessMode?: "read_only" | "read_write";
  yoloMode: boolean;
}

interface ProviderSkillSelection { name: string; path?: string; source: "slash_picker" | "text_shortcut" | "workflow_default"; }
interface ProviderApprovalDecisionInput { workflowRunId: string; workflowStepRunId?: string; providerSessionId: string; providerTurnId?: string; approvalId: string; decision: string; }
interface ProviderInterruptInput { providerSessionId: string; providerTurnId?: string; reason: string; }
interface ProviderCloseInput { providerSessionId: string; reason: "completed" | "cancelled" | "failed" | "shutdown"; }
interface ProviderSkill { name: string; path?: string; description?: string; source: "provider" | "flowpilot" | "workspace"; }
interface ProviderCapabilities { streaming: boolean; resume: boolean; approvalEvents: boolean; fileEvents: boolean; skillSelection: boolean; mcp: boolean; interrupt: boolean; }
```

Events use a discriminated union over a correlation base; **every** persisted/streamed
event extends the base:

```ts
interface ProviderEventBase {
  id: string; workflowRunId: string; workflowStepRunId?: string;
  providerSessionId: string; providerKey: ProviderKey; providerTurnId?: string;
  occurredAt: string;
}

type ProviderEvent =
  | (ProviderEventBase & { type: "turn_started"; providerTurnId: string })
  | (ProviderEventBase & { type: "message_delta"; text: string })
  | (ProviderEventBase & { type: "message_completed"; text: string })
  | (ProviderEventBase & { type: "tool_started"; toolName: string; input?: unknown })
  | (ProviderEventBase & { type: "tool_completed"; toolName: string; output?: unknown; status: "success" | "failed" | "cancelled" })
  | (ProviderEventBase & { type: "file_changed"; path: string; changeType?: "created" | "modified" | "deleted" | "renamed" })
  | (ProviderEventBase & { type: "permission_required"; approvalId: string; provider: ProviderKey; details: unknown })
  | (ProviderEventBase & { type: "user_question_required"; questionId: string; prompt: string; options: { label: string; description?: string }[]; multiSelect?: boolean })
  | (ProviderEventBase & { type: "turn_failed"; error: string; recoverable: boolean })
  | (ProviderEventBase & { type: "turn_completed"; finalMessage: string });
```

Registry + typed unsupported error (cross-phase):

```ts
interface ProviderRegistration {
  key: ProviderKey; displayName: string;
  status: "available" | "disabled" | "placeholder";
  capabilities: ProviderCapabilities;
  createAdapter: () => ProviderRuntimeAdapter;
}
class UnsupportedProviderRuntimeError extends Error {
  constructor(providerKey: ProviderKey) { super(`${providerKey} controlled runtime is not implemented yet`); this.name = "UnsupportedProviderRuntimeError"; }
}
```

Implementation rule: runner core depends only on these shared types + the registry;
Codex/Claude/Gemini specifics stay in their own files; clients receive serialized
event DTOs derived from `ProviderEvent`.

## Cross-Cutting Test Plan

End-to-end scenarios (the per-phase `T-xx` ids are tracked in
`06-DOD-And-Verification-Checklist.md`):

1. select workflow and step in the desktop app, send turn, receive final answer
2. provider requests command approval, user approves, turn completes
3. provider requests command approval, user denies, command is blocked
4. file change event appears and opens in the user's IDE
5. desktop app disconnects and reconnects, timeline replays
6. finalizer fails and can be retried (turn stays completed)
7. Admin Web shows provider event audit for the same run
8. Claude/Gemini placeholders are visible but unavailable

## Resolved Decisions

- **Single backend:** the **Go runner** owns all logic and calls Supabase directly;
  admin-web server orchestration is ported into it (P5). Web UI + Desktop are thin.
- **App-server scoping:** one shared app-server bound to the **active provider
  account**; recreate on active-account change; **threads are account-scoped**
  (persist `(workspace, account, threadId)`; resume only while that account active).
- **Session model:** a chat session **is a Codex thread**; a workspace is a Codex
  `cwd`. Lean on Codex thread APIs; no custom session registry.
- **Approval:** **YOLO is the single source of truth** — one value drives runner
  policy + Codex sandbox/approval per turn. Retires the `="approve"` hack. Real
  safety = `permission_required` + Codex sandbox + FlowPilot policy together.
- **Interactive client:** cross-platform **Electron desktop app** (mock-first),
  usable alongside any IDE; optional VS Code/JetBrains plugins reuse the webview
  later.
- **Concurrency:** the shared app-server uses an **async dispatcher** (id→waiter
  map, per-thread routing, non-blocking read loop, stdin write mutex) — not the
  current synchronous single-in-flight model.
- **User interaction (confirm/options popup):** structured "ask the user" prompts
  use the **same pause/resume bridge as approvals**, surfaced as a
  `user_question_required` event with options. **Two origination paths:** (1) a
  registered **`ask_user` MCP tool** the model may call — model-driven,
  **best-effort** (discovered via `tools/list`, reinforced by prompt injection);
  (2) a **workflow-driven** question the runner emits directly — **deterministic**,
  for required asks. Both render the same card. See `04-01` (UI) + `04-04` (bridge).

## Open Implementation Questions

- Go Supabase access approach (official client vs hand-rolled REST/RPC; service-role
  auth in the runner) and TS→Go parity validation (P5).
- HTTP/WebSocket vs local socket first; desktop client auth to the local runner.
- How multiple desktop windows attach to the same run/thread.
- Approval decisions the installed Codex app-server exposes; whether `thread/start`
  accepts per-thread `mcpServers` (as Gemini ACP `session/new` does, `sessions.go:276`).
- Multi-workspace registration/binding API and active-`cwd` selection.
- Event payload size persisted for audit vs storage.

## Risk Notes

- App-server improves approval UX but is **not** a complete security model — keep
  sandbox + provider approval + proxy + YOLO policy explicit.
- The **orchestration port (P5) is the largest workstream** (TS→Go + Supabase in
  Go); sequence with parity checks and keep the `ExecutePrompt` fallback until
  parity.
- The desktop client must not duplicate workflow business logic.
- Admin Web is not deleted; it becomes a thin config/audit client.
- Claude and Gemini must not be forced through Codex app-server.
