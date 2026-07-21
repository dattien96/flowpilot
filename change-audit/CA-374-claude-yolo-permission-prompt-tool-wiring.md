# CA-374: Claude flow coding child under YOLO no longer fails every gated tool call

## Summary

Fixed BUG-296: a Claude coder child running inside a flow-engine workflow with YOLO=true (V9-21 `forceShellBridge`, which keeps `ClaudePermissionMode="default"` while `RunnerAutoApprove=true` so the git-commit denylist stays reachable) launched Claude gated with no `--permission-prompt-tool` configured, because `claudeArgs` attached that flag based on `!posture.RunnerAutoApprove` — false in this combination. Headless Claude (no TTY) then failed every gated tool call (Write, Edit, Bash) closed on its own side, before FlowPilot's approval bridge — which would have auto-approved — ever saw a request. `approvals.ndjson` recorded zero entries for the affected run despite its dispatch envelope showing `yolo:true` on every turn.

Root cause was a regression from commit `968d210` (2026-07-16, BUG-288), which introduced `resolveYoloPostureForTurn` (V9-21) and, for the first time, decoupled `ClaudePermissionMode` from `RunnerAutoApprove`. `claude_permission_mcp.go`'s `claudeArgs` — unchanged since the file's origin commit, when the two fields were always in lockstep — was never updated to match.

Fix: `claudeArgs` now attaches `--permission-prompt-tool` whenever `ClaudePermissionMode != "bypassPermissions"` (i.e., whenever Claude's own mode is actually gated), independent of `RunnerAutoApprove`. Confirmed Codex and Grok are unaffected and need no fix: Codex's `handleInbound` routes every approval-shaped request to the shared bridge unconditionally (no launch-time gate), and Grok's `handleInbound` auto-approves via its own `yolo` branch before ever consulting `GrokPermissionMode`/`forceShellBridge`.

## Verification

- `go test ./internal/runner -run TestBug296 -count=1`: 4 passed — the fix itself (posture → `claudeArgs` now attaches the flag), non-regression (plain YOLO still omits it), the shared `RequestApproval` auto-approve decision (provider-neutral), and a Codex wiring regression guard for the same combination class.
- `go test ./internal/runner -run 'TestClaudeArgs|TestV9Matrix|TestClaudeAdapter|TestCodexAdapter|TestGrokAdapter|TestRequestApproval|TestBug296|TestClaudeSendTurn|TestCodexPermissions|TestCodexMcp|TestBug294|TestBug295' -count=2`: 172 passed, run twice — fully deterministic.
- Full `go test ./internal/runner -count=1`: run twice (once fixed, once on a stashed baseline) for comparison — each run produced a DIFFERENT set of ~19-24 failures, none in a file this fix touches or referencing `ClaudePermissionMode`/`RunnerAutoApprove`/`claudeArgs`; confirmed pre-existing environmental flakiness (no Codex CLI binary, machine-specific home paths, Grok account slot state, git guard shim executability on this machine), not a regression from this change.
- `-race` not run: this machine lacks gcc/CGO.
- GitNexus impact analysis unavailable (`%1 is not a valid Win32 application`); performed localized tracing instead — enumerated every consumer of `ClaudePermissionMode`/`RunnerAutoApprove` and both other providers' inbound-approval handlers before changing the single shared line, and confirmed via `git log -L`/`git show --stat` exactly which commit introduced the regression and that it never touched the file being fixed.

## Files

- `apps/local-runner/internal/runner/claude_permission_mcp.go`: `claudeArgs` gates `--permission-prompt-tool` on `ClaudePermissionMode != "bypassPermissions"` instead of `!RunnerAutoApprove`.
- `apps/local-runner/internal/runner/bug296_yolo_permission_prompt_tool_test.go`: additive regression + non-regression tests (Claude fix, shared bridge decision, Codex wiring guard).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-296
change_type: bugfix
summary: Claude flow-coding children under YOLO now correctly route gated permission prompts to FlowPilot's approve tool instead of failing closed with no way to answer, restoring the runner's auto-approve behavior; Codex and Grok confirmed unaffected.
# --->8---
