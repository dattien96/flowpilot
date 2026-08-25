# CA-643 — Question card timeline rendering is part-colored

## What

In the chat timeline, a question card rendered as one flat purple block:
`[QUESTION] <prompt>`, every option row and the post-answer
`Answered: <choice>` confirmation all shared the same `--ask` purple
(`styleGate`, #b07cff). Hard to scan: no visual separation between the
question, the choices and the resolution. Operator report: "cả câu hỏi
[QUESTION], các option và câu trả lời sau khi tôi rep (Answered: …) đều chung
1 color là màu tím rất khó nhìn".

## Why

`renderQuestionText` (app.go) rendered every line of a `FormatHint
"question"` message with `styleGate`, only re-styling small tokens
(`1)`, `/approve`, …) via `styleLink`. The "Answered:" message uses the same
hint, so it was purple too.

## Fix

`apps/local-runner/internal/tui/app/app.go`:

- New styles (opencode-style hue hierarchy, reusing the existing palette):
  - `styleQuestionHead` — bold amber (#f0b429): `[QUESTION]` head + option indexes
  - `styleQuestionBody` — light (#ececec): prompt body, answer text, hints
  - `styleQuestionOpt` — accent blue (#4c8dff): option labels (clickable look)
  - `styleQuestionDesc` — dim (#9b9b9b): option descriptions, multi-select hint
  - `styleAnswer` — bold green (#3fb950): the `Answered:` confirmation head
- `renderQuestionText` rewritten as a part-aware renderer: branches on the
  line shape (`[QUESTION]` head, `N)` option rows with optional ` — desc`,
  `Answered:` confirmation, `[multi-select]` hint, prompt continuation).

The interactive question bar (`renderQuestionBar`) was already multi-colored
and is unchanged.

## Tests

Additive only — legacy suites untouched (no test asserted the old question
rendering):

- `apps/local-runner/internal/tui/app/ca643_question_render_test.go` (new):
  head+body split, option row with/without description, answered
  confirmation, already-answered replay, multi-select hint, wrapped-prompt
  continuation, and a plain-text-preservation sweep across all shapes.

## Verification

- `go test ./internal/tui/app/ -count=1 -run 'TestRenderQuestionText'` → ok.
- `go test ./internal/tui/app/ -count=1` → full TUI suite green (8.1s).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: question card timeline rendering is part-colored (head/body/options/answer distinct) (CA-643)
# --->8---