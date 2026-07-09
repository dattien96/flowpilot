# Task-204: Wire Production `mcp.driver` Adapter (Google Drive Backing)

## Metadata

- Document ID: `Task-204`
- Title: `Wire Production mcp.driver Adapter (Google Drive Backing)`
- Phase: `task`
- Status: `draft`
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

- `T-1` (proposed, confirm before coding) `FlowContextHints` gains an `AccountHome string` (or equivalent account-identity) field, populated by `behaviorContextProduce`'s caller from the same account-resolution path the interactive run already uses for its own Google Drive proxy calls — not a new/parallel resolution mechanism.
- `T-2` (proposed) The production adapter (`googleDriveDriverAdapter` or similar) implements `MCPDriverAdapter.Fetch(ctx, driverRef) (string, error)` by resolving an access token via the same credential-lookup chain `proxyMcpServer.accessToken()` uses (`s.proxyOAuthClient()` → `s.proxyAccountConnection()` → `s.runner.loadGoogleDriveCredentialByAccount(accountID)`), then calling `readGoogleDriveDocument(accessToken, driverRef)`.
- `T-3` (open) Should `accessToken()`'s resolution logic be extracted into a standalone, `proxyMcpServer`-independent function so both the MCP server path and this new context-source adapter path call the same code, instead of two places re-deriving Google Drive credentials? Recommended, to avoid drift between the two auth paths.

### Constraints

- Must reuse the existing Google Drive credential/account-resolution chain — do not invent a second way to look up which account backs a workspace/run.
- Must preserve Task-195's contract: `Deterministic()==true` (explicit lookup by `driverRef`, not search), bounded content, hard timeout, degrade-to-warning on failure — no change to those semantics, only to what backs `Fetch`.
- `mcp.driver` must remain excluded from `defaultContextSourceIDs` (opt-in only, per Task-194/196 binding) even after a real adapter is wired.
- Getting the wrong account/credentials must fail closed (adapter error → Collect degrades to warning) — never silently return another project's/account's content.

### Open Questions

- `Q-1` Exactly how does the interactive run currently know which Google Drive account is "active" for a given workspace/run, at the point `behaviorContextProduce`/`startInlineEntryChain` runs? (Needs the same research depth already done for `google_drive_proxy_mcp.go`, but for the *account selection* side — `proxyAccountConnection()` and its callers — before `T-1`/`T-2` can be implemented correctly.)
- `Q-2` Is `driverRef` (per `FlowContextHints.MCPDriverRef`, Task-195) a Google Drive file ID directly, or an indirection (e.g. a named "driver" config that maps to a file ID)? Task-196's UI/step-definition work only carries a context-source *id* (`mcp.driver`), not a per-step driver reference yet — a driver-ref input surface may need its own small UI addition.
- `Q-3` Per Task-204 `T-3`.

### Source Refs

- `Task-195` (seam, fake-adapter DOD-1).
- current code: `apps/local-runner/internal/runner/google_drive_proxy_mcp.go` (`readGoogleDriveDocument:446`, `proxyMcpServer.accessToken:879`, `proxyAccountConnection`, `s.runner.loadGoogleDriveCredentialByAccount`), `apps/local-runner/internal/runner/context_source_mcp.go` (`MCPDriverAdapter`, `mcpDriverSource`), `apps/local-runner/internal/runner/behavior_registry_builtin.go` (`behaviorContextProduce`), `apps/local-runner/internal/runner/flow_executor.go` (`startInlineEntryChain` — the one production dispatch site).

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

- `T-1` Answer `Q-1` (account resolution) via research before writing code.
- `T-2` Add account-identity to `FlowContextHints`, threaded from `startInlineEntryChain`.
- `T-3` Implement the production adapter per Key Decision `T-2`.
- `T-4` Register the production adapter as the default backing for `mcp.driver` on `DefaultContextSourceRegistry` (still excluded from `defaultContextSourceIDs`).
- `T-5` Tests: real-account-resolution unit tests (with fakes at the credential-lookup boundary, not live network calls), wrong-account-must-not-leak test, degrade-on-missing-credential test.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/context_source_mcp.go` (new production adapter type)
  - `apps/local-runner/internal/runner/flow_context_package.go` (`FlowContextHints` account field)
  - `apps/local-runner/internal/runner/flow_executor.go` (`startInlineEntryChain` — populate the new hint)
  - `apps/local-runner/internal/runner/google_drive_proxy_mcp.go` (possible extraction of a standalone credential-resolution helper, `T-3`/`Q-3`)
- modules: runner context assembly, Google Drive MCP integration
- routes: none
- tables: none

## 6. Acceptance Check

- A flow with `mcp.driver` enabled and a valid, connected Google Drive account + `driverRef` (file id) produces a section with real document content and a `SourceRef`.
- A flow with `mcp.driver` enabled but no connected/matching account degrades to a warning (existing Task-195 behavior), never errors the Plan step, and never returns another account's content.
- No regression to `internal/runner`'s existing Google Drive proxy MCP tests.

## 7. Out of Scope

- A driver-ref input UI surface (`Q-2`) if it turns out to be needed — separate small UI task.
- Any MCP backend other than Google Drive (e.g. Jira) — separate adapters per Task-195's own follow-up note.

## 8. Completion Notes

- result: `TBD`
- follow-ups: `TBD`
- upstream docs updated: `TBD`
