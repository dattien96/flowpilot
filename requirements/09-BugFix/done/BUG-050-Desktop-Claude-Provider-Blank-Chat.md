# BUG-050: Desktop Claude Provider Blank Chat

## Metadata

- Document ID: `BUG-050`
- Title: `Desktop Claude provider blank chat`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot maintainers`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-009-Run-With-Gemini-Failed.md`, `requirements/09-BugFix/done/BUG-010-Run-With-Claude-Failed.md`, `change-audit/CA-060-fix-desktop-claude-provider-run-start.md`
- Replaces: `none`
- Tags: `desktop, local-runner, claude, provider-runtime, regression`

## AI Quick View

### Summary

- Selecting Claude in the desktop direct chat cleared the composer but left the chat area empty.
- Codex worked because the live runner registry always had a selectable Codex adapter.
- Claude failed before the prompt was rendered because the live registry still required `FLOWPILOT_CLAUDE_ADAPTER`.
- The desktop store also swallowed rejected `startRun` promises with no visible timeline error.

### Current Ask

- Make Claude selectable in the live runner without the old feature flag and ensure failed run starts show the user's prompt plus an error.

### Key Decisions

- `F-1` Keep `DefaultProviderRegistry()` placeholder behavior for demos and registry tests.
- `F-2` Register Claude as available in `ProviderRegistryFor(r)` because that is the live desktop runner path.
- `F-3` Render the prompt/thinking row before `startRun` and replace thinking with a system error on failure.
- `V-1` Verify the live runner registry selects Claude without `FLOWPILOT_CLAUDE_ADAPTER`.
- `V-2` Verify desktop TypeScript compiles with the run-start error path.

### Constraints

- GitNexus MCP tools were not exposed in this thread, so required symbol impact checks were performed with local call-graph search and diff review.
- The default registry must remain placeholder-only for Claude so existing non-live tests do not require a real `claude` binary.

### Open Questions

- A full live desktop Claude turn should still be exercised manually against the user's authenticated Claude account.

### Source Refs

- User report on `2026-06-15`: Claude provider send clears/empties UI while Codex works.
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/local-runner/internal/runner/provider_registry.go`
- `apps/local-runner/internal/runner/claude_adapter_test.go`
- `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`

## 1. Issue Summary

When a user selected a project and workflow, chose Claude as the provider, typed a prompt, and sent it from the desktop chat, the composer cleared and the UI became empty with no run activity or visible error. The same flow worked with Codex.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: Desktop app connected to the local Go runner on `2026-06-15`.
- reproduction steps:
  1. Open desktop chat.
  2. Select a project and workflow.
  3. Select `codex`, send a prompt, and observe a run starts.
  4. Select `claude`, send a prompt.
- frequency: Reproducible whenever the live runner did not have `FLOWPILOT_CLAUDE_ADAPTER=1`.

## 4. Expected vs Actual

- expected: Claude behaves like a selectable provider in the live runner. If a run cannot start, the prompt remains visible and the timeline shows the failure.
- actual: `startRun({ providerKey: "claude" })` returned `422 provider_unavailable`; the desktop cleared the input and did not render either the prompt or the error.

## 5. Impact

- users affected: Desktop users trying to run a workflow or direct chat with Claude.
- workflows affected: Provider-selection launches where Claude is chosen explicitly.
- severity: `high` for Claude workflows because the UI looked blank and no actionable error appeared.

## 6. Root Cause

- hypothesis: Claude was still registered as a non-selectable placeholder or the desktop failed to handle run-start errors.
- confirmed cause: Both conditions were present. `ProviderRegistryFor(r)` only registered Claude as available when `FLOWPILOT_CLAUDE_ADAPTER` was set, while the desktop provider selector allowed choosing Claude. `sendPrompt` awaited `client.startRun(...)` before adding any timeline items and had no catch path visible to the user.
- evidence: Local code inspection showed `ProviderRegistryFor` gated Claude behind `claudeAdapterEnabled()`, `createRun` rejected non-selectable providers with `provider_unavailable`, and `ChatInput.send()` intentionally ignored the returned promise after clearing the input.

## 7. Fix Strategy

- `F-1` Updated `ProviderRegistryFor(r)` to register Claude as `ProviderStatusAvailable` in the live runner without `FLOWPILOT_CLAUDE_ADAPTER`.
- `F-2` Removed the old Claude adapter env-gate helper from the runner process layer.
- `F-3` Updated `TestClaudeRegistryGating` to assert default registry placeholder behavior and live runner availability.
- `F-4` Completed the desktop `sendPrompt` error path by adding `runErrorMessage`, keeping the prompt visible, removing the thinking row, and appending a system error when `startRun` or `sendTurn` fails.
- `F-5` Updated `07-Claude-Adapter-Plan.md` so operator instructions no longer require `FLOWPILOT_CLAUDE_ADAPTER`.

## 8. Validation

- `V-1` `go test ./internal/runner -run TestClaudeRegistryGating` from `apps/local-runner` passed.
- `V-2` `npm run typecheck` from `apps/desktop-flowpilot` passed.

## 9. Regression Guard

- tests: `TestClaudeRegistryGating` now guards that Claude remains placeholder-only in `DefaultProviderRegistry()` but is selectable in the live runner registry.
- alerts: none added.
- audit checks: `change-audit/CA-060-fix-desktop-claude-provider-run-start.md` records the runtime and UI behavior change.

## 10. Follow-Up Document Updates

- upstream docs that must change: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md` was updated to remove the obsolete Claude adapter feature-flag requirement.
- notes left unchanged on purpose: `SS-05` and `SD-06` already describe Claude as a supported provider and do not need behavior changes for this bug.
