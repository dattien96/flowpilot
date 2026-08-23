# CA-580: Fix TUI gate message showing empty Options on reprompt/warn

## What

- `apps/local-runner/internal/tui/app/app.go:1606` — `flow_gate_violation` handler now calls `buildGateMessage(status, error, opts, regressed)` with status-aware rendering.
- `apps/local-runner/internal/tui/app/app.go:4641` — `buildGateMessage` varies by `status`:
  - `block` + has `GateOptions` → `[GATE] Flow gate blocked.` + `Regressed tests:` + `Options: ...` (legacy decision card, armed gate).
  - `reprompt` → `[GATE] auto-reprompt — <error>` (no `Options:` line; no gate arm).
  - `warn` → `[GATE] warn — <error>` (no `Options:`).
  - `block` without options → `[GATE] blocked — <error>` (no `Options:`).
  - Empty `status` + has opts (legacy tests without status) still renders the block card for view compat.
  - Regressed tests line only when present; never emits `Options:` when `opts` is empty.
- `apps/local-runner/internal/tui/app/ca545_gate_message_test.go` (new, additive) — 9 cases × claude/codex/grok parity: reprompt/warn/block-no-opts have no `Options:`, block-with-opts still shows it, case-insensitive status, and event-flow integration (gate not armed for non-blocking).

## Why

- run-204658 (C1) and run-103672 class: runner emits `flow_gate_violation` for every `reprompt`/`warn` (and for `block` without `r-reg` options) with empty `GateOptions`. CA-536 stopped arming the composer for those verdicts, but the TUI still appended `buildGateMessage`'s unconditional `  Options:` line, so chat showed:
  ```
  [GATE] Flow gate triggered.
    Options:
  ```
  and appeared to wait for a decision while the runner was actually auto-reprompting (system prompt hidden by `liveTurnStartedDisplayPrompt` by design). User reported it as "k làm gì cả".

## Tests

- `go vet ./internal/tui/app` clean.
- `go test ./internal/tui/app -run TestGate -count=1` 17/17 pass (CA-536 + CA-545).
- `go test ./internal/tui/app -count=1` green (13.4s) including `TestA7_11_GateViolation_ShowsOptionsInView` / `TestA9_4_GateViolation_ShowsOptionsInView` (legacy empty-status + opts still renders).

## Residual

- Runner still hides the `[SYSTEM_PROMPT] The flow gate is asking you to...` remediation prompt from the chat bubble (intentional, `isSystemPrompt` → `liveTurnStartedDisplayPrompt` returns `""`). This fix only cleans the user-visible GATE line.
- Desktop `timelineReducer` / `store` already gate modal on `block|warn + !historyReplaying`; no desktop change needed.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: fix TUI gate message showing empty Options on reprompt/warn by making buildGateMessage status-aware and omitting Options when empty
# --->8---
