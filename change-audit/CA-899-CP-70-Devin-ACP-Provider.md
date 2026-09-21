# CA-899 — CP-70: Devin CLI as a first-class provider over ACP

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-70
change_type: feature
summary: Integrates the Devin CLI as a sixth provider — `devin acp` JSON-RPC transport with initialize→authenticate boot (PKCE), a controlled adapter mapping session/update notifications onto the shared TurnBridge contract, per-session model/mode config via session/set_config_option, slug-session resume through the turn log, a stdio MCP shim because Devin advertises http/sse=false, provider-registry/accounts/models/compat/desktop/TUI wiring, and a `devin -p` one-shot summarizer path — all additive after existing providers
# --->8---

## Why

CP-70 (Task-400..403) adds Devin as a peer of Claude/Codex/Grok/Gemini/
OpenCode. Live probes against `devin 3000.10.31` established the contract
the plan needed but could not guess: ACP mode refuses local credentials
("ACP host is the sole source of credentials") so every process boots with
`initialize` + `authenticate{methodId:"devin-browser"}` (PKCE, ~3s, no
click); session ids are adjective-noun slugs (`sore-router`), never `ses_*`;
model+mode are per-session `configOptions` (set_config_option switches both
mid-session); `mcpCapabilities{http:false,sse:false}` means the loopback
HTTP MCP every other provider uses cannot be injected — a stdio shim is
required; sessions live in a shared SQLite `cli/sessions.db` (CA-688 shape);
and the stream emits `_cognition.ai/*` extension notifications the
dispatcher must tolerate.

## Change

- `devin_acp_types.go` / `devin_acp.go` / `devin_process.go` (new): JSON-RPC
  dispatcher over stdio with request multiplexing, notification dispatch,
  inbound `session/request_permission` routing, boot = initialize (fs
  capabilities false — writes stay behind the permission bridge) then
  authenticate; process registry keyed by account scope + child segment
  (BUG-334 parity: children get their own process so per-turn MCP tokens
  don't clobber the parent's in-flight calls); env isolation via
  FLOWPILOT_DEVIN_BIN/FLOWPILOT_DEVIN_AGENT and per-account HOME/
  XDG_CONFIG_HOME/XDG_DATA_HOME.
- `devin_adapter.go` / `devin_event_mapper.go` / `devin_reasoning.go` (new):
  controlled adapter — session/new→prompt→stopReason mapping onto
  message_delta/tool_call/turn_completed, `session/load` for resume and
  mid-chat model switch (BUG-329 analog: synthetic thread ids never reach
  the CLI — isDevinRealSessionID gates on the slug shape), model catalog
  captured from configOptions into the models cache, permission requests
  mapped to ApprovalDetails through the shared bridge (kind inferred from
  toolCall title/rawInput), and reasoning effort folded into the model id
  suffix (`swe-2-high`→`swe-2-xhigh`) since Devin has no separate effort
  knob. Session mode resolution: read-only postures → plan/ask, gated flow
  nodes → ask, YOLO → bypass (accept-edits when the shell bridge is
  forced), otherwise auto — normal chat never gets accept-edits because
  that would auto-approve workspace writes behind FlowPilot's back.
- `devin_mcp_stdio.go` + `cli/root.go`: `flowpilot devin-mcp-stdio --url
  <endpoint>?token=<turn-token>` — newline-delimited JSON-RPC on stdin
  proxied to the per-turn HTTP MCP endpoint (202 notifications get no
  stdout line; transport failures resolve the pending call with -32000 so
  the session never hangs).
- `devin_models_cache.go` (new): 30-min catalog cache under the user config
  dir, sourced from ACP configOptions (the REPL `devin models` store is a
  different credential and was unusable); `devinPrefixedProviderModels`
  re-adds the `devin/` namespace for inventory.
- `runner.go`: provider spec (key devin, binary devin, default
  devin/swe-2-high), detectModels via ACP catalog probe with cache
  fallback, auth status/candidates/env isolation/install command (the real
  install.sh curl, not a brew cask), and the `devin -p` one-shot adapter —
  `--respect-workspace-trust false` always, `--model` stripped of the
  prefix, `--permission-mode dangerous|accept-edits` for yolo/write, `-p`
  last so the prompt is its inline value.
- `provider_registry.go` / `provider_event.go` / `provider_accounts.go` /
  `compat.go` / `session_file_locator.go` / `turn_log.go` /
  `interactive_resume.go` / `interactive_service.go` / `summarizer.go` /
  `agentpack/pack.go`: appended `ProviderKeyDevin` branches after existing
  providers — registry adapter factory, account discovery
  (.devinHomeN managed slots, XDG footprint validation), compat
  TestedDevinVersion=3000.10.31, DB-aware session locator (no per-session
  file exists), devin_session turn-log kind + resume handle promotion,
  history-open auth gate, and `devin/` model-prefix routing.
- Desktop/TUI: ProviderKey union + provider card (icon) + accounts panel +
  install button + providerLabel/default-model pick + vision gate
  (per-model `_meta.supportsImages` like opencode) + TUI image gate and
  provider color. admin-web deliberately untouched — its unions never
  gained grok/opencode either.
- Tests (additive only): devin_acp_test (types/param builders/extractors),
  devin_process_test (fake ACP server: boot order, auth, dispatch,
  EOF/failure draining), devin_adapter_test (event mapping, permission→
  bridge routing, mode resolution matrix, model/effort mapping, slug-id
  gating), devin_event_mapper_test, devin_e2e_test (InteractiveService→
  registry→real adapter→fake ACP: slug promotion, session/load on model
  switch, permission card on the run), devin_integration_test (prompt-exec
  args, summarizer model, effort tables, model cache TTL, account
  discovery, compat seeds, MCP shim request/notification/parse-error/
  endpoint-error), plus testdata/devin_acp fixtures slimmed from live
  captures.

## Verification

- `go test ./internal/runner -run 'Devin|…'` — all devin suites green.
- `go build ./...` clean; desktop `tsc --noEmit` clean (one pre-existing
  store.chat-mode-persist.test.ts error reproduces on the unmodified base).
- Full runner suite: `TestSyncedChatCanResumeAfterServiceRestart` and
  `TestBUG327_EmitLockedDoesNotUnparkWaitingChild` fail identically on base
  7402f26d — pre-existing, not CP-70 regressions. Two TUI stop-button tests
  likewise fail on base.
- Live verification: see CP-70-Test-Steps §R results (gate-sandbox, HTTP
  drive, devin + grok-4.5 regression).
