# Task-287: TUI Resume, Headless, And Session Reset (CP-56 P-8)

## Metadata

- Document ID: `Task-287`
- Title: `TUI Resume Headless And Session Reset`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui, chat-history`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-8), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-286](./Task-286-TUI-Approval-And-Question-Gates.md)
- Child Documents: `none`
- Related Documents: `GET/POST .../resume`, desktop history open
- Replaces: `None`
- Tags: `cli-tui, resume, headless`

## AI Quick View

### Summary

- `/new` clears session; `--resume <runId>` cold-opens + replays stream from seq 0.
- `-p/--print <prompt>` non-TTY one-shot (no Bubble Tea): ensure runner → turn → print final → exit code.
- Optional glamour for assistant markdown (soft).

### Current Ask

Implement P-8 daily-driver completeness.

### Key Decisions

- `T-1` Headless never starts Bubble Tea; still uses EnsureRunner unless `--no-start-runner`.
- `T-2` Headless exit `0` on turn_completed; non-zero on turn_failed / ensure failure / timeout.
- `T-3` Resume uses `GetRun`/`ResumeRun` then `StreamEvents(afterSeq=0)` until `lastEventSeq` caught up, then interactive.
- `T-4` `/new` already partially in 280 — complete: clear timeline, arms, pending skills/images, focus.
- `T-5` Replay must not stop at the first historical `turn_completed`; it stops only after `seq >= RunHandle.lastEventSeq`, then follows live from that cursor.
- `T-6` Headless cannot answer interactive gates. On permission/question/flow-gate/blocked-loop input, interrupt the run and exit non-zero with guidance; never wait until timeout.
- `T-7` Unexpected SSE EOF reconnects with the last monotonic `afterSeq` using bounded backoff; cancellation and terminal events stop reconnecting.

### Constraints

- No runner edits.

### Open Questions

- None.

### Source Refs

- CP-56 P-8, D-16; A8.1–A8.8; M9, M10, M16.

---

## 1. Goal

Resume chats, reset sessions, and script one-shot prompts from the CLI.

## 2. Parent Links

- coding plan: CP-56 P-8
- tech design: existing resume, history, run snapshot, and SSE cursor contracts
- system spec: chat history/resume behavior already owned by runner
- specific upstream ids: CP-56 A8.1–A8.8

## 3. Trigger

Tasks 278–286 provide the full interactive surface; this slice adds cold replay, reconnect, reset, and non-TTY behavior.

## 4. Exact Change

### 4.1 Files

```text
internal/tui/app/resume.go
internal/tui/app/headless.go
internal/tui/app/headless_test.go
internal/tui/app/resume_test.go
internal/cli/chat.go   # branch: if -p → RunHeadless; if --resume → seed model
```

### 4.2 Code

```go
func (c *Client) GetRun(ctx context.Context, runID string) (RunSnapshot, error)
func (c *Client) ResumeRun(ctx context.Context, runID string) (RunHandle, error)
func (c *Client) ListRunHistory(ctx context.Context, projectID string) ([]RunHistoryItem, error)

func RunHeadless(ctx context.Context, cfg Config, prompt string) (exitCode int, err error)
// EnsureRunner → StartRun or Resume → SendTurn → consume StreamEvents until terminal → fmt.Stdout final message

func (m Model) ResetSession() Model // /new
func ReplayHistoryCmd(client *client.Client, runID string, untilSeq int64) tea.Cmd
func (c *Client) StreamWithReconnect(ctx context.Context, runID string, afterSeq int64) (<-chan ProviderEvent, <-chan error)
```

### 4.3 Flags

```text
-p, --print string
--resume string
--timeout duration   # headless max wait
```

### 4.4 Headless algorithm

```text
1. EnsureRunner (unless NoStart)
2. StartRun(normal_chat) with controls from flags, or ResumeRun when `--resume` is supplied
3. SendTurn(prompt, skills/images if flags added later — v1 prompt only OK)
4. Read events until this turn's terminal event, an interactive gate, or ctx timeout
5. On a gate: Interrupt, print actionable guidance to stderr, return the documented non-zero gate exit code
6. Print finalMessage/text; exit
```

### 4.5 Resume/replay algorithm

1. Resolve the selected project and locate the run in its existing history to restore chat-vs-workflow metadata.
2. `ResumeRun(runID)` returns authoritative `stepId`, status, and `lastEventSeq`.
3. Replay from seq 0 through `lastEventSeq` even when older terminal events occur.
4. Seed pending approval/question from `GetRun` if the replay stream has no new frame.
5. Continue a live stream from the replay cursor; follow-up sends use the resumed `stepId`.

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/{resume,headless}.go`, `internal/tui/client` replay/reconnect helpers, `internal/cli/chat.go`, additive tests
- modules: existing TUI packages
- routes: existing resume, get-run, history, interrupt, and event stream routes
- tables: none

## 6. Acceptance Check

- [ ] A8.1–A8.8 green, including multi-turn replay cursor, reconnect, headless gate exit, and resumed follow-up step ID
- [ ] Manual M9, M10, M16

## 7. Out of Scope

- Full project history browser; Drive restore UI

## 8. Completion Notes

- result: pending
- follow-ups: Task-288
- upstream docs updated: CP-56-Test-Steps evidence and CP-56 completion state when done
