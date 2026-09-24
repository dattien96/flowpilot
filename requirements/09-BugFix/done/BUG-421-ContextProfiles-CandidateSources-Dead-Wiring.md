# BUG-421: `contextProfiles.*.candidateSources` is dead wiring — `knowledge.flow` can never reach builtin-flow prompts

## Metadata

- Document ID: `BUG-421`
- Title: `Packages build from the producing node's ContextSources; resolveEnabledContextSourceIDs only consulted for the inline entry node — candidateSources ignored everywhere`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-66-Test-Steps](../../07-Coding-Plan/done/CP-66-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp66/RESULT.md` (BUG-LIVE-66-1)
- Feature Keys: `context-profiles`, `knowledge-flow`, `flow-context-package`, `context-sources`

## AI Quick View

### Summary

- Every live `flow_context_package` contains only the 6 default `SourceType`s (`canonical.head`, `feature.history`, `change.contract`, `source.dependence`, `source.excerpt`, `chat.summary`); `knowledge.flow` is absent from `context`, `test_signatures`, and every downstream prompt.
- `contextProfiles.scout/plan_writer.candidateSources` never reach consuming-node prompts: packages are built from the **producing** node's `ContextSources`, and the only call site that consults `ContextProfile.CandidateSources` (`resolveEnabledContextSourceIDs`) runs once — for the inline entry node — which `task-harness`'s `agent.delegate` entry bails out of before resolution.
- Wiring-only defect, no crash — but `knowledge.flow` is dead in every builtin flow; unit tests exercise the source list directly, not the runtime path.

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

On a bed with `.gitnexus` + `.flowpilot/knowledge` populated, a `task-harness` run emits `flow_context_package` events whose `SourceType` fields are exactly the 6 defaults; the `plan_writer` prompt has the `[FlowPilot flow context package]` envelope with no `## Context — Execution Flow Knowledge`/`## Flow:` section.

### Expected

Per CP-66 AC ("planner receives flow summary"): scout/plan_writer profiles carry `knowledge.flow` in `candidateSources` → their prompts should contain the locus-matched execution-flow knowledge section.

### Actual

- All emitted packages (evt-164 `context` 22:35:04Z, `test_signatures` 23:00:10Z, and both `context` packages) list only the default six; `knowledge.flow` never appears.
- `plan_writer` prompt (`L-66-2-planwriter-prompt.txt`) contains the FCP envelope with empty bodies and no flow-knowledge section.
- No built-in flow narrows the source set either (all 8 flows with a `context.produce` node leave `contextProfile`/`contextSources`/`artifactBindings` empty) — so even the profile path is unexercised live.

### Impact

`knowledge.flow` cannot appear in any prompt of any builtin flow — the CP-66 feature is inert end-to-end on the live path. Tests `TestFlowPlanWriterReceivesExecutionFlowContext`/`TestCoderNodeDoesNotReceiveKnowledgeFlow` pass while exercising only the source list, not the runtime wiring, so the gap is invisible to the suite.

## Reproduction

1. Fresh bed with `.gitnexus` index + `.flowpilot/knowledge` content.
2. `POST /client/workflow-runs` + turn with `flowRef:"task-harness"`.
3. Inspect `flow_context_package` events (runner-cwd `.flowpilot/chats/run-*-flow-events.ndjson`) and the spawned `plan_writer` prompt → only the 6 default sources; no `knowledge.flow`.

## Root cause

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:189` — `runContextProduceNode` builds the package via `BuildFlowContextPackageWithSources(ctx, workspace, hints, node.ContextSources)` — never calls `resolveEnabledContextSourceIDs`; the `context` node declares neither field → nil → `defaultContextSourceIDs`.
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:2097` and `:2111` — `advanceFlowThroughFreezeChain`'s `buildAndStorePackage(mid.ContextSources)` / `(writerNode.ContextSources)` — same producing-node pattern for freeze→writer hops.
- `apps/local-runner/internal/runner/context_sources_builtin.go:144` — `resolveEnabledContextSourceIDs` (the sole consumer of `ContextProfile.CandidateSources`) is called exactly once, for the inline entry node at `apps/local-runner/internal/runner/flow_executor.go:488` (`startInlineEntryChain`); `task-harness`'s entry node is `agent.delegate` → that path bails before resolution.

## Evidence

- `~/fp-beds/lt-evidence/cp66/RESULT.md` — BUG-LIVE-66-1 (L-66-2 FAIL row): `run-1-flow-events.ndjson` (3 `flow_context_package` events, all 6-default), `L-66-2-{scout,planwriter,planreviewer}-prompt.txt`, `L-66-2-steps-runtime.json`.
- Line numbers verified on main worktree HEAD `435e336b` (evidence cited :2073/:2110 under the lt-cp66 tree; equivalent call sites are :2097/:2111 at `435e336b`).

## Severity

- `medium` — feature-level dead wiring (high for CP-66 AC-2/AC-3: planner never receives `knowledge.flow` live); no crash.

## Completion Notes (implemented 2026-09-23, CA-924b)

- Root cause: `resolveEnabledContextSourceIDs` (which honors
  profile `candidateSources`) was only called on `startInlineEntryChain`;
  `runContextProduceNode` and the freeze-chain `buildAndStorePackage` used
  the producing node's own source list, so consumer-profile sources like
  `knowledge.flow` never reached mid-flow packages.
- Fix: new `producedContextSourceIDs(flowDef, produceNode, consumerNode)`
  in `internal/runner/context_profile.go`/`context_sources_builtin.go`
  resolves artifact binding → consumer profile candidateSources → node
  ContextSources → defaults; `runContextProduceNode` resolves its forward
  target and uses the consumer's set, and freeze-chain package builds pass
  each hop's consumer.
- Tests: `TestBug421_ProduceResolvesConsumerProfileSources`,
  `TestBug421_FreezeChainPackageUsesConsumerProfile` (red by assertion
  pre-fix).
- Live: `/tmp/fp-live-i` run-3221 — the `context` node's package carries
  exactly the `plan_writer` profile's candidateSources
  (`conventions, knowledge.flow, canonical.head, feature.history,
  change.contract, source.excerpt`) in its sections.
