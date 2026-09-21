# CA-900 — CP-70: 28-row checklist gap closure + Windows live test (R1–R10)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-70
change_type: bugfix
summary: Closes the remaining checklist gaps found auditing CA-899 (generic CheckTool detection, devin skill/agent catalogs, Drive+Jira MCP provider config, compat 5th row, AgentsPanel badge, provider labels, devin CSS brand) and fixes a live-verified permission-mode bug — resolveDevinSessionMode sent session mode "auto" which is not a valid Devin session config value and was silently coerced to accept-edits, auto-approving writes AND exec under yolo=false; now maps to "smart" so dangerous ops emit session/request_permission through the approval bridge. Live-tested on Windows with devin 3000.10.31: R1 chat, R2 write, R3 model switch, R4 approval gate+deny, R5 restart-resume via session/load slug, R8 cancel, R9 Grok regression, R10 token usage — all pass.
# --->8---

## Why

Audit of CA-899 against `CP-Guide-Add-New-Provider-Checklist.md` (28 rows)
found six missing branches — all additive, none touching existing provider
paths. Live testing then surfaced a real DV-04 gap: the non-YOLO default
mapped to session mode `"auto"`, a CLI `--permission-mode` value that is
NOT a `session/set_config_option` mode. Devin coerced it to `accept-edits`
(the session default), which auto-approves workspace edits and exec — a
`yolo=false` run wrote `gated_write.txt` and ran `git init` with zero
`session/request_permission`.

## Change

- `tooling/check.go`: `CheckTool("devin")` — `FLOWPILOT_DEVIN_BIN` override,
  else `devin`, `--version` probe.
- `interactive_catalog.go` / `agent_catalog.go`: `.devin/skills` +
  `.devin/agents` project roots and provider-home roots.
- `devin_mcp_provider_config.go` (new): writes/reads Devin's dedicated
  `mcp_config.json` under the account config dir (`ensureDevinMcpServer`,
  `CheckDevinMcpConfig`) — stdio entries only, since ACP advertises
  `mcpCapabilities{http:false,sse:false}`.
- `google_drive_mcp_provider_config.go` / `jira_mcp_provider_config.go`:
  devin branch in ensure/status/path routing
  (`ensureDevinJiraMcpConfig`, `checkDevinJiraMcpConfig`).
- `runner.ts` + `CheckVersionSettings.tsx`: 5th compat row
  (`testedDevinVersion`/`installedDevinVersion`, `CompatTestedDevinVersion`
  already existed runner-side).
- `GoogleDriveSettings.tsx` / `McpSettings.tsx` / `AgentsPanel.tsx` /
  `styles.css`: `DEVIN` provider labels, `isDevinSource` card badge,
  provider-override chip, `--devin-brand` + `.prov-devin` +
  `.provider-chip-devin`.
- `devin_adapter.go`: `resolveDevinSessionMode` non-YOLO default
  `"auto"`→`"smart"`; stale comment claiming "auto" was a live-verified
  session mode corrected.
- `devin_adapter_test.go`: mode-matrix expectations updated to `smart`
  (same uncommitted CP-70 changeset — the old expectation encoded the
  invalid value).
- `devin_mcp_provider_config_test.go` / `tooling_devin_check_test.go`
  (new): path resolution, write+check round-trip, idempotent re-ensure,
  missing-config status, bin-override + missing-binary checks.

## Live-test evidence (Windows, devin 3000.10.31, runner :4317)

Workspace `C:/working/fp-devin-sandbox`; log `runner-live.log`.

- **R1 chat** — run-1, session slug `trusted-airmail`. Boot
  `initialize`→`authenticate{devin-browser}` (PKCE ~3s, no click despite
  REPL "Not logged in") →`session/new`→`session/prompt`→`end_turn`;
  reply `DEVIN_LIVE_OK`.
- **R2 write** — `hello_devin.txt` created on disk; `tool_call`
  `write` lifecycle in_progress→completed.
- **R3 model switch** — `session/set_config_option{model:
  grok-4-5-low}` on the SAME slug (no new session); reply
  `MODEL_SWITCH_OK`; effort suffix `low` folded into model id.
- **R4 gate (post-fix)** — run-83 `yolo=false`, `rm -rf` prompt →
  `session/request_permission` with `allow_once/allow_session/
  allow_always/reject_once` → run `waiting_approval` `appr-97` →
  `POST /client/approvals/appr-97/decision {deny}` → adapter answered
  `reject_once`, tool failed "User rejected", `danger_dir` absent.
  Known ceiling: routine writes auto-approve under `smart` (provider
  judges them safe); stricter write-gating would need a
  `permissions.ask` config overlay — flagged for CP-70 note.
- **R5 resume** — runner restarted; `POST .../run-1/resume` restored
  `providerSessionId=trusted-airmail` (db-backed, no file check),
  replayed 10 events; new turn issued `session/load{trusted-airmail}`
  → `end_turn` `RESUMED_OK`.
- **R8 cancel** — `POST .../run-1/interrupt` → `session/cancel` →
  `stopReason:"cancelled"`, `agent_stopped{cause:"cancelled"}`.
- **R9 Grok regression** — run-120 `grok-4.5` `reasoningEffort=low`
  → `GROK_REGRESSION_OK`; base provider unaffected.
- **R10 usage** — normalized `token_usage_updated` events
  (`last/total`, `modelContextWindow:500000` from `usage_update.size`).

Deferred: R6 flow gates (needs a workflow), R7 child-agent isolation,
UI verification.

## Verification

- `go test ./internal/runner -run 'Devin'` — green (19.9s).
- `go test ./internal/tooling -run 'Devin'` — green.
- `go build ./...` + `go vet` — clean.
- Desktop `tsc --noEmit` — 1 pre-existing error in
  `store.chat-mode-persist.test.ts` (BUG-340 file, untouched here);
  all CP-70-edited files compile.
- Base regression: 5 TempDir file-lock flakes + 1 pre-existing
  `TestOpencodeProcessEnvIsolatesWindows` path-separator bug — none
  CP-70 related.
