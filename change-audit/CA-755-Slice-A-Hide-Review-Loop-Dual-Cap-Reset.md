# CA-755 — Slice A: hide review-loop from pickers + dual-loop cap 5 with per-phase Round reset

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: feature
summary: review-loop hidden from chat/flow pickers (cloneable reference only); task-harness and bug-plan-harness cap 3 to 5 with LoopState.Round reset to 0 on plan approve so each phase gets its own budget
# --->8---

## Safe-fix header (contract §Fix guide)

- Prior CA read: 754 (BUG-360 scout draft), 752/753 (Task-325 approve semantics), 749 (plan park), 747/748 (bug-plan-harness, smoke hide), 744 (CP-58 closeout).
- Will-not-undo: dual back-edge continue routing (Task-304 / CA-712); freeze-before-code + cap semantics (CP-58 / CA-731); prose-DONE freeze path (CA-736/737); one-decision guard (BUG-353/289); park-cancel/Stop-wins (CA-741 / CA-749); BUG-360 scout-draft cache (CA-754); Task-325 park/approve contract (CA-749/752); review-loop stays cloneable (CA-403 lineage — template retained as reference, never deleted).
- R2 classification: engine/orchestrator + pack-YAML path, zero adapter branches → provider-agnostic; proven (not claimed) by the Claude/Codex/Grok matrix in the new tests below plus an agnostic grep (no `ProviderKey`/`providerKey` reference in `plan_approval_park.go` or the `interactive_service.go` hunk).
- Scope note: CP-53 P-2 (machine verdict gate before `advanceHubDoneThroughEdge`) is explicitly NOT in this slice — it moves to a separate CP-61; CP-53 closes with that leftover (Slice B docs).

## Change (pack YAML + runner, no migration)

- `agentpack/flow-pack/flows/review-loop.yaml` + `manifest.yaml`: `selectableIn: []`, drop `chatSubModes`/`chatUI` (rag-harness hiding pattern). The review-until-clean loop now lives inside the harness flows; the template stays `cloneable: true` as a reference (`startResolvedFlow` still resolves it).
- `agentpack/flow-pack/flows/task-harness.yaml` + `bug-plan-harness.yaml`: `policy.cap` 3 → 5 (dual-loop per-phase budget: plan review + code review share one `LoopState.Round`). Single-loop flows (`bug-harness`, `rag-harness`, `cp-harness`, `review-loop`) keep cap 3. No topology/adapter/CP-53-gate change.
- `runner/plan_approval_park.go` (new `resetPlanPhaseRound`): sets `LoopState.Round = 0` + diag log. Called from two sites, both only after a successful dispatch: `resumePlanApproval` approve path (Task-325) and `advanceHubDoneThroughEdge` for the `plan_synthesis --done--> preflight_contract_freeze` edge (`interactive_service.go`, keyed on `planSynthesisNodeID`/`planFreezeNodeID` identifiers — no new literals, `TestDomainHardcodeGuardMatchesFrozenBaseline` green). Never reset: parks (return earlier), continues, `synthesis --done--> audit` (code-loop hub must keep counting), terminal dones.

## Verification (R3 matrix + R1 + R2)

- 2 new test files, race path unchanged: `runner/plan_phase_round_reset_test.go` (freeze-dispatch reset × 3 providers, resume-approve reset × 3 providers, `synthesis→audit` no-reset, continue still parks blocked/cap at 5, Bug sub-mode offers nothing + old ref rejected) and `agentpack/dual_loop_cap_test.go` (2 dual-loop cap=5, 4 single-loop cap=3, hidden-but-present cloneable). All green, incl. re-run after sync.
- Old-test edits (operator-approved allow-list, minimal contract updates only): `bug_plan_harness_pack_test.go` (cap 3→5), `pack_test.go` (ChatUI empty), `chat_builtin_orchestration_test.go` + `chat_builtin_orchestration_handler_test.go` (Bug mode offers nothing; old ref rejected), `flow_executor_test.go` (3 `TestChatMode*` tests now lock the `400 invalid_flow_ref` rejection: never spawns, never flags engine-driven, never records history).
- R1: full `internal/runner` suite 19 failures ≅ baseline envelope 18 + 1 known flake. Stash-proven on the clean tree: the same 16-failure subset fails identically without this change, the 2 `TestValidatePassed*` fail identically (Windows `%PATH%` has no `true` executable — env), and `TestIntentClear_NoTOCTOUResurrectionUnderConcurrentAccess` is the documented full-suite-only concurrency flake (passes isolated 3/3 `-count=3` on this branch, passed on baseline this run). No failure touches the review-loop/cap/park paths; all 12 targeted tests green.
- `internal/agentpack` green; `internal/changecontract` green; `internal/flowgate` 2 failures are pre-existing Windows-env (`.sh` oracle fixtures: `%1 is not a valid Win32 application`), untouched by this slice. `git diff --check` clean; `gofmt -l` is repo-wide noise (CRLF checkout — 534 files flagged on the clean tree too), no new whitespace introduced.
