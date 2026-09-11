# CA-835 — Task-331: r-dod-complete block-or-explained gate + runner done-signal wiring

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-331
change_type: feature
summary: add r-dod-complete block gate (warn-downgrade with explanation) and wire DodTransitionedToDone/DodStatus signals from Task-*/BUG-*.md done transitions into both root and child flow gates
# --->8---

## Why

CP-47 P-3/P-4/P-5: a Task/BUG doc must not be marked done while its Definition-of-Done checkboxes remain open, unless the turn explains the deferral. Completes the CP-47 gate family started by CA-833.

## Change

- `flowgate/rules.go`: `TurnResult` += `DodTransitionedToDone` + `DodStatus`; rule `r-dod-complete` {step, marked_done_with_open_dod, dod_all_checked_or_explained, block}; `DocScopeRuleIDs` extended (verified against all 3 consumers: hubs skip, coding-child tier-1 enforces, audit tier-3 safely no-ops).
- `flowgate/evaluate.go`: case fires only when transitioned && Checked < Total; Total==0 delegates to r-dod-present; `hasValidDodExplanation` (## Deferred/## Open Items sections, phrase adjacent to open checkbox, FinalMessage) downgrades block→warn; block Detail lists OpenItems verbatim; `DodDoneTransition` reads metadata `Status: done` first, `done/` path segment backup — never commit messages; multi-doc transitions fail closed (open checklist wins).
- `runner/gate_hook.go`: `applyDodSignals` on both root (`runFlowGateAtEpoch`) and child (`runChildArtifactOutputGateAtEpoch`) TurnResults before `flowgate.Evaluate`; violations route through the existing `EventFlowGateViolation` Decision-Card emit (no new UI code).
- GitNexus impact: TurnResult 0/LOW, checkRule 4/LOW, DefaultRules 8/LOW, runFlowGateAtEpoch 7/LOW, runChildArtifactOutputGateAtEpoch 7/LOW.

## Tests

`r_dod_complete_test.go` (7 §10 signatures + 8 extra incl. merge + metadata-priority + fail-closed bias) and `task331_gate_hook_dod_test.go` (3 §10 signatures incl. emitted gate event). `go test ./internal/flowgate/...` fully green; runner targeted suite green. Full runner suite has ~24 pre-existing failures verified identical at HEAD-equivalence (incl. TestBUG327 deadlock from committed 162c7b02) — zero involve r-dod-complete.

Disclosed legacy-test touch: `TestDefaultRules` manifest extended append-only with `"r-dod-complete"` (repo rule-addition convention).

## Providers

Case 1 agnostic: deterministic Go over on-disk doc state + FinalMessage; no LLM, no provider adapters — identical for Claude/Codex/Grok turns.

## Prior claims intact

CA-833 (r-dod-present parser/gate), CA-834 (docscan), CA-695/CA-442/CA-441 — untouched; this change only appends fields, a rule, and a trigger case. CP-47 closed: doc moved to `07-Coding-Plan/done/` with all DOD items checked (task-harness E2E via TestTaskHarness* + gate-hook integration tests).
