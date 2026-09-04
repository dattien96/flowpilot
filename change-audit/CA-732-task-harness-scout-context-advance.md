# CA-732 — task-harness scout→context→plan_writer auto-advance (run-198699 hub_stalled retry loop)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: bugfix
summary: tryAdvanceFlowThroughInline now dispatches context.produce mid-flow (task-harness scout -> context -> plan_writer), and a continue onto a never-spawned reinvoke-lifecycle agent.delegate spawns a fresh child — eliminating the run-198699 S2 retry loop (scout completion reinvoked the hub, which prose-answered without submit_review_outcome and hub_stalled; each Retry continue no-op'd on a nonexistent plan_writer)
# --->8---

## Problem

- Live run-198699 (`/flow task-harness`, S2): `preflight_contract_plan` (Scout) completed → `flow_advance_targets_resolved` pointed at `context`, but `tryAdvanceFlowThroughInline` had no `context.produce` case (only validate/audit/notify/freeze — the comment documented context as freeze-chain-only). The scout's completion fell back to `advanceOrNotifyHub`'s note+reinvoke-hub path.
- The hub then prose-answered ("…Review submitted as approved, advancing…") WITHOUT calling the MCP `submit_review_outcome`; no `flow_control_received`; after 2m the F-0 watchdog parked `blocked/hub_stalled`.
- Retry (continue) resolved `plan_synthesis → plan_writer` (back-edge) but plan_writer (`lifecycle: reinvoke`, `agent.delegate`) had no child; `reinvokeMatchingFlowChild` no-op'd silently → `plan_writer` step RUNNING with nothing behind it → 2m later `hub_stalled` again (reproduced 2-3 Retries).
- Second UX issue: the TUI blocked banner hardcoded "click Retry / Stop / Allow above" but `hub_stalled`/`cap`/`member_stalled` parks render only `[Retry] [Stop]` (Allow is frozen-contract-drift-only, Task-309).

## Changes

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`:
  - `flowNodeInlineDispatchable` + `tryAdvanceFlowThroughInline` switch: added `context.produce` (kept in lock-step).
  - New `runContextProduceNode`: produces + stores the FlowContextPackage (mirroring `advanceFlowThroughFreezeChain`/`startInlineEntryChain`: stash `planContextPackage`, emit `EventFlowContextPackage`, persist), then advances to the single forward target — `agent.delegate` spawned with the rendered package + node prompt (startInlineEntryChain shape), or `agent.code` spawned as a frozen-contract writer (`spawnFrozenWriterChild`).
- `apps/local-runner/internal/runner/interactive_service.go` (`maybeReinvokeCoderForContinue`): when the continue target is a reinvoke-lifecycle `agent.delegate` with no existing child, spawn a fresh child instead of silently no-op'ing. Scoped to `agent.delegate` only — `agent.code` reinvoke writers keep the pre-existing no-op (they go through the frozen-contract path, and the dual-loop fixtures declare code reinvoke nodes without children).
- `apps/local-runner/internal/tui/app/step_runtime.go` (`showBlockedBanner`): the banner now lists only the chips actually rendered — "Retry / Stop", plus " / Allow" only for a non-cap/non-stalled frozen-contract drift park.

### Reviewer-driven fixes (sub-agent review, applied)

1. **Failure no longer falls back to note+reinvoke-hub**: `runContextProduceNode` now escalates (`applyFlowControl escalate` → Retry/Stop card) on produce/spawn/shape/missing-contract failures instead of returning `false` into the run-198699 hub_stalled class. Matches `advanceFlowThroughFreezeChain`.
2. **Context stamped DONE only after spawn succeeds** (V9-11): a spawn failure escalates and Retry can re-dispatch `context.produce`; the context node is no longer left DONE with nothing behind it.
3. **Terminal guard split**: terminal run → `return true` (matches `runContractFreezeNode`; never resurrects the hub after Stop); not-advancing → `return false`.
4. **Continue spawn renders the stored FlowContextPackage**: `spawnContinueChild` now loads `planContextPackage` and applies `renderFlowContextPromptWithSecret` + `FCPMarkerProvenanceRunID`, same as the forward-entry spawn — a Retry-continue onto a never-spawned `plan_writer` gets the S2 context handoff.
5. **Stale lock-step comments updated** to include `context.produce`.

## Tests added (new files only)

- `run198699_scout_context_advance_test.go`: provider matrix (Claude/Codex/Grok via `registerKeyedCapture`):
  - scout → context → plan_writer auto-advance (context DONE, plan_writer spawned, prompt carries scout plan **and** `flowContextHandoffPrefix`),
  - continue on a never-spawned `agent.delegate` reinvoke target spawns the child (RUNNING) **with the stored context package rendered into the prompt**,
  - near-miss: continue on a never-spawned `agent.code` reinvoke target keeps the no-op,
  - lock-step guard: `flowNodeInlineDispatchable(context.produce) == true` (additive; `TestBUG327` left unedited).
- `tui_blocked_banner_copy_test.go`: banner copy matrix — `hub_stalled`/`cap`/`member_stalled` advertise only "Retry / Stop"; drift park advertises "Retry / Stop / Allow".

## R1 (old suite untouched)

- Related pre-existing tests green, unedited: `TestTaskHarnessPlanLoopContinueReusesPlanWriter`, `TestTaskHarnessCodeLoopContinueStaysOutOfPlanLoop`, `TestApplyFlowControlContinueHubRouting`, `TestBUG327_FlowNodeInlineDispatchable`, `TestResolveContinueBackEdgeIsSourceAware`, `TestBlockedBar_RetryStopAlways_AllowOnlyOnDrift`, freeze-chain/context cluster, rag-harness edge cluster, `TestBug353`, `TestCP53`, `TestBug289/302/308/318`.
- `TestRun144900_*` (PreexistingSkillDoesNotCauseScopeDrift / MutatedLeftoverStillBlocks) fails on the stashed baseline identically ("baseline must capture leftover SKILL.md") — pre-existing environmental flake in skill-scaffold baseline capture, untouched by this change (verified via `git stash` + `-count=1` on baseline).

## R2 (provider parity)

- Provider-agnostic by construction: `tryAdvanceFlowThroughInline`/`runContextProduceNode`/`maybeReinvokeCoderForContinue` take no provider key and never branch on one (grep evidence); routing reads only edge/node data. The new runner matrix tests over Claude/Codex/Grok lock a future provider-specific drift; the banner test already iterates all three.

## Prior CA not undone

- CA-731 (hub done-successor one-decision stamp), CA-712 (CP-58 dual-loop routing + delegate deviation), BUG-243/CA-426 (inline dispatch + freeze-chain context hops), Task-309 (Allow = drift-only) — all intact; this change only adds the missing mid-flow `context.produce` dispatch and the delegate continue-spawn, and corrects banner copy to match the chips.

## Verification

- `go test ./internal/runner/ -run 'TestRun198699' -count=1`: 3 tests × 3 providers PASS (incl. FCP handoff assertions).
- `go test ./internal/tui/app/ -run 'TestBlockedBannerCopyListsOnlyShownChips' -count=1`: PASS.
- Related old clusters above PASS. `go vet ./internal/runner/ ./internal/tui/app/`: clean.
- Sub-agent review (independent reviewer) applied findings 1–5; no Critical findings remained after the fixes.
- Live-verify pending: rebuild runner, fresh `/flow task-harness` + GCD prompt → scout completes → `context` DONE → `plan_writer` RUNNING (S2 advance, no hub reinvoke/stall).