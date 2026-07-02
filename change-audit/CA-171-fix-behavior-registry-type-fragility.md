# CA-171: Fix Type-Fragile exitCode/issues Handling in Behavior Registry (BUG-NOTE-CP42 #27, #30)

## Scope

Verified and fixed two P2/P3 issues from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: `behaviorCommandValidate` and `behaviorValidationSummarize` both used bare type assertions that silently misread valid data of a different-but-equivalent Go type.

## The bugs

- **#27**: `behaviorCommandValidate` read `in.RawArgs["exitCode"].(int)`. `RawArgs` is frequently populated by decoding JSON into `map[string]any`, where every JSON number (including a plain exit code) decodes as `float64`, never `int`. A real, nonzero exit code arriving this way silently failed the type assertion, fell back to the zero value, and the behavior reported `"command validation passed"` for a command that actually failed.
- **#30**: `behaviorValidationSummarize` read `in.RawArgs["issues"].([]any)`. An internal Go caller passing a `[]string` or a typed issue slice (e.g. `[]ReviewIssue`) instead of `[]any` would fail the same way — the assertion silently produced a nil slice, `len() == 0`, and the behavior reported `"no open issues"` for a call that actually had some.

Both bugs share the same root cause (a single-type assertion standing in for "give me this value in whatever numeric/slice shape it's actually in") and the same class of consequence: a real failure/issue silently reads as success.

## Fix

Added two small coercion helpers in `behavior_registry_builtin.go`:

- `toIntArg(v any) (int, bool)` — handles `int`, `int32`, `int64`, `float32`, `float64`.
- `issuesLen(v any) int` — handles `[]any`, `[]string`, `[]ReviewIssue` directly, and falls back to `reflect.ValueOf(v).Len()` for any other slice/array shape, so a future typed slice doesn't need its own case added here.

`behaviorCommandValidate` and `behaviorValidationSummarize` now go through these instead of a bare type assertion.

## Verification

- New test `TestBehaviorCommandValidateHandlesJSONDecodedFloat64ExitCode`: round-trips `{"exitCode":1}` through a real `json.Unmarshal` (confirming the test's premise — it decodes as `float64`, not `int`) and asserts the behavior reports `continue`, not `done`.
- New test `TestBehaviorValidationSummarizeCountsNonAnySliceIssues`: passes a `[]string` issues list and asserts the behavior reports `continue`, not `done`.
- Existing `TestBehaviorCommandValidateMapsExitCode`/`TestBehaviorValidationSummarizeWithIssuesContinues` (the pre-existing `[]any`/plain-`int` coverage) pass unchanged.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: replace bare type assertions in behaviorCommandValidate/behaviorValidationSummarize with coercion helpers (toIntArg/issuesLen) so a JSON-decoded exit code or a non-[]any issues slice no longer silently reads as success/empty
# --->8---
