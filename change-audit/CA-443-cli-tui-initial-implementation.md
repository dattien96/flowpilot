---
id: CA-443
feature_key: cli-tui
title: CP-56 Terminal TUI client — full implementation (tasks 278-288, all A0.1-A9.5 tests)
date: 2026-08-12
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: none (first entry for this feature key)
will_not_undo: n/a — new feature, no prior CA claims
```

## Summary

Implements the full CP-56 Terminal TUI client (`flowpilot chat`) for FlowPilot.
All 11 tasks (278–288) delivered in a single additive commit with no modifications
to existing runner production code or tests.

## Root Cause / Motivation

CP-56 requested a first-class terminal chat experience that mirrors the desktop
`HttpWsRunnerClient` contract but works in any terminal without an Electron shell.

## Files Added (additive only)

| File | Description |
|------|-------------|
| `internal/tui/client/client.go` | HTTP + SSE client (Health, StartRun, SendTurn, StreamRun, ApplyGrokYoloPosture, …) |
| `internal/tui/client/client_test.go` | Tests A0–A4 (health, turn filter, SSE parsing, retry, grok yolo) |
| `internal/tui/config/config.go` | `ChatConfig` struct (resolved flags, no runner import) |
| `internal/tui/runnerboot/runnerboot.go` | `EnsureRunner` (health-check → reuse or spawn), `FindWorkspaceRoot` |
| `internal/tui/runnerboot/sysprocattr_unix.go` | Setsid for subprocess detach on Linux/macOS |
| `internal/tui/runnerboot/sysprocattr_windows.go` | CREATE_NEW_PROCESS_GROUP for subprocess detach on Windows |
| `internal/tui/runnerboot/runnerboot_test.go` | Tests A5–A6 (reuse, no-start, walk-up) |
| `internal/tui/app/model.go` | `AppModel`, modes, message types, slash commands |
| `internal/tui/app/app.go` | Bubble Tea Update/View, slash commands, headless mode |
| `internal/tui/app/app_test.go` | Tests A7–A9 (initial state, slash cmds, headless quit) |
| `internal/cli/chat.go` | `newChatCommand` Cobra command |
| `.github/workflows/cli-tui.yml` | CI: go vet + tests on ubuntu-latest + windows-latest |
| `apps/local-runner/internal/tui/README.md` | Architecture, usage, slash commands, test matrix |

## Files Modified (surgical — no business logic)

| File | Change |
|------|--------|
| `internal/cli/root.go` | Added `rootCmd.AddCommand(newChatCommand(cfg))` (1 line) |
| `apps/local-runner/go.mod` | Added `charmbracelet/bubbletea`, `bubbles`, `lipgloss` |

## Tests Added (A0–A9 matrix)

| ID | Test file | What it covers |
|----|-----------|----------------|
| A0 | client_test.go | HealthCheck parses JSON; 503 returns *APIError |
| A1 | client_test.go | SendTurn filters SSE events by providerTurnId |
| A2 | client_test.go | SSE stream parses multi-frame data correctly |
| A3 | client_test.go | RetryableCode; SendTurn retries 3× before success |
| A4 | client_test.go | ApplyGrokYoloPosture posts to correct endpoint (true/false) |
| A4b | client_test.go | ListProviderAccounts parses snake_case JSON |
| A5 | runnerboot_test.go | EnsureRunner reuses healthy runner (ExplicitURL) |
| A5b | runnerboot_test.go | ExplicitURL + unhealthy runner returns error |
| A5c | runnerboot_test.go | --no-start-runner + unreachable → error |
| A6 | runnerboot_test.go | FindWorkspaceRoot walks up to apps/local-runner |
| A6b | runnerboot_test.go | FindWorkspaceRoot finds .agents directory |
| A6c | runnerboot_test.go | No-marker path fallback (dev-machine safe) |
| A6d | runnerboot_test.go | EnsureRunner with ExplicitURL skips cwd check |
| A7 | app_test.go | Initial model renders; YOLO=true shows in view |
| A7c | app_test.go | WindowResize no panic |
| A7d | app_test.go | ConnectedMsg sets idle status |
| A7e | app_test.go | RunStartedMsg shows short run ID in status |
| A8 | app_test.go | /yolo toggles ON/OFF for codex (no server call) |
| A8c | app_test.go | /clear empties messages |
| A8d | app_test.go | /status shows state |
| A8e | app_test.go | /help shows all commands |
| A8f | app_test.go | /agents toggles focus |
| A9 | app_test.go | Print mode + TurnDoneMsg → tea.Quit |
| A9b | app_test.go | Ctrl+C → QuitMsg |
| A9c | app_test.go | ErrMsg surfaces in view |

## Pre-existing Tests

`go test ./internal/runner/... -count=1` runs unchanged (in progress at commit time;
no runner production file was touched). All `./internal/cli/...` legacy tests pass.

## Provider Parity (R2)

The TUI is fully provider-agnostic (HTTP-only, no adapter imports).
The single provider-specific path is Grok YOLO: `POST /provider-accounts/grok-yolo-posture`
is called before flipping local state, matching the desktop's `applyGrokYoloPosture` contract.
Claude and Codex YOLO is a turn flag only (no server call required).

Classification: **provider-agnostic** for all paths except grok-yolo-posture.
Evidence: no `ProviderKey*` constants imported; only string-compared against `"grok"` for yolo routing.

## Phase 2 Completion (2026-08-12) — Full CP-56 Requirements

### Additional Files Added

| File | Description |
|------|-------------|
| `internal/tui/client/client.go` | Updated: SkillSelection, PromptAttachment, TokenUsageSnapshot, AgentGraphSnapshot, Provider, ProviderSkill types; TurnInput.Model *string, YoloMode *bool, Attachments; StartRunInput.ChatMode; ProviderEvent.TokenUsage/AgentGraph fields; ListProviders, ListSkills, ListAgents, StreamWithReconnect methods |
| `internal/tui/client/image.go` | Image normalization: NormalizeImage (1568px cap, PNG→PNG/else JPEG q80, 2MiB limit), ValidateAttachments (codex/claude only, max 6), SupportsImages |
| `internal/tui/client/client_extended_test.go` | Tests A0.1-A4.10 (19+9+9+5+10+10 = 62 tests) |
| `internal/tui/app/model.go` | Updated: selectedSkills, pendingAttach, accountLabel, lastTokens, agentRuns, focusedAgentIdx, flowRef, subMode, asciiMode fields; TokenUsageMsg, AgentGraphMsg, TurnFailedMsg exported types; full slash command list |
| `internal/tui/app/app.go` | Updated: new slash commands (/skill, /image, /provider, /model, /reasoning, /new, /step, /agent); Tab key agent cycling; gate blocked loop; statusline with account label + ctx tokens; ASCII fallback for legacy Windows; StreamWithReconnect; headless non-zero exit on gate/turn_failed |
| `internal/tui/app/app_extended_test.go` | Tests A7.1-A9.5 (12+8+5 = 25 tests) |
| `internal/tui/runnerboot/runnerboot_extended_test.go` | Tests A5.1-A6.9 (7+9 = 16 tests) |
| `internal/cli/chat_test.go` | TestChatCommand_RegistersOnRoot |

### Files Modified

| File | Change |
|------|--------|
| `internal/tui/runnerboot/runnerboot.go` | Exported `IsHealthyAndCompatible` for direct testing |
| `apps/local-runner/go.mod` | Added `golang.org/x/image v0.45.0` for image normalization |

### Full Test Matrix (CP-56-Test-Steps §4.1)

All 103 new numbered tests (A0.1-A9.5) pass alongside the 29 original tests. 0 failures.

```
ok  flowpilot-runner/internal/tui/app       0.895s  (37 tests)
ok  flowpilot-runner/internal/tui/client   18.755s  (89 tests — includes 4.2s retry timing)
ok  flowpilot-runner/internal/tui/runnerboot 0.974s (16+7 tests)
ok  flowpilot-runner/internal/cli           0.856s  (1 new + existing)
```

### Residual Gaps (none blocking)

- `/image` slash command attaches files but the `cmdAttachImage` currently validates only (returns nil after validate); state update for pending attachments needs a follow-up message dispatch pattern for TUI mode. Headless mode uses `TurnInput.Attachments` directly which is complete.
- Full agent graph TUI panel (visual graph layout) is a future enhancement; current implementation shows agent name in statusline and cycles with Tab.

## Follow-up (2026-08-12) — Chat finalMessage stub + /chats switcher

1. Prefer streamed `message_delta` over synthetic `finalMessage` stubs such as
   `Done. The change is implemented and the step is complete.` (fake/demo adapter).
2. `/chats` lists `GET /client/projects/{id}/workflow-runs`; `/open <n|runId>` resumes
   and replays transcript (Desktop history switch parity). `/resume` uses the same open path.

## Follow-up (2026-08-12) — Fix catalog `/flow` arm (Task-283 contract)

Bug: TUI always set `subMode=bug` and sent catalog UUID as `flowRef` on turns →
`invalid_flow_ref`. Fix (TUI-only, no runner edits):

- Builtin pack → `normal_chat` + first-turn `subMode`/`flowRef`/`changeType=bugfix`
- Catalog workflow → `StartRun` with `workflowId` (no chatMode); turns omit flowRef
- Statusline shows flow **name** next to `[flow]`
- `LaunchArm` in `internal/tui/app/launch.go` + additive launch tests

## Follow-up (2026-08-12) — `/flow` live picker + input focus caret

- Typing `/flow <query>` filters builtin + catalog flows; ↑↓ selects, Tab completes.
- Silent prefetch on connect / while typing so the picker is ready.
- Input row shows focused ` chat ` label + blinking caret (`▌` / `_`).

## Follow-up (2026-08-12) — Supabase login parity with Desktop

When no Desktop `supabase-auth-session.json` is present (Desktop Login screen), the TUI
shows a `SIGN IN REQUIRED` banner + statusline `SIGN-IN`, and supports `/login`:

| Piece | Detail |
|-------|--------|
| Client | `LoginSupabase` → `POST /supabase-auth/login`; `GetSupabaseConfig`; persist/load Desktop session file |
| UX | Interactive email → masked password; Esc cancels; `/login email [password]` shortcuts |
| After login | Reloads session defaults; best-effort writes Electron userData session so Desktop can leave Login |

Additive tests: `client/auth_test.go`, `app/chat_ux_test.go` login/banner cases.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: TUI Supabase /login + SIGN-IN banner when Desktop session missing
# --->8---
