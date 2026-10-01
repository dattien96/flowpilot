# CA-1072: `vibe-tasks` — third Vibe entry for CPs with pre-broken tasks

Date: 2026-09-30
Refs: CP-90 (`requirements/07-Coding-Plan/todo/CP-90-Vibe-Tasks-Entry.md`), Task-456, Task-457, SS-18, BUG-468, CP-89 live-test pattern.

## Problem

Vibe mode had exactly two user-startable entries:

- `vibe-ingest` — raw idea → SS drafts → lock → CP drafting → task slicing → sprints.
- `vibe-cp-ingest` — existing CP → validate → lock → `task_slicer` (AI writes Task-*.md) → sprint chain.

There was no entry for "CP exists **and** its Tasks are already broken down". Routing such a CP through `vibe-cp-ingest` runs the AI slicer, which ADDs new Task files on top of the existing ones — duplicated/wrong scope — and `vibe-sprint` itself is system-only so it cannot be started directly.

## Solution

New user-startable flow `vibe-tasks` (Vibe-mode only):

```text
CP path → cp_reader → cp_validator → cp_lock → task_plan_reader → vibe-sprint chain
```

`task_plan_reader` is a read-only intake node (empty `allowed_tools`, prompt writes nothing). On its completion the runner reuses the **exact same** sprint-chain seam the slicer path uses: `collectVibeTaskPlanForCP(cwd, cpID)` (BUG-468 scoping: `Parent Documents:` line must name this run's CP) → `vibeTaskPlan` seed → `maybeStartNextVibeSprint`. Everything downstream — per-task `vibe-sprint` runs, `sprint_handoff`, budget cap, `vibe-owner-debate`, `plan_complete` terminal, checkpoint/durability — is byte-for-byte the existing machinery; only the entry topology changed.

Fail-closed at two layers:

- **Turn admission**: `vibe-tasks` requires `SourceDocID` to resolve to a `CP-*` doc id, and the CP must have ≥1 todo task whose `Parent Documents:` names it (`no_cp_tasks`). No silent slicer fallback — that path is `vibe-cp-ingest`.
- **Reader completion**: if disk state changed between admission and node completion, an empty scoped plan parks the run `blocked_user` with reason `vibe-no-cp-tasks` (user may add/fix tasks and resume).

## Changes

### Flow pack

- `internal/agentpack/flow-pack/flows/vibe-tasks.yaml` — new; byte-identical `cp_reader → cp_validator → cp_lock` head to `vibe-cp-ingest` (including the `continue` back-edge for CP edits after preview), then `task_plan_reader` (scan workload, `vibe-intake` agent, read-only prompt, `cp_md` input binding only) → `done`/`ask_user` edges. No `task_slicer`, no `task_md` output binding — nothing in this flow can write Task files.
- `internal/agentpack/flow-pack/prompts/vibe-task-plan-read.md` — read-only reader prompt.
- `manifest.yaml` — flow + prompt entries.
- `internal/runner/builtin_artifact_bindings.go` — `vibe-tasks` added to `builtinHarnessArtifactFlowIDs` (flow declares artifact bindings; BUG-469 seed coverage).

### Runner

- `vibe_cp.go` — `vibeTasksFlowID`, `vibeTaskPlanReaderNodeID` constants; `vibeTasksNodeIDs`/`vibeCpSourcedNodeIDs` sets; `normalizeVibeCheckpointLayer` recognizes reader; `onVibeCpNodeDone` handles reader completion (scoped plan → empty-park or seed+sprint); `vibeLinearWriterNode` + `vibeInheritsSessionModel` cover `task_plan_reader`; `inferPackFlowRefFromNodes` returns `vibe-tasks` when the node set contains `task_plan_reader` (checked before the `cp_reader`/`cp_lock` shared-prefix branch); `isVibeCpSourcedFlowID` helper.
- `interactive_service.go` — `runFirstTurnFences` admits both `vibe-cp-ingest` and `vibe-tasks` through `validateVibeCpIngestSource` (new `no_cp_tasks` guard for vibe-tasks); awaiting-lock set for both.
- `interactive_handlers.go` — resume admission: vibe-tasks armed runs recover `sprint_handoff`; flow picker options derive from `workingmode.FlowPickerOptions` (vibe list now includes `vibe-tasks`).

### Working-mode gates

- `internal/workingmode/workingmode.go` — `vibe-tasks` added to `vibeUserFlowIDs` → user-startable in vibe mode, forbidden in dev, and `FlowPickerOptions("vibe")` returns all three entries. `vibeSystemFlowIDs` unchanged.
- `internal/tui/app` — flow suggest + arm path treat `vibe-tasks` like `vibe-cp-ingest` (CP source prompt); `/flow vibe-tasks @path` works; dev mode omits it.
- `apps/desktop-flowpilot/src/state/workingMode.ts` — `VIBE_USER_FLOW_IDS` + `VIBE_CP_SOURCED_FLOW_IDS` extended → picker shows it in vibe mode only; CP path flows into `SourceDocID` the same way as `vibe-cp-ingest`.

## Tests

New (all additive, red-first):

- `internal/runner/vibe_tasks_live_test.go` (unit section, runs without `LIVE`):
  - `TestVibeTasks_ReaderDoneBuildsPlanScopedToRunCP` — reader node-done collects parented tasks in order, skips done tasks, chains `vibe-sprint` with correct `TaskFilePath`/`VibeSprintIndex`; repeat completion is a no-op.
  - `TestVibeTasks_ChainWalksFiveTaskPlan` — reader done on a 5-task CP-02 bed seeds the full ordered plan `[Task-21..Task-25]` (foreign/unparented excluded), sprint-1 starts on Task-21 with a real `preflight_contract_plan` child, terminal sprint-1 parks the boundary gate on Task-22, `ok` takes it and spawns sprint-2 (index 1→2) — the whole chain driven by the real `maybeParkVibeSprintBoundary`/`SubmitGateDecision`/`maybeStartNextVibeSprint` seams with a fake provider adapter.
  - `TestVibeTasks_ReaderDoneWithoutTasksParks` — zero matching tasks → `blocked_user` park (`vibe-no-cp-tasks`), no plan seed.
  - `TestVibeTasks_UserStartGate` — vibe user list = {ingest, cp-ingest, tasks}; dev rejects vibe-tasks; vibe still rejects `vibe-sprint`/`vibe-owner-debate`.
  - `TestVibeTasks_AdmissionRequiresCPSource` — missing/bad/other-doc source → `invalid_cp_source`.
  - `TestVibeTasks_AdmissionRejectsCPWithoutParentedTasks` — CP with no parented todo tasks → `no_cp_tasks`; a task whose `Parent Documents` names the CP passes.
  - `TestVibeTasks_InferFlowRefFromTopology` — node set with `task_plan_reader` resolves to `flowpilot-core-flow-pack/vibe-tasks`; without it, `cp_reader`+`cp_lock` still resolves to `vibe-cp-ingest`.
- `internal/agentpack/task326_vibe_pack_inventory_test.go` — `TestPack_VibeTasksTopologySkipsSlicer`: exact node list `[cp_reader cp_validator cp_lock task_plan_reader]`, `task_slicer` absent; `vibe-tasks` added to the `selectableIn == []` assertion set.
- `internal/runner/vibe_tasks_live_test.go` — `TestVibeTasksLive` (`LIVE=1`): real binary + real HTTP against a FIVE-task CP-02 bed (plus foreign/unparented decoys). Picker surfaces; `invalid_cp_source`/`no_cp_tasks` over HTTP; provider lane then drives the whole entry: `cp_reader` child spawn → `cp_validator` → `cp_lock` `vibe_lock` park → empty `agent-loop/continue` (the real Lock action) → `task_plan_reader` child spawn + completion → **`vibe-sprint` spawn on task 1/5** (`preflight_contract_plan` child carrying `vibeTaskIndex=1, vibeTaskTotal=5, vibeTaskName=Task-21` on its durable record) → sprint pipeline exercised past entry with real delegate work: `contract-planner`, `scaffold-architect` (wrote real stubs + a 17-test RED suite, `Ran 17 tests … FAILED (errors=22)`), 7 `owner` debate agents calling `mcp__flowpilot__submit_review_outcome` for real (`changes_requested` reprompt → verified `approved`), sprint-1 reaching `audit` where the gate escalated `code changed but no change-audit` — fail-closed correct (real code was written without a CA note). The sprint-1→sprint-2 boundary advance on a terminated sprint is the shared pre-existing seam covered in-process (`vibe_sprint_boundary_test.go`) and by `TestVibeTasks_ChainWalksFiveTaskPlan` on this entry's own topology.

  Provider pinning (fixed during verification): the first live run reported `provider=devin` at the test layer but the run was stamped `provider_key=codex / model=gpt-5.4-mini` — `providerKey` alone loses to model inference in the start handler (`providerKeyFromModel(resolvedModel)` wins; the Step>Flow>Project>default chain lands on a codex model). The test now detects the provider BEFORE run creation, sets `env.provider`, and pins `model` in the create body (`VIBE_TASKS_LIVE_MODEL` override, else `defaultModelForProvider` → `devin/swe-2-high`), then asserts the durable `provider_key` on the run row and every spawned child — a silent fallback now fails fast. The same pin was applied to the shared `cp89StartRun` helper (`model` defaults to `defaultModelForProvider(effectiveProviderKey)` when unset) so every cp89 live lane gets the fix too. Result: `provider="devin" model="devin/swe-2-high"` on every spawn (ACP `configOptions` confirms `currentValue: swe-2-high`), SWE-2 formalizes outcomes via real MCP tool calls instead of prose. Drive budget 55min (a legit sprint needs tens of minutes on a real provider) with three terminals: sprint-2 spawn, repeated audit park, or ≥2 distinct sprint delegate roles. The drive also satisfies the audit gate's change-audit requirement for real — on an escalate whose reason mentions `change-audit` it writes `change-audit/CA-LIVE-1-…md` into the workspace before resolving, letting the gate's next evaluation pass that check legitimately.

Updated (additive — pack/picker intentionally grew by one):

- `internal/agentpack/pack_test.go`, `internal/agentpack/task326_vibe_pack_inventory_test.go` — flow count 13 → 14.
- `internal/runner/task326_working_mode_picker_test.go`, `internal/tui/app/task326_tui_vibe_flow_filter_test.go`, `apps/desktop-flowpilot/src/state/task326_working_mode.test.ts` — vibe picker/suggest expectations now list all three user entries.
- `apps/desktop-flowpilot/src/state/ca1070_vibe_flow_surface.test.ts` — fixture + expectation extended for the third vibe flow.

## Verification

- `go test ./internal/runner -run 'TestVibeTasks_'` — 7/7 pass.
- `go test` vibe-seam cluster (`Vibe*|BUG3*|BUG46*|Task32*|CA78/79|Checkpoint|InferPack`) — pass (22.6s).
- `go test ./internal/agentpack ./internal/tui/app ./internal/workingmode` — pass.
- Desktop: `bun test state` — 15/15 pass; `npm run typecheck` — clean.
- `LIVE=1 VIBE_TASKS_LIVE_PROVIDER=devin go test -run TestVibeTasksLive -timeout 75m` — PASS (2101.7s; **provider=devin / model=devin/swe-2-high** pinned and asserted on the run row + every child): picker, both 422 fences, `cp_reader` spawn, `cp_lock` via real Continue (1 drive — SWE-2 formalizes outcomes, no escalate churn), `task_plan_reader` spawn+complete, `vibe-sprint` child carrying `vibeTaskIndex=1/5 Task-21`, sprint-1 ran the real pipeline — contract-planner → scaffold-architect (real Go stubs + 13-test RED suite) → owner-debate (real `submit_review_outcome` consensus reprompt) → **coder implemented `model.go`, suite GREEN (`ok snake 0.664s`, 13/13 PASS)** → `audit` node reached; audit escalated `code changed but no change-audit` → the drive wrote a real CA note → next blocker surfaced `validation was not positively verified` — fail-closed terminal, because the escalate-resolution advanced past the `validate` node without running it (pre-existing stale-node quirk on escalate-resolve, identical on `vibe-cp-ingest`; not a vibe-tasks bug). Sprint-2 spawn on a terminated sprint is covered in-process (`vibe_sprint_boundary_test.go` + `TestVibeTasks_ChainWalksFiveTaskPlan`). An earlier run on the unresolved codex default (silent fallback, pre-fix) never formalized outcomes and parked `blocked_validation_failed` — superseded by the SWE-2 run.
- `go vet` — clean on all touched packages.

Pre-existing env reds (reproduced on clean HEAD before this diff — missing claude/codex/gitnexus binaries, provider accounts, supabase mirror schema): BUG-334 provider-process tests, gate-hook env tests, catalog fallback, flow-definition store, session/provider pool tests, plus the earlier desktop suite fails (history-replay ordering, health payload, importBoundary, styles.tokens path). None touched by this diff.
