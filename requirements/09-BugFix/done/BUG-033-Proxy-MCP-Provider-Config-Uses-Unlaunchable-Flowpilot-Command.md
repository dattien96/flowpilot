# BUG-033: Proxy MCP Provider Config Uses Unlaunchable `flowpilot` Command

## Metadata

- Document ID: `BUG-033`
- Title: `Proxy MCP Provider Config Uses Unlaunchable flowpilot Command`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-09`
- Last Updated: `2026-06-09`
- Parent Documents: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-032-Codex-Google-Drive-MCP-Model-And-Approval.md`, `change-audit/CA-042-cp29-runner-proxy-hardening.md`
- Replaces: `none`
- Tags: `google-drive, proxy-mcp, codex, provider-config, startup, regression`

## AI Quick View

### Summary

- CP-29 proxy provider config generation wrote `command = 'flowpilot'` for Codex, Gemini, and Claude.
- In local dev environments where `flowpilot` was not installed on `PATH`, provider MCP startup failed before the proxy server could boot.
- Codex surfaced the failure as `MCP startup incomplete (failed: google-drive)` even though the generated args and env were otherwise correct.

### Current Ask

- Record the provider-config startup regression and the fallback command fix used for local dev checkouts.

### Key Decisions

- `V-1` Proxy provider config must prefer a real `flowpilot` binary only when it is actually discoverable on `PATH`.
- `V-2` Dev workspaces without a global `flowpilot` binary must fall back to `go -C <workspace>/apps/local-runner run ./cmd/flowpilot`.
- `V-3` Provider status detection must treat both proxy command forms as valid and not mark the fallback form as stale.

### Constraints

- Do not break installed environments where `flowpilot` is intentionally available as a standalone binary.
- Preserve the same proxy MCP args, allowlist, approval mode, and env semantics across both launch shapes.
- Keep the fix scoped to provider-config generation and status recognition; do not alter proxy auth or Google API behavior.

### Open Questions

- Should FlowPilot eventually install and manage a stable global `flowpilot` launcher for all local provider accounts instead of relying on a Go fallback in dev mode?

### Source Refs

- `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go`
- `apps/local-runner/internal/runner/google_drive_mcp_provider_config_test.go`
- Observed local config: `~/.codex/config.toml`
- Observed failure: `MCP startup incomplete (failed: google-drive)`

## 1. Issue Summary

CP-29 proxy provider setup generated a Google Drive MCP server block that pointed to `command = 'flowpilot'`. That shape only works when a real `flowpilot` executable is already installed on the machine. In a local source checkout where the runner is normally started with `go run ./cmd/flowpilot`, the generated provider config was valid in structure but unlaunchable in practice.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: local FlowPilot source checkout with proxy MCP enabled and Codex account config under `~/.codex`
- reproduction steps:
  1. Enable `FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP=true`.
  2. Configure provider MCP settings from the Google Drive setup page.
  3. Open the generated `~/.codex/config.toml` and observe `command = 'flowpilot'`.
  4. Start Codex with the configured `google-drive` MCP server.
- frequency: reproducible on machines where `command -v flowpilot` returns nothing.

## 4. Expected vs Actual

- expected: provider config should generate a proxy MCP command that the current environment can launch successfully.
- actual: provider config generated a `flowpilot` command even when no `flowpilot` binary existed on `PATH`, so MCP startup failed immediately.

## 5. Impact

- users affected: developers and local users validating CP-29 proxy MCP from a source checkout without a globally installed `flowpilot` binary.
- workflows affected: Google Drive proxy MCP provider smoke tests, Codex startup, and any provider session that depends on the generated proxy server block.
- severity: high for local CP-29 validation because it blocks MCP startup entirely.

## 6. Root Cause

- hypothesis: provider-config generation assumed the FlowPilot CLI executable name was always globally installed and launchable.
- confirmed cause: `expectedCodexGoogleDriveMcpServer`, `expectedGeminiGoogleDriveMcpServer`, and `expectedClaudeGoogleDriveMcpServer` emitted `command = "flowpilot"` without checking whether `flowpilot` was present on `PATH`.
- evidence:
  - `command -v flowpilot` returned no path on the affected machine.
  - The generated config still used `command = 'flowpilot'`.
  - Manual command validation showed `go -C /Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner run ./cmd/flowpilot google-drive-mcp --workspace /Users/tiendat/Desktop/flowpilot/flowpilot --help` worked in the same environment.

## 7. Fix Strategy

- `F-1` Add launch-command selection that prefers `flowpilot` only when it is discoverable on `PATH`.
- `F-2` Fall back to `go -C <workspace>/apps/local-runner run ./cmd/flowpilot` for proxy MCP provider config in local dev workspaces.
- `F-3` Extend proxy command parsing and stale-config detection so both launch forms are accepted as valid proxy config.

## 8. Validation

- `V-1` Focused runner tests confirm proxy provider config chooses the expected command shape for both installed-binary and dev-workspace cases.
- `V-2` Focused runner tests confirm stale-config detection accepts the `go -C ... run ./cmd/flowpilot` fallback form.
- `V-3` Manual config inspection confirms the generated `google-drive` block now uses the launch form that exists on the current machine.

## 9. Regression Guard

- tests:
  - `TestGoogleDriveProxyMcpCommandWithLookup`
  - `TestParseGoogleDriveProxyMcpInvocation_AcceptsGoRunFallback`
  - focused `EnsureGoogleDriveMcpProviderConfig` proxy tests in `google_drive_mcp_provider_config_test.go`
- alerts:
  - surface detected provider command and args in the Google Drive setup status UI
- audit checks:
  - verify `command -v flowpilot` before emitting the direct binary form
  - verify provider status recognizes both proxy command shapes as `configKind = proxy`

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- notes left unchanged on purpose:
  - business intent, MCP tool surface, and proxy auth ownership stay the same; only provider-launch mechanics changed.
