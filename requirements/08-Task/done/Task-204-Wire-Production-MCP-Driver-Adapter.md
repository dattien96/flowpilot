# Task-204: Wire Production `mcp.driver` Adapter (Google Drive Backing)

## Metadata

- Document ID: `Task-204`
- Title: `Wire Production mcp.driver Adapter (Google Drive Backing)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)
- Child Documents: `None`
- Related Documents: [Task-195: MCP-Backed Context Source Adapter](Task-195-MCP-Backed-Context-Source-Adapter.md), [Task-196: Per-Step Context Source Selection UI](Task-196-Per-Step-Context-Source-Selection-UI.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, context-source, mcp, google-drive, follow-up`

## AI Quick View

### Summary

- Task-195 shipped the `mcp.driver` context source **seam only**: `MCPDriverAdapter` interface, timeout, bounded content, degrade-on-error — verified end-to-end with a **fake adapter** (its own DOD-1 explicitly scopes to "fake adapter", not a live backend).
- Research during this session found a real, usable backing primitive: `readGoogleDriveDocument(accessToken, fileID) (string, error)` in `apps/local-runner/internal/runner/google_drive_proxy_mcp.go:446` — this is the "driver file" mechanism already working in Chat mode the user referenced.
- The blocker: `accessToken()` (`google_drive_proxy_mcp.go:879`) is a method on `proxyMcpServer{runner, workspace, accountHome, ...}` — resolving *which* Google Drive account backs a given Plan-time context build requires account/credential context that `ContextSource.Fetch(ctx, hints FlowContextHints)` does not currently carry.
- Wiring this correctly means threading account-resolution context through `FlowContextHints` → `behaviorContextProduce` → its one production call site (`flow_executor.go`'s `startInlineEntryChain`), which does have access to the interactive run's resolved account. Getting this wrong risks fetching content under the wrong Google Drive account — a real correctness/security issue, not just a missing feature — so this was deliberately **not implemented blind** in the same session as CP-44's Task-191..196, and parked here for a scoped follow-up with explicit design review.

### Current Ask

- Design and implement a production `MCPDriverAdapter` backed by `readGoogleDriveDocument`, with correct per-run/per-project Google Drive account resolution, and register it (opt-in, not in `defaultContextSourceIDs`) so `mcp.driver` becomes a real, live context source instead of only a tested seam.

### Key Decisions

- `T-1` **(RESOLVED, differs from the original proposal)** No `AccountHome`/account-identity field was added to `FlowContextHints` at all. Research (`Q-1`) found the entire account-resolution chain (`accessToken()` → `proxyOAuthClient()` → `proxyAccountConnection()` → `loadGoogleDriveCredentialByAccount()`) depends only on `*Runner` (workspace-scoped config files: `loadGoogleDriveWorkspaceConfigFile`, `resolveGoogleDriveProxyAccountSelection`, `loadArtifactStorageGoogleDriveState`), never on any per-run/per-interactive-run state. The production adapter is bound to a `*Runner` once (at `AttachRunner`), not re-resolved per Plan-time call — there was nothing to thread through hints.
- `T-2` **(DONE, as proposed)** `googleDriveDriverAdapter{runner *Runner}` implements `MCPDriverAdapter.Fetch(ctx, driverRef) (string, error)` via `resolveGoogleDriveAccessTokenForRunner(runner)` then `readGoogleDriveDocument(accessToken, driverRef)`.
- `T-3` **(RESOLVED: yes, extracted)** `accessToken()`, `proxyOAuthClient()`, and `proxyAccountConnection()` on `proxyMcpServer` are now thin wrappers around new standalone functions (`resolveGoogleDriveAccessTokenForRunner`, `resolveGoogleDriveOAuthClientForRunner`, `resolveGoogleDriveAccountConnectionForRunner`) taking `*Runner` directly — a pure, behavior-preserving extraction (confirmed via the pre-existing `TestProxyMcpAccessToken_*` suite, all still passing unchanged). Both the MCP proxy path and the new adapter now call the exact same code.

### Constraints

- Must reuse the existing Google Drive credential/account-resolution chain — do not invent a second way to look up which account backs a workspace/run. — satisfied via `T-3`'s extraction.
- Must preserve Task-195's contract: `Deterministic()==true` (explicit lookup by `driverRef`, not search), bounded content, hard timeout, degrade-to-warning on failure — no change to those semantics, only to what backs `Fetch`. — unchanged; `mcpDriverSource.Fetch` itself was not touched, only what `adapter` points to.
- `mcp.driver` must remain excluded from `defaultContextSourceIDs` (opt-in only, per Task-194/196 binding) even after a real adapter is wired. — unchanged; `SetMCPDriverAdapter` only swaps the adapter, never touches `defaultContextSourceIDs`.
- Getting the wrong account/credentials must fail closed (adapter error → Collect degrades to warning) — never silently return another project's/account's content. — `Fetch` returns an error (never a fallback account's content) on any resolution failure; `TestGoogleDriveDriverAdapterDegradesOnMissingCredential` covers the no-connected-account case.

### Open Questions

- `Q-1` **(RESOLVED)** See `T-1` above — the chain is `*Runner`-scoped (workspace config on disk), not per-run.
- `Q-2` **(RESOLVED)** `driverRef` is the Google Drive file id directly, no indirection — `readGoogleDriveDocument` already takes one. A small UI surface WAS added (originally deferred to Out of Scope, but done in this same pass at the owner's request for genuine end-to-end usability): a "Google Drive File ID" text field on a `context_artifact.v1` artifact instance's editor (`WorkflowsSettings.tsx`, Artifacts tab), shown when its "MCP Driver (Google Drive)" source checkbox is ticked, writing `config_json.mcpDriverFileId`. Threaded via a new `resolveArtifactBoundMCPDriverRef` (sibling to `resolveArtifactBoundContextSources`) → `BehaviorInput.MCPDriverRef` → `FlowContextHints.MCPDriverRef`.
- `Q-3` Resolved as part of `T-3`.

### Source Refs

- `Task-195` (seam, fake-adapter DOD-1).
- code: `apps/local-runner/internal/runner/google_drive_proxy_mcp.go` (`resolveGoogleDriveAccessTokenForRunner`, `resolveGoogleDriveOAuthClientForRunner`, `resolveGoogleDriveAccountConnectionForRunner`, `readGoogleDriveDocument`), `apps/local-runner/internal/runner/context_source_mcp.go` (`googleDriveDriverAdapter`, `ContextSourceRegistry.SetMCPDriverAdapter`), `apps/local-runner/internal/runner/artifact_type_registry.go` (`resolveArtifactBoundMCPDriverRef`), `apps/local-runner/internal/runner/behavior_registry.go` (`BehaviorInput.MCPDriverRef`), `apps/local-runner/internal/runner/behavior_registry_builtin.go` (`behaviorContextProduce`), `apps/local-runner/internal/runner/flow_executor.go` (`startInlineEntryChain`), `apps/local-runner/internal/runner/interactive_service.go` (`AttachRunner`), `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (driver-ref field, `contextSourceOptions` label update).

## 1. Goal

Give `mcp.driver` a real, correctly-account-scoped backing implementation, so a flow that opts into it (Task-194/196 binding) actually retrieves live Google Drive document content at Plan-time instead of always degrading with "no adapter configured".

## 2. Parent Links

- coding plan: `CP-44`
- tech design: `SD-22`
- system spec: `SS-14` (`US-9`, `AC-16`)
- specific upstream ids: `CP-44 P-5`, `Task-195`

## 3. Trigger

Discovered while closing out CP-44 (Task-191..196): the `mcp.driver` seam is complete and tested, but wiring it to a real backend surfaced a genuine open design question (account resolution) that the owner asked to park as a scoped follow-up rather than have implemented without review.

## 4. Exact Change

- `T-1` Answer `Q-1` (account resolution) via research before writing code. — done, see Key Decisions.
- `T-2` Add account-identity to `FlowContextHints`, threaded from `startInlineEntryChain`. — superseded; not needed (T-1).
- `T-3` Implement the production adapter per Key Decision `T-2`. — done: `googleDriveDriverAdapter`.
- `T-4` Register the production adapter as the default backing for `mcp.driver` on `DefaultContextSourceRegistry` (still excluded from `defaultContextSourceIDs`). — done: `AttachRunner` calls `DefaultContextSourceRegistry().SetMCPDriverAdapter(...)`.
- `T-5` Tests: real-account-resolution unit tests (with fakes at the credential-lookup boundary, not live network calls), wrong-account-must-not-leak test, degrade-on-missing-credential test. — done, see §6/§8.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/context_source_mcp.go` (`googleDriveDriverAdapter`, `ContextSourceRegistry.SetMCPDriverAdapter`)
  - `apps/local-runner/internal/runner/artifact_type_registry.go` (`resolveArtifactBoundMCPDriverRef`)
  - `apps/local-runner/internal/runner/behavior_registry.go` (`BehaviorInput.MCPDriverRef`)
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (`behaviorContextProduce` copies it into `FlowContextHints`)
  - `apps/local-runner/internal/runner/flow_executor.go` (`startInlineEntryChain` resolves + passes `MCPDriverRef`)
  - `apps/local-runner/internal/runner/interactive_service.go` (`AttachRunner` wires the adapter)
  - `apps/local-runner/internal/runner/google_drive_proxy_mcp.go` (extracted `resolveGoogleDriveAccessTokenForRunner`/`resolveGoogleDriveOAuthClientForRunner`/`resolveGoogleDriveAccountConnectionForRunner`)
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (driver-ref UI field, `contextSourceOptions` label)
- modules: runner context assembly, Google Drive MCP integration, desktop Artifacts tab
- routes: none
- tables: none (uses `config_json` on the existing `artifact_instances` row, no schema change)

## 6. Acceptance Check

- A flow with `mcp.driver` enabled and a valid, connected Google Drive account + `driverRef` (file id) produces a section with real document content and a `SourceRef`. — `TestGoogleDriveDriverAdapterFetchesLiveDocument`.
- A flow with `mcp.driver` enabled but no connected/matching account degrades to a warning (existing Task-195 behavior), never errors the Plan step, and never returns another account's content. — `TestGoogleDriveDriverAdapterDegradesOnMissingCredential` (adapter-level); `TestMcpDriverSourceAdapterErrorDegrades` (pre-existing, Collect-level degrade, unaffected).
- No regression to `internal/runner`'s existing Google Drive proxy MCP tests. — confirmed: the full pre-existing `TestProxyMcpAccessToken_*` suite passes unchanged after the extraction.

## 7. Out of Scope

- ~~A driver-ref input UI surface (`Q-2`) if it turns out to be needed — separate small UI task.~~ Done in this pass instead — see `Q-2`.
- Any MCP backend other than Google Drive (e.g. Jira) — separate adapters per Task-195's own follow-up note.

## 8. Completion Notes

- result: `done` — production `mcp.driver` adapter implemented, wired, tested, and given a minimal config UI (Google Drive File ID field). `go build`/`go vet`/`go test ./internal/...` clean; the only failures present are the same pre-existing environment-dependent flakes (Codex/Claude CLI resume, skills-merge, live provider tests) already confirmed unrelated earlier in this session.
- follow-ups: none required for this task's own scope. A natural next step (not requested, not started) would be a non-Google-Drive `MCPDriverAdapter` (e.g. Jira) — explicitly out of scope per Task-195's own note.
- upstream docs updated: [CP-44](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md) §11.5/§11.6 should be re-annotated to note mcp.driver now has a real production backing (previously noted as fake-adapter-only).
