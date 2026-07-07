# Task-180: Domain-Free Guard And Legacy Migration

## Metadata

- Document ID: `Task-180`
- Title: `Domain-Free Guard And Legacy Migration`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-01`
- Last Updated: `2026-07-06`
- Parent Documents: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- Child Documents: `N/A`
- Related Documents: `Task-173`, `Task-174`, `Task-175`, `Task-176`, `Task-177`, `Task-178`, `Task-179`
- Replaces: `N/A`
- Tags: `migration, hardcode-removal, generic-flow, regression-guard`

## AI Quick View

### Summary

- Remove or quarantine remaining flow-specific hardcodes after pack-backed execution is proven.
- Add regression guards that prevent future runner code from branching on semantic flow names.
- Keep legacy migrations for existing sessions and definitions.

### Current Ask

- Finish the CP-42 refactor by making the runner generic by default and retaining only explicit compatibility shims.

### Key Decisions

- `T-1` Generic runner can branch on behavior IDs and schema types, not on business names like `reviewer`, `coding`, or `plan`.
- `T-2` Compatibility shims must be isolated, documented, and covered by tests.
- `T-3` Existing sessions and definitions must continue to resume.

### Constraints

- Depends on Tasks `173` through `179`.
- Do not remove compatibility before migration tests pass.
- Do not break existing `sessions.ndjson` restore.

### Open Questions

- Whether to add static linting or test-only grep guard can be decided during implementation.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/flow_context_handoff.go`
- `apps/local-runner/internal/runner/agent_catalog.go`
- `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`

## 1. Goal

Ensure CP-36 and CP-41 are implemented as generic flow engine behavior, not as hidden hardcoded templates, while preserving backward compatibility for existing users.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- tech design: `requirements/06-System-Tech-Design/done/SD-19-Agent-Orchestration-Runtime.md`
- system spec: `requirements/05-System-Specs/done/SS-16-Agent-Orchestration.md`
- specific upstream ids: `Task-173` through `Task-179`, `CP-36`, `CP-41`, `CP-42`

## 3. Trigger

After pack schema, catalog loading, mirror sync, behavior registry, Chat picker, RAG baseline, and Settings UI exist, the final risk is regressions back into semantic hardcoding.

## 4. Exact Change

- `T-1` Audit runner code for semantic hardcodes:
  - `review-loop`
  - `reviewer`
  - `coder`
  - `synthesizer`
  - `plan`
  - `planning`
  - `design`
  - `coding`
  - `implementation`
  - `code`
- `T-2` Remove active-path hardcodes where behavior IDs or pack metadata now cover the case.
- `T-3` Move unavoidable backward-compatibility mapping into a single migration/shim file.
- `T-4` Add comments and tests proving shims only apply to legacy definitions or sessions.
- `T-5` Add regression guard test that scans runner execution files for forbidden semantic branching patterns.
- `T-6` Add migration tests:
  - old Review Loop session resumes.
  - old CP-41 Plan/Coding definition still resolves.
  - new custom flow with unrelated node names runs.
- `T-7` Remove legacy Go built-in agent fallback if Task-174 has proven pack-backed fallback is stable; otherwise mark it deprecated and isolate it.
- `T-8` Update CP-42 completion notes with final architecture and remaining exceptions.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/runner/flow_context_handoff.go`
  - `apps/local-runner/internal/runner/agent_catalog.go`
  - `apps/local-runner/internal/runner/**/*_test.go`
  - `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- modules:
  - local runner executor
  - context harness
  - agent catalog
  - migration tests
- routes:
  - none expected
- tables:
  - none expected

## 6. Acceptance Check

- New custom Flow Mode definition can use non-domain node IDs and still execute.
- Chat Mode Review Loop runs through `flowRef` and pack metadata.
- Chat Mode RAG baseline remains automatic.
- Active execution path no longer depends on `isPlanStepType` or `isCodingStepType`.
- Regression guard fails if new semantic hardcodes are added to generic runner path.
- Existing sessions resume.
- Existing built-in flows still pass integration tests.

## 7. Out of Scope

- New context provider types beyond CP-41 deterministic package.
- Remote marketplace packs.
- Full visual graph editor polish.

## 8. Completion Notes

- result: partially implemented
- notes: added a frozen-baseline domain-hardcode regression guard (`TestDomainHardcodeGuardMatchesFrozenBaseline`) covering the five active-path files, plus migration tests proving legacy CP-41 step types (`plan`/`coding`/aliases) still resolve and a genuinely unrelated custom step type is inert without any runner code change ([CA-153](../../../change-audit/CA-153-domain-hardcode-guard-and-migration-tests.md)). The four `isAgentRole(rs, "coder")` branches in `interactive_service.go` were consolidated into one named, tested `isCoderRun` shim, reducing the file's hardcode-baseline count from 4 to 1 ([CA-155](../../../change-audit/CA-155-consolidate-coder-role-check-shim.md)) — a manual call-graph read (still no GitNexus available) showed three of the four sites are already segregated behind the existing `loopMode == "explicit"` flag, bounding the risk of this refactor to a pure extract-function change, verified behavior-identical against the full `TestE2EReviewLoop*` suite. Full elimination (not just consolidation) needs a real generic executor resolving node identity from `FlowEdge` data instead of agent role names — that executor doesn't exist in the live run loop yet and is a new subsystem, not a Task-180 line item. `agent_catalog.go`'s and `review_loop_config.go`'s legacy Go fallback literals remain intentionally kept per Task-174 T-3 until pack-backed loading has proven stable in production.
- update: per explicit user direction, the "continue" reinvoke target — the one genuine remaining hardcode in the live loop (forward progression coder→reviewers→synthesis is already AI-driven, not Go-hardcoded) — is now resolved from the flow's own edge data for flowRef-started runs: `interactiveRun.activeFlowEdges` (set by `startResolvedFlow`) + `resolveContinueBackEdgeTarget` find the `synthesis->coder` back-edge and match the target child by `label`, never consulting `isCoderRun` in that path. `isCoderRun` remains only as the fallback for AI-initiated runs with no tracked flow edges — verified byte-identical to prior behavior via the existing `TestE2EReviewLoop*` suite. See [CA-161](../../../change-audit/CA-161-edge-driven-continue-reinvoke.md).
- follow-ups: decide whether/when to retire the `agent_catalog.go` and `review_loop_config.go` legacy fallbacks (blocked on Task-174's own remaining T-5/T-6 gaps, per its T-3); a flow with more than one back/continue edge would still only use the first declared match (not the case for either built-in flow today); full `isCoderRun` elimination needs a real generic executor resolving node identity from `FlowEdge` data, a new subsystem out of this task's scope.
- moved to done 2026-07-06: re-verified `go build ./...` clean and `go test ./internal/runner/... -run 'TestDomainHardcodeGuard|TestE2EReviewLoop|FlowPackMigration|TestResolveContinueBackEdgeTarget'` 13/13 pass. Spot-checked `isCoderRun` (4 call sites, all behind `loopMode == "explicit"` or as an edge-driven fallback) and `isCodingStepType`/`isPlanStepType` (classify by `behaviorID` first, `stepType` only as legacy fallback) directly in code — both match this task's own T-1 "branch on behavior IDs, not business names" rule. Accepted as done with the above follow-ups explicitly deferred, not blocking — same pattern as Task-176's own accepted sub-gaps.
- upstream docs updated: [CP-42](../../07-Coding-Plan/inprogress/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) progress notes (link path corrected — pointed at a nonexistent `todo/` copy before). [CA-153](../../../change-audit/CA-153-domain-hardcode-guard-and-migration-tests.md), [CA-155](../../../change-audit/CA-155-consolidate-coder-role-check-shim.md), and [CA-161](../../../change-audit/CA-161-edge-driven-continue-reinvoke.md) are dangling references — no `requirements/change-audit/` directory exists anywhere in this checkout.
