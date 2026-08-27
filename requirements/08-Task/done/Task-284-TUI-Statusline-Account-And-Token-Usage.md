# Task-284: TUI Statusline — Account And Token Usage (CP-56 P-5)

## Metadata

- Document ID: `Task-284`
- Title: `TUI Statusline Account And Token Usage`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui, token-usage`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-5), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-283](./Task-283-TUI-Flow-And-Step-Slash-Launch.md)
- Child Documents: `none`
- Related Documents: Desktop `ChatInput.usageSummaryLine`, `ProviderAccountsPanel`
- Replaces: `None`
- Tags: `cli-tui, statusline, token-usage`

## AI Quick View

### Summary

- Replace status placeholder with always-on bottom bar: active account, provider/model/reasoning/yolo, mode, run status, token/context usage.
- Consume `token_usage_updated` events; refresh accounts via `ListProviderAccounts`.

### Current Ask

Implement P-5 statusline (agents column stubbed empty until Task-285).

### Key Decisions

- `T-1` Format usage like desktop: context used/window, last turn tokens, in/out when present.
- `T-2` Truncate gracefully when width < content (lipgloss / rune count).
- `T-3` Pick account where `providerKey` matches + `isActive` (fallback first matching provider).
- `T-4` Refresh accounts after each terminal event as well as provider changes so runner-side account rotation cannot leave a stale footer.
- `T-5` If an event omits `modelContextWindow`, use the selected model's context metadata already loaded from `GET /providers`.

### Constraints

- No runner edits. Agent strip filled in Task-285.

### Open Questions

- None.

### Source Refs

- CP-56 P-5, D-6; A5.1–A5.7.

---

## 1. Goal

Desktop-parity account + token visibility in the TUI footer.

## 2. Parent Links

- coding plan: CP-56 P-5, D-6
- tech design: existing provider-account and token event DTOs
- system spec: n/a; client display delta
- specific upstream ids: CP-56 A5.1–A5.7

## 3. Trigger

Task-283 completes both launch modes; the footer can now project shared run/account/usage state.

## 4. Exact Change

### 4.1 Files

```text
internal/tui/app/status.go
internal/tui/app/status_test.go
```

### 4.2 Code

```go
type StatusModel struct {
    AccountLabel  string
    Provider      string
    Model         string
    Reasoning     string
    Yolo          bool
    Mode          Mode
    RunStatus     string // wire status only; local `Streaming` remains separate presentation state
    Usage         *client.TokenUsageSnapshot
    SkillsPending []string
    ImagesPending []string
    Agents        []AgentStatusLine // empty until 285
    FocusRunID    string
}

func FormatUsageLine(u *client.TokenUsageSnapshot) string
func (s *StatusModel) RefreshAccount(accounts []client.ProviderAccountSummary, provider string)
func (s *StatusModel) ApplyTokenUsage(u client.TokenUsageSnapshot)
func (s *StatusModel) ApplyControls(c SessionControls, mode Mode, runStatus string)
func (s StatusModel) View(width int) string
```

### 4.3 Wire

- On `token_usage_updated` in `MapEvent` / Update → `ApplyTokenUsage`
- On Init, `/provider` change, and matching turn terminal → `ListProviderAccounts` + `RefreshAccount`
- `View()`: viewport / status / input — status uses full width lipgloss bar

### 4.4 Account type (client)

```go
type ProviderAccountSummary struct {
    ID           string `json:"id"`
    ProviderKey  string `json:"provider_key"` // match actual JSON from /client/provider-accounts
    DisplayLabel string `json:"display_label"`
    IsActive     bool   `json:"is_active"`
    // … map fields as returned by runner (snake_case per cli provider_account JSON)
}
```

**Verify** against `buildProviderAccountSummaryResponses` / desktop mapper — use exact tags from live JSON.

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/status.go`, additive tests
- modules: existing TUI packages
- routes: existing `GET /client/provider-accounts`; events stream
- tables: none

## 6. Acceptance Check

- [ ] A5.1–A5.7 green, including exact snake_case account DTO decoding, account refresh after terminal, and context-window fallback
- [ ] Manual: after a turn, status shows context/last tokens + account label

## 7. Out of Scope

- Agent cycling UI (285); gate prompts (286)

## 8. Completion Notes

- result: pending
- follow-ups: Task-285
- upstream docs updated: CP-56-Test-Steps evidence when complete
