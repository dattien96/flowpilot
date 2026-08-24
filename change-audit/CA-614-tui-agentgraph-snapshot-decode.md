# CA-614: TUI decodes agentGraphSnapshot and shows blocked chips (run-127174)

## What

run-127174 đã escalate `blocked: escalate` và step chuyển `audit WAITING_USER_APPROVAL`, nhưng TUI không hiện `[Continue]/[Stop]`, `Thinking 3m17s` + `workIsLive() == true`. Do SSE `agent_graph_updated` dùng field `agentGraphSnapshot` (desktop-aligned) trong khi TUI chỉ decode `agentGraph`.

## Why

- `provider_event.go:165` emit `json:"agentGraphSnapshot"`; `tui/client/client.go:443` chỉ có `json:"agentGraph"` → `ev.EffectiveAgentGraph() == nil` trên live stream.
- `app.go:1746 handleEvent agent_graph_updated` gọi `ev.AgentGraph` trực tiếp → `applyAgentGraph` không chạy → `flowLoopStatus` không thành `blocked` → `renderBlockedBar` không vẽ, `workIsLive` qua `shouldPollStepsRuntime` vẫn true.
- Bolt: nếu SSE miss, poll `StepsRuntimeMsg` sau đó vẫn thấy `WAITING_USER_APPROVAL` nhưng không hydrate graph.

## Fix

- `apps/local-runner/internal/tui/client/client.go:442` thêm field `AgentGraphSnapshot json:"agentGraphSnapshot"` + helper `EffectiveAgentGraph() *AgentGraphSnapshot` ưu tiên `AgentGraph` rồi `AgentGraphSnapshot` (tương thích 2 chiều). `apps/local-runner/internal/runner/provider_event.go:165` thêm cả `AgentGraph` + `AgentGraphSnapshot` để decode payload cũ/mới.
- `apps/local-runner/internal/tui/app/app.go:1746` `handleEvent` dùng `ev.EffectiveAgentGraph()`.
- `apps/local-runner/internal/tui/app/app.go:1326` `StepsRuntimeMsg`: nếu có step `WAITING_USER_APPROVAL` và `!flowLoopBlocked() && !flowHasActiveAgents()` thì `cmdHydrateAgentGraph` (fallback khi SSE miss).
- Will not undo: BUG-231 blocked chips, CA-587 WAITING stamp, CA-610/611/612 paste/mouse.

## Tests

- New `tui/client/agent_graph_alias_test.go` (additive):
  - `TestProviderEvent_AgentGraphAliasSnapshot` — payload `agentGraphSnapshot` blocked → `EffectiveAgentGraph()` ok; legacy `agentGraph` vẫn ok; cả hai → legacy wins.
- New `tui/app/run127174_sse_blocked_chips_test.go` (additive, claude/codex/grok):
  - `TestRun127174_SSEAgentGraphSnapshotShowsBlockedChips` — SSE snapshot blocked+escalate → `flowLoopStatus=blocked`, banner warn, view có `[Continue]` `[Stop]`, `workIsLive()==false`
  - `TestRun127174_LegacyAgentGraphStillShowsBlocked` — payload cũ vẫn blocked
  - `TestRun127174_WaitingStepsHydratesBlockedGraph` — WAITING poll queues hydrate cmds
- Old suite green: `go vet ./internal/tui/client ./internal/tui/app`, `go test ./internal/tui/app -count=1` (19s), `go test ./internal/tui/client -count=1` pass. Không sửa test cũ.

## Provider parity

Agnostic + 3-provider matrix: chip render/click tests lặp `claude/codex/grok` như `tui_blocked_action_chips_test.go`; `EffectiveAgentGraph` không nhánh ProviderKey (grep 0 hit trên `tui/client/client.go` path đổi).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: decode agentGraphSnapshot and show blocked Continue/Stop chips (run-127174)
# --->8---
