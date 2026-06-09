# CP-13: Direct MCP Runtime for AI Providers

## Metadata

- Document ID: `CP-13`
- Title: `Direct MCP Runtime for AI Providers`
- Phase: `coding_plan`
- Status: `superseded`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `TBD`
- Last Updated: `2026-06-09`
- Parent Documents: [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [CP-05-03: Google Drive MCP Current Implementation Notes](../priority/CP-05-03-Driver-Mcp.md)
- Child Documents: `None`
- Related Documents: [CP-29: FlowPilot Proxy MCP Server For Google Drive](../priority/CP-29-MCP-Proxy-Google-Drive.md)
- Replaces: `None`
- Tags: `mcp, ai-provider, direct-runtime, superseded`

## AI Quick View

### Summary

- CP-13 originally proposed direct provider-side MCP runtime.
- The read-only version of this runtime model is now covered by CP-05-03.
- CP-05-03 is the current source of truth for provider-owned Google Drive MCP runtime, provider config generation, prompt augmentation, preflight, and provider-driven smoke tests.
- CP-13 should not be implemented as a separate runner-direct MCP client plan.
- Future write-mode/per-request approval work belongs to CP-27.

### Current Ask

- Keep CP-13 as historical context and redirect implementation to CP-05-03.

### Key Decisions

- `P-1` Treat CP-05-03 as the implemented direct provider MCP runtime plan for Google Drive read-only tools.
- `P-2` Do not add a separate direct MCP orchestration layer from CP-13.
- `P-3` Use the CP-05-03 provider-owned pattern for future read-only MCP integrations.
- `P-4` Use CP-27 for self-hosted MCP server and write-mode approval work.

### Constraints

- CP-05-03 supports Phase A and Phase B read-only behavior only.
- FlowPilot currently does not observe each provider-side MCP tool call.
- Write tools must remain out of CP-13 and CP-05-03.
- Other MCPs such as SonarQube, Jira, Firebase, and Telegram still need their own read-only provider config/preflight/prompt/test extensions if they are added.

### Open Questions

- Should the generic multi-MCP provider-owned runtime be captured in a new coding plan, or should each MCP type extend CP-05-03-style patterns independently?
- Which non-Google MCP should be implemented next after Google Drive read-only runtime?

### Source Refs

- CP-05-03 Section 11: provider CLI owns MCP tool calls
- CP-05-03 Sections 11.13 and 11.14: Phase A provider config/smoke test and Phase B workflow prompt runtime
- CP-27: self-hosted MCP server for future write-mode/per-request approval

## 1. Goal

This document no longer defines active implementation work.

The original goal was to enable the AI provider itself to connect to MCP backends directly, instead of FlowPilot always fetching MCP data and attaching it to the prompt first.

That goal is now represented by CP-05-03 for Google Drive read-only runtime:

```text
workflow step requires google_drive
-> runner validates local Google Drive MCP setup/auth
-> runner ensures selected AI provider account has google-drive MCP config
-> runner improves the workflow prompt with strict MCP usage instructions
-> runner launches Claude/Codex/Gemini using the existing provider CLI path
-> provider CLI starts/connects to google-drive-mcp from its own MCP config
-> provider CLI calls Drive MCP tools
-> provider CLI returns output to runner
```

## 2. Input Documents

- [CP-05-03: Google Drive MCP Current Implementation Notes](../priority/CP-05-03-Driver-Mcp.md)
- [CP-29: FlowPilot Proxy MCP Server For Google Drive](../priority/CP-29-MCP-Proxy-Google-Drive.md)
- [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)

## 3. Implementation Strategy

No standalone implementation should be started from CP-13.

Use this mapping instead:

```text
CP-13 original intent
-> covered by CP-05-03 for Google Drive read-only provider-owned MCP runtime

CP-13 write-capable or exact tool approval needs
-> moved to CP-27 self-hosted MCP server design

CP-13 generic multi-MCP expansion
-> create a new MCP-specific plan or generic provider-MCP registry plan
```

### 3.1 What CP-05-03 Already Covers

CP-05-03 covers the read-only direct-runtime pattern:

- provider account-home aware config generation
- provider MCP server config for `google-drive`
- read-only tool allowlists where supported
- Google Drive MCP auth/setup preflight
- provider config preflight
- prompt augmentation for `requiredMcps: ["google_drive"]`
- provider-driven MCP smoke tests
- workflow runtime behavior through the provider CLI
- explicit MCP failure-code handling

### 3.2 What CP-13 Must Not Reintroduce

Do not reintroduce:

- runner-direct MCP `tools/call` execution for normal workflow steps
- a separate generic direct-runtime launcher that duplicates CP-05-03
- write tools in provider config
- prompt-only write approval
- Telegram as an MCP backend by default

### 3.3 Future Multi-MCP Read-Only Pattern

For SonarQube, Jira, Firebase, Telegram-like integrations, follow the CP-05-03 shape where the integration is safe as read-only:

1. Add MCP type registry entry.
2. Define provider-visible server name.
3. Define command/args/env or remote endpoint details.
4. Define read-only tool allowlist.
5. Add setup and auth readiness checks.
6. Add provider config generation for Codex, Claude, and Gemini where supported.
7. Add provider config preflight.
8. Add prompt instructions for `requiredMcps`.
9. Add provider-driven smoke tests.
10. Keep write/side-effect tools disabled unless CP-27-style approval exists.

## 4. Work Breakdown

No active CP-13 work remains.

If a future agent is asked to implement direct MCP runtime, it must start from CP-05-03 and CP-27 instead:

- `P-1` For Google Drive read-only runtime, use CP-05-03.
- `P-2` For write-mode approval, use CP-27.
- `P-3` For another read-only MCP type, create a new plan that copies the CP-05-03 provider-owned runtime pattern.
- `P-4` For a generic provider-MCP registry, create a new coding plan rather than modifying CP-13.

## 5. Touched Areas

No new touched areas from CP-13.

Historical areas now owned by CP-05-03:

- `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go`
- `apps/local-runner/internal/runner/mcp_prompt_instructions.go`
- `apps/local-runner/internal/runner/sessions.go`
- `apps/local-runner/internal/runner/runner.go`
- `apps/local-runner/internal/cli/root.go`
- `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
- `apps/admin-web/src/routes/_authenticated/settings/mcp-servers/mcp-connect-test.tsx`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`

## 6. Data or Migration Steps

No CP-13 migration is required.

Any generic multi-MCP expansion should define its own migration only if it needs new persistent fields.

## 7. Validation Plan

Validate CP-05-03 instead of CP-13:

- Google Drive MCP install, verify, auth, and refresh
- provider config write for at least one provider account
- provider-driven MCP smoke test
- workflow step with `requiredMcps: ["google_drive"]`
- missing auth preflight
- missing provider config preflight
- explicit MCP failure marker handling
- quoted guidance false-positive check

## 8. Rollout and Fallback

No CP-13 rollout.

Fallback remains:

- use CP-05-03 read-only provider-owned MCP runtime for Google Drive
- use prompt hydration or no MCP for unsupported integrations
- use CP-27 only when self-hosted MCP/write approval is ready

## 9. Risks

- `R-1` Future agents may try to implement CP-13 as a duplicate direct-runtime layer. Mitigation: this document is marked `superseded` and points to CP-05-03.
- `R-2` Teams may assume CP-05-03 covers every MCP type. Mitigation: CP-05-03 currently proves the Google Drive pattern only.
- `R-3` Teams may enable write tools without self-host approval. Mitigation: CP-13 explicitly redirects write-mode work to CP-27.

## 10. Definition of Done

- CP-13 is marked `superseded`.
- CP-13 points to CP-05-03 as the current source of truth for read-only direct provider MCP runtime.
- CP-13 points to CP-27 for future self-hosted MCP/write-mode approval.
- No active implementation remains in CP-13.
