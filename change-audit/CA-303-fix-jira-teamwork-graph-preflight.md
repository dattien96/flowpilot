# CA-303: Fix Jira Teamwork Graph bootstrap instructions

## Summary

Tightened the Jira MCP prompt contract so provider-driven Jira MCP turns stop generating invalid Teamwork Graph bootstrap calls.

- `buildJiraMcpInstructions` now tells the model to call `getAccessibleAtlassianResources` first and use the returned resource `id` as `cloudId`.
- The same block now tells the model to call `atlassianUserInfo` before any Teamwork Graph lookup of the current user, then use the returned `account_id` as `objectIdentifier`.
- The Jira MCP instructions now explicitly forbid the two payload mistakes observed in live testing:
  - `cloudId: ""`
  - `objectIdentifier: "current"` for `getTeamworkGraphContext`
- The instructions also now steer the model away from Teamwork Graph for basic ticket reads and toward direct Jira tools first.
- Added a regression test that locks the new bootstrap instructions into the injected Jira MCP block.

## Why this failed

The bad Teamwork Graph payload was not hardcoded in FlowPilot. Live calls showed Atlassian MCP accepted the credential and listed accessible resources, but the model still chose an invalid Teamwork Graph bootstrap request:

- empty `cloudId`
- `objectIdentifier: "current"` with `objectType: "AtlassianUser"`

Atlassian returned:

`We couldn't verify your connection settings. Please contact your administrator for assistance.`

That made the failure look like a token or admin-policy problem even though the underlying issue was argument selection during MCP tool use.

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run 'TestInjectRequiredMcpInstructionsJiraProducesJiraBlockNotDrive|TestPreflightJiraMcp' -count=1
```

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-06
change_type: bugfix
summary: tighten Jira MCP instructions so Teamwork Graph bootstrap uses real cloudId/account_id instead of empty/current placeholders
# --->8---
