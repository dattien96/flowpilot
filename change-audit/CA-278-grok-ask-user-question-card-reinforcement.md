# CA-278 — Grok Ask-User Steers Onto FlowPilot QuestionCard Path

## Scope

Closes the desktop gap reported after CP-46 Grok adapter work: when the user asks Grok (inside FlowPilot) to pick among structured options (e.g. Python / TypeScript / Go), Codex shows the Question card while Grok fell back to plain text ("The interactive picker isn't available in this environment"). Implements Task-209 / GR-06 primary path (MCP `ask_user` + prompt reinforcement); does not claim full live DOD-1 until desktop E2E is re-confirmed by the user.

## Root Cause

- Live Grok `promptPrep` only ran `injectSelectedSkills` and dropped any ask-user guidance (unlike Claude/Codex, which append reinforcement).
- Grok's native `ask_user_question` runs inside `grok agent stdio` with no TTY picker → tool fails → model prints a numbered list instead of calling FlowPilot MCP `ask_user` → no `user_question_required` / QuestionCard.
- `grok agent` has no `--disallowed-tools` (headless-only flag on `grok -p`); Claude-style hard disable is not available on the ACP process.

## Changes

- `apps/local-runner/internal/runner/grok_adapter.go`
  - Added `grokAskUserReinforcement` (act-first bias + explicit FlowPilot MCP `ask_user` on server `flowpilot`; forbid native `ask_user_question` / plain-text lists).
  - Default `preparePrompt` now appends the reinforcement when no `promptPrep` override is set.
- `apps/local-runner/internal/runner/provider_registry.go`
  - Live Grok factory `promptPrep` now appends `grokAskUserReinforcement` after skill injection (mirrors Claude/Codex).
- `apps/local-runner/internal/runner/grok_mcp_test.go` + `grok_adapter_test.go`
  - Tests: default + registry-style prep, wire prompt includes reinforcement, MCP `tools/call ask_user` → `bridge.AskQuestion` round-trip; `fakeGrokBridge` captures question args.
- MCP registration / `claudeMCPServer` / QuestionCard UI: **unchanged** (reuse only).

## Verification

- `go test ./internal/runner -run 'Grok.*(AskUser|Mcp|Prompt|Native)'` — PASS.
- Native-tool disable via launch flag: **not applied** (verified unsupported on `grok agent --help`; headless-only per Grok docs).
- Live desktop E2E (QuestionCard for language pick): **confirmed by user 2026-07-10** after runner restart — structured form appears; Task-209 DOD-1 closed.

## Follow-ups

- Task-209 remaining: DOD-2 spawn_agent E2E, DOD-4/5/7/8/9.
- If a future Grok version ignores reinforcement and prefers native `ask_user_question`, add a PreToolUse / agent-profile denylist once a proven ACP-side lever exists (Task-221 adjacent).

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: Task-209
change_type: feature
summary: Grok ask_user QuestionCard parity — prompt reinforcement steers model onto FlowPilot MCP ask_user (not native ask_user_question) so desktop options form can render
# --->8---
