# CA-266: Wire Production Google Drive mcp.driver Adapter

## Scope

Implemented Task-204: `mcp.driver` (CP-44's first external context source, Task-195) had only a test-only fake adapter — production always degraded to "no adapter configured". Wired a real Google Drive backing, reusing the exact same account/credential resolution chain the Chat-mode Google Drive MCP proxy already uses, plus a minimal UI surface to configure which file a flow reads.

## Changes

- `google_drive_proxy_mcp.go`: extracted `proxyMcpServer.accessToken()`/`proxyOAuthClient()`/`proxyAccountConnection()`'s bodies into standalone `resolveGoogleDriveAccessTokenForRunner`/`resolveGoogleDriveOAuthClientForRunner`/`resolveGoogleDriveAccountConnectionForRunner` functions taking `*Runner` directly (behavior-preserving; the methods are now thin wrappers) so a second caller with no `proxyMcpServer` of its own can reuse the same logic.
- `context_source_mcp.go`: new `googleDriveDriverAdapter{runner *Runner}` implements `MCPDriverAdapter` via the extracted resolver + `readGoogleDriveDocument`. New `ContextSourceRegistry.SetMCPDriverAdapter` mutates the already-registered `mcp.driver` source's adapter in place.
- `interactive_service.go`: `AttachRunner` wires `DefaultContextSourceRegistry().SetMCPDriverAdapter(&googleDriveDriverAdapter{runner: r})`.
- `artifact_type_registry.go`: new `resolveArtifactBoundMCPDriverRef` reads `config_json.mcpDriverFileId` off the same context_artifact.v1 output binding `resolveArtifactBoundContextSources` reads `sources` from.
- `behavior_registry.go`/`behavior_registry_builtin.go`/`flow_executor.go`: threaded `MCPDriverRef` through `BehaviorInput` → `FlowContextHints` → `startInlineEntryChain`'s dispatch.
- `WorkflowsSettings.tsx`: context_artifact.v1 instance editor gained a "Google Drive File ID" field, shown when its "MCP Driver (Google Drive)" source checkbox is ticked; updated the checkbox label (no longer "not yet wired").
- Tests: `context_source_mcp_production_test.go` (adapter-level: live fetch, missing-credential degrade, nil-runner guard, `SetMCPDriverAdapter` wiring), `artifact_type_registry_test.go` (+2, `resolveArtifactBoundMCPDriverRef`).

## Verification

- `go build ./...`, `go vet ./...` — clean.
- `go test ./internal/runner/ -run 'TestGoogleDriveDriverAdapter|TestSetMCPDriverAdapter|TestResolveArtifactBoundMCPDriverRef|TestProxyMcpAccessToken'` — all pass, confirming the extraction didn't change the pre-existing Google Drive proxy MCP behavior.
- `go test ./internal/...` — no new failures beyond the pre-existing, already-confirmed-unrelated environment-dependent flakes.
- `npm --prefix apps/desktop-flowpilot run typecheck` (`tsc --noEmit`) — clean.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-204
change_type: feature
summary: wire mcp.driver's production Google Drive adapter, reusing the Chat-mode MCP proxy's own account/credential resolution chain, plus a driver-ref config field on the context_artifact.v1 instance editor
# --->8---
