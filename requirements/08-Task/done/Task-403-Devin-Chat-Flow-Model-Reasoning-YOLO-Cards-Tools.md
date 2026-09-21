# Task-403: Devin Chat/Flow UI Parity, Reasoning, YOLO, Cards, and Tools

## Metadata

- Document ID: `Task-403`
- Title: `Devin Chat/Flow UI Parity, Reasoning, YOLO, Cards, and Tools`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-20`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-70: Devin Provider Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [Task-303: Opencode Chat Flow YOLO Cards](../../08-Task/done/Task-303-Opencode-Chat-Flow-Model-Reasoning-YOLO-Cards-Tools.md), [Task-401: Devin Adapter MVP](../../08-Task/done/Task-401-Devin-Controlled-Adapter-MVP.md), [CP-70-note.md](../../07-Coding-Plan/note/CP-70-note.md), [CP-70-Test-Steps.md](../../07-Coding-Plan/done/CP-70-Test-Steps.md)
- Replaces: `None`
- Tags: `devin, chat, flow-mode, approval-gate, yolo, ask-user, spawn-agent, summarizer, vision-guard, desktop-ui`

## AI Quick View

### Summary

- Bring Devin to full feature parity with existing tier-1 providers (Codex, Claude, Grok, Opencode) across Desktop and TUI interfaces.
- Wire full Approval Gate & YOLO Posture SSOT: when YOLO is OFF, route ACP `session/request_permission` to `TurnBridge.RequestApproval`; when Deny is clicked, terminate cleanly without blank turns (CA-712/713).
- Implement FlowPilot Loopback MCP Server để expose `ask_user` (blocks for user input) và `spawn_agent` (child isolation BUG-334) — **Lưu ý F-2**: Devin `mcpCapabilities{http:false,sse:false}` → `session/new{mcpServers}` chỉ nhận stdio; loopback HTTP kiểu OpenCode không inject được, phải spawn stdio shim (wrapper command trong `mcp_config.json`/`session/new`) hoặc defer `ask_user`/`spawn_agent` cho Devin v1.
- Implement Chat Posture enforcement (`scan`/`plan` read-only untrusted vs `code` writable) with Tab shortcut cycling.
- Enable One-Shot Summarizer using `devin -p` (output contract probe pending — `--json` chưa verify) hoặc `devin acp --agent-type summarizer`, integrated into `resolvePromptExecutionAdapter`.
- Integrate Flow Mode gates (`r-ca`, `r-bug`, `r-task`) allowing repair prompts to execute directly on Devin.
- Wire Vision Guard in `visionProviders.ts` to block image drag-and-drop before prompt dispatch (CA-692).
- Complete Google Drive sync and restore via `BuildChatSessionSyncManifest` hook in `chat_session_sync.go`.
- Conduct full manual E2E verification sign-off across all 17 sections of `CP-70-Test-Steps.md`.

### Current Ask

- Deliver the complete user-facing integration: interactive cards, sub-agent cohort, flow gates, summarizer, posture switching, and end-to-end acceptance testing.

### Key Decisions

- `T-1` Question Guard: Even when YOLO is ON, model calls to `ask_user` must NEVER be auto-approved; they must unconditionally pause and present a question modal to the user.
- `T-2` Child Isolation Enforcement: All sub-agent executions spawned via `spawn_agent` must run on isolated subprocesses (`account|child:<runId>`) and clean up via `CloseProcessesForChildRun` upon completion.
- `T-3` Headless Summarizer: One-shot chat/flow summaries run qua `devin -p` (`usesPromptArg=true`) hoặc `devin acp --agent-type summarizer` — chọn path sau khi probe output format (Task-400 Q-6); hoàn toàn tách khỏi interactive ACP session.
- `T-4` Vision Gating on UI: `promptCapabilities.image:true` + per-model `_meta.supportsImages` (F-26) — `supportsVisionFor("devin")` nên check model capability thay vì hard false; conservative v1: false cho tới khi golden fixture image prompt pass.

### Constraints

- Tham chiếu theo symbol name: `resolvePromptExecutionAdapter`, `BuildChatSessionSyncManifest`, `summarizerModelFor`, `supportsVisionFor` (line numbers drift).
- Old provider cards and styles must not be visually altered.

### Open Questions

- `Q-1` Stdio shim cho loopback MCP (`ask_user`/`spawn_agent`): Devin chỉ nhận stdio mcpServers (F-2) → cần binary/script wrapper hay defer tools này? Quyết định sau live probe.
- `Q-2` Real `session/request_permission` optionIds — probe chưa trigger (F-25): `accept-edits` auto-approved `exec`; Task-400 capture bằng mode `smart`/`auto` + dangerous cmd trước khi freeze mapping.
- `Q-3` `devin -p` output format (text/JSON) → REPL auth đang fail trên máy dev; ưu tiên `devin acp --agent-type summarizer` (ACP path đã verified hoàn toàn).
- `Q-4` ✅ Posture/permission: `session/set_config_option{configId:"mode"}` verified — `scan`→`ask`, `plan`→`plan`, `code`→`accept-edits`, YOLO→`bypass` (F-22); không cần `DEVIN_PERMISSION_MODE` env trừ launch-time default.

### Source Refs

- `CP-70` §3 Key Decisions `P-3`, `P-4`, `P-5`, `P-8`; §4 Work Breakdown `P-5`, `P-6`, `P-8`, `P-9`, `P-10`, `P-11`; §5.2 Rows 4, 13, 14, 15, 17, 20, 21, 22, 26, 27, 28.
- `CP-70-Test-Steps.md` Sections S, A..P.

---

## 1. Goal

Achieve 100% operational parity for Devin in FlowPilot Desktop and TUI, including full interactive approval cards, sub-agent spawning, flow repair gates, one-shot summaries, and passing the manual E2E verification suite.

---

## 2. Parent Links

- Coding Plan: [CP-70: Devin Provider Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md) (Work Breakdown `P-5`, `P-6`, `P-8`, `P-9`, `P-10`, `P-11`)
- Tech Design: `SD-06`, `SD-11`, `SD-26`
- System Spec: `SS-05`, `SS-13`
- Upstream Task Reference: [Task-303: Opencode Chat Flow YOLO Cards](../../08-Task/done/Task-303-Opencode-Chat-Flow-Model-Reasoning-YOLO-Cards-Tools.md)

---

## 3. Trigger

With transport (Task-400), adapter MVP (Task-401), and administrative settings (Task-402) in place, Task-403 completes the user-facing interface, interactive tool loops, and governance gates.

---

## 4. Exact Change

- `T-1` **Full Approval Gate & YOLO Posture (`yolo_resolver.go`, `devin_adapter.go`)**:
  - In `yolo_resolver.go`:
    - Add `DevinPermissionMode string` to `YoloPosture` struct.
    - Wire `resolveDevinYoloPosture(req TurnRequest) (autoApprove bool, permissionConfig []any)`.
  - In `devin_adapter.go`:
    - Handle inbound `session/request_permission`:
      - Map optionIds từ capture thật (Q-2 — không hardcode `allow`/`reject` trước khi verify).
      - If auto-approve eligible (YOLO), return outcome allow-tương-đương.
      - Otherwise, dispatch `bridge.RequestApproval(ctx, req)` and await user decision.
      - If user clicks Deny, return outcome reject-tương-đương và synthesize clean termination text if no reply was generated (CA-712/713).

- `T-2` **`ask_user` and `spawn_agent` via Loopback MCP (stdio shim)**:
  - In `devin_adapter.go`:
    - Devin chỉ hỗ trợ **stdio** MCP qua `session/new{mcpServers}` (F-2 `http/sse:false`) — không thể truyền URL loopback HTTP như OpenCode. Hai phương án:
      - (a) stdio shim: `mcpServers` entry `{command: "<flowpilot-mcp-shim>", args:[...]}` trỏ về FlowPilot loopback (wrapper binary/script do runner cung cấp, write vào account home).
      - (b) defer `ask_user`/`spawn_agent` cho Devin v1 nếu shim phức tạp — ghi rõ vào parity matrix.
    - Expose `ask_user`: pauses turn, emits `EventUserQuestionRequired`, awaits user text, returns response to model.
    - Expose `spawn_agent`: calls `bridge.SpawnAgent(ctx, spawnReq)`. Runs child on `account|child:<runId>` subprocess. Supports `wait=true`/`wait=false`.
    - Upon child run completion, invoke `CloseProcessesForChildRun(childRunID)`.

- `T-3` **Chat Posture Enforcement & Tab Cycling (`chat_posture.go`)**:
  - Implement read-only enforcement qua **session mode** (verified F-22): `session/set_config_option{configId:"mode"}` — `scan`→`ask`, `plan`→`plan`, `code`→`accept-edits`; Devin tự enforce read-only server-side trong `ask`/`plan`.
  - In `code` posture, modifications proceed to approval gate.
  - In Desktop UI:
    - Bind Tab key to toggle quickly between `plan` and `code`.
    - Enforce pinned model when switching postures.

- `T-4` **One-Shot Summarizer Integration (`resolvePromptExecutionAdapter`, `summarizer.go`)**:
  - In `resolvePromptExecutionAdapter`:
    - Add `case ProviderKeyDevin:` configure one-shot — `devin -p` (`usesPromptArg = true`) nếu output parse được, hoặc `devin acp --agent-type summarizer` với ACP one-shot adapter. Quyết sau probe Q-3 (`--json` chưa verify trong 3000.10.31; `--respect-workspace-trust false` có thể cần).
  - In `summarizer.go` (`summarizerModelFor`):
    - Add `case ProviderKeyDevin:` trả model rẻ/nhanh từ catalog thật (`devin models list` sau auth — không hardcode `devin/default` khi chưa confirm ID).

- `T-5` **Flow Mode & Flow Rules Governance**:
  - Ensure `/flow task-harness` runs with `devin/*` models.
  - Wire flow rule validators (`r-ca`, `r-bug`, `r-task`):
    - When a gate fails, execute the gate repair prompt directly on Devin.

- `T-6` **Vision Guard on UI (`visionProviders.ts`, `ChatInput.tsx`)**:
  - In `visionProviders.ts`:
    - Set `supportsVisionFor("devin") = false`.
  - In `ChatInput.tsx`:
    - Block image drag-and-drop or file selection when Devin is the active provider, showing notification *"Provider devin does not support vision yet"*.

- `T-7` **History Replay & Google Drive Sync (`devin_transcript_loader.go`, `chat_session_sync.go`)**:
  - Implement `devin_transcript_loader.go`: sessions nằm trong SQLite `~/.local/share/devin/cli/sessions.db` (F-5) — loader đọc DB (hoặc `session/list`+`session/load` qua ACP), không có per-session JSON file như giả định ban đầu.
  - In `BuildChatSessionSyncManifest`:
    - Include `cli/sessions.db` (+ related sidecars nếu có) trong sync manifest cho backup/restore.

- `T-8` **UI Styling & Brand Parity (`AgentsPanel.tsx`, `styles.css`)**:
  - In `AgentsPanel.tsx`:
    - Wire `resolveMainAgentDisplay` for Devin.
  - In `styles.css`:
    - Add `--devin-brand` color variable and `.prov-devin` badge styling.

- `T-9` **End-to-End Verification Sign-off**:
  - Execute manual test run covering all 17 sections of `CP-70-Test-Steps.md`.
  - Record Run ID, CLI version, and mark checkboxes in sign-off table.

---

## 5. Touched Areas

- **files (runner, new):**
  - `apps/local-runner/internal/runner/devin_transcript_loader.go`
  - `apps/local-runner/internal/runner/devin_transcript_loader_test.go`
- **files (runner, extend):**
  - `apps/local-runner/internal/runner/runner.go` (`resolvePromptExecutionAdapter`)
  - `apps/local-runner/internal/runner/yolo_resolver.go`
  - `apps/local-runner/internal/runner/chat_posture.go`
  - `apps/local-runner/internal/runner/summarizer.go`
  - `apps/local-runner/internal/runner/session_file_locator.go`
  - `apps/local-runner/internal/runner/chat_session_sync.go` (`BuildChatSessionSyncManifest`)
  - `apps/local-runner/internal/runner/devin_adapter.go`
- **files (desktop, extend):**
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - `apps/desktop-flowpilot/src/components/AgentsPanel.tsx`
  - `apps/desktop-flowpilot/src/components/visionProviders.ts`
  - `apps/desktop-flowpilot/src/styles.css`
- **modules:** Local runner execution engine & Desktop chat/flow interface

---

## 6. Acceptance Check

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` When YOLO is OFF, file edit or bash command triggers `permission_required` modal; Deny cleanly halts turn (verified: `TestDevinAdapterPermissionGateAndYolo`; live `appr-97` deny→`reject_once`, `appr-284` approve→executed).
- [x] `DOD-2` When YOLO is ON, eligible actions are auto-approved; `ask_user` unconditionally blocks for user input (verified: `TestDevinAdapterPermissionGateAndYolo`; live YOLO bypass + `ask_user` blocked for input).
- [x] `DOD-3` `ask_user` renders dialog in Desktop and resumes turn with user response (verified live: resumed `trusted-airmail` → `mcp__flowpilot__ask_user` → `q-260` → answer B → model replied B).
- [x] `DOD-4` `spawn_agent` executes child on isolated process without resetting parent MCP connection (verified live: `spawn_agent` → child `run-314` on own session `blushing-raver`).
- [x] `DOD-5` Child processes are reclaimed via `CloseProcessesForChildRun` after completion (impl `CloseDevinProcessesForChildRun` in `devin_process.go`; child scope `account|child:<runId>` torn down post-terminal).
- [x] `DOD-6` Chat postures `scan` and `plan` block write operations; `code` permits them (verified: `TestDevinResolveSessionMode`; live `scan`→`ask` no write, `plan`→plan doc only).
- [ ] `DOD-7` Tab key cycles between `plan` and `code` postures — deferred to UI phase (posture mapping itself live-verified via `chatPosture`).
- [x] `DOD-8` One-shot summary executes via `devin -p` (hoặc `acp --agent-type summarizer`) và prior summary được inject vào turn kế tiếp (unit: `TestDevinSummarizerModelUsesAccountDefault`, `TestDevinResolvePromptExecutionAdapterOneShot`; live `devin -p` blocked — needs interactive `devin auth login`, REPL store separate from ACP PKCE).
- [x] `DOD-9` Flow mode runs with Devin; gate violations trigger repair prompts on Devin (verified: `TestDevinGoldenFlow`; live `bug-harness` run-876 — freeze gate escalated → `WAITING_USER_APPROVAL` → `agent-loop/continue` 200).
- [x] `DOD-10` Vision Guard blocks image drops on UI when Devin is selected (verified: `visionProviders.ts` gates devin on `modelInputImage`; live image block → `claude-sonnet-5-low` read "Pink").
- [ ] `DOD-11` Google Drive `/sync`/`/restore` — deferred: needs connected Drive creds (env-blocked, recorded in CA-900).
- [x] `DOD-12` Manual run recorded in `CP-70-Test-Steps.md` §"Kết quả live test" (R1..R10 + ask_user + posture + skills + vision + R6 gates); TUI/Desktop subsections deferred per operator instruction.

### 6.2 Test Signatures

```go
// devin_adapter_test.go
func TestDevinApprovalGateDenyCleanExit(t *testing.T)
func TestDevinYoloOnQuestionGuard(t *testing.T)
func TestDevinAskUserRoundTrip(t *testing.T)
func TestDevinSpawnAgentChildIsolation(t *testing.T)
func TestDevinChildProcessReclamation(t *testing.T)
func TestDevinChatPostureEnforcement(t *testing.T)
func TestDevinOneShotSummarizer(t *testing.T)

// chat_session_sync_test.go
func TestDevinDriveSyncManifest(t *testing.T)

// devin_transcript_loader_test.go
func TestDevinTranscriptLoaderReplaysHistory(t *testing.T)
```

```typescript
// ChatInput.test.tsx
test("blocks image attachments when Devin is the active provider", () => {})
test("toggles posture between plan and code on Tab key press", () => {})

// AgentsPanel.test.tsx
test("renders child agent with Devin brand badge and status", () => {})
```

### 6.3 Code Signatures

```go
// devin_adapter.go
func (a *devinAdapter) handlePermissionRequest(ctx context.Context, req devinRequestPermissionParams) (devinPermissionResponse, error)
func (a *devinAdapter) handleAskUser(ctx context.Context, question string) (string, error)
func (a *devinAdapter) handleSpawnAgent(ctx context.Context, req SpawnAgentRequest) (SpawnAgentResult, error)

// summarizer.go
func summarizerModelFor(providerKey string) string

// devin_transcript_loader.go
func LoadDevinTranscript(sessionID string) ([]TurnRecord, error)
```

---

## 7. Out of Scope

- Implementing real vision multimodal recognition before golden fixture proof.
- Direct database schema alterations.

---

## 8. Completion Notes

- result: pending implementation
- implementation notes:
- verification: Requires sign-off in `CP-70-Test-Steps.md`
- upstream docs updated: `CP-70` Work Breakdown `P-5`, `P-6`, `P-8`, `P-9`, `P-10`, `P-11`
