# CA-902 — Devin turns stalled 30s on a Claude-style MCP readiness gate

Found during CP-70 desktop live verification: every Devin turn took ~52s to
first token. Timeline from `run-1585740` showed `session/new` +
`session/set_config_option` completing in <1s, then a deterministic 30s gap
before `session/prompt`, then ~21s of genuine SWE-2 Max TTFT.

Root cause: `devinAdapter.SendTurn` copied Claude's readiness contract —
`mcpServer.waitReady(token, 30s)` waits for the MCP client to POST
`tools/list`. Devin's ACP client never does this eagerly: it emits
`_cognition.ai/mcp/serversChanged` and spawns the configured stdio MCP
server lazily, only once the prompt is in flight and the model decides to
use a tool. The gate therefore always burned the full 30s without ever
proving the shim was up.

Fix (Devin-only; Claude/Grok/OpenCode readiness paths untouched):

- `devin_adapter.go` no longer calls `waitReady` before `session/prompt`.
  A `[devin-mcp] lazy readiness` log line marks the decision point instead.
- All MCP wiring is preserved: the per-turn token registration, the stdio
  `flowpilot devin-mcp-stdio` entry in `session/new`, the
  `<cwd>/.devin/mcp_config.local.json` upsert (required because
  `session/load` ignores the ACP `mcpServers` param), the ask_user /
  spawn_agent prompt reinforcement, and token unregister on turn end.
  The token stays registered for the entire prompt, so lazy MCP calls that
  arrive mid-turn resolve to the same bridge.

New tests (`devin_adapter_test.go`):

- `TestDevinSendTurnDoesNotWaitForClaudeMCPReadyTimeout` — a token that
  never signals ready no longer delays `session/prompt` (was 30s).
- `TestDevinSendTurnPreservesMCPServerEntryWithoutReadyWait` — session/new
  still carries the shim entry with the per-turn token; the token resolves
  mid-prompt and is released after the turn.
- `TestDevinResumeWritesLocalMCPConfigBeforeSessionLoad` — the project-local
  config file exists before `session/load` and keeps the shim entry.

Live-verified on a test runner (port 4319, bed `fp-beds/cp70-devin`):

- Latency: `lazy readiness` → `session/prompt` in the same second (was +30s);
  first agent chunk at +4s on swe-2-high.
- `ask_user` on a resumed session (`session/load` → slug `season-millennium`):
  Devin lazily spawned `devin-mcp-stdio` at prompt time, `tools/list`
  returned `approve/ask_user/spawn_agent`, the tool call produced question
  `q-23`, the answer resolved it and the turn completed.
- `spawn_agent` (wait=true): child `run-40` spawned with its own Devin
  session `rare-sheep`, returned `CHILD_OK`, parent completed.

Cross-provider: no shared code changed — `claudeMCPReadyDefaultTimeout` and
`waitReady` still serve Claude (eager tools/list contract), Grok and
OpenCode unchanged.

Test status: `go test -run TestDevin` passes (includes the three new tests).
The full `internal/runner` suite shows ~30 failures in unrelated domains
(Google Drive provider discovery, flow engine, Supabase catalog, Firebase);
spot-checked `TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget`,
`TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted` and
`TestCatalogStoreForFallsBackToFake` all fail identically on the base commit
with this change stashed — pre-existing environment-dependent failures, no
`TestDevin*` regressions.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-70
change_type: bugfix
summary: Removes the Claude-style 30s tools/list readiness gate from Devin SendTurn — Devin spawns its stdio MCP shim lazily at prompt time, so the gate only added latency; all MCP wiring (session/new entry, mcp_config.local.json, per-turn token, unregister) is preserved and live-verified for ask_user and spawn_agent.
# --->8---
