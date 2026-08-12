# Task-278: TUI Go Client, Ensure-Runner Boot, And Chat Entry (CP-56 P-0)

## Metadata

- Document ID: `Task-278`
- Title: `TUI Go Client, Ensure-Runner Boot, And Chat Entry`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-0), [CP-56-Test-Steps](../../07-Coding-Plan/inprogress/CP-56-Test-Steps.md), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md)
- Child Documents: `none`
- Related Documents: [Task-279](./Task-279-Bubble-Tea-Chat-Stream-Shell.md) (next), Desktop [`HttpWsRunnerClient.ts`](../../../apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts), [`scripts/supervisor.js`](../../../scripts/supervisor.js) (ensure-runner reference)
- Replaces: `None`
- Tags: `cli-tui, thin-client, http, sse, runnerboot`

## AI Quick View

### Summary

- Create isolated `apps/local-runner/internal/tui/{client,runnerboot,config}` packages plus the thin `flowpilot chat` Cobra entry.
- Implement the typed HTTP/SSE client, including turn-correlation (`providerTurnId`) and `afterSeq`.
- Implement `runnerboot.EnsureRunner`: validate health → reuse, else spawn the same executable as `runner serve`, wait `/health`, and tolerate concurrent ensure races.

### Current Ask

Implement P-0 exactly. Do not build the interactive TUI shell (Task-279).

### Key Decisions

- `T-1` One existing Go module/binary, separate `internal/tui` package tree — **must not** import `internal/runner`.
- `T-2` JSON field names match desktop/runner contract (`projectId`, `chatMode`, `attachments`, … camelCase as runner expects).
- `T-3` Ensure-runner: reuse if healthy; spawn only when down; `--no-start-runner` fails closed; leave runner running on exit (v1).
- `T-4` Default URL `http://127.0.0.1:${FLOWPILOT_RUNNER_PORT||4317}`.
- `T-5` `SendTurnEvents` captures the turn ID and only terminates on that turn's terminal event; replayed events for other turns are filtered.
- `T-6` Resolve one stable FlowPilot runner workspace before health/spawn and compare it with `Health.Cwd`. The selected project path is a separate per-run cwd.

### Constraints

- Forbidden: edits to `apps/local-runner/internal/runner/**` business logic.
- Only additive composition changes in `apps/local-runner/internal/cli/chat.go` and `root.go`.
- Additive tests in new TUI packages; existing runner tests remain unchanged.

### Open Questions

- None. `os.Executable()` is the canonical spawn target in both compiled and `go run` development; no source-tree lookup fallback.

### Source Refs

- CP-56 P-0, D-7, D-9, D-12; Test-Steps A0.1–A0.19, M3c–M3e, M13.

---

## 1. Goal

Ship a runnable chat entrypoint that can reach a healthy runner (auto-ensure) and speak the `/client/*` HTTP+SSE contract — foundation for all later TUI tasks.

## 2. Parent Links

- coding plan: CP-56 P-0
- tech design: SD-02 (clients thin), 04-02 APIs
- system spec: n/a (client surface)
- specific upstream ids: CP-56 D-7, D-9, D-12, D-14

## 3. Trigger

CP-56 approved; first implementation slice.

## 4. Exact Change

### 4.1 Layout

```text
apps/local-runner/
  internal/cli/chat.go
  internal/tui/client/
    client.go
    types.go
    sse.go
    client_test.go
    sse_test.go
  internal/tui/runnerboot/
    ensure.go
    detach_unix.go
    detach_windows.go
    ensure_test.go
    stub_runner_test.go   # fake binary for spawn tests
  internal/tui/config/
    config.go             # BaseURL, Port, NoStartRunner, Workspace
```

### 4.2 `internal/tui/client/types.go` (minimum for P-0 + stubs for later)

```go
type Health struct {
    Status        string `json:"status"`
    RunnerVersion string `json:"runnerVersion"`
    Cwd           string `json:"cwd"`
    OS            string `json:"os"`
    StartedAt     string `json:"startedAt"`
}

type Project struct {
    ID   string `json:"id"`
    Name string `json:"name"`
    Path string `json:"path"`
}

type StartRunInput struct {
    ProjectID       string `json:"projectId"`
    WorkflowID      string `json:"workflowId,omitempty"`
    StepID          string `json:"stepId,omitempty"`
    ProviderKey     string `json:"providerKey,omitempty"`
    Model           string `json:"model,omitempty"`
    YoloMode        *bool  `json:"yoloMode,omitempty"`
    ReasoningEffort string `json:"reasoningEffort,omitempty"`
    ChatMode        string `json:"chatMode,omitempty"`
    Cwd             string `json:"cwd,omitempty"`
}

type RunHandle struct {
    RunID             string `json:"runId"`
    ProviderSessionID string `json:"providerSessionId"`
    ProviderKey       string `json:"providerKey"`
    Status            string `json:"status"`
    StepID            string `json:"stepId,omitempty"`
    LastEventSeq      int64  `json:"lastEventSeq,omitempty"`
}

type TurnInput struct {
    StepID          string             `json:"stepId"`
    Prompt          string             `json:"prompt"`
    SelectedSkills  []SkillSelection   `json:"selectedSkills,omitempty"`
    ReasoningEffort string             `json:"reasoningEffort,omitempty"`
    Model           *string            `json:"model,omitempty"` // non-nil empty string explicitly restores provider default in chat mode
    YoloMode        *bool              `json:"yoloMode,omitempty"`
    Attachments     []PromptAttachment `json:"attachments,omitempty"`
    SubMode         string             `json:"subMode,omitempty"`
    FlowRef         string             `json:"flowRef,omitempty"`
    ChangeType      string             `json:"changeType,omitempty"`
    SourceDocID     string             `json:"sourceDocId,omitempty"`
}

type SkillSelection struct {
    Name   string `json:"name"`
    Path   string `json:"path,omitempty"`
    Source string `json:"source,omitempty"`
}

type PromptAttachment struct {
    ID           string `json:"id"`
    Kind         string `json:"kind"`
    OriginalName string `json:"originalName"`
    MimeType     string `json:"mimeType"`
    Data         string `json:"data"`
    SizeBytes    int64  `json:"sizeBytes"`
    Width        int    `json:"width,omitempty"`
    Height       int    `json:"height,omitempty"`
}

type TokenUsageSnapshot struct {
    Last               *TokenUsageBreakdown `json:"last,omitempty"`
    Total              *TokenUsageBreakdown `json:"total,omitempty"`
    ModelContextWindow *int64               `json:"modelContextWindow,omitempty"`
}

type TokenUsageBreakdown struct {
    CachedInputTokens     int64 `json:"cachedInputTokens"`
    InputTokens           int64 `json:"inputTokens"`
    OutputTokens          int64 `json:"outputTokens"`
    ReasoningOutputTokens int64 `json:"reasoningOutputTokens"`
    TotalTokens           int64 `json:"totalTokens"`
}

// ProviderEvent — discriminated by Type; decode flexibly (map + typed helpers OK for P-0)
type ProviderEvent struct {
    ID               string              `json:"id"`
    Type             string              `json:"type"`
    WorkflowRunID    string              `json:"workflowRunId"`
    ProviderSessionID string             `json:"providerSessionId"`
    ProviderKey      string              `json:"providerKey"`
    ProviderTurnID   string              `json:"providerTurnId,omitempty"`
    WorkflowStepRunID string             `json:"workflowStepRunId,omitempty"`
    Seq              int64               `json:"seq"`
    OccurredAt       string              `json:"occurredAt"`
    Text             string              `json:"text,omitempty"`
    FinalMessage     string              `json:"finalMessage,omitempty"`
    TokenUsage       *TokenUsageSnapshot `json:"tokenUsage,omitempty"`
    ToolName         string              `json:"toolName,omitempty"`
    Input            any                 `json:"input,omitempty"`
    Output           any                 `json:"output,omitempty"`
    Status           string              `json:"status,omitempty"`
    Path             string              `json:"path,omitempty"`
    ChangeType       string              `json:"changeType,omitempty"`
    ApprovalID       string              `json:"approvalId,omitempty"`
    Details          *ApprovalDetails    `json:"details,omitempty"`
    Decision         string              `json:"decision,omitempty"`
    QuestionID       string              `json:"questionId,omitempty"`
    Prompt           string              `json:"prompt,omitempty"`
    Options          []QuestionOption    `json:"options,omitempty"`
    MultiSelect      bool                `json:"multiSelect,omitempty"`
    Answer           []string            `json:"answer,omitempty"`
    Error            string              `json:"error,omitempty"`
    Recoverable      bool                `json:"recoverable,omitempty"`
    GateOptions      []string            `json:"gateOptions,omitempty"`
    AgentGraphSnapshot *AgentGraphSnapshot `json:"agentGraphSnapshot,omitempty"`
    AgentName        string              `json:"agentName,omitempty"`
    ChildRunID       string              `json:"childRunId,omitempty"`
}

type ApprovalDetails struct {
    Command   string                   `json:"command,omitempty"`
    Cwd       string                   `json:"cwd,omitempty"`
    Reason    string                   `json:"reason,omitempty"`
    Kind      string                   `json:"kind,omitempty"`
    Decisions []ApprovalDecisionOption `json:"decisions"`
}
type ApprovalDecisionOption struct {
    Value string `json:"value"`
    Label string `json:"label"`
}
type QuestionOption struct {
    Label       string `json:"label"`
    Description string `json:"description,omitempty"`
    Value       string `json:"value,omitempty"`
}
```

### 4.3 `internal/tui/client/client.go`

```go
type Client struct {
    BaseURL    string
    HTTPClient *http.Client
}

func New(baseURL string) *Client

func (c *Client) Health(ctx context.Context) (Health, error)
func (c *Client) ListProjects(ctx context.Context) ([]Project, error)
func (c *Client) ListProviders(ctx context.Context) ([]Provider, error) // GET /providers
func (c *Client) StartRun(ctx context.Context, in StartRunInput) (RunHandle, error)
func (c *Client) SendTurn(ctx context.Context, runID string, in TurnInput) (turnID string, err error)
// POST .../turns → {turnId}; do not stream body
func (c *Client) StreamEvents(ctx context.Context, runID string, afterSeq int64) (<-chan ProviderEvent, <-chan error)
func (c *Client) SendTurnEvents(ctx context.Context, runID string, afterSeq int64, in TurnInput) (<-chan ProviderEvent, <-chan error)
func (c *Client) Interrupt(ctx context.Context, runID string) error
func (c *Client) SubmitApproval(ctx context.Context, approvalID, decision string, remember bool) error
func (c *Client) AnswerQuestion(ctx context.Context, questionID string, choice any) error
func (c *Client) ListSkills(ctx context.Context, provider, cwd string) ([]ProviderSkill, error)
func (c *Client) ListProviderAccounts(ctx context.Context) ([]ProviderAccountSummary, error)
// Stubs OK if unused until later tasks, but prefer full methods now for A0 coverage
```

Error envelope: map non-2xx to typed `APIError{Code, Message, Status}` reading `{error:{code,message}}` or `{error:string}`.

### 4.4 `internal/tui/client/sse.go`

- `GET /client/workflow-runs/{runId}/events/stream?afterSeq=N`
- Parse SSE `data:` JSON lines (and/or NDJSON if runner uses that — **match desktop client**).
- Frames are separated by blank lines; ignore `id:`, `event:`, and `:` comment lines, parse the `data:` JSON, and tolerate CRLF.
- Forward `ProviderEvent`; close channels on ctx cancel / EOF.
- `SendTurnEvents`: record the pre-turn cursor, POST the turn, then filter events with a non-empty mismatched `providerTurnId`; stop only on matching `turn_completed`/`turn_failed`.
- Honor monotonic `seq` for reconnect (reconnect helper can wait until Task-287; P-0 just streams once).

### 4.5 `internal/tui/runnerboot/ensure.go`

```go
type EnsureOptions struct {
    BaseURL      string
    Port         int
    Workspace    string        // required resolved FlowPilot application root
    BinaryPath   string
    ReadyTimeout time.Duration // default 90s
    NoStart      bool
}

type EnsureResult struct {
    AlreadyRunning bool
    StartedByCLI   bool
    BaseURL        string
    LogPath        string
}

func EnsureRunner(ctx context.Context, opt EnsureOptions) (EnsureResult, error)
func WaitHealthy(ctx context.Context, baseURL string, timeout time.Duration) error
```

Algorithm:

0. Reuse inherited root flags; do not register chat-local `--workspace`, `--host`, or `--port`. Resolve runner workspace: inherited `--workspace` → `FLOWPILOT_WORKSPACE` → strict FlowPilot app-root marker walk; fail if unresolved. Never substitute selected `Project.Path`.
1. Resolve BaseURL: explicit `--runner-url` → `FLOWPILOT_RUNNER_URL` → inherited host/port.
2. If `Health` is valid, require `Health.Cwd` to match the resolved workspace unless `--runner-url` was explicit; then reuse.
3. If `NoStart` → error `runner_not_reachable`.
4. Resolve binary: `BinaryPath` test/advanced override → `os.Executable()`.
5. Invoke `<self> runner serve --workspace <root> --host 127.0.0.1 --port P` with cwd `<root>`; detach using build-tagged Unix/Windows helpers; redirect to `<root>/.flowpilot/cli-runner.log`; call `Process.Release`.
6. `WaitHealthy` polls every 300ms up to 90s (cold `go run` compatible).
7. Validate `status=="online"` and non-empty `runnerVersion`; a foreign JSON responder is not healthy.
8. If the child exits or loses a concurrent bind race, recheck health and reuse the matching winner. Detect bind/address-in-use in the log and fail fast for a foreign listener.
9. Do not retain a process handle and do not kill the runner on chat exit.

### 4.6 `internal/cli/chat.go`

```go
// chat-local flags: --runner-url, --no-start-runner, --project (optional P-0)
// inherited root flags: --workspace, --host, --port
// 1) EnsureRunner
// 2) client.Health + ListProjects (print JSON or one-line OK for smoke)
// 3) exit 0
// Bubble Tea deferred to Task-279
```

```go
rootCmd.AddCommand(newChatCommand()) // directly calls tui/app.Run; no subprocess delegate
```

### 4.7 go.mod deps (P-0)

- stdlib only for client/boot; Bubble Tea lands in Task-279 in the same module.

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/{client,runnerboot,config}/**`, `internal/cli/chat.go`, additive root registration
- modules: existing `flowpilot-runner`
- routes: consume existing `/health`, `/providers`, `/client/*` only
- tables: none

## 6. Acceptance Check

- [ ] A0.1–A0.19 client/runnerboot tests green (`httptest` + stub self executable)
- [ ] `go test ./internal/tui/... ./internal/cli/...` in `apps/local-runner`
- [ ] Manual M3c/M3d/M3e (or defer M3c live to after binary install — unit spawn stub required)
- [ ] package-boundary check: zero imports of `flowpilot-runner/internal/runner` from `internal/tui`
- [ ] No runner core diffs

## 7. Out of Scope

- Bubble Tea UI, slash commands, statusline, flow launch, approvals UI
- `--stop-runner-on-exit`
- Image/skill UX

## 8. Completion Notes

- result: pending
- follow-ups: Task-279
- upstream docs updated: mark A0.* in CP-56-Test-Steps when done
