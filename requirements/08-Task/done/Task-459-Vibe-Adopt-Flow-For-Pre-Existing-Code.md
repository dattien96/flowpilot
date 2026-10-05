# Task-459 — Vibe-Adopt Flow For Pre-Existing Code

## Metadata

- Document ID: `Task-459`
- Title: `vibe-adopt flow — review-first sprint for code written outside FlowPilot`
- Phase: `task`
- Status: `done`
- Owner: `flowpilot`
- Reviewers: ``
- Created: `2026-10-05`
- Last Updated: `2026-10-05`
- Parent Documents: `CP-90`
- Child Documents: ``
- Related Documents: `Task-456`, `Task-457`, `Task-382`, `vibe-sprint.yaml`, `bug-plan-harness.yaml`, `CA-1205`
- Replaces: ``
- Tags: `vibe`, `flow`, `adopt`, `review-first`, `scaffold-gate`, `existing-code`

## AI Quick View

### Summary

- `vibe-sprint` assumes greenfield TDD: the `tdd` scaffold node hard-requires a RED suite, so running it against code that already exists (written by another AI / by hand) wedges at the scaffold gate or burns reprompt rounds.
- This task adds a **`vibe-adopt`** flow variant: same node set and remediation edges as `vibe-sprint`, but the first pass enters at `validate`/`spec_align`+`reviewer` instead of `tdd`/`coder` — verify-first on the pre-existing diff.
- Findings re-enter the normal remediation loop (`synthesis_negotiation --continue--> tdd → coder → validate → reviewers → synthesis`) so defects get honest RED reproduction tests.
- One narrow tool change is required: a **characterization mode** for the scaffold RED gate, scoped strictly to adopt-mode coverage writes — otherwise green-by-design tests on existing impl are misread as "stubs contain real implementation".

### Current Ask

- Implement `vibe-adopt` per §4/§6, prove §7 tests green, keep `vibe-sprint` byte-identical in behavior.

### Key Decisions

- `T-1` New flow is **data-only**: a new yaml in `flow-pack/flows/`; no edits to `vibe-sprint.yaml`, `vibe-tasks.yaml`, or any shared node behavior.
- `T-2` First-pass entry edge is `context → validate` (not `→ spec_align`): the suite runs first so reviewers see a real baseline signal.
- `T-3` Remediation for **defects** (spec `CONTRADICTS`/`OUTDATED`) re-enters at `tdd` with the normal RED gate ON — a reproduction test that cannot fail is not a reproduction test.
- `T-4` Characterization allowance applies **only** to coverage gaps (`MISSING` in the spec-align table) in adopt mode, never to defect fixes, never to `vibe-sprint`/`bug-plan-harness`/`task-harness` runs.
- `T-5` No new engine concepts: adopt flow reuses `acceptance_nodes`, cohort joins, debate overlay, gate config, and audit as-is.

### Constraints

- **Zero blast radius on existing flows.** `vibe-sprint`, `vibe-tasks`, `vibe-cp-ingest`, `vibe-owner-debate`, `bug-*-harness` must behave byte-identically. The scaffold gate default path is unchanged unless the run's flow id is `vibe-adopt` AND the turn is a coverage write.
- `flow-pack` is `go:embed`ed (`agentpack/pack.go`) — new yaml has no effect on a running binary; it lands on next build. Verify pack validation still passes on boot (a malformed yaml must fail loudly at pack validation, not mid-run).
- Fail-closed preserved: if adopt mode cannot prove a test is characterization-shaped (declared marker), it must not bypass the RED gate.
- Durability contract §2 unchanged — no new persistence/session/registry model.

### Open Questions

- Should `vibe-adopt` be reachable from `vibe-tasks` sprint enumeration (mixed CP where some tasks have code and some don't)? Proposal: v1 is standalone flow id; a follow-up task can add per-task adopt detection in `task_slicer` via a `<!-- adopt -->` marker in the Task doc.
- Characterization marker format: prose-declared in `submit_scaffold_outcome` (`failure_type: characterization`) vs a dedicated face field. Proposal: extend `failure_type` enum — schema-first, no prose parsing (contract §1).

### Source Refs

- `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml` — node/edge donor
- `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml` — review-cohort reference
- `apps/local-runner/internal/flowgate/scaffold_red_rule.go` — `r-scaffold-red` gate to extend
- `apps/local-runner/internal/runner/scaffold_gate.go` — TurnResult population (`WrittenPaths`, unregistered-test detection from CA-1205)
- `internal/agentpack/flow-pack/tools/submit-scaffold-outcome.yaml` — `failure_type` enum

## 1. Goal

A FlowPilot flow that takes a CP/Task whose code **already exists** (written by another AI or by hand) and produces the same `.flowpilot` value as a normal sprint — frozen contract with DeclaredPaths, context package, spec-alignment table over the SS→SD→CP→Task chain, reviewer machine verdicts, drift events, gate metrics, canonical snapshot on audit — without forcing a fake RED scaffold pass on code that is already green.

## 2. Parent Links

- CP-90 (vibe flows): the sibling of `vibe-sprint` for the "code exists, verify + remediate" use case.
- Live evidence: `run-262417` (PrivateVault CP-04) — reviewer flagged a real silent-fail defect; owner debates consumed 2 rounds on drift. The remediation machinery is proven; the gap is only the entry shape for pre-existing code.

## 3. Trigger

Operator has a CP where another AI already implemented several tasks. Two candidate ways to run it in FlowPilot were evaluated:

1. **Full `vibe-sprint`**: wedges at `tdd` — the scaffold contract requires a RED suite; tests written against existing impl are green → gate reprompts "stubs contain real implementation" → cap → escalate. (Observed live: this gate fired 4× in run-262417 even for the intended greenfield shape.)
2. **Review-only slice**: works for verification but loses the honest-RED remediation path and, without a flow record, does not populate `.flowpilot` consistently.

`vibe-adopt` resolves both: review-first entry, full remediation loop on findings, full ledger artifacts.

## 4. Exact Change

- `T-1` Add `flow-pack/flows/vibe-adopt.yaml`: clone of `vibe-sprint.yaml` node set (`preflight_contract_plan`, `preflight_contract_freeze`, `context`, `tdd`, `coder`, `validate`, `spec_align`, `reviewer`, `synthesis`, `synthesis_negotiation`, `audit`) with **one** entry-edge change — `context --done--> validate` instead of `context --done--> tdd`. All back-edges (`validate --continue--> coder`, `synthesis --continue--> coder`, `synthesis_negotiation --continue--> tdd`, `synthesis_negotiation --done--> synthesis`, `synthesis --done--> audit`, `audit --done--> done`) are copied unchanged. `acceptance_nodes`, `policy` (cap 20 / negotiationCap 5), `contextProfiles`, tool faces, and `builtin.selectableIn` mirror `vibe-sprint`.
- `T-2` Extend `submit-scaffold-outcome` / `TurnResult` with a characterization marker: `failure_type` gains `characterization` (tests assert behavior of already-implemented code; green-by-design expected). Declared via the existing schema face — no prose parsing.
- `T-3` `r-scaffold-red` gate: accept `failure_type: characterization` **iff** `run.flowID == "vibe-adopt"` AND the turn's spec-align context classified the covered requirements as `MISSING` (coverage gap), not `OUTDATED`/`CONTRADICTS` (defect). All other inputs unchanged; unknown/unverifiable input still fails closed.
- `T-4` `context`/`coder`/`spec_align`/`reviewer` prompts get one adopt-aware line via existing context-package composition: "implementation pre-exists; you are verifying alignment, not authoring" — sourced from flow id, not a new prompt layer.
- `T-5` `builtin` registry: register `vibe-adopt` as mirror-required builtin, selectable wherever `vibe-sprint` is selectable. No picker UI work in this task (desktop selection via existing flow picker is acceptable).

## 5. Touched Areas

- files: `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-adopt.yaml` (new), `tools/submit-scaffold-outcome.yaml`, `internal/flowgate/scaffold_red_rule.go`, `internal/runner/scaffold_gate.go`, builtin registry/list wherever `vibe-sprint` is whitelisted
- modules: `internal/agentpack`, `internal/flowgate`, `internal/runner`
- routes: none
- tables: none (contract §1 — no new persistence model; marker rides existing TurnResult/tool schema)

## 6. Code Guide Signatures

```go
// apps/local-runner/internal/flowgate/scaffold_red_rule.go
// T-3 — characterization acceptance, adopt-scoped only
func checkScaffoldRedRule(rule Rule, tr TurnResult) *Violation // extend: tr.Characterization && tr.FlowID=="vibe-adopt" && tr.CoverageKind==Missing → pass; all other paths unchanged

// TurnResult additions (whichever struct owns WrittenPaths)
type TurnResult struct {
    // ...existing...
    FailureType string // extended enum: not_implemented|assertion_failure|characterization  // T-2
    CoverageKind string // missing|outdated|contradicts — populated from spec-align verdict input // T-3
}
```

```yaml
# apps/local-runner/internal/agentpack/flow-pack/flows/vibe-adopt.yaml
# T-1 — new file; node set identical to vibe-sprint.yaml except:
edges:
  - from: context
    to: validate        # was: to: tdd (vibe-sprint) — adopt enters at verification
    when: done
    kind: forward
  # ... all remaining edges copied verbatim from vibe-sprint.yaml
```

## 7. Test Signatures

- `TestVibeAdopt_GraphSeedsEntryAtValidate` — mounted adopt graph has `context → validate` forward edge and `tdd`/`coder` unreachable until remediation (covers AC-1)
- `TestVibeAdopt_FirstPassRunsSuiteThenReviewers` — dispatch order validate → spec_align+reviewer → synthesis with zero tdd/coder turns (covers AC-1)
- `TestVibeAdopt_DefectFinding_ReentersTddWithRedGate` — spec_align `CONTRADICTS` → negotiation `continue` → tdd leg dispatched → green characterization claim rejected → reproduction RED required (covers AC-2, T-3/T-4)
- `TestVibeAdopt_MissingCoverage_CharacterizationAccepted` — spec_align `MISSING` + `failure_type:characterization` + adopt flow → gate passes; same input on `vibe-sprint` → gate rejects (covers AC-3, T-3)
- `TestVibeSprint_ScaffoldGateBehaviorUnchanged` — existing RED-enforcement cases still green; characterization input rejected (covers constraint: byte-identical behavior)
- `TestVibeAdopt_PackValidationBoot` — pack load validates new yaml; malformed variant fails at validation not at dispatch (covers fail-closed)
- `TestVibeAdopt_AuditPopulatesLedgerArtifacts` — done sprint writes contract freeze, gate metrics, canonical snapshot identically to vibe-sprint (covers AC-4)

## 8. Acceptance Check

- A `vibe-adopt` run on a repo with pre-existing impl + green suite completes `validate → spec_align → reviewer → synthesis → audit` with no scaffold-RED reprompt.
- A `changes_requested` verdict routes through `synthesis_negotiation → tdd` and the returned tdd leg is held to RED (defect) or characterization (coverage) correctly by input shape, not prompt prose.
- `.flowpilot` artifacts are populated identically to a vibe-sprint run (contracts, ledger, gate-metrics, drift events, canonical).
- `go test -count=1 ./internal/flowgate/... ./internal/runner/... ./internal/agentpack/...` green; `detect_changes` shows zero overlap with `vibe-sprint` execution paths outside the declared seam.

## 9. Out of Scope

- Auto-detecting "code pre-exists" per task inside `vibe-tasks` (follow-up; needs task-doc marker contract).
- Desktop picker UX for the new flow.
- Migrating/rewriting history for code already committed before adopt.

## 10. Definition of Done

- [ ] All §6 signatures implemented exactly (or deviation documented in §11)
- [ ] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [ ] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [ ] Provider parity: gate/context changes are provider-agnostic or proven on Claude+Codex+Grok (R2)
- [ ] `feature_key` set; CA ledger entry written; FEATURE-KEYS.md already contains the key
- [ ] §8 acceptance checks verified by hand or test
- [ ] GitNexus `detect_changes` shows only expected symbols before commit — zero overlap with vibe-sprint behavior outside the declared seam

## 11. Completion Notes

- result: shipped as TWO flows + one engine seam (deviation from v1's
  single-flow plan, driven by the operator requirement "task OR cp scope"):
  - `vibe-adopt.yaml` — wrapper whose only node `adopt_select` is an engine
    question card, never a spawned delegate. Stage scope offers [task, cp];
    the resolved stage re-emits the matching candidate list. task pick →
    single-element vibeTaskPlan; cp pick → union of todo+done tasks
    parented to the CP, then the EXISTING sprint chain seam runs them
    task-by-task (no second scheduler).
  - `vibe-adopt-sprint.yaml` — vibe-sprint node set with entry edge
    `context --done--> validate`. Remediation back-edge unchanged.
  - Engine seams: `vibeSprintFlowRefFor` (adopt mounts adopt-sprint),
    adopt sprints skip `stampVibeTaskInProgress`, adopt-aware prompt line,
    `inferPackFlowRefFromNodes` adopt_select/sticky-ref cases,
    `vibe_adopt_select` question kind routed in AnswerQuestion,
    `SubmitGateDecision` 409 `adopt_select_pending` while a card is live.
  - T-3 deviation: no CoverageKind plumbed into TurnResult (spec-align
    classification is prose in `feedback`, not typed). As-built requires
    the typed CONTRACT itself — `failure_type: characterization` AND
    `red_tests: []`, adopt-scoped — with the MISSING-only rule carried by
    the scaffold-contract prompt; any red_tests-declared suite is a defect
    and rides the normal RED gate even on adopt. Fail-closed preserved.
  - Topology anchor added after pack validation: inert
    `synthesis_negotiation --remediate--> tdd` forward edge keeps `tdd`
    out of the entry-node set; `remediate` is never emitted.
- follow-ups: auto per-task adopt detection inside vibe-tasks (v1 §OQ);
  typed coverage classification in the verdicts schema if machine-level
  MISSING-vs-OUTDATED enforcement is ever wanted beyond the contract shape.
- upstream docs updated: CA-1216; workingmode registry + desktop
  workingMode.ts; scaffold-contract-tdd.md characterization clause.
