# CA-277 — Claude `xhigh` Reasoning Effort No Longer Silently Remapped To `max`

## Scope

Follow-up from [Task-215](../requirements/08-Task/inprogress/Task-215-Per-Model-Reasoning-Effort-Detection-And-Model-Aware-UI.md): while live-verifying Claude's `--effort` surface (no per-model catalog exists for Claude, so Task-215's auto-detect only covers Codex/Grok), inspection of the installed `@anthropic-ai/claude-code` binary (2.1.191) found its real `--effort` validator accepts a fixed five-value array — `GD=["low","medium","high","xhigh","max"]` — where `xhigh` and `max` are **distinct** effort tiers, not synonyms. Two code paths in the runner assumed otherwise and silently upgraded a user's `xhigh` selection to `max` before sending it to the CLI.

## Changes

- `apps/local-runner/internal/runner/sessions.go`: `normalizeClaudeEffort` no longer maps `"xhigh"` to `"max"` — it now only lowercases/trims the value. Used by the interactive session turn path (`claudeArgs`/`claude_permission_mcp.go`).
- `apps/local-runner/internal/runner/runner.go`: `resolvePromptExecutionAdapter`'s Claude branch (the one-shot summarizer/prompt-execution path, which duplicated the same remap inline rather than calling `normalizeClaudeEffort`) had the identical `xhigh` -> `max` special case removed.
- `apps/desktop-flowpilot/src/components/ChatInput.tsx`: `FALLBACK_REASONING_OPTIONS` (the Reasoning dropdown's fallback list for Claude, which has no per-model catalog) widened from `low/medium/high` to the full live-verified `low/medium/high/xhigh/max`, so both tiers are actually selectable.
- Tests: `runner_test.go`'s `TestResolvePromptExecutionAdapterReasoningEffort` table gained a `claude/claude-opus/xhigh -> --effort xhigh` case (previously asserted `--effort max`) plus a new `claude/claude-opus/max -> --effort max` case to keep both tiers covered.

## Verification

- `go build ./...` — clean.
- `go test ./internal/runner/... -run 'TestResolvePromptExecutionAdapterReasoningEffort|TestClaude|Reasoning|Effort'` — all pass, including the corrected `xhigh`/`max` cases.
- `npx tsc --noEmit` (`apps/desktop-flowpilot`) — clean after the `FALLBACK_REASONING_OPTIONS` widen.
- Not re-verified against a live `claude` turn in this pass (no network/auth in this environment) — the fix is a direct removal of a special case proven wrong by static binary inspection, not a new detector; the existing byte-for-byte argument tests are the regression guard.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-215
change_type: bugfix
summary: Claude's --effort xhigh is sent to the CLI as xhigh instead of being silently upgraded to max — the two are distinct effort tiers on the real CLI, not synonyms; the desktop Reasoning fallback list now offers both
# --->8---
