# CP-56: Terminal TUI Chat And Flow Client

## Metadata

- Document ID: `CP-56`
- Title: `Terminal TUI Chat And Flow Client`
- Phase: `coding_plan`
- Status: `approved` (2026-08-12 — operator approved after full-plan audit and correction)
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-13` (Task-290 P-8c load-earlier + Task-291 YOLO write-gate captured; plan intent unchanged)
- Parent Documents: [SD-02: Architecture](../../06-System-Tech-Design/SD-02-Architecture.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [03 - Solution And System Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md), [04-01 Phase1 Desktop Mock MVP](../../10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md), [04-02 Runner Contracts And APIs](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md)
- Child Documents: [Task-278](../../08-Task/todo/Task-278-TUI-Go-Client-Ensure-Runner-And-Chat-Entry.md) (P-0), [Task-279](../../08-Task/todo/Task-279-Bubble-Tea-Chat-Stream-Shell.md) (P-1), [Task-280](../../08-Task/todo/Task-280-TUI-Session-Controls-Provider-Model-Reasoning-Yolo.md) (P-2), [Task-281](../../08-Task/todo/Task-281-TUI-Slash-Skill-Attachments.md) (P-3), [Task-282](../../08-Task/todo/Task-282-TUI-Image-Attach-Via-Existing-Turn-API.md) (P-3b), [Task-283](../../08-Task/todo/Task-283-TUI-Flow-And-Step-Slash-Launch.md) (P-4), [Task-284](../../08-Task/todo/Task-284-TUI-Statusline-Account-And-Token-Usage.md) (P-5), [Task-285](../../08-Task/todo/Task-285-TUI-Sub-Agent-Status-And-Focus-Switch.md) (P-6), [Task-286](../../08-Task/todo/Task-286-TUI-Approval-And-Question-Gates.md) (P-7), [Task-287](../../08-Task/todo/Task-287-TUI-Resume-Headless-And-Session-Reset.md) (P-8), [Task-289](../../08-Task/done/Task-289-TUI-Codex-Style-Assistant-Markdown.md) (P-8b), [Task-290](../../08-Task/done/Task-290-TUI-Long-Chat-Load-Earlier-Windowing.md) (P-8c), [Task-291](../../08-Task/todo/Task-291-Chat-Yolo-On-Must-Not-Prompt-Ordinary-File-Write.md) (P-2 residual), [Task-288](../../08-Task/todo/Task-288-TUI-Docs-Boundary-And-Rollout-Evidence.md) (P-9)
- Related Documents: [CP-36 Agent Review Loop](../done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [CP-42 Flow Pack](../done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md), [CP-51 Durable Turn Dispatch](../done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-56-Test-Steps](./CP-56-Test-Steps.md), [Product Vision](../../01-Vision/Product-vision.md)
- Replaces: `None`
- Tags: `cli-tui, chat-ui, agent-flow-engine, terminal, bubbletea, thin-client`
- Feature Keys: `cli-tui`

---

## AI Quick View

### Summary

- Ship a **terminal TUI client** (`flowpilot chat`) beside `desktop-flowpilot`, same UX jobs for chat + flow, **zero business-logic changes** in the Go runner core.
- Tech: **Bubble Tea + Lip Gloss + Bubbles** in isolated packages under `apps/local-runner/internal/tui/`, talking HTTP/SSE to the existing runner contract. The Cobra `flowpilot chat` command is only a composition root. The TUI packages must not import `internal/runner`. Includes image attach via the existing turn `attachments` API and **auto-ensures the runner** by spawning the same executable as `runner serve`.
- Two modes only: **Chat mode** (`normal_chat`) and **Flow / Step mode** (`workflow_step_auto` or first-turn `subMode`+`flowRef` orchestration). Settings / authoring stay in Desktop Settings.
- Required UI parity for interactive use: provider/model/reasoning, YOLO, `/skill`, `/flow`, `/step`, bottom statusline (account + tokens), and sub-agent strip with focus switch back to main.

### Current Ask

- **Approved.** Implement Tasks 278→288 in order, starting at Task-278. [Task-289](../../08-Task/done/Task-289-TUI-Codex-Style-Assistant-Markdown.md) is P-8b polish (Codex-style markdown). [Task-290](../../08-Task/done/Task-290-TUI-Long-Chat-Load-Earlier-Windowing.md) is P-8c long-chat windowing. [Task-291](../../08-Task/todo/Task-291-Chat-Yolo-On-Must-Not-Prompt-Ordinary-File-Write.md) is a P-2 residual: chat YOLO=on must not prompt ordinary file writes. They do not block Task-287 or Task-288. Update [CP-56-Test-Steps](./CP-56-Test-Steps.md) evidence as each Task lands.

### Key Decisions

- `D-1` **Thin client only.** TUI never owns workflow progression, flow gates, YOLO policy, or provider adapters. It only calls the same APIs as `HttpWsRunnerClient`.
- `D-2` **Bubble Tea over Ink.** Go TUI, no Node/Bun. Desktop React is not reused as a renderer.
- `D-3` **No Settings surface in CLI.** Provider connect, flow/workflow authoring, MCP, Supabase, Drive, skill packs, account home slots stay in Desktop Settings. CLI **loads** catalogs and chats.
- `D-4` **Two modes.** `chat` = `StartRunInput.chatMode = "normal_chat"`. `flow` / `step` = launch via `/flow` or `/step` (maps to `workflow_step_auto` startRun **or** chat-first-turn `flowRef`/`subMode` for built-in orchestration — see §4.3).
- `D-5` **Slash commands are the primary control surface** for skills/flows/steps (Claude/Codex-like). Provider/model/reasoning/YOLO use `/provider`, `/model`, `/reasoning`, `/yolo` plus optional startup flags.
- `D-6` **Statusline is first-class.** Bottom bar always shows active account + token/context usage (parity with desktop right bar / ChatInput usage line). When sub-agents exist, statusline also lists them and supports focus switch.
- `D-7` **Package split (operator Q-4):** keep one distributable `flowpilot` Go module/binary, but isolate all new UI code in `apps/local-runner/internal/tui/{client,app,runnerboot}`. `internal/cli/chat.go` may import the TUI composition package; TUI packages must **not** import `internal/runner`. This resolves the earlier two-module/delegate ambiguity and guarantees the requested `flowpilot chat` entrypoint. **Forbidden:** edits to `InteractiveService` business methods, provider adapters, flow executor, gates, or dispatch state machines.
- `D-8` **Additive tests only** for new TUI packages; no edits to pre-existing desktop/runner tests without operator approval.
- `D-9` **Auto-ensure runner (operator Q-2 revised).** Reuse the root command's existing persistent `--workspace`, `--host`, and `--port` flags; the chat command must not shadow them. Resolve the **runner workspace** independently from the selected project: inherited `--workspace` → `FLOWPILOT_WORKSPACE` → walk up for the FlowPilot application root marker (`apps/local-runner` plus `.agents`); fail with setup guidance if unresolved. URL precedence is explicit `--runner-url` → `FLOWPILOT_RUNNER_URL` → inherited host/port. Health-first reuse validates the FlowPilot payload and requires `health.cwd` to equal the resolved root; only the explicit `--runner-url` flag is a deliberate workspace-mismatch escape (`FLOWPILOT_RUNNER_URL` still requires a match). If down, spawn `os.Executable() runner serve --workspace <root> --host 127.0.0.1 --port P` with cwd `<root>`, detached from the TTY with platform-specific process attributes and logs redirected. Tolerate concurrent-client bind races by rechecking health. `--no-start-runner` fails closed. The selected project's `Project.path` is sent separately as `StartRunInput.cwd`; it never changes runner storage/config root. The runner remains alive when the TUI exits.
- `D-10` **Image attach in scope (operator Q-3).** Reuse the existing turn `attachments[]` API. TUI v1: `/image <path>` (+ multi) and pending list on the statusline; send base64 `PromptAttachment` on the next chat turn. Match Desktop safety limits: vision providers only (`codex`, `claude`), at most 6 images, 1568px maximum edge, 2 MiB maximum normalized image, plus bounded raw bytes and decoded pixels before allocation. Preserve PNG sources as PNG; normalize JPEG/WebP/GIF sources to JPEG quality 80 because the selected pure-Go stack has no WebP encoder. Terminal pixel preview is out of scope.
- `D-11` **Existing catalog routes only.** Project/workflow/step/skills/account data comes from `/client/*`. Provider/model/reasoning options come from existing `GET /providers` (`Provider.Models`, including reasoning capabilities); no new runner endpoint and no static model list.
- `D-12` **Turn-correlated streaming.** The client records `afterSeq` before POSTing a turn, captures `{turnId}`, filters replayed events whose non-empty `providerTurnId` belongs to another turn, and only ends the send state on `turn_completed`/`turn_failed` for that turn. Focus/replay streams remain unfiltered by turn.
- `D-13` **Flow blocking states are interactive.** Besides permission/question prompts, the TUI handles `flow_gate_violation` options and blocked agent-loop Continue/Stop/feedback actions through existing endpoints so Flow mode cannot deadlock on an unsupported desktop-only card. Approval values come only from the event; `remember` is offered/sent only for explicit exec approvals.
- `D-14` **Safe turn submission.** Generate one non-`durable-` idempotency key per logical turn and reuse it only while retrying the pre-mint transient codes `turn_in_progress`, `gate_in_progress`, and `hub_parked` (6 attempts, 700ms).
- `D-15` **Provider override parity.** Chat turns send a non-nil model pointer, including empty string to restore provider default. Grok YOLO first applies the existing account posture endpoint and flips local state only after success; other providers use per-turn YOLO.
- `D-16` **Reconnect and terminal compatibility.** Unexpected SSE EOF reconnects from the last monotonic sequence with bounded backoff. Legacy/non-UTF Windows consoles use ASCII status/agent glyphs; modern terminals retain Unicode styling.

### Constraints

- Do not duplicate agent-flow-engine, approval policy, or skill injection in the TUI.
- Do not add a second source of truth for projects/workflows/flows — read via existing GET routes.
- Do not implement Desktop Settings screens in the TUI.
- Preserve desktop behavior byte-for-byte; TUI is parallel, not a replacement.
- Windows + macOS must both work (Bubble Tea / Lip Gloss are cross-platform).
- GitNexus impact before editing any existing symbol; warn on HIGH/CRITICAL.
- Change-audit note + `feature_key: cli-tui` on every landed Task.

### Open Questions

- `Q-1` **RESOLVED (operator):** both — builtin pack refs first, then workflow catalog match; `/flow help` documents the order.
- `Q-2` **RESOLVED (operator revised):** **auto-ensure runner** — same client behavior as Desktop (Desktop supervisor already starts the runner; CLI does the equivalent ensure-or-reuse). See `D-9` and §13.
- `Q-3` **RESOLVED (operator):** image attach **in scope**. See `D-10` and P-3b. Effort: **not large** — runner/desktop API already exists; TUI work is pick file → base64 → `attachments` on turn (~0.5–1.5 day with tests if path-based `/image`; longer only if full ANSI image preview / clipboard paste matrix).
- `Q-4` **RESOLVED (plan review):** Go + Bubble Tea in a **separate TUI package tree**, not in `internal/runner`, while retaining one `flowpilot` module/binary. This preserves the requested package separation and removes the unresolved cross-module `flowpilot chat` delegation/release problem.

### Source Refs

- `04-01` `RunnerClient` contract; `04-02` `/client/*` APIs + SSE `afterSeq`.
- Desktop: `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, `src/state/store.ts`, `src/components/ChatInput.tsx`, `ProviderAccountsPanel.tsx`.
- Runner routes: `apps/local-runner/internal/runner/interactive_handlers.go`.
- Flow engine: SD-19, CP-36, CP-42.

---

## 1. Goal

Deliver a Claude/Codex-like **interactive terminal client** for FlowPilot that:

1. Runs beside Desktop (not instead of it).
2. Shares the existing Go runner core unchanged.
3. Supports **Chat mode** and **Flow/Step mode**.
4. Leaves configuration/authoring to Desktop Settings.
5. Implements the interactive feature set in §4.

Success looks like:

```text
$ flowpilot chat --project gate-sandbox
# Bubble Tea session
> hello
…streaming assistant…
────────────────────────────────────────────────────────
acc: codex · alice@… │ yolo:off │ ctx 12.4k/128k │ last 1.2k
agents: [main*] coder running · reviewer-1 idle
```

---

## 2. Input Documents

| Doc | Why it matters |
|---|---|
| `03-Solution-And-System-Design` | Runner owns logic; clients render only |
| `04-01` / `04-02` | Locked `RunnerClient` + HTTP/SSE contract |
| `SD-19` / `CP-36` / `CP-42` | Flow mode vocabulary + built-in packs |
| Desktop `HttpWsRunnerClient` | Exact API shapes to port to Go |
| Desktop `store.ts` sendPrompt path | Mode / turn / flowRef / skills mapping |

---

## 3. Implementation Strategy

### 3.1 Overall approach

```text
┌────────────────────────────────────────────┐
│ flowpilot chat  (Bubble Tea TUI)           │
│  model / update / view                    │
│  slash router · statusline · agent focus   │
└─────────────────┬──────────────────────────┘
                  │ HTTP + SSE (loopback)
┌─────────────────▼──────────────────────────┐
│ local-runner InteractiveService (UNCHANGED)│
│  /client/workflow-runs …                   │
│  providers · skills · flows · approvals    │
└────────────────────────────────────────────┘
```

Port the TypeScript `RunnerClient` surface into a Go `tui.Client` that is **behaviorally equivalent** to `HttpWsRunnerClient` for the methods the TUI needs. Do not invent new runner endpoints in this CP.

### 3.2 Sequencing logic

Scaffold client → chat stream → session controls → slash skills → flow/step → statusline → agents → gates → resume/headless → docs/DOD.

Each phase ends with green tests listed in [CP-56-Test-Steps](./CP-56-Test-Steps.md) and a Task + CA note.

### 3.3 Dependencies

- Local runner binary/module available to spawn (`flowpilot runner serve` — same as Desktop supervisor uses under `apps/local-runner`).
- At least one provider account configured via Desktop (or already on disk).
- Projects / workflows / built-in flows already exist (Settings / packs).
- No new Supabase migrations.
- Port default `4317` / env `FLOWPILOT_RUNNER_PORT` aligned with Desktop supervisor.

### 3.4 Non-goals

- Desktop Settings parity (MCP, Drive, Supabase, workflow builder, account connect UI).
- Embedding React/Ink.
- Changing provider adapters or flow executor.
- Full markdown/diff studio (syntax highlighter, mermaid, `bubbles/viewport` rewrite). Styled assistant markdown is [Task-289](../../08-Task/done/Task-289-TUI-Codex-Style-Assistant-Markdown.md) (Codex-style goldmark walk + cache), not Charm Glamour.
- Rich ANSI inline image preview (optional polish; filenames + count are enough for DOD).
- Clipboard multi-OS paste matrix beyond “path / file drop / `/image`” unless cheap on Windows.
- Replacing desktop as the primary coding UI.
- Replacing Desktop’s full supervisor (admin-web, Vite, etc.) — CLI only ensures the **runner** process, not the whole monorepo stack.

---

## 4. Work Breakdown

Code/test signatures shown below are implementation anchors, not the complete inventory. The authoritative, current test matrix is [CP-56-Test-Steps §4.1](./CP-56-Test-Steps.md#41-automated-test-signatures-by-phase); each Task acceptance range must match it.

**Product design (locked for this CP)**

### 4.1 Modes

| Mode | How user enters | Runner mapping |
|---|---|---|
| **Chat** | default; `/mode chat` | `StartRunInput{ chatMode: "normal_chat", providerKey, model, yoloMode, cwd }` then turns |
| **Flow** | `/flow <ref>` then first prompt | Prefer built-in: first turn `subMode`+`flowRef` on a `normal_chat` run (desktop bugfix picker path). Else Supabase workflow start with `workflowId` |
| **Step** | `/step <workflow>/<step>` or `/step <stepId>` | `StartRunInput{ workflowId, stepId, … }` (`workflow_step_auto` equivalent) |

Child agent focus is **not** a third mode — it is a **viewport focus** over an existing run tree (read-only child transcript; send from main only — same rule as desktop).

### 4.2 Settings boundary

| Concern | Desktop Settings | CLI |
|---|---|---|
| Connect provider account | ✅ | ❌ (read active account only) |
| Create/edit workflow / flow / step | ✅ | ❌ |
| MCP / Drive / Supabase | ✅ | ❌ |
| List projects / skills / flows | ✅ | ✅ load via GET |
| Chat / approve / interrupt | ✅ | ✅ |
| Pick provider/model/reasoning/YOLO for a session | ✅ | ✅ session-local |

If a catalog is empty, CLI prints: `No flows found. Configure in Desktop → Settings.` and stays usable for plain chat.

### 4.3 Required interactive features

#### 4.3.1 Provider / model / reasoning

- Commands: `/provider [codex|claude|grok|gemini]`, `/model [name|list]`, `/reasoning [none|low|medium|high|…]`
- Startup flags: `--provider`, `--model`, `--reasoning`
- Values are sent on `startRun` and re-sent per chat turn (desktop BUG-063 parity).
- Changing provider after a run has started: **blocked** (desktop rule) — show error, suggest `/new`.

#### 4.3.2 YOLO

- `/yolo [on|off|toggle]` session-local; sent on start + each chat turn.
- Flow mode: respect runner-enforced YOLO rules (do not invent CLI bypass). If runner rejects, surface error text.

#### 4.3.3 Skills — `/skill`

- `/skill` or `/skill list` → picker/list from `GET /client/provider-skills?provider=&cwd=`
- `/skill <name>` → attach for next turn (`selectedSkills` with `source: "slash_picker"`)
- `/skill clear` → clear pending attachments
- Pending skills shown in statusline or above input (`skills: foo, bar`).

#### 4.3.4 Flow / Step — `/flow` `/step`

- `/flow list` → built-in orchestration options (`GET /client/chat/builtin-orchestration-options`) + workflow catalog when available
- `/flow <flowRef>` → arm Flow mode for next/first turn
- `/step list` → steps for selected workflow
- `/step <id|name>` → arm Step launch
- `/mode chat|flow|step` → explicit mode switch (clears incompatible arms)

#### 4.3.5 Statusline (account + tokens)

Always-on bottom bar (desktop right-bar / ChatInput usage parity):

| Segment | Source |
|---|---|
| Active account | `GET /client/provider-accounts` (active for selected provider) |
| Provider · model · reasoning · yolo | session model |
| Context used / window · last turn tokens | `token_usage_updated` events (`TokenUsageSnapshot`) |
| Run status | local projection (`idle|streaming|waiting_approval|blocked|…`) |

#### 4.3.6 Sub-agent strip + focus switch

When `GET …/agent-graph` or `agent_graph_updated` shows children:

```text
agents: [main*] coder● reviewer-1○ reviewer-2○   Tab/Ctrl+O cycle · /agent main
```

- Focus child → subscribe/render that run’s event stream (`focusAgentRun` equivalent); composer send disabled with message (desktop parity).
- Focus main → restore send.
- Statusline updates live on spawn/complete.

#### 4.3.7 Gates (required for usable CLI, even if not numbered 4.x)

- `permission_required` → inline `[y/n/…]` → `POST /client/approvals/{id}/decision`
- `user_question_required` → numbered options → `POST /client/questions/{id}/answer`
- Esc / `/stop` → `POST …/interrupt`

---

### P-0 — Scaffold isolated TUI packages + Go RunnerClient port + ensure-runner

**Goal:** `flowpilot chat --help` works; typed HTTP client covers health + projects + start/turn/stream stubs; **ensure-or-reuse runner** matches Desktop client lifecycle.

**Deliverables**

```
apps/local-runner/
  internal/cli/chat.go       # Cobra composition root; registers `flowpilot chat`
  internal/tui/
    client/
      client.go              # HTTP wrapper
      types.go               # DTOs mirroring the wire contract
      sse.go                 # event stream afterSeq
    runnerboot/
      ensure.go              # health → reuse OR spawn self as runner serve
      detach_unix.go         # build-tagged process detachment
      detach_windows.go
    config/
      config.go              # URL/workspace/host/port precedence without shadowing root flags
    app/                     # Bubble Tea (later phases)
```

**Code signatures**

```go
// package client (apps/local-runner/internal/tui/client)
type Client struct {
    BaseURL    string
    HTTPClient *http.Client
}

func New(baseURL string) *Client

func (c *Client) Health(ctx context.Context) (Health, error)
func (c *Client) ListProjects(ctx context.Context) ([]Project, error)
func (c *Client) ListProviderAccounts(ctx context.Context) ([]ProviderAccountSummary, error)
func (c *Client) ListProviders(ctx context.Context) ([]Provider, error) // existing GET /providers
func (c *Client) StartRun(ctx context.Context, in StartRunInput) (RunHandle, error)
func (c *Client) SendTurn(ctx context.Context, runID string, in TurnInput) (turnID string, err error)
func (c *Client) StreamEvents(ctx context.Context, runID string, afterSeq int64) (<-chan ProviderEvent, <-chan error)
func (c *Client) SendTurnEvents(ctx context.Context, runID string, afterSeq int64, in TurnInput) (<-chan ProviderEvent, <-chan error)
func (c *Client) Interrupt(ctx context.Context, runID string) error
func (c *Client) SubmitApproval(ctx context.Context, approvalID, decision string, remember bool) error
func (c *Client) AnswerQuestion(ctx context.Context, questionID string, choice any) error
func (c *Client) ListSkills(ctx context.Context, provider, cwd string) ([]ProviderSkill, error)
func (c *Client) ListBuiltinOrchestrationOptions(ctx context.Context, subMode string) ([]BuiltinFlowOption, error)
func (c *Client) ListWorkflows(ctx context.Context, projectID string) ([]Workflow, error)
func (c *Client) ListSteps(ctx context.Context, workflowID string) ([]Step, error)
func (c *Client) GetAgentGraph(ctx context.Context, runID string) (AgentGraphSnapshot, error)
func (c *Client) ListAgentRuns(ctx context.Context, parentRunID string) ([]AgentRunSummary, error)

// package runnerboot — Desktop-parity ensure (see scripts/supervisor.js runner spawn)
type EnsureOptions struct {
    BaseURL     string        // e.g. http://127.0.0.1:4317
    Port        int
    Workspace   string        // required resolved FlowPilot application root; never the selected project
    BinaryPath  string        // test/advanced override; empty = os.Executable()
    ReadyTimeout time.Duration
    NoStart     bool          // --no-start-runner
}

type EnsureResult struct {
    AlreadyRunning bool
    StartedByCLI   bool
    BaseURL        string
    // Process handle only when StartedByCLI — used for optional shutdown policy
}

func EnsureRunner(ctx context.Context, opt EnsureOptions) (EnsureResult, error)
func WaitHealthy(ctx context.Context, baseURL string, timeout time.Duration) error
```

**Lifecycle rules (lock)**

0. Resolve the FlowPilot application root from the inherited `--workspace`, `FLOWPILOT_WORKSPACE`, or a strict app-root marker walk. Keep selected project cwd separate.
1. Health OK → validate `status`, `runnerVersion`, and workspace match; reuse with `StartedByCLI=false`. Explicit `--runner-url` is the only workspace-mismatch escape.
2. Health fail → spawn `os.Executable() runner serve --workspace <root> --host 127.0.0.1 --port <port>` with cwd `<root>` (env `FLOWPILOT_RUNNER_PORT` set), detach from the terminal, redirect logs to `<root>/.flowpilot/cli-runner.log`, release the process handle, and wait healthy.
3. Port already taken by non-FlowPilot process → error with clear message (do not kill foreign process).
4. Concurrent ensure race: if the spawned process exits or loses the bind, recheck valid FlowPilot health and adopt the winner; otherwise surface the startup log path.
5. **Shutdown:** v1 default = **do not kill** a runner the CLI started when the TUI exits. No `go run` wrapper fallback is used, avoiding the known Windows orphaned-child failure.
6. `--no-start-runner`: if unhealthy → exit non-zero (for CI / attach-only).

**Tests (signatures)**

```go
func TestClientHealth_OK(t *testing.T)
func TestClientStartRun_NormalChatBodyShape(t *testing.T)
func TestClientStreamEvents_RespectsAfterSeqAndParsesTokenUsage(t *testing.T)
func TestClientSendTurn_IncludesSelectedSkillsAndYolo(t *testing.T)
func TestClientSendTurnEvents_FiltersOtherTurnsAndStopsOnOwnTerminal(t *testing.T)
func TestEnsureRunner_ReusesWhenHealthy(t *testing.T)
func TestEnsureRunner_NoStartFlag_ErrorsWhenDown(t *testing.T)
func TestWaitHealthy_TimesOut(t *testing.T)
// Spawn path: integration-style test with a fake "runner" binary stub, or build-tagged live test
func TestEnsureRunner_SpawnsWhenDown(t *testing.T)
func TestEnsureRunner_ConcurrentBindWinnerIsReused(t *testing.T)
func TestEnsureRunner_RejectsForeignHealthPayload(t *testing.T)
func TestEnsureRunner_RejectsWorkspaceMismatchOnReuse(t *testing.T)
func TestResolveRunnerWorkspace_DoesNotUseSelectedProject(t *testing.T)
func TestSpawnAttrs_DetachedPerOS(t *testing.T)
func TestEnsureRunner_SpawnFailsFastOnForeignPortUse(t *testing.T)
```

Use `httptest.NewServer` fakes for client tests; runnerboot spawn tests use a stub executable that listens and serves `/health`.

---

### P-1 — Bubble Tea shell + normal chat stream

**Goal:** Interactive REPL for chat mode: type prompt → stream deltas → turn complete.

**Deliverables**

```
apps/local-runner/internal/tui/app/
  app.go           # tea.Program entry
  model.go         # Model struct
  update.go        # Update(msg)
  view.go          # View()
  msgs.go          # streamDeltaMsg, turnDoneMsg, errMsg, …
  render_events.go # ProviderEvent → timeline lines
```

**Code signatures**

```go
type Mode int // ModeChat, ModeFlow, ModeStep

type Model struct {
    client     *client.Client
    session    SessionState
    timeline   []TimelineItem
    input      textarea.Model // bubbles
    viewport   viewport.Model
    status     StatusModel
    width, height int
    err        error
}

func NewModel(cfg Config) Model
func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m Model) View() string

func waitStreamEventCmd(events <-chan ProviderEvent, errs <-chan error) tea.Cmd // one event; Update must re-arm
func mapEvent(e ProviderEvent) []TimelineItem
func ResolveStartupProject(projects []Project, explicit, cwd string) (Project, bool, error) // bool=false opens picker
```

**Tests**

```go
func TestModel_SubmitPrompt_StartsRunThenTurn(t *testing.T)
func TestMapEvent_MessageDeltaAppendsStreamingLine(t *testing.T)
func TestMapEvent_TurnCompletedFinalizesAssistant(t *testing.T)
func TestModel_StopKey_CallsInterrupt(t *testing.T)
func TestStreamEventMsg_RearmsUntilOwnTurnTerminal(t *testing.T)
func TestResolveStartupProject_ExplicitThenCwdThenPicker(t *testing.T)
```

---

### P-2 — Provider / model / reasoning / YOLO controls

**Goal:** Session-local controls + startup flags; per-turn resend in chat mode.

**Slash / flags**

- `/provider`, `/model`, `/reasoning`, `/yolo`; list/set options are loaded from existing `GET /providers` model capability data
- `--provider --model --reasoning --yolo`
- Residual live defect: [Task-291](../../08-Task/todo/Task-291-Chat-Yolo-On-Must-Not-Prompt-Ordinary-File-Write.md) — chat YOLO=on must not prompt an ordinary workspace write (`test.txt`). If the failing layer is the adapter/bridge rather than the TUI payload, that task may edit runner YOLO wiring; it is not a new CP-56 chrome feature and does not lift D-1 for other TUI work.

**Code signatures**

```go
type SessionControls struct {
    ProviderKey     string
    Model           string
    ReasoningEffort string
    Yolo            bool
}

func (s *SessionControls) ApplySlash(cmd SlashCommand) error
func (s SessionControls) TurnOverrides() TurnInput // model/yolo/reasoning fields
func ParseChatFlags(fs *pflag.FlagSet) (SessionControls, error)
func OptionsFromProviders(providers []Provider, selectedProvider string) ControlOptions
```

**Tests**

```go
func TestSessionControls_YoloToggle(t *testing.T)
func TestSessionControls_ProviderLockedAfterRunStarted(t *testing.T)
func TestSendTurn_ChatModeResendsModelAndYoloEachTurn(t *testing.T)
func TestParseChatFlags_Defaults(t *testing.T)
func TestOptionsFromProviders_UsesLiveModelsAndReasoningCapabilities(t *testing.T)
```

---

### P-3 — `/skill` picker and attachment

**Goal:** Load skills from runner; attach to next turn.

**Code signatures**

```go
type SkillAttachment struct {
    Name   string
    Path   string
    Source string // "slash_picker"
}

type SkillState struct {
    Catalog []ProviderSkill
    Pending []SkillAttachment
}

func (s *SkillState) HandleSlash(args []string) (msg string, err error)
func (s *SkillState) ConsumePendingForTurn() []SkillAttachment
```

**Tests**

```go
func TestSkillState_ListAndAttach(t *testing.T)
func TestSkillState_Clear(t *testing.T)
func TestSkillState_UnknownNameErrors(t *testing.T)
func TestTurnInput_IncludesPendingSkillsThenClears(t *testing.T)
```

---

### P-3b — Image attach (`/image`)

**Goal:** Attach images to the next chat turn using the **existing** runner API (no core changes). Desktop already sends `TurnInput.attachments[]` (`PromptAttachment`: id, kind, originalName, mimeType, data base64, sizeBytes).

**Why not long:** runner + providers already accept vision attachments (Task-052). TUI only needs load file → (optional normalize) → put on turn body.

**UX (v1 DOD)**

- `/image <path>` — queue one file (png/jpg/webp/gif as runner allows)
- `/image list` / `/image clear`
- Statusline or composer hint: `images: a.png, b.webp`
- On send (chat mode only): include `attachments` then clear pending
- Allow only `codex`/`claude`; queue at most 6 images
- Normalize to at most 1568px on the long edge, then reject output above 2 MiB

**Out of DOD for P-3b:** pixel preview in terminal, drag-drop from Explorer (nice-to-have if Bubble Tea mouse paste is free).

**Code signatures**

```go
type ImagePending struct {
    Path         string
    Attachment   PromptAttachment // filled after read/normalize
}

type ImageState struct {
    Pending []ImagePending
}

func LoadImageAttachment(path string) (PromptAttachment, error)
func NormalizeImage(raw []byte, mime string) (out []byte, outMime string, width, height int, err error)
func (s *ImageState) HandleSlash(args []string) (msg string, err error)
func (s *ImageState) ConsumePendingForTurn() []PromptAttachment
```

**Tests**

```go
func TestLoadImageAttachment_PNG(t *testing.T)
func TestLoadImageAttachment_RejectsMissingFile(t *testing.T)
func TestLoadImageAttachment_RejectsOversize(t *testing.T)
func TestLoadImageAttachment_DownscalesLongEdgeAndSetsDimensions(t *testing.T)
func TestImageState_RejectsNonVisionProviderAndSeventhImage(t *testing.T)
func TestImageState_AttachListClear(t *testing.T)
func TestTurnInput_IncludesAttachmentsThenClears(t *testing.T)
func TestTurnInput_WorkflowModeOmitsAttachments(t *testing.T) // desktop parity: chat-only
```

**Effort estimate:** ~0.5–1.5 engineer-days including tests (path-based). Add ~1 day only if clipboard paste + normalize parity with desktop is required in the same slice.

---

### P-4 — `/flow` `/step` and mode switching

**Goal:** Arm and launch Flow/Step using existing APIs only.

**Resolution algorithm (`Q-1` → (c))**

```text
/flow <ref>
  1. Query supported chat subModes (v1: bug); exact flowRef match → arm chat+flowRef+matched subMode
     and first-turn changeType:"bugfix" for the bug subMode
  2. Else match workflow id/name in project catalog → arm ModeFlow workflowId
  3. Else error with hint to Desktop Settings
```

**Code signatures**

```go
type LaunchArm struct {
    Mode       Mode
    FlowRef    string
    SubMode    string // e.g. "bug" when using builtin orchestration
    ChangeType string // e.g. "bugfix" for the bug subMode
    SourceDocID string
    WorkflowID string
    StepID     string
}

func ResolveFlowRef(ctx context.Context, c *client.Client, projectID, ref string) (LaunchArm, error)
func ResolveStepRef(ctx context.Context, c *client.Client, projectID, workflowHint, ref string) (LaunchArm, error)
func (a LaunchArm) ToStartRunInput(projectID string, ctrl SessionControls, cwd string) StartRunInput
func (a LaunchArm) FirstTurnExtras() (subMode, flowRef, changeType, sourceDocID string)
```

**Tests**

```go
func TestResolveFlowRef_BuiltinPack(t *testing.T)
func TestResolveFlowRef_WorkflowCatalog(t *testing.T)
func TestResolveFlowRef_MissingHintsDesktopSettings(t *testing.T)
func TestResolveStepRef_ByName(t *testing.T)
func TestLaunchArm_FirstTurnSendsSubModeAndFlowRef(t *testing.T)
func TestModeSwitch_ClearsIncompatibleArm(t *testing.T)
func TestCatalogResolvers_FilterProjectAndWorkflowClientSide(t *testing.T)
func TestResolveFlowRef_SubModeCarriedFromMatchedOption(t *testing.T)
func TestLaunchArm_FirstTurnSendsChangeTypeBugfix(t *testing.T)
```

---

### P-5 — Statusline (account + tokens)

**Goal:** Bottom bar parity with desktop account/usage display.

**Code signatures**

```go
type StatusModel struct {
    AccountLabel string
    Provider     string
    Model        string
    Reasoning    string
    Yolo         bool
    Mode         Mode
    RunStatus    string
    Usage        *TokenUsageSnapshot
    SkillsPending []string
    Agents       []AgentStatusLine // filled in P-6
    FocusRunID   string
}

func (s StatusModel) View(width int) string
func (s *StatusModel) ApplyTokenUsage(u TokenUsageSnapshot)
func (s *StatusModel) RefreshAccount(accounts []ProviderAccountSummary, provider string)
func FormatUsageLine(u *TokenUsageSnapshot) string // mirror ChatInput.usageSummaryLine semantics
```

**Tests**

```go
func TestFormatUsageLine_ContextAndLastTurn(t *testing.T)
func TestStatusModel_RefreshAccount_PicksActive(t *testing.T)
func TestStatusModel_View_TruncatesGracefully(t *testing.T)
func TestStatusModel_ApplyTokenUsage_UpdatesSegments(t *testing.T)
```

---

### P-6 — Sub-agent strip + focus switch

**Goal:** Show live agents; cycle focus; child read-only.

**Code signatures**

```go
type AgentStatusLine struct {
    RunID  string
    Name   string
    Role   string
    Status string // running|completed|failed|…
    IsMain bool
}

type FocusState struct {
    MainRunID string
    RunID     string
    Timelines map[string][]TimelineItem
    AfterSeq  map[string]int64
    CancelChild context.CancelFunc // main orchestration stream is independent and never cancelled by focus
}

func ProjectAgents(graph AgentGraphSnapshot, mainRunID string) []AgentStatusLine
func (m Model) FocusAgent(runID string) (Model, tea.Cmd)
func (m Model) FocusMain() (Model, tea.Cmd)
func cycleAgentCmd(agents []AgentStatusLine, current string, dir int) string
```

**Keybindings:** `Tab` / `Shift+Tab` cycle; `/agent main|<name|id>`; click N/A in v1.

**Tests**

```go
func TestProjectAgents_OrdersMainFirst(t *testing.T)
func TestFocusAgent_DisablesSendOnChild(t *testing.T)
func TestFocusMain_RestoresSend(t *testing.T)
func TestCycleAgent_Wraps(t *testing.T)
func TestAgentGraphUpdated_RefreshesStatusline(t *testing.T)
func TestFocusRoundTrip_PreservesPerRunTimelineAndCursor(t *testing.T)
func TestFocusSwitch_CancelsPriorStream(t *testing.T)
func TestChildFocus_MainStreamStillUpdatesAgentsAndTokens(t *testing.T)
```

---

### P-7 — Approvals, questions, flow gates, blocked-loop controls, interrupt UX

**Goal:** CLI usable when YOLO=off, ask_user fires, a regression gate blocks, or an agent loop awaits Continue/Stop/feedback.

**Code signatures**

```go
type GatePrompt struct {
    Kind       string // approval|question|flow_gate|blocked_loop
    ID         string
    Prompt     string
    Options    []string
    MultiSelect bool
}

func GateFromEvent(e ProviderEvent) (GatePrompt, bool)
func (m Model) HandleGateAnswer(raw string) (Model, tea.Cmd)
func (c *Client) SubmitGateDecision(ctx context.Context, runID, option, customText string) error
func (c *Client) ContinueFlow(ctx context.Context, runID, feedback string) error
func (c *Client) StopAgentLoop(ctx context.Context, runID string) error
```

**Tests**

```go
func TestGateFromEvent_PermissionRequired(t *testing.T)
func TestGateFromEvent_UserQuestion(t *testing.T)
func TestHandleGateAnswer_SubmitsApprovalDecision(t *testing.T)
func TestHandleGateAnswer_RejectsInvalidOption(t *testing.T)
func TestInterrupt_WhileStreaming(t *testing.T)
func TestFlowGateViolation_SubmitsAdvertisedDecision(t *testing.T)
func TestBlockedLoop_ContinueAndStopUseExistingEndpoints(t *testing.T)
```

---

### P-8 — Resume, `/new`, headless `-p`, polish

**Goal:** Daily-driver completeness.

- `/new` — clear session, new run on next prompt
- `--resume <runId>` — cold open + replay stream from seq 0
- `-p/--print <prompt>` — non-TTY one-shot (no Bubble Tea); print final message + exit code. If an approval/question/flow gate appears, interrupt and exit non-zero with guidance instead of hanging.
- Optional styled assistant markdown: [Task-289](../../08-Task/done/Task-289-TUI-Codex-Style-Assistant-Markdown.md) (P-8b). Codex-style goldmark → Lip Gloss lines with per-message cache. Charm Glamour + `bubbles/viewport` is out (CA-463).
- Long-chat windowing: [Task-290](../../08-Task/done/Task-290-TUI-Long-Chat-Load-Earlier-Windowing.md) (P-8c). Desktop Timeline parity — newest 6 prompt groups first, Load earlier for the rest. Render-side only.

**Code signatures**

```go
func RunHeadless(ctx context.Context, cfg Config, prompt string) (exitCode int, err error)
func (c *Client) GetRun(ctx context.Context, runID string) (RunSnapshot, error)
func (c *Client) ResumeRun(ctx context.Context, runID string) (RunHandle, error)
```

**Tests**

```go
func TestRunHeadless_PrintsFinalMessage(t *testing.T)
func TestRunHeadless_GateInterruptsAndExitsNonZero(t *testing.T)
func TestResume_ReplaysThroughLastEventSeqBeforeFollowingLive(t *testing.T)
func TestRunHeadless_NonZeroOnTurnFailed(t *testing.T)
func TestResume_ReplaysFromSeqZero(t *testing.T)
func TestNewCommand_ClearsRunID(t *testing.T)
```

---

### P-8c — Long-chat load-earlier windowing

**Goal:** Long TUI chats do not paint from turn 1. Match Desktop Timeline: last 6 user-prompt groups visible, `↑ Load earlier prompts (N)` pages older groups.

See [Task-290](../../08-Task/done/Task-290-TUI-Long-Chat-Load-Earlier-Windowing.md). Does not change Task-287 seq-0 replay contract in this slice.

**Tests**

```go
func TestChatWindow_ShowsNewestSixPromptGroups(t *testing.T)
func TestChatWindow_LoadEarlierRevealsPreviousPage(t *testing.T)
func TestChatWindow_PendingGateStaysVisible(t *testing.T)
func TestChatWindow_NewResetsWindow(t *testing.T)
```

---

### P-9 — Docs, operator checklist, feature-key hygiene

**Goal:** Operator-facing README snippet + CP/Test-Steps evidence closed; FEATURE-KEYS already has `cli-tui`.

- Short `apps/local-runner/internal/tui/README.md` (how to run, mode table, settings boundary, ensure-runner / `--no-start-runner`)
- Update root README “clients” blurb (one paragraph)
- Fill CP-56-Test-Steps evidence table
- No runner core changes verification: `git diff` scoped allowlist

**Tests / checks**

```go
func TestPackageBoundary_TuiDoesNotImportRunnerInternals(t *testing.T)
// optional: archtest / grep in CI script
```

---

## 5. Touched Areas

| Area | Path | Allowed change |
|---|---|---|
| New CLI TUI packages | `apps/local-runner/internal/tui/**` | create (HTTP client, Bubble Tea, runner boot, tests, README) |
| Cobra composition | `apps/local-runner/internal/cli/chat.go`, `root.go` | additive `flowpilot chat` registration only |
| Runner module dependencies | `apps/local-runner/go.mod`, `go.sum` | Bubble Tea, Bubbles, Lip Gloss, image decode/resize libraries |
| TUI CI | `.github/workflows/cli-tui.yml` | additive Windows + Ubuntu vet/test job for `internal/tui/...` and `internal/cli/...` |
| Docs | `apps/local-runner/internal/tui/README.md`, root README blurb | add/update |
| Feature keys | `change-audit/FEATURE-KEYS.md` | `cli-tui` (already added) |
| Runner interactive core | `apps/local-runner/internal/runner/**` | **forbidden** in this CP |
| Desktop | `apps/desktop-flowpilot/**` | **forbidden** (read-only reference for normalize rules) |
| DB / migrations | — | none |

---

## 6. Data or Migration Steps

- schema: none
- data backfill: none
- config: optional `FLOWPILOT_RUNNER_URL` / reuse `FLOWPILOT_RUNNER_PORT`; no new required env for MVP

---

## 7. Validation Plan

See [CP-56-Test-Steps](./CP-56-Test-Steps.md) for the full matrix.

Summary:

| Layer | What |
|---|---|
| Unit | Client body shapes, slash parsers, statusline formatters, agent projection, gate mapping |
| Integration (fake HTTP) | start→turn→SSE→approval round-trip |
| Manual | Live against real runner: chat, `/skill`, `/flow` review-loop, YOLO off approval, agent Tab switch |
| Regression | Desktop + `go test ./internal/runner/...` remain green; TUI changes must not touch runner packages |

Failure cases to assert:

- Runner down → auto-start or clear fail-closed error with `--no-start-runner`
- Foreign listener / concurrent ensure race → no kill, no second runner, deterministic message
- Replayed events from another turn → ignored by turn-correlated send stream
- Empty skills/flows → Settings hint, no panic
- Multiple projects with no exact cwd match → startup picker, not arbitrary first-project selection
- Provider change mid-run → rejected
- Child focus send → blocked with message
- Invalid `/flow` ref → error, stay in chat
- Headless permission/question/flow gate → interrupt and non-zero exit, never hang

---

## 8. Rollout and Fallback

- Rollout: ship behind normal binary; feature is opt-in (`flowpilot chat`).
- Fallback: Desktop remains full UI; CLI can be ignored.
- Monitoring: runner logs unchanged; TUI logs to stderr only on errors.
- Kill switch: omit `chat` subcommand registration if emergency (not expected).

---

## 9. Risks

| ID | Risk | Mitigation |
|---|---|---|
| `R-1` | Temptation to “quickly” add a runner endpoint for TUI convenience | Hard forbid in D-7; escalate as separate Task |
| `R-2` | Flow ref ambiguity (pack vs workflow) | Explicit resolver + help text (Q-1) |
| `R-3` | Bubble Tea + Windows console quirks | Early P-1 smoke on Windows; viewport/width tests |
| `R-4` | Statusline overcrowding with many agents | Truncate + `/agent list` detail |
| `R-5` | SSE reconnect gaps | Mirror desktop `afterSeq` / Last-Event-ID behavior in client |
| `R-6` | Scope creep into Settings | Non-goals + empty-catalog copy pointing to Desktop |
| `R-7` | Double-start / port fight with Desktop | Health-first reuse; never kill foreign listener; align default port with supervisor |
| `R-8` | Orphan runner after CLI exit (esp. Windows) | Document leave-running v1; follow Desktop/supervisor patterns; optional `--stop-runner-on-exit` later |
| `R-9` | Separate Go module cannot cleanly provide `flowpilot chat` or self-start the runner | One module/binary with isolated `internal/tui` packages (D-7) |
| `R-10` | Model picker drifts from Desktop/static lists | Load live provider models and reasoning metadata from existing `GET /providers` |
| `R-11` | Flow mode blocks on gate/cap states the TUI cannot answer | P-7 handles permission, question, flow-gate, and blocked-loop controls |

---

## 10. Definition of Done

- [ ] `flowpilot chat` launches TUI; if runner down, **auto-starts** `runner serve` then connects (reuse if already up)
- [ ] `--no-start-runner` fails closed when runner down
- [ ] Chat mode: stream, stop, provider/model/reasoning/YOLO, `/skill`, `/image`
- [ ] Flow/Step: `/flow` and `/step` launch via existing APIs; no Settings in CLI
- [ ] Statusline shows active account + token/context usage
- [ ] Sub-agent strip + focus switch (child read-only)
- [ ] Approvals, questions, flow-gate options, and blocked-loop Continue/Stop work with YOLO off
- [ ] Headless `-p` works for scripting
- [ ] All phase test signatures implemented and green
- [ ] Additive TUI CI passes on Windows and Ubuntu
- [ ] Manual checklist in CP-56-Test-Steps filled
- [ ] No forbidden diffs under `internal/runner` business logic
- [ ] Each phase has Task + CA with `feature_key: cli-tui`
- [ ] Operator README documents Settings boundary

---

## 11. Task Cut (approved)

| Phase | Task |
|---|---|
| P-0 | [Task-278](../../08-Task/todo/Task-278-TUI-Go-Client-Ensure-Runner-And-Chat-Entry.md) |
| P-1 | [Task-279](../../08-Task/todo/Task-279-Bubble-Tea-Chat-Stream-Shell.md) |
| P-2 | [Task-280](../../08-Task/todo/Task-280-TUI-Session-Controls-Provider-Model-Reasoning-Yolo.md) |
| P-3 | [Task-281](../../08-Task/todo/Task-281-TUI-Slash-Skill-Attachments.md) |
| P-3b | [Task-282](../../08-Task/todo/Task-282-TUI-Image-Attach-Via-Existing-Turn-API.md) |
| P-4 | [Task-283](../../08-Task/todo/Task-283-TUI-Flow-And-Step-Slash-Launch.md) |
| P-5 | [Task-284](../../08-Task/todo/Task-284-TUI-Statusline-Account-And-Token-Usage.md) |
| P-6 | [Task-285](../../08-Task/todo/Task-285-TUI-Sub-Agent-Status-And-Focus-Switch.md) |
| P-7 | [Task-286](../../08-Task/todo/Task-286-TUI-Approval-And-Question-Gates.md) |
| P-8 | [Task-287](../../08-Task/todo/Task-287-TUI-Resume-Headless-And-Session-Reset.md) |
| P-8b | [Task-289](../../08-Task/done/Task-289-TUI-Codex-Style-Assistant-Markdown.md) |
| P-8c | [Task-290](../../08-Task/done/Task-290-TUI-Long-Chat-Load-Earlier-Windowing.md) |
| P-2 residual | [Task-291](../../08-Task/todo/Task-291-Chat-Yolo-On-Must-Not-Prompt-Ordinary-File-Write.md) |
| P-9 | [Task-288](../../08-Task/todo/Task-288-TUI-Docs-Boundary-And-Rollout-Evidence.md) |

---

## 12. Example Session Scripts (acceptance demos)

### 12.1 Normal chat

```text
$ flowpilot chat --project gate-sandbox --provider codex --model gpt-5.4-mini
> /yolo off
> /skill list
> /skill my-skill
> explain the runner health endpoint
…stream…
(approval prompt) → y
```

### 12.2 Flow mode (built-in review-loop)

```text
$ flowpilot chat --project gate-sandbox
> /flow flowpilot-core-flow-pack/review-loop
> fix the off-by-one in calc.go
…hub + agents appear on statusline…
Tab → focus coder (read-only)
/agent main
```

### 12.3 Step mode

```text
> /step my-workflow/plan
> start
…workflow step run…
```

### 12.4 Headless

```text
$ flowpilot chat -p "summarize README" --provider claude --project gate-sandbox
# prints final message, exit 0
```

### 12.5 Image attach

```text
> /image ./shots/bug.png
> /image list
> what is wrong in this screenshot?
…turn carries attachments[]…
```

---

## 13. Plain-language note: Q-2 (runner lifecycle) — final

Desktop today (via `scripts/supervisor.js`) **starts the runner if it is not already on the port**, otherwise reuses it. Closing Desktop does not always mean “runner must die.”

CLI is the same class of client:

| Step | Behavior |
|---|---|
| Runner healthy | Reuse — do not spawn a second server |
| Runner down | Spawn `flowpilot runner serve` in background, wait for `/health`, then open TUI |
| Escape hatch | `--no-start-runner` for CI / attach-only |
| Exit TUI | v1: leave runner running (Desktop-like); optional `--stop-runner-on-exit` later if CLI started it |

**Final operator decision:** auto-ensure runner — **yes**.
