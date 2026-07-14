# CA-311: Prefix FlowPilot MCP server names with `flowpilot_`

## Summary

Renamed the four FlowPilot-managed MCP **server names** (the namespace key the
AI model calls and the key written into each provider's config) to a
`flowpilot_` prefix so they can be mentioned unambiguously and never collide
with a provider's own native connectors (e.g. Codex's built-in Google Drive
connector, which is literally named `google-drive`):

| constant | old value | new value |
|---|---|---|
| `jiraMcpServerName` | `jira` | `flowpilot_jira` |
| `googleDriveMcpServerName` | `google-drive` | `flowpilot_drive` |
| `firebaseMcpServerName` | `firebase` | `flowpilot_firebase` |
| `telegramMcpServerName` | `telegram` | `flowpilot_telegram` |

Because almost every use site references the CONSTANT (not a literal), changing
the constant value propagates across all provider-config writers, the per-turn
live merge, preflight, and detection. Additional changes:

- **Prompt/instruction + error text**: every model- and user-facing string that
  names the SERVER (e.g. "use tools from the `jira` server", "If `google-drive`
  is unavailable", preflight "not configured with the … MCP server", the
  Telegram write-contract "on the `telegram` MCP server", the per-step Jira/
  Firebase target notes in `flow_executor.go`) now interpolates the constant, so
  the model is told the real namespace to call.
- **Migration (auto-remove old entries)**: each provider-config writer now
  deletes the pre-rename key (`jira`/`google-drive`/`firebase`/`telegram`) via
  the shared `legacyMcpServerName(current)` helper when it writes the new one,
  so old and new never coexist — and the legacy `google-drive` entry stops
  shadowing Codex's native connector. The Codex generic-map splice
  (`spliceCodexServerEntry`) and the Drive live-sync do this too.

## NOT changed (deliberately)

- The **required-MCP KEYS** used by workflow steps and `mcpInstructionSpecs`
  (`google_drive`, `jira`, `firebase`) — these are workflow-facing identifiers,
  not server names; changing them would break existing step configs. The
  "requires FlowPilot MCP `google_drive`" line still uses the required-key.
- **Credential / backend-state keys** (`jiraCredentialKey`, `records["jira"]`,
  `detect*Backend`, provider-account keys) and **providerType / integration-type**
  literals — all independent of the MCP server name.

## Why

Reported live: asking Codex "which Google account is connected" returned a
different account than Claude/Grok. Root cause was Codex's OWN native
`google-drive` connector answering instead of FlowPilot's MCP — the shared
name `google-drive` made "google-drive" ambiguous. Prefixing FlowPilot's
servers with `flowpilot_` lets the user (and the model) target FlowPilot's MCP
explicitly (`flowpilot_drive`, `flowpilot_jira`).

## Verification

```bash
cd apps/local-runner
go build ./... && go vet ./internal/runner/
go test ./internal/runner -run 'GoogleDrive|Jira|Firebase|Telegram|Mcp|Provider|ExtraMCP|FlowExecutor|Artifact' -count=1
```

Server-name/prompt/config tests pass. Remaining failures are the pre-existing
set unrelated to this change (real `codex` binary needed, Windows-path env,
skills-merge, session-home) — identical on a clean tree.

> Migration note: existing on-disk provider configs keep the old-named entry
> until the next Configure Providers / live sync rewrites them, at which point
> the old entry is removed and the `flowpilot_`-named one written.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-06
change_type: refactor
summary: rename FlowPilot MCP server names to flowpilot_jira/flowpilot_drive/flowpilot_firebase/flowpilot_telegram (model-facing namespace + config keys + prompts), auto-removing the old-named entries, to avoid collision with providers' native connectors
# --->8---
