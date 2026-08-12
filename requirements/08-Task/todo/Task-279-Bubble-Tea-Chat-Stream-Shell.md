# Task-279: Bubble Tea Chat Stream Shell (CP-56 P-1)

## Metadata

- Document ID: `Task-279`
- Title: `Bubble Tea Chat Stream Shell`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-1), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-278](./Task-278-TUI-Go-Client-Ensure-Runner-And-Chat-Entry.md)
- Child Documents: `none`
- Related Documents: [Task-280](./Task-280-TUI-Session-Controls-Provider-Model-Reasoning-Yolo.md)
- Replaces: `None`
- Tags: `cli-tui, bubbletea, chat`

## AI Quick View

### Summary

- Add Bubble Tea app: viewport timeline + textarea input + basic status placeholder.
- Normal chat only: ensure runner → startRun(`normal_chat`) → sendTurn → stream events → render deltas.
- Esc/`/stop` → Interrupt. No slash skills/flow yet (stub router that ignores unknown).

### Current Ask

Implement P-1 on top of Task-278 client/boot.

### Key Decisions

- `T-1` Elm architecture: `Model` / `Update` / `View`; stream via `tea.Cmd` messages.
- `T-2` Optimistic user prompt line + thinking row; finalize on `message_completed` / `turn_completed`.
- `T-3` One turn at a time; block send while streaming (simple flag).
- `T-4` Every stream command waits for exactly one event; `Update` re-arms it until the matching turn terminal event, preventing a one-event dead stream.
- `T-5` Project resolution order is explicit flag/id/name → exact/ancestor cwd match → one-project auto-select → startup picker. Never silently choose the first of multiple unrelated projects.
- `T-6` Generate one non-`durable-` `Idempotency-Key` per logical turn. Retry only pre-mint transient API codes `turn_in_progress`, `gate_in_progress`, and `hub_parked` up to 6 times at 700ms using that same key.

### Constraints

- Depends on Task-278. No runner core edits. Chat mode only.

### Open Questions

- None.

### Source Refs

- CP-56 P-1, D-12; Test-Steps A1.1–A1.9; M1, M15.

---

## 1. Goal

Interactive REPL: type prompt → stream assistant → stop.

## 2. Parent Links

- coding plan: CP-56 P-1
- tech design: 04-02 start-run, turn, interrupt, and SSE contracts
- system spec: n/a; client-only shell
- specific upstream ids: CP-56 D-12, D-14

## 3. Trigger

HTTP client ready; need human-usable shell.

## 4. Exact Change

### 4.1 Layout

```text
apps/local-runner/internal/tui/
  app/
    app.go            # Run(cfg) tea.NewProgram
    model.go
    update.go
    view.go
    msgs.go
    render_events.go
    session.go        # runID, stepID, afterSeq, streaming bool
    app_test.go
    render_events_test.go
apps/local-runner/internal/cli/chat.go  # call app.Run after EnsureRunner
```

### 4.2 Deps

```text
github.com/charmbracelet/bubbletea
github.com/charmbracelet/bubbles/textarea
github.com/charmbracelet/bubbles/viewport
github.com/charmbracelet/lipgloss
```

### 4.3 Model

```go
type TimelineItem struct {
    Kind string // prompt|assistant|thinking|tool|file|system|error
    ID   string
    Text string
}

type Model struct {
    cfg        Config
    client     *client.Client
    session    SessionState
    timeline   []TimelineItem
    input      textarea.Model
    viewport   viewport.Model
    width, height int
    err        error
    ready      bool
}

type SessionState struct {
    ProjectID   string
    ProviderKey string
    Model       string
    Cwd         string
    RunID       string
    StepID      string
    AfterSeq    int64
    Streaming   bool
}
```

### 4.4 Messages (`msgs.go`)

```go
type streamEventMsg struct{ Ev client.ProviderEvent }
type streamErrMsg struct{ Err error }
type streamDoneMsg struct{}
type submitPromptMsg struct{ Text string }
type windowSizeMsg tea.WindowSizeMsg
type projectResolvedMsg struct{ Project client.Project }
```

### 4.5 Update flow

1. User Enter on textarea (non-slash) → if `!Streaming` → append prompt+thinking → `startOrContinueCmd`.
2. `startOrContinueCmd`:
   - if `RunID==""` → `StartRun{chatMode:normal_chat, providerKey, model, cwd, projectId}` → store `RunID`/`StepID`
   - `SendTurnEvents{stepId, prompt, idempotencyKey}` captures `turnId` and emits only this turn's relevant events; retry only the three known pre-mint transient codes
   - schedule `waitStreamEventCmd`; after each event, `Update` schedules the next wait until matching terminal
3. `render_events.MapEvent`:
   - `message_delta` → append/update streaming assistant line
   - `message_completed` / `turn_completed` → finalize; clear thinking
   - `tool_*` / `file_changed` → compact rows
   - `turn_failed` → error row; `Streaming=false`
4. Esc while streaming → `Interrupt` Cmd; render `cancelling…` and wait for the authoritative terminal event.
5. `/stop` same as Esc; `/quit` / Ctrl+C → quit program.

### 4.6 View

```text
┌ viewport (timeline) ─────────────────┐
│ user / assistant / tools             │
├ status placeholder (Task-284) ───────┤
│ idle | project | provider            │
├ textarea ────────────────────────────┤
│ > _                                  │
└──────────────────────────────────────┘
```

### 4.7 Config flags (main)

`--project` optional ID/name, `--provider` optional seed, `--model` optional, `--cwd` defaults to process cwd. Before first send, resolve the project by `T-5`; show a Bubble Tea list picker when ambiguous. Provider selection is completed by Task-280 before sending.

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/**`, additive `internal/cli/chat.go` wiring, matching tests
- modules: existing `flowpilot-runner` plus Bubble Tea/Bubbles/Lip Gloss dependencies
- routes: existing start-run, turn, interrupt, and SSE routes
- tables: none

## 6. Acceptance Check

- [ ] A1.1–A1.9 green, including stream re-arm, project resolution, and transient retry with stable idempotency key
- [ ] Manual M1: stream visible; stop works
- [ ] No import of runner internals

## 7. Out of Scope

- YOLO/model slash (280), skills (281), images (282), flow (283), real statusline (284), agents (285), gates UI (286)

## 8. Completion Notes

- result: pending
- follow-ups: Task-280
- upstream docs updated: CP-56-Test-Steps evidence when complete
