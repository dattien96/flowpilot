# CA-860 — CP-62 P-6 enrichment: decision-card choices and tampered tests into the sprint handoff

# ---8<--- flowpilot:change-ledger
feature_key: sprint-handoff
source_doc_id: Task-346
change_type: task
summary: sprint_handoff.v1 now consumes two more verified sources — the parked decision card (question + the human's chosen option or the runner's recommendation, remaining options as alternatives) captured from the continue feedback via captureDecisionChoice, and the oracle guard's tampered pre-existing test files (rs.lastTamperedTestPaths, captured at both gate_hook oracle sites) rendered as weakened_tests with justification; schema field names unchanged, empty sources keep the file byte-identical (fields omitted)
# --->8---

## Why

Task-342 shipped the full handoff schema but populated it only from verdict rows — `alternatives`, `weakened_tests` and the decision-card `chosen_option` had no source, so the next sprint could undo a prior sprint's deliberate choices or miss which tests were touched.

## Change

- `runner/interactive_service.go`: `interactiveRun` gains `decisionCardChosen` + `lastTamperedTestPaths` (additive fields).
- `runner/interactive_handlers.go`: `handleContinueFlow` calls `captureDecisionChoice(runID, feedback)` before resume — an option id/label match (case-insensitive) stamps the choice; prose answers leave it empty (the fallback never guesses).
- `runner/sprint_handoff.go`: `emitSprintHandoff` reads card/chosen/tampered under the existing `s.mu` snapshot; card entry appended to `decisions` (`what` = "user chose <id> — <question>" when chosen, else the question; `why` = chosen label + consequence, else detail, else "recommended: <id>"; `alternatives` = remaining option labels); tampered paths append `WeakenedTest{path, justification}`. New helpers `captureDecisionChoice` + `setTamperedTestPaths` in the same file.
- `runner/gate_hook.go`: both oracle-evaluation sites (root hook and child path) surface `oracle.Tampered` onto run state via `setTamperedTestPaths`.

## Tests

`runner/sprint_handoff_enrichment_test.go` (4, additive): card + chosen (captured through the real `captureDecisionChoice` path) → decisions entry with chosen why and alternatives [Stateless JWT]; card without choice → "recommended:" fallback, no "user chose"; tampered path → weakened_tests entry with justification; no sources → JSON omits decisions/weakened_tests/risks/alternatives (byte-compatibility with Task-342 output). Combined CP-62 follow-up surface (`HandoffEnrichment|SprintHandoff|DriftPause|ReviewACCoverage|InjectSkillContent|DecisionCard|RequestUserDecision`) green with `-race`.

## Providers

Case 1 provider-agnostic — run-state capture and deterministic JSON assembly; no adapter involvement.

## Prior claims intact

Task-342 schema and file format untouched (field names identical; JSON-still-valid-YAML-1.2); Task-342 verified-state-only rule preserved (card choice comes only from an explicit id/label match; tamper paths only from the oracle guard); Task-345 UI answer channel unchanged (the capture rides the same continue call the TUI/desktop already make); `open` stays an intentionally reserved empty field as documented.
