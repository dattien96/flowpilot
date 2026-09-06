# CA-754 — BUG-360: durable scout preflight draft cache (restart-proof freeze)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-360
change_type: bugfix
summary: scout preflight draft cached on parent at child settle and persisted via session snapshot plus runtime blob so post-restart freeze parses it after the transient child is gone
# --->8---

## Safe-fix header

- Prior CA: 749 (plan park), 747/748 (bug-plan-harness, smoke), 744 (CP-58), 741-743 (park/gate bounds). Will-not-undo: all of CA-749's list (dual back-edge routing, freeze-before-code, cap semantics, prose-DONE freeze, one-decision guard, park-cancel/Stop-wins).
- Correction to the filed BUG-360 Q-1: doc-derivation (option b as specced) is the wrong layer — the frozen input is the preflight (scout) JSON draft, not the Task/BUG doc. Implemented as a durable scout-draft cache instead. Same durability, correct source.
- R2: orchestrator/persist path, zero adapter branches → provider-agnostic (no provider switch in touched code; the freeze dispatch path is already provider-matrixed by run201295).

## Change (runner, no migration)

- Capture: `cachePreflightDraftLocked` (new, in `plan_approval_park.go`) called from `settleFlowChildTurnCompletedLocked` (caller holds `s.mu`); parse-gated with the same `ParsePreflightDraft` freeze uses — prose never overwrites, last parseable wins.
- Durable: `interactiveRun.preflightDraftResult` → `sessionStateOf` → `ProviderSessionState.PreflightDraftResult` → `session_runtime` jsonb (`preflight_draft_result`, additive) → `applySessionRuntimeBlob` → `reconstructRunInternal` mapping. Local file store carries the full struct; Supabase needs no migration.
- Read: `findPlannerResultForFreeze` tries the parent stash (parse-validated) after the live-children scan, before returning "".
- Behavior change surface: only the previous dead end (prose + no scout + no stash → escalate unchanged, fail-closed preserved).

## Verification (R3 + R1 + R2)

- 5 new tests (`bug360_preflight_draft_durable_test.go`), race-clean: capture parse-gating table (incl. nil/orphan safety), fallback order live > stash > "", e2e freeze-from-stash with dead scout child (the reported shape), fail-closed without any draft, restart round-trip (snapshot → local file store → reconstruct + blob leg).
- R1: related suites (freeze/contract, run201295/198699/63960/202550/45103/203966, continue/resume, session-runtime) green except stash-proven `TestResumeFlowWithFeedbackAfterEscalate` (identical message on clean tree); full suite 19 ≅ baseline 19 with membership churn both ways (`TestRun12613` flakes full-suite-only, passes isolated on both trees and together with new tests); zero old-test edits.
- agentpack + flowgate green; `go vet` clean; gofmt clean on new lines (4 flagged files pre-existing churn).
