# Task-113 — r-task Rule: Missing Task Doc Reprompt

## Metadata

- Document ID: `Task-113`
- Title: `r-task rule — reprompt when Task-NNN referenced but no Task doc added`
- Phase: `task`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `CP-35`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: `CP-35`
- Child Documents: none
- Related Documents: `Task-099`, `Task-100`, `BUG-140`, `CA-128`, `CA-126`
- Replaces: none
- Tags: `context-regression-engine, flow-gate, r-task`

## AI Quick View

### Summary

- Adds `r-task` as the 6th default rule in the Flow Gate
- Fires when AI final message references `Task-NNN` but no Task document appears in the git diff
- Issues a `reprompt` (same severity as r-ca and r-bug) with explicit file-creation instructions
- Mirrors r-bug exactly: FORMAT-REFERENCE guard, combined reprompt, `remediationFor` case

### Current Ask

- Implement r-task end-to-end: rule definition, evaluation, observation, reprompt guidance, tests

### Key Decisions

- `T-1` Trigger heuristic: `regexp.MustCompile(`\bTask-\d+\b`)` on `FinalMessage`; `ChangeType == "task"` reserved for future explicit signal
- `T-2` FORMAT-REFERENCE guard applied to `HasTaskDoc` to prevent scaffold templates from satisfying the predicate (same fix as BUG-141 for r-bug)
- `T-3` `remediationFor("task_referenced")` added to `enforce.go` so AI receives file-level creation instructions, not just a symptom string

### Constraints

- Must not fire when a real Task doc is present in the diff
- FORMAT-REFERENCE-TASK.md must never satisfy `HasTaskDoc`
- Reprompt text must include path `requirements/08-Task/done/Task-<NNN>.md` and FORMAT-REFERENCE-TASK.md reference

### Open Questions

- None

### Source Refs

- `CP-35`, `SD-20 §2`

## 1. Goal

When the AI completes a step and its final message references a Task-NNN identifier (indicating it is closing a tracked task), the Flow Gate should reprompt it to create the corresponding Task document if that document is absent from the git diff. This mirrors r-bug's behavior for BugFix documents and r-ca's behavior for change-audit notes.

## 2. Parent Links

- coding plan: `CP-35-Context-And-Regression-Engine-Rollout.md`
- tech design: `SD-20-Context-And-Regression-Engine.md`
- system spec: none directly
- specific upstream ids: `CP-35`, `SD-20 §2`

## 3. Trigger

During E2E testing of r-bug and r-ca, the same gap was identified for Task documents: the AI would complete a step, reference Task-113 in its summary, but never add `requirements/08-Task/done/Task-113-*.md`. No rule existed to catch this. The gap is symmetric to r-bug and should be closed with the same reprompt mechanism.

## 4. Exact Change

- `T-1` `rules.go` — Added `r-task` rule `{ID: "r-task", Trigger: "task_referenced", RequiredOutput: "task_doc", Action: "reprompt", Enabled: true}` between r-bug and r-tests
- `T-2` `observe.go` — Added `HasTaskDoc(diff []ChangedFile) bool` with FORMAT-REFERENCE guard (mirrors `HasBugFixDoc`)
- `T-3` `evaluate.go` — Added `taskIDRegex` (`\bTask-\d+\b`) package-level var; added `task_referenced` case in `checkRule` using regex on `FinalMessage`
- `T-4` `enforce.go` — Added `task_referenced` case in `remediationFor()` returning explicit file-creation instructions naming `requirements/08-Task/done/Task-<NNN>.md` and `FORMAT-REFERENCE-TASK.md`
- `T-5` `flowgate_test.go` — Updated `TestDefaultRules` to expect 6 rules; added `TestEvaluateRTaskFiresWhenTaskRefMissingDoc`, `TestEvaluateRTaskNoViolationWhenDocPresent`, `TestHasTaskDocIgnoresFormatReferenceFile`, `TestRepromptPromptRTaskIsActionable`

## 5. Touched Areas

- files: `apps/local-runner/internal/flowgate/rules.go`, `observe.go`, `evaluate.go`, `enforce.go`, `flowgate_test.go`
- modules: `flowgate`
- routes: none
- tables: none

## 6. Acceptance Check

- `go test ./internal/flowgate/...` passes all 30 tests including 4 new r-task tests
- `go build ./...` succeeds
- E2E-14: AI message containing "Task-NNN" with no Task doc in diff → gate fires r-task reprompt with file path instructions

## 7. Out of Scope

- Detecting Task references in tool calls or file changes (heuristic only covers `FinalMessage` for v1)
- `ChangeType == "task"` explicit signal (reserved for future runner-level signaling)
- UI changes for r-task display

## 8. Completion Notes

- result: All 4 r-task tests pass; full flowgate suite (30 tests) passes; clean build
- follow-ups: E2E-14 manual verification; SD-20 §2 update to add r-task to rule table
- upstream docs updated: CP-35 E2E table updated with E2E-14 row
