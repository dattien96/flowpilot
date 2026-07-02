# CA-170: Gate submit_review_outcome to Flow-Hub Turns Only (BUG-NOTE-CP42 #24)

## Scope

Verified and fixed the last P1 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: `submit_review_outcome` was unconditionally exposed to every turn on every provider, including plain `normal_chat`.

## The bug

Codex's `SendTurn` always included `codexReviewOutcomeDynamicTool()` in `dynamicTools` (`codex_adapter.go`); Claude's MCP `tools/list` handler always returned `submit_review_outcome` via `claudeMCPToolDefs()` (`claude_mcp_server.go`). CP-36's design explicitly says normal chat must not offer this tool — only a flow's hub, after a reviewer cohort joins, should be able to call it. Worse: if a model in ordinary chat called it anyway, `turnBridge.SubmitFlowControl` routes straight to `applyFlowControl`, which only checks that the target run exists before mutating its loop state (`round++`, cap checks, status transitions) — there was no check that the run was actually participating in a flow at all.

## Fix

Added `TurnRequest.OfferReviewOutcomeTool bool` (`provider_registry.go`), set in `runTurn` (`interactive_service.go`) from `rs.autoOrchestrate` — the existing flag that's true only for a run genuinely acting as a flow hub (set the moment `startResolvedFlow` or an AI-driven `spawn_agent(autoOrchestrate: true)` call starts a flow's entry node). It's false for plain `normal_chat` runs and for spawned reviewer/coder children (whose own `autoOrchestrate` is independently false regardless of their parent's), so this single flag correctly covers both halves of the bug note's concern — including doubling as extra insurance against BUG#13/CA-167's reviewer-cohort issue, since a reviewer's `dynamicTools`/MCP tool list no longer even contains the tool.

- **Codex** (`codex_adapter.go`): `dynamicTools` only appends `codexReviewOutcomeDynamicTool()` when `req.OfferReviewOutcomeTool`. A new per-thread `allowReviewOutcome map[string]bool` (set alongside `bridges[threadID]`, cleaned up in the same `defer`) lets `handleDynamicToolCall` re-check the flag before acting — defense in depth against a model calling the tool anyway (e.g. from stale resumed-session context that still remembers the tool from an earlier turn).
- **Claude** (`claude_mcp_server.go`): `register(bridge, allowReviewOutcome bool)` now takes the flag (stored per-token, cleaned up in `unregister`). `tools/list` passes it to `claudeMCPToolDefs(allowReviewOutcome)`, which omits the `submit_review_outcome` entry when false. `tools/call`'s `submit_review_outcome` case re-checks the same per-token flag before dispatching to `handleClaudeSubmitReviewOutcome`, for the same defense-in-depth reason.
- `claude_adapter.go`'s `a.mcpServer.register(bridge)` call site now passes `req.OfferReviewOutcomeTool` through.

## Verification

- New tests: `TestCodexAdapterDoesNotAdvertiseReviewOutcomeToolByDefault` (asserts `thread/start`'s `dynamicTools` omits the tool for a plain turn) and `TestCodexAdapterRejectsReviewOutcomeCallWhenNotOffered` (asserts a call anyway is rejected and never reaches `SubmitFlowControl`), `TestClaudeMCPToolsListIncludesReviewOutcomeOnlyWhenAllowed` and `TestClaudeMCPSubmitReviewOutcomeRejectedWhenNotAllowed` (Claude-side mirrors of the same two properties).
- Fixed two pre-existing tests that (correctly) needed updating for the new default-off behavior: `TestClaudeMCPInitializeAndList` (an unregistered/no-token `tools/list` now returns 3 tools, not 4) and `TestCodexAdapterSubmitReviewOutcomeDynamicToolRouting` (now explicitly sets `OfferReviewOutcomeTool: true`, since it specifically tests the tool's routing when legitimately offered).
- Full suite: 1001 passed, 15 pre-existing/environmental failures (unchanged from before this fix — Windows paths, missing local `codex` CLI, fixture assumptions — none in a file touched by this change).
- `go build ./...` and `go vet ./...` clean (only the pre-existing, unrelated `gitnexus.go` vet warning remains).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: gate submit_review_outcome's exposure (both as a Codex dynamicTool and a Claude MCP tool) to turns whose run is actually acting as a flow hub, with defense-in-depth rejection at call time, instead of unconditionally offering it to every turn including normal chat
# --->8---
