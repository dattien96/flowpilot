# CA-079 — Fix Claude YOLO=off Always Auto-Approves (BUG-069)

## Scope

Runner — `apps/local-runner/internal/runner/claude_permission_mcp.go` + its test file.

The `ensureClaudeConfigSettings` function was rewritten from write-once to merge-and-enforce.

## Completed

**Root cause**: `ensureClaudeConfigSettings` previously skipped writing when `settings.json`
already existed ("respect an existing settings.json"). Claude CLI persists allow-rules to
`settings.json` each time the user approves a tool via the `--permission-prompt-tool` MCP
route (e.g., `{"allow":["Bash(*)"]}` after approving a Bash command). On the next YOLO=false
turn the stale allow-rules bypass our `--permission-mode default --permission-prompt-tool`
gating entirely — Claude auto-runs the tool without calling the MCP approval endpoint, so
`EventPermissionRequired` is never emitted and no approval card appears.

**Fix**: `ensureClaudeConfigSettings` now:
1. Creates the config dir if needed (unchanged).
2. Reads any existing `settings.json` into a generic map (best-effort; ignore parse errors).
3. **Always** overwrites the `permissions` section with `{defaultMode: "default", allow: []}`.
4. Writes the merged map back, preserving all non-permission fields (e.g. `"theme"`).

This ensures that allow-rules accumulated from previous approved tool calls are cleared before
every Claude adapter creation, restoring the gating posture for every YOLO=false turn.

## Verification

- `go test ./internal/runner -run "TestEnsureClaudeConfigSettings" -count=1` — both
  `TestEnsureClaudeConfigSettingsWritesGatingPosture` (fresh dir) and the new regression
  guard `TestEnsureClaudeConfigSettingsClearsAllowRules` (seeded allow-rule + theme) pass.
- `go test ./internal/runner -run "TestClaudeArgs|TestEnsureClaudeConfig|TestHandleClaude|TestClaudeAdapterApprovalRoundTrip|TestClaudeAdapterYoloTrueNoPermission|TestWriteClaudeMCPConfig" -count=1` — all 11 tests pass.
- `go build ./...` passes in `apps/local-runner`.

## Residual Notes

- The test suite (`TestClaudeAdapterApprovalRoundTrip`) still drives the in-stream
  `control_request` fallback path (no `mcpServer` wired in tests). The live path goes
  through the HTTP MCP server; `TestLiveClaudeHTTPMCPDenyBlocks` covers that path.
- Whether Claude CLI itself also writes allow-rules via paths other than the
  `--permission-prompt-tool` response is not yet verified. The fix is defensive for all
  such cases since it runs on every adapter creation.
- `item/permissions/requestApproval` (Codex) and the analog Claude `ask_user` path remain
  out of scope (separate concerns).

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: BUG-069
change_type: fix
summary: Fix Claude YOLO=off Always Auto-Approves
# --->8---
