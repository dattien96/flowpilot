# BUG-069: Claude YOLO=off Always Auto-Approves — No Approval UI Shown

## Metadata

- Document ID: `BUG-069`
- Title: `Claude YOLO=off Always Auto-Approves — No Approval UI Shown`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-09: Approval Gates & YOLO Mode](../../06-System-Tech-Design/SD-09-Approval-Gates.md), [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- Child Documents: `none`
- Related Documents: [BUG-066: Codex V2 Approval Decision Rejected By App-Server](./BUG-066-Codex-V2-Approval-Decision-Rejected-By-AppServer.md), [BUG-064: Codex Approval Decision Value Rejected By App-Server](./BUG-064-Codex-Approval-Decision-Value-Rejected-By-AppServer.md), [BUG-063: Desktop Chat Controls Not Applied To Provider Runs](./BUG-063-Desktop-Chat-Controls-Not-Applied-To-Provider-Runs.md), [CA-079: Fix Claude YOLO=off Always Auto-Approve](../../../change-audit/CA-079-fix-claude-yolo-off-always-auto-approve.md)
- Replaces: `none`
- Tags: `claude, approval, yolo, gating, settings, regression, high`

## AI Quick View

### Summary

- After BUG-066 fixed Codex V2 approval, testing Claude with YOLO off revealed that the approval card never appears — Claude executes tools without prompting.
- `ensureClaudeConfigSettings` was write-once: it skipped writing `settings.json` when the file already existed.
- Claude CLI persists allow-rules to `settings.json` when it processes an `allow` response from the `--permission-prompt-tool` MCP route (e.g., `"allow":["Bash(*)"]` after approving a Bash command).
- On the next YOLO=false turn the stale allow-rules bypass `--permission-mode default --permission-prompt-tool` entirely — Claude auto-runs the tool without calling our MCP endpoint and `EventPermissionRequired` is never emitted.

### Current Ask

- Done. `ensureClaudeConfigSettings` now always enforces the `permissions` section (merge existing file, force `defaultMode=default, allow=[]`), so allow-rules accumulated from prior approved calls are cleared before every Claude adapter creation.

### Key Decisions

- `V-1` Always overwrite the `permissions` section of `settings.json`; preserve non-permission fields (e.g. theme) by merging the existing file.
- `V-2` The "respect an existing settings.json" policy is removed: FlowPilot manages the account config dir (`~/.claudeHome<N>/.claude`) and must own the gating posture in it.
- `V-3` Regression guard: `TestEnsureClaudeConfigSettingsClearsAllowRules` seeds an existing settings.json with `allow:["Bash(*)"]` and verifies it is cleared while `"theme"` is preserved.

### Constraints

- No change to the desktop approval card vocabulary, runner policy engine, or YOLO resolver.
- Codex and Gemini provider paths are unaffected.
- `item/permissions/requestApproval` (Codex) remains out of scope.

### Open Questions

- Whether Claude CLI also persists allow-rules through paths other than the `--permission-prompt-tool` response (e.g., auth flow, inline approval prompts). The fix is defensive for all such cases.

### Source Refs

- User retest on `2026-06-16`: after fixing BUG-066 (Codex), tested Claude with YOLO=off — approval card never appeared, Claude auto-ran file creation.
- Root cause traced to `ensureClaudeConfigSettings` write-once policy + Claude CLI's allow-rule persistence behavior (spike finding: "gating only engages if the launch config has NO broad allow-rules").
- Spike finding in `claude_permission_mcp.go` comment: under ambient allow-rules, Claude auto-runs every tool and the approve MCP tool is never called.

## 1. Issue Summary

When YOLO mode is off, the desktop chat UI should show an approval card for each Claude tool use and block until the user clicks Approve or Deny. After BUG-066 fixed Codex V2 approval, the same YOLO=off path was tested with the Claude provider. The approval card never appeared — Claude executed tools directly without any user prompt.

## 2. Parent Links

- impacted coding plan: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- impacted tech design: [SD-09: Approval Gates & YOLO Mode](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- impacted system spec: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` chat mode + `apps/local-runner` with Claude provider, YOLO toggle off.
- reproduction steps:
  1. Open desktop chat mode with Claude and YOLO off.
  2. Ask Claude to create `yolo-test.txt`.
  3. Observe: no approval card appears; Claude creates the file immediately.
  4. (On the FIRST run after a fresh account dir the approval card may appear; the bug manifests on the SECOND run after Claude has persisted allow-rules from the first approved tool call.)
- frequency: reproducible on the second or later YOLO=false Claude turn after at least one approved tool call has been made.

## 4. Expected vs Actual

- expected: every Claude tool use with YOLO=off emits `EventPermissionRequired` and blocks until the user clicks Approve or Deny on the approval card.
- actual: `EventPermissionRequired` is never emitted; Claude auto-runs tools using allow-rules it previously wrote to `settings.json`; the `--permission-prompt-tool` MCP endpoint is never called.

## 5. Impact

- users affected: all desktop chat users running Claude with YOLO=off.
- workflows affected: every Claude tool use (Bash, Write, Edit, etc.) when YOLO is disabled.
- severity: high — YOLO=false gating is silently bypassed, eliminating user oversight of Claude tool calls.

## 6. Root Cause

- hypothesis: `ensureClaudeConfigSettings` write-once behavior allows Claude-written allow-rules to persist.
- confirmed cause: `ensureClaudeConfigSettings` checked `os.Stat(path)` and returned early when `settings.json` existed. Claude CLI writes allow-rules to `settings.json` when it processes an `allow` response from the `--permission-prompt-tool` MCP route. On subsequent turns the allow-rules cause Claude to bypass `--permission-mode default` gating and auto-run tools without calling the MCP approval endpoint.
- evidence:
  - Spike finding in `claude_permission_mcp.go`: "Gating only engages if the launch config has NO broad allow-rules. Under the user's ambient `~/.claude` (allow: [Edit, Write, Bash(*)]) every tool auto-ran and the approve tool was never called."
  - `TestEnsureClaudeConfigSettingsClearsAllowRules` fails before the fix (file with `Bash(*)` is not modified) and passes after (allow list is cleared).
  - Build and all targeted approval tests pass after the fix.

## 7. Fix Strategy

- `F-1` Rewrite `ensureClaudeConfigSettings` to merge-and-enforce instead of write-once: read existing `settings.json` (best-effort), always overwrite the `permissions` section with `{defaultMode: "default", allow: []}`, write the merged result back.
- `F-2` Remove the `os.Stat` early-return; create the config dir with `os.MkdirAll` unconditionally (was already done before the write; the stat check is now unnecessary).
- `F-3` Update the doc comment to explain the new behavior and the root cause of BUG-069.

## 8. Validation

- `V-1` `go test ./internal/runner -run "TestEnsureClaudeConfigSettings" -count=1` passes (both the existing posture test and the new regression guard).
- `V-2` `go test ./internal/runner -run "TestClaudeArgs|TestEnsureClaudeConfig|TestHandleClaude|TestClaudeAdapterApprovalRoundTrip|TestClaudeAdapterYoloTrueNoPermission|TestWriteClaudeMCPConfig" -count=1` — all 11 tests pass.
- `V-3` `go build ./...` passes in `apps/local-runner`.

## 9. Regression Guard

- tests: `TestEnsureClaudeConfigSettingsClearsAllowRules` (new, seeds allow-rule, verifies it is cleared with non-permission fields preserved), `TestEnsureClaudeConfigSettingsWritesGatingPosture` (existing, fresh dir).
- alerts: none.
- audit checks: spike finding cross-referenced; write-once policy removed and documented in source comment.

## 10. Follow-Up Document Updates

- upstream docs that must change: `none` — gating is an implementation detail of the runner adapter, not a product behavior change visible in SS or SD.
- notes left unchanged on purpose:
  - FlowPilot UI and runner policy continue using `approve` / `deny`.
  - The in-stream `control_request` fallback path (used in offline/test mode when `mcpServer` is nil) is unaffected.
  - Whether Claude CLI persists allow-rules through other code paths (auth flow, inline approval prompts) is left as a follow-up question; the fix is defensive for all such cases.
