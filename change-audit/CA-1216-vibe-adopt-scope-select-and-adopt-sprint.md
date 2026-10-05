# CA-1216 — vibe-adopt: scope-select wrapper + verify-first adopt sprint (Task-459)

## Evidence / ask
Operator flow required for pre-existing code: PrivateVault CP-05 was fully
implemented on `feat/cp05` outside FlowPilot; re-running greenfield
`vibe-sprint` wedges at the scaffold RED gate. Requirement: one `vibe-adopt`
entry, radio `task | cp` — task shows the Task list and adopts one; cp shows
the CP list and adopts every parented task sequentially, task-by-task.

## Implementation

### Two flows (data) + one engine seam

- `flow-pack/flows/vibe-adopt.yaml` — wrapper, single `adopt_select`
  (`user.confirm`) node, `selectableIn: [user]`. It is never dispatched as a
  delegate: `startResolvedFlow` mounts the topology then parks on an engine
  question card (`parkVibeAdoptSelect`).
- `flow-pack/flows/vibe-adopt-sprint.yaml` — `vibe-sprint.yaml` node set with
  entry edge `context --done--> validate` (verify-first). Defect remediation
  keeps `synthesis_negotiation --continue--> tdd` (`kind: back`). An inert
  forward anchor `synthesis_negotiation --remediate--> tdd` keeps `tdd` out
  of the entry-node set (`agent.scaffold` entries are illegal);
  `remediate` is never emitted, dispatch still rides the back-edge only.
- `manifest.yaml`, `workingmode` (`vibeUserFlowIDs`/`vibeSystemFlowIDs`/
  `FlowPickerOptions`), desktop `workingMode.ts` — registered everywhere
  `vibe-sprint`/`vibe-tasks` is selectable.

### Select card (durable, 2-stage)

- `vibe_adopt_select` question kind, kind persisted in the prompt prefix
  (`vibe_adopt_select:<stage>: …`) so rehydrated records route the same.
  Stage `scope` offers `[task, cp]`; the resolved stage re-emits the
  candidate list (Task docs union todo/inprogress/done; CP docs union
  todo/inprogress/approved/done).
- `applyVibeAdoptSelectAnswer` (called from `AnswerQuestion`'s kind router
  after the durable resolved-commit): task pick → single-element
  `vibeTaskPlan`; cp pick → `collectVibeAdoptPlanForCP` (union of
  `collectVibeTaskPlanForCP` todo + `collectVibeDoneTasksForCP` done,
  numeric-id sorted) + `vibeCpDocID`. Empty-CP re-emits the card instead of
  dead-ending.
- `finishVibeAdoptSelect` → step DONE + loop unblocked →
  `tryAdvanceFlowFromNode` → `onVibeCpNodeDone(adopt_select)` →
  `maybeStartNextVibeSprint` — the existing chain seam, no second scheduler.
- Stray `SubmitGateDecision` while a select card is pending → 409
  `adopt_select_pending` (the generic blocked-loop path must not consume it).
- `parkVibeAdoptSelect` re-parks behind a live pending card instead of
  re-asking (rehydrate/re-entry safe).

### Sprint mounting

- `vibeSprintFlowRefFor(rs)`: adopt parents mount
  `pack:vibe-adopt-sprint`, others keep `pack:vibe-sprint` — used at the two
  hardcoded mount sites (initial start + boundary continuation).
- Adopt sprints skip `stampVibeTaskInProgress` (the doc's status is already
  its truth; verification must not rewrite it) and append the adopt-aware
  prompt line: "implementation already exists — verifying alignment, not
  authoring".
- `inferPackFlowRefFromNodes`: `adopt_select` topology → `vibe-adopt`;
  stored `vibe-adopt-sprint` ref is authoritative on the shared sprint
  topology (sticky, like `vibe-sprint`).

### Characterization gate (spec deviation — contract-level, not prose)

Spec §T-3 proposed a `CoverageKind` field plumbed from spec-align verdicts;
the spec-align classification lives in `feedback` prose, not a typed field,
so wiring it would violate schema-first. As-built instead requires the
typed contract itself to be characterization-shaped:

- `submit-scaffold-outcome` `failure_type` enum += `characterization`.
- `vibeScaffoldCharacterizationForTask` = task-section scan requiring BOTH
  `failure_type: characterization` AND `red_tests: []` (a defect remediation
  names red tests — it can never take the waiver, even on adopt).
- `gate_hook` sets `tr.ScaffoldRedWaived` only when `vibeSprintIsAdopt` —
  normal sprints treat the declaration as inert (RED applies unchanged).
- `scaffold-contract-tdd.md` instructs: characterization only when the
  spec-align verdict classified the requirement MISSING; OUTDATED/
  CONTRADICTS keep honest RED. Missing/verifiable-absent → fail closed.

## Tests
`ca1216_vibe_adopt_test.go` — 8 tests, all green:
- `TestVibeAdoptSprint_TopologyEntersAtValidate` — context→validate forward,
  no context→tdd, remediation back-edge intact (AC-1/AC-2).
- `TestVibeAdopt_SprintFlowRefSelection` — adopt → adopt-sprint; non-adopt
  → vibe-sprint.
- `TestVibeAdopt_FlowRefInference` — adopt_select → vibe-adopt; adopt-sprint
  ref sticky on shared topology; stale ref corrects to vibe-sprint.
- `TestVibeAdopt_ScopeSelectEmitsCard` — card options [task,cp], node
  WAITING, loop blocked on `vibe_adopt_select`.
- `TestVibeAdopt_TaskPickSetsSinglePlanAndChains` — scope→task emits the
  task-list card; pick sets 1-task plan and takes sprint index 1.
- `TestVibeAdopt_CpPickBuildsUnionPlan` — todo+done union, foreign CP tasks
  excluded, `vibeCpDocID` set, index consumed.
- `TestVibeAdopt_CpPickEmptyReemits` — empty CP re-parks, no dead end.
- `TestVibeAdopt_GateDecisionRejectedWhileSelecting` — stray decision →
  `adopt_select_pending`.
- `TestVibeAdopt_CharacterizationScopedToAdopt` — section-scoped contract
  detection; red_tests non-empty + characterization is NOT a waiver.
- Updated (additive-only): `TestVibeTasks_UserStartGate` picker list;
  `TestLoadBuiltinPack`/`TestPack_InventoryUnchanged` 14→16 flows;
  `TestSubmitScaffoldOutcomeSchemaValidation` failure_type enum.

## Risk
- `vibe-sprint`/`vibe-tasks`/`vibe-cp-ingest` byte-identical: all adopt
  branches key on `vibeSprintIsAdopt`/`vibeAdoptSelectNodeID`; the only
  shared-code edits are additive `if adopt` guards and the new node-id case
  in `inferPackFlowRefFromNodes`/`onVibeCpNodeDone`.
- The inert `remediate` forward edge cannot dispatch (status never emitted,
  kind forward); topology validation passes; sprint runtime dispatch is
  untouched.
