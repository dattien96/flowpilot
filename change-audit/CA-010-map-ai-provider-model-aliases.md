# CA-010 Map AI Provider Model Aliases

## Scope

Mapped UI/Database model names to the actual model aliases expected by underlying provider CLIs (`claude` and `gemini`). This ensures model-specific prompt executions do not crash due to unrecognized model entity names.

## Completed

- Updated [runner.go](file:///c:/working/flowpilot/apps/local-runner/internal/runner/runner.go):
  - In `resolvePromptExecutionAdapter`, added model name mapping before constructing execution arguments for `claude` (Claude Code) and `gemini` CLI providers.
  - Strips the `claude-` prefix for Claude model names (e.g. `claude-sonnet` -> `sonnet`, `claude-opus` -> `opus`, `claude-haiku` -> `haiku`).
  - Strips the `gemini-` prefix for Gemini model names (e.g. `gemini-pro` -> `pro`, `gemini-flash` -> `flash`).
- Updated [runner_test.go](file:///c:/working/flowpilot/apps/local-runner/internal/runner/runner_test.go):
  - Created `TestResolvePromptExecutionAdapterMapsModelNames` unit test validating all mapped and unmapped combinations across `claude`, `gemini`, and `codex` providers.

## Verification

- Ran `go test ./...` in the `apps/local-runner` module (all CLI and Runner tests passed successfully).
- Verified local runner compiles and starts correctly.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-010
change_type: feature
summary: Map AI Provider Model Aliases
# --->8---
