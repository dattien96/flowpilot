# CA-282: Grok Spawn Prompt Composition Parity Test (Task-209 DOD-5)

## Scope

Closes Task-209 DOD-5 / BUG-128 regression lock: Grok child spawns must produce the same first-turn composed prompt as Claude and Codex for identical inputs.

## Changes

- `apps/local-runner/internal/runner/interactive_service_test.go`
  - `TestGrokSpawnPromptCompositionMatchesClaudeCodexBaseline` — spawns built-in `coder` with `wait=true` from Claude/Codex/Grok parents on a shared cwd; asserts child `TurnRequest.Prompt` is byte-identical across providers and contains the `composeAgentSpawnPrompt` core.

## Verification

- `go test ./internal/runner -run TestGrokSpawnPromptCompositionMatchesClaudeCodexBaseline` — PASS.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: Task-209
change_type: test
summary: Grok spawn child prompt composition parity test vs Claude/Codex baselines (DOD-5 BUG-128)
# --->8---