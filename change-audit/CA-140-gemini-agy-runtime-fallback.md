# CA-140: Gemini AGY Runtime Fallback

## Summary

Replaced the broken Gemini ACP runtime path with an Antigravity-compatible `agy --print` fallback and tightened project workspace attachment so Gemini starts against the active FlowPilot project instead of drifting into Antigravity's default project.

## What Changed

- Reworked `apps/local-runner/internal/runner/gemini_adapter.go` to launch `agy` in one-shot print mode per turn instead of attempting ACP JSON-RPC initialization.
- Added stable FlowPilot-scoped Gemini project ids so repeated turns in the same chat reuse `agy --continue` continuity instead of depending on removed ACP session ids.
- Updated the legacy runner session path in `apps/local-runner/internal/runner/sessions.go` so Gemini sessions are virtual handles backed by per-message `agy` invocations rather than a long-lived ACP subprocess.
- Updated prompt execution and provider auth launch paths to use `agy`.
- Tightened Gemini provider capabilities so the live registry no longer advertises unsupported streaming/approval/MCP features for the Antigravity fallback path.
- Replaced Gemini runner tests that assumed ACP initialize / session/new / session/prompt behavior with coverage for the new `agy --print` argument contract and virtual session flow.
- Resolve the selected FlowPilot workspace to an existing Antigravity project id by matching `~/.gemini/config/projects/*.json` folder URIs before launching `agy`.
- Bootstrap a minimal Antigravity project config for the selected workspace when no matching config exists, then pass that generated project id to `agy --project`.
- Treat FlowPilot chat thread ids (`thread-*`) as synthetic Gemini ids so new projects bootstrap an Antigravity project config instead of passing the FlowPilot thread id to `agy --project`.
- Add `[gemini-agy]` runner diagnostics that log workspace project resolution, generated config paths, sanitized launch arguments, and AGY result/error summaries for follow-up debugging.
- Include `--new-project` on first-turn AGY print launches after resolving the workspace project id, because AGY logs showed `--project` alone still created the print-mode conversation under `default-cli-project`.
- Move `--print` to the end of AGY invocations with the actual prompt as its value; AGY logs showed `promptLength=9` because the old argv order made `--print` consume `--project` as the prompt.
- Replace Gemini's injected project-context wording so it points the model at the attached cwd and no longer mentions provider registry commands that the model could parrot back to the user.
- Fail Gemini run creation early when the selected project has no usable local workspace path instead of silently falling back to the global/default project.
- Reset the desktop active chat run when switching projects so a stale Gemini run from the prior project cannot continue after selecting a different project.
- Re-enable Gemini chat-history reopen after a server restart by validating persisted AGY project configs instead of rejecting disk-resumed Gemini runs as unsupported.
- Rebind/copy Gemini AGY project config files during cross-account reopen so resumed chats continue to point at the stored AGY project id.
- Rehydrate a minimal Gemini transcript from persisted FlowPilot session metadata when no provider-owned transcript file exists, so reopened Gemini chats are readable after restart.

## Verification

- `go test ./internal/runner -count=1`
- `go test ./internal/runner -run 'TestGemini|TestStartSessionGemini|TestSendMessageGemini|TestDetermineProviderSessionID|TestResolvePromptExecutionAdapter' -count=1`
- `go test ./internal/runner -run 'TestCreateRunGeminiRequiresUsableWorkspacePath|TestGeminiAdapter|TestSendTurnWithRetryDoesNotRetryGeminiWorkspaceRequired'`
- `go test ./internal/runner -run 'TestResumeRunGemini|TestResumeRunInMemoryCompletedGeminiRemainsReadable' -count=1`
- `npm --prefix apps/desktop-flowpilot run typecheck`

## Notes

- This fix restores chat viability against the currently installed `agy` CLI, but it is intentionally conservative: provider-owned approval events, MCP transport, and true streaming remain disabled until Antigravity exposes a stable machine-readable turn protocol the runner can own safely.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-167
change_type: bugfix
summary: Replace broken Gemini ACP runtime calls with Antigravity print-mode fallback and active workspace project binding
# --->8---
