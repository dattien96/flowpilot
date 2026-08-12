# FlowPilot TUI — `flowpilot chat`

Terminal UI client for the FlowPilot runner, implemented with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Architecture (CP-56)

```
internal/tui/
  client/       HTTP + SSE client (mirrors TypeScript HttpWsRunnerClient)
  config/       ChatConfig struct (resolved flags)
  runnerboot/   Auto-ensure runner: health-check → reuse or spawn
  app/          Bubble Tea model, view, update loop
internal/cli/
  chat.go       `flowpilot chat` Cobra command entry point
```

**Boundary rule**: `internal/tui/**` MUST NOT import `flowpilot-runner/internal/runner`.
All runner interaction is via HTTP to the `/client/*` API.

## Usage

`just` always runs from the FlowPilot checkout, so the **target project path is required** (first argument).

```bash
# Windows — both slash styles accepted
just chat-dev D:/working/gate-sandbox
just chat-dev D:\working\gate-sandbox

# macOS / Linux
just chat-dev /Users/me/gate-sandbox

# Extra flags after the path
just chat-dev D:/working/gate-sandbox --yolo

# Direct binary
flowpilot chat --project D:/working/gate-sandbox
flowpilot chat --project /Users/me/gate-sandbox
```

On open, the statusline shows `provider · model` (active runner account). Change with `/provider` / `/model` before the first turn.

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--runner-url` | | Explicit runner URL (skips auto-start) |
| `--no-start-runner` | | Don't spawn a runner if not running |
| `--project` | `-P` | Target project directory (Windows `\`/`/` and Unix paths; required via `just chat-dev <path>`) |
| `--provider` | | Optional override (default: active account / runner default) |
| `--model` | | Optional model override |
| `--reasoning` | | Reasoning effort: `high`, `medium`, `low` |
| `--yolo` | | Enable YOLO mode (auto-approve all) |
| `--print` | `-p` | Headless: send prompt, print response, exit |
| `--prompt` | | Initial prompt (for headless) |
| `--resume` | | Resume a previous run by ID |
| `--timeout` | | Session timeout (e.g. `30m`) |

Root persistent flags (`--workspace`, `--host`, `--port`) are inherited and not shadowed.

## Slash Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/clear` | Clear conversation history |
| `/exit` | Exit the TUI |
| `/yolo` | Toggle YOLO mode |
| `/agents` | Toggle agents focus panel |
| `/flow` | List/arm a flow — type `/flow ` then filter; ↑↓ + Tab to pick |
| `/chat` | Switch to chat mode |
| `/status` | Show current connection status |
| `/login` | Sign in to Supabase (email/password); writes Desktop session file when possible |
| `/chats` | List recent project chats (Desktop history); then `/open <n>` |
| `/open <n\|runId>` | Switch to an existing chat (resume + replay transcript) |
| `/resume <run-id>` | Same as `/open <runId>` |
| `/approve` | Approve pending approval |
| `/deny` | Deny pending approval |

When Desktop is on the Login screen (no `supabase-auth-session.json`), the TUI shows
`SIGN IN REQUIRED` and statusline `SIGN-IN`. Use `/login` (interactive email → password,
password masked) or `/login you@email.com`. Successful login calls
`POST /supabase-auth/login` and best-effort persists the same session file Desktop reads.

## Runner Auto-Start

On `flowpilot chat`, the TUI:

1. Resolves the runner URL from `--runner-url` OR `http://<host>:<port>` (root flags).
2. Calls `GET /health`. If `status=online` and `cwd` matches workspace → reuses the runner.
3. Otherwise spawns `<exe> runner serve --workspace <root> --host 127.0.0.1 --port <port>` detached.
4. Polls `/health` until online (20s timeout).
5. Logs runner output to `<workspace>/.flowpilot/cli-runner.log`.

## Grok YOLO

For Grok, toggling YOLO calls `POST /provider-accounts/grok-yolo-posture` before updating local state. This is required because Grok's YOLO=false direction requires server-side config rewrite and process respawn (see `grok_process.go`).

## Provider Parity

The TUI is provider-agnostic: it speaks the `/client/*` HTTP API and never inspects `providerKey` for routing decisions, except for the Grok YOLO endpoint. Claude, Codex, and Grok all work through the same `StartRun → SendTurn → SSE stream` flow.

## Tests

```bash
# TUI packages
go test ./internal/tui/... -count=1

# CLI (includes existing provider-account tests)
go test ./internal/cli/... -count=1
```

Test coverage (CP-56 signatures A0–A9):

| ID | Test | Coverage |
|----|------|----------|
| A0 | Health check parsing | `client_test.go` TestA0_HealthCheck |
| A1 | SendTurn + providerTurnId filter | `client_test.go` TestA1_SendTurn_FiltersByProviderTurnId |
| A2 | SSE stream frame parsing | `client_test.go` TestA2_SSEStreamParsing |
| A3 | Retry on turn_in_progress (6×@700ms) | `client_test.go` TestA3b_SendTurn_RetriesOnTurnInProgress |
| A4 | Grok YOLO posture endpoint | `client_test.go` TestA4_ApplyGrokYoloPosture |
| A5 | EnsureRunner reuses healthy runner | `runnerboot_test.go` TestA5_EnsureRunner_ReusesHealthyRunner |
| A6 | FindWorkspaceRoot walk-up | `runnerboot_test.go` TestA6_FindWorkspaceRoot |
| A7 | Initial TUI model state | `app_test.go` TestA7_* |
| A8 | Slash commands (/yolo, /clear, /help, …) | `app_test.go` TestA8_* |
| A9 | Headless/print mode + Ctrl+C quit | `app_test.go` TestA9_* |
