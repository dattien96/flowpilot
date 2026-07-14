# BUG-281: Grok Telegram MCP Keyring Home Isolation

## Metadata

- Document ID: `BUG-281`
- Title: `Grok Telegram MCP Keyring Home Isolation`
- Phase: `bugfix`
- Status: `implemented`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-14`
- Last Updated: `2026-07-14`
- Parent Documents: [CP-05-05: Telegram MCP As An Output Notification Artifact](../../07-Coding-Plan/todo/CP-05-05-Tele-Mcp.md), [Task-232: Telegram Output Artifact Type And Bot-API Proxy MCP](../../08-Task/todo/Task-232-Telegram-Output-Artifact-Type-And-Bot-API-Proxy-MCP.md), [Task-233: Telegram Output Write-Contract, Verify Gate, And Approval](../../08-Task/todo/Task-233-Telegram-Output-Write-Contract-Verify-Gate-And-Approval.md), [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- Child Documents: `None`
- Related Documents: `run-689`, `/Users/tiendat/.codex/config.toml`, `/Users/tiendat/.grokHome2/config.toml`, [CA-315: Telegram MCP Configure Providers Workspace](../../../change-audit/CA-315-telegram-mcp-configure-providers-workspace.md), [CA-316: Telegram MCP loopback for provider HOME keyring isolation](../../../change-audit/CA-316-telegram-mcp-loopback-keyring-isolation.md)
- Replaces: `None`
- Tags: `mcp`, `telegram`, `grok`, `keyring`, `multi-account`, `provider-home`, `security`, `regression`

## AI Quick View

### Summary

- Grok chat `run-689` failed after Telegram MCP was configured because `flowpilot_telegram` runs as a Grok-spawned MCP child under `HOME=/Users/tiendat/.grokHome2`.
- The Telegram MCP child currently reads the runner keyring directly via `resolveConnectedTelegramCredential`; with Grok account-home isolation, macOS keyring lookup returns `secret not found in keyring`.
- Codex passes with the same MCP command because it runs under the normal user home, so the runner keyring service can find `telegram:<integrationId>`.
- The fix plan is option 3: keep provider `HOME` isolation, but move keyring access and Telegram send execution back into the main FlowPilot runner through an authenticated loop-back API.

### Current Ask

- Implemented (CA-316): option 3 loop-back — `telegram-mcp` calls `POST /internal/mcp/telegram/send`; runner owns keyring + Bot API + approval. Remaining: manual Grok verify (V-7) after Configure Providers + runner restart.

### Key Decisions

- `F-1` Keep `HOME=/Users/tiendat/.grokHome2` for the Grok provider process; this is required for multi-account isolation and must not be reverted.
- `F-2` Do not put Telegram bot token into Grok/Codex/Claude config TOML, command args, or MCP env by default.
- `F-3` Make `telegram-mcp` a thin local client for Telegram sends when launched by provider CLIs; the main runner owns keyring lookup, approval state, and Bot API execution.
- `F-4` Authenticate the MCP child to the local runner with a short-lived runner-issued token, not with the Telegram bot token.

### Constraints

- The fix must preserve FlowPilot's multi-account provider homes (`GROK_HOME`, `CODEX_HOME`, Claude config dirs).
- Telegram send is an outward-facing side effect; approval queue semantics from Task-233 must remain intact.
- Bot token must stay out of provider-visible config, provider logs, prompts, and debug dumps.
- The local runner might be unavailable; the MCP child must fail with a typed actionable error, not silently bypass or fall back to sending directly with leaked credentials.

### Open Questions

- `Q-1` Should the loop-back auth token reuse the existing runner-hosted MCP token machinery, or should Telegram MCP get a dedicated short-lived `FLOWPILOT_RUNNER_MCP_TOKEN` env value?
- `Q-2` Should `telegram-mcp` keep a local direct-keyring fallback for manual/offline invocation, or should all provider-configured runs require the runner server?
- `Q-3` Should the same loop-back pattern later replace keyring reads in other FlowPilot-owned MCP proxies?

### Source Refs

- `run-689`: `.flowpilot/chats/sessions.ndjson` shows provider `grok`, account `b7c9c56bebbac8819fc7a60c0b52c50e`, cwd `/Users/tiendat/Desktop/BE/gate-sandbox`, failed with `Invalid params`.
- Live repro, 2026-07-14: `telegram-mcp --workspace /Users/tiendat/Desktop/flowpilot/flowpilot` passes under normal `HOME`; with `HOME=/Users/tiendat/.grokHome2`, `backends detect` reports `secret not found in keyring` for Jira and Telegram.
- Config comparison: `/Users/tiendat/.codex/config.toml` and `/Users/tiendat/.grokHome2/config.toml` both launch `flowpilot_telegram` through `go -C ... run ./cmd/flowpilot telegram-mcp --workspace ...`.
- Impact analysis: `RunTelegramProxyMcpServer` LOW, `telegramLiveMCPServer` LOW, `resolveConnectedTelegramCredential` MEDIUM.

## 1. Issue Summary

Grok cannot use the configured `flowpilot_telegram` MCP server even though the equivalent Codex CLI configuration works. The observed MCP failure is:

```text
telegram-mcp: secret not found in keyring
```

The failure is not caused by a missing Telegram entry in `/Users/tiendat/.grokHome2/config.toml`. Grok and Codex launch the same FlowPilot Telegram MCP command. The failure happens because Grok's provider account isolation changes `HOME` to `/Users/tiendat/.grokHome2`, and the child `telegram-mcp` process inherits that environment before reading the runner keyring.

## 2. Parent Links

- impacted coding plan: `CP-05-05` (`P-5`, `DOD-6`, secret boundary), `CP-46` (`P-10`, multi-account via `GROK_HOME`)
- impacted task: `Task-232` (`RunTelegramProxyMcpServer`, keyring credential resolution), `Task-233` (approval queue and Telegram send semantics)
- impacted tech design: `SD-11` §6 (secret boundary), `SD-23` `D-8` (OUTPUT write contract)
- impacted system spec: `SS-14` if Telegram output notification acceptance criteria require live end-to-end delivery

## 3. Environment and Reproduction

- environment:
  - macOS runner using keychain-backed `go-keyring`
  - workspace: `/Users/tiendat/Desktop/flowpilot/flowpilot`
  - Grok account home: `/Users/tiendat/.grokHome2`
  - Codex config: `/Users/tiendat/.codex/config.toml`
  - Grok config: `/Users/tiendat/.grokHome2/config.toml`
- reproduction steps:
  - Configure Telegram MCP in FlowPilot.
  - Verify Codex MCP entry can launch `flowpilot_telegram`.
  - Start Grok chat under account home `/Users/tiendat/.grokHome2`.
  - Ask Grok to use Telegram MCP, or inspect backend status under `HOME=/Users/tiendat/.grokHome2`.
- frequency:
  - deterministic when the MCP child process inherits a provider account home that cannot read the runner's keyring item.

## 4. Expected vs Actual

- expected:
  - Grok multi-account isolation remains active.
  - `flowpilot_telegram` can still send through the configured Telegram integration.
  - Telegram bot token stays in runner keyring and is never exposed to provider config/env/prompt.
- actual:
  - `telegram-mcp` reads keyring directly inside the provider-spawned child.
  - Grok sets `HOME=/Users/tiendat/.grokHome2` for account isolation.
  - Keyring lookup fails with `secret not found in keyring`.
  - The provider-visible symptom can surface as Grok MCP unavailable/failure and, in some ACP paths, `Invalid params` during MCP initialization.

## 5. Impact

- users affected:
  - users with Grok multi-account homes and connected Telegram MCP.
  - likely any provider path that launches a FlowPilot-owned MCP child under a synthetic account home while that child reads runner keyring directly.
- workflows affected:
  - Telegram output artifact `telegram.v1`.
  - chat-driven Telegram `send_message` use through Grok.
  - future output-notification flows that rely on `message_id` verification.
- severity:
  - high for Grok + Telegram MCP feature usability.
  - medium platform risk because the root boundary issue can recur in other keyring-backed MCP children.

## 6. Root Cause

- hypothesis:
  - The Telegram MCP command in Grok config is stale or different from Codex.
- confirmed cause:
  - The command is effectively the same. The failing variable is process environment: Grok provider account isolation changes `HOME` to `.grokHome2`, and the MCP child directly reads the runner keyring in that altered home context.
- evidence:
  - Direct `telegram-mcp --workspace ...` under normal `HOME` returns a valid MCP `initialize` response.
  - Running backend detection with `HOME=/Users/tiendat/.grokHome2 GROK_HOME=/Users/tiendat/.grokHome2` reports `secret not found in keyring` for Telegram and Jira.
  - Running backend detection with `HOME=/Users/tiendat` and `GROK_HOME=/Users/tiendat/.grokHome2` reports Telegram installed and connected.

## 7. Fix Strategy

- `F-1` Add a runner-local Telegram send endpoint.
  - Add an internal HTTP route on the existing local runner server, for example `POST /internal/mcp/telegram/send`.
  - Request body should include `text`, optional approval-scope identifiers, and the caller identity token.
  - The endpoint must call the existing runner-side Telegram credential resolver and Telegram Bot API send path.
  - The endpoint returns the same `message_id` evidence contract that Task-233 gate logic already expects.

- `F-2` Add short-lived loop-back authentication for provider-spawned FlowPilot MCP children.
  - Generate a per-run or per-MCP-launch token in the main runner.
  - Pass only this internal token to `flowpilot_telegram` through MCP env or args.
  - Reject calls without the token, with a wrong token, or with an expired token.
  - Do not reuse the Telegram bot token as this auth token.

- `F-3` Change `telegram-mcp` runtime behavior.
  - When loop-back runner endpoint details are present, `telegram-mcp` should not call `resolveConnectedTelegramCredential`.
  - It should call the local runner endpoint for `send_message`.
  - It should keep JSON-RPC tool shape stable: `tools/list` still exposes `send_message`; `tools/call` still returns text containing `message_id`.
  - If the runner endpoint is unavailable, return a typed error such as `MCP_UNAVAILABLE: FlowPilot runner is not reachable for Telegram send`.

- `F-4` Thread runner endpoint config through provider MCP entries.
  - Update `expectedClaudeTelegramMcpServer`, `expectedCodexTelegramMcpServer`, `expectedGrokTelegramMcpServer`, and `expectedGeminiTelegramMcpServer` only as needed to pass the loop-back endpoint and token.
  - For Grok ACP live merge, `telegramLiveMCPServer` must include the loop-back env values when the runner server is active.
  - Preserve `--workspace` for state lookup and diagnostics, but do not rely on child keyring lookup for provider-spawned usage.

- `F-5` Preserve existing approval semantics.
  - Reuse `telegram_proxy_approval.go` state machine where possible.
  - If approval scope is already available in env, endpoint should create/resolve the same approval queue records.
  - If approval scope is missing, preserve the current fallback behavior: require explicit `AutoApprove` before sending.
  - Ensure duplicate identical retries replay the recorded result rather than sending twice.

- `F-6` Keep a narrow direct mode for manual invocation only if needed.
  - If `telegram-mcp` is run outside a provider and no runner endpoint is provided, decide between:
    - direct keyring mode as a developer/manual fallback, or
    - typed failure requiring the runner server.
  - This must be explicit in tests and docs; provider-configured entries should prefer loop-back mode.

## 8. Validation

- `V-1` Reproduce the current failure in a unit/integration test by running Telegram credential resolution under a synthetic provider `HOME`; document that this is the old failure mode.
- `V-2` Add a test where `telegram-mcp` runs with `HOME=/tmp/provider-home` but loop-back endpoint env is present; `tools/call send_message` succeeds and returns `message_id`.
- `V-3` Add a test that loop-back endpoint rejects missing/invalid token and never sends Telegram.
- `V-4` Add a test that the endpoint reads the keyring through the main runner path, not through the MCP child process.
- `V-5` Add a test that approval queue behavior is preserved: pending on first call, send only after approve, no double-send after executed.
- `V-6` Add a test for runner-unavailable path: `telegram-mcp` returns a typed `MCP_UNAVAILABLE` error and does not attempt direct token/env fallback in provider-configured mode.
- `V-7` Manual validation: start Grok under `/Users/tiendat/.grokHome2`, invoke `flowpilot_telegram.send_message`, confirm Telegram receives the message and Grok no longer reports `secret not found in keyring`.
- `V-8` Run targeted tests:
  - `go test ./internal/runner -run 'Telegram|telegram' -count=1`
  - `go test ./internal/flowgate -run 'Telegram|telegram' -count=1`
  - `go test ./internal/cli -run 'Telegram|telegram' -count=1` if CLI tests exist or are added.

## 9. Regression Guard

- tests:
  - `TestTelegramMcpLoopbackDoesNotReadChildKeyring`
  - `TestTelegramMcpLoopbackWorksUnderProviderHome`
  - `TestTelegramMcpLoopbackRequiresRunnerToken`
  - `TestTelegramMcpLoopbackPreservesApprovalQueue`
  - `TestGrokTelegramLiveMCPServerIncludesLoopbackEnv`
  - `TestTelegramMcpDirectModeDisabledOrExplicitWhenRunnerEndpointMissing`
- alerts:
  - log runner-side endpoint failures without bot token, chat message body beyond bounded preview, or auth token.
  - classify loop-back auth failure separately from Telegram API failure.
- audit checks:
  - verify bot token does not appear in provider config TOML, MCP env snapshots, prompt text, or tool output.
  - verify Grok account `HOME` remains `.grokHome2` for the provider process.

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - `Task-232`: update completion notes or add a follow-up note that Telegram proxy no longer owns keyring reads in provider-spawned mode.
  - `Task-233`: update approval addendum if loop-back endpoint becomes the standard approval bridge for detached MCP children.
  - `CP-05-05`: update DOD-6 if the runner loop-back API becomes the canonical architecture for Telegram output sends.
- notes left unchanged on purpose:
  - `CP-46` multi-account home isolation remains correct; this bugfix must not weaken `GROK_HOME`/`HOME` isolation for the Grok provider process.
  - `flowpilot_drive` can continue using its existing token-env model; this bugfix is specifically for the stronger Telegram secret boundary.
