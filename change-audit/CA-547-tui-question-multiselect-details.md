# CA-547 - TUI question card drops multiSelect/description/answer; no multi-select submit

## Problem

Runner `provider_event.go` already ships rich question data — `MultiSelect`,
`QuestionOption{Label, Description, Value}`, and (on replay) `Answer []string`
(BUG-StaleQuestion) — and the runner's `handleAnswerQuestion` accepts a JSON
string **or** string array. Desktop (`QuestionCard.tsx`) renders option
descriptions, checkbox toggles for multi-select, and a Submit button that posts
`choice: string[]`. The TUI kept ignoring all of it: options rendered label-only,
`user_question_required` with an `Answer` replay re-showed an interactive form
for a question already answered, and `AnswerQuestion` always posted a single
string.

## Fix (TUI + shared client only; runner + desktop unchanged)

G3 of the G1/G2/G3 interactive-card parity plan. No provider branch.

- `internal/tui/client/client.go`: `ProviderEvent` gains `Answer []string`;
  `QuestionInfo` gains `MultiSelect bool`; new `AnswerQuestionMulti` posts
  `choice` as a string array (runner `parseChoice` already accepts arrays).
- `model.go`: `QuestionState` gains `MultiSelect bool` and `Selected []string`
  (toggled option values; submitted only on explicit submit).
- `app.go` `user_question_required`: replay (`len(ev.Answer) > 0`) renders a
  read-only `[QUESTION] {id} already answered: {answers}` line, never mounts a
  card; otherwise the card carries `MultiSelect`. New `/submit` slash command.
  `renderQuestionBar` shows `[x]`/`[ ]` markers and a `[Submit]` chip for
  multiSelect questions (single-select bar output unchanged).
- `chat_question.go`: `formatQuestionMessage` now takes a multi flag, appends
  each option's `description` (`label — description`) and a multi-select hint
  line; `submitQuestionAnswer` toggles instead of submitting in multi mode;
  `toggleQuestionSelection` flips one option in `Selected`;
  `submitQuestionSubmit` sends the set via `cmdAnswerQuestionMulti`
  (`QuestionResolvedMsg` joined with ", "); empty selection is rejected.
- `mouse.go`: `hitQuestionChrome` maps multi options to `qtoggle:<i>` and the
  chip to `qsubmit` (approve/deny not consumed by option clicks in multi mode);
  dispatch toggles or submits.
- `history.go`: hydrate `MultiSelect` from the run snapshot so a rehydrated
  pending question keeps its multi semantics.

## Files

- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/model.go`
- `apps/local-runner/internal/tui/app/mouse.go`
- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/chat_question.go`
- `apps/local-runner/internal/tui/client/client.go`
- `apps/local-runner/internal/tui/app/ca367_question_multiselect_test.go`
  (new)

## Tests

Additive: `ca367_question_multiselect_test.go` covers answered-question replay
rendering read-only with no card, MultiSelect/description carried from the event
with the multi hint + Submit chip, typing `1`/`2`/label toggling (never
submitting) with `[x]` markers, `/submit` posting a `choice` string array to
the mock runner, empty-selection `/submit` rejection, clickable `qtoggle` /
`qsubmit` targets, click-toggle then click-submit posting the array, and a
single-select click still submitting an immediate single string (no array).

Pre-existing question/approval tests pass unchanged (`chat_question_select_test.go`,
`ca195_...`, `ca246_...`, `run97624_...`, `chat_ux_actions_test.go`,
`task291_...`, `tui_approval_waiting_copy_test.go`, `cp56_signatures_test.go`).

## Verification

- `go test ./internal/tui/app/ ./internal/tui/client/ ./internal/runner/
  -count=1` green except the two pre-existing environmental
  `TestCmdFocusAgent_*` network failures in `tui/app` (confirmed unchanged).
- gofmt clean on all edited/new files (LF-normalized temp copies); CRLF
  preserved; `go vet` clean.
- Provider-agnostic (cross-provider-parity Case 1): no `providerKey` branch
  anywhere; the card renders the runner's `Options`/`MultiSelect`/`Answer`
  verbatim.

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CA-547",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-question", "tui-gates", "tui-composer", "tui-client"],
    "upstream_docs": ["SS-07", "CP-05", "BUG-246", "CA-545", "CA-546"],
    "status": "verified"
  }
}
```
