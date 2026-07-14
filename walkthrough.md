# Jira-MCP-API-Token-Auth — Final Review Walkthrough

## Verdict

**PASS** (after NEEDS_FIX loop)

## What was fixed in the loop

| # | Prior finding | Fix |
|---|---------------|-----|
| 1 Critical | Desktop `apiToken` not stripped from Supabase config | `secretConfigFields.jira = ["apiToken"]` + unit test |
| 2 Important | Runbook still required Bearer | Manual E2E normalized to Basic primary |
| 3 Important | `EnsureClaudeJiraMcpConfig` Bearer→wrong URL | `jiraRemoteURLForAuthorization` |
| 4 Important | Thin Basic coverage | Codex/Gemini/Grok + Grok ACP exact Basic tests |
| 5 Important | Static Basic secret decision | Documented in CA-301 |

## Residual (non-blocking)

- GitNexus impact/detect not run (tools unavailable this session).
- Worktree still contains Firebase/Telegram Task-234 changes outside this Jira-only slice — split commits when shipping if desired.
- Live E2E against real Atlassian org still manual (admin must enable Rovo API token auth).

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run 'Jira|GrokACP|GrokServer|ExtraMCP|PreflightJira|EnsureGrok|EnsureClaudeJira' -count=1
# PASS (66 tests in re-review run)
```

## Ledger

- `change-audit/CA-301-jira-rovo-mcp-api-token-auth.md`
