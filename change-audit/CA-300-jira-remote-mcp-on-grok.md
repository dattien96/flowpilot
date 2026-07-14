# CA-300: Jira remote MCP on Grok (G2)

## Summary

Close Task-234 Q-2 / G2 so Grok can use Atlassian remote Jira MCP like Claude/Codex:

1. `EnsureJiraMcpProviderConfig(providerKey=grok)` writes `[mcp_servers.jira]` into Grok `config.toml` (`url` + `headers.Authorization` + `enabled`) via generic-map TOML splice.
2. `grokACPExtraMCPServers` forwards HTTP-shaped extra servers (URL set, Command empty) into ACP `session/new` `mcpServers[]`, so live Grok turns receive Jira from `flowpilotClaudeExtraMCPServers` / `jiraLiveMCPServer`.
3. `PreflightJiraMcp("grok", …)` checks non-stale on-disk config (review finding).
4. UI copy and manual E2E runbook updated; `grokMcpServer` gains additive `URL`/`Headers`.

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run 'Jira|GrokACP|GrokServer|ExtraMCP|EnsureGrok|PreflightJira' -count=1
```

## Docs updated

- Task-234 §8: `Q-2` **CLOSED** + test list
- CP-05-06 DOD-4 / DOD-6 notes (Grok provider-config + preflight)
- Manual E2E runbook: G2 DONE, §11 automated tests, J-C* / J-R* Grok cases
- `implementation_plan.md` acceptance checklist, `walkthrough.md` PASS

## Source

- CP-05-06, Task-234 Q-2, implementation_plan.md (G2)

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: Task-234
change_type: feature
summary: Wire Jira remote MCP for Grok (config.toml + ACP HTTP forward + preflight)
# --->8---
