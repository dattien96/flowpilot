# CA-544 - post-flow follow-up chat flashes "thinking…→done" and labels the turn as a flow re-run

## Problem

After a flow loop reaches `done` (all steps DONE, no active agents), sending a
follow-up chat turn in the TUI flashed the statusline: "thinking…" appeared,
then immediately snapped to "done", then to "streaming…/turn running…" when the
reply arrived. The runner is correct (the follow-up is admitted as plain hub
chat and the reply is produced); the flash is TUI-only (run-107774).

Root cause: `processInput` sets `connStatus = ConnRunning` + `"thinking…"` and
returns `cmdSendTurn`, but the SSE stream only opens later via
`turnStreamOpenedMsg`. A steps poll (or agent graph message) landing in that gap
hit `settleFlowIfDone()`, which saw `flowLoopDone()==true` + `turnStream==nil`
+ no focused child and flipped the chrome to ConnIdle/"done" before the reply
started. Separately, the follow-up turn was relabeled "flow running…" by
`turn_started`/`turnStreamOpenedMsg`, and `turn_completed` re-entered
ConnWaiting/"flow running…" because an orchestration stream was attached.

## Fix (TUI-only; runner unchanged — desktop unaffected)

- `model.go`: new `turnSendPending bool` — a user turn is POSTed but the stream
  has not opened yet.
- `app.go` `processInput`: set `turnSendPending = true` on both the first-run
  (`cmdStartRun`) and follow-up (`cmdSendTurn`) send paths.
- `app.go` `turnStreamOpenedMsg` / `RunStartedMsg` / `turnStreamClosedMsg` /
  `ErrMsg`: clear the flag when the stream opens, run starts, stream closes, or
  a send fails.
- `step_runtime.go` `settleFlowIfDone`: return early while `turnSendPending` so
  the send→stream window is never settled to "done". (Kept precise: a stale
  `ConnRunning`/"flow running…" on a finished flow still settles — run-101411.)
- `app.go` `turn_started` and `turnStreamOpenedMsg`: when `flowLoopDone()`, use
  plain-chat labels ("turn running…"/"thinking…") instead of "flow running…" so
  the follow-up reads as normal chat, not a flow restart.
- `app.go` `turn_completed`: when `flowLoopDone()`, settle to ConnIdle/"done"
  + `TurnDoneMsg` instead of re-entering ConnWaiting/"flow running…".
- `app.go` `message_delta`: post-done follow-ups stream with "streaming…" like
  plain chat (normal in-flight flow chrome stays quiet, CA-511).

## Files

- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/model.go`
- `apps/local-runner/internal/tui/app/step_runtime.go`
- `apps/local-runner/internal/tui/app/ca544_post_done_followup_chat_chrome_test.go`
  (new)

## Tests

Additive: `ca544_post_done_followup_chat_chrome_test.go` (3 providers) covers the
send-gap steps poll must NOT settle (stays ConnRunning/"thinking…", handle keeps
"running", [stop] stays armed), stream-opened keeps "thinking…", `turn_started`
uses "turn running…", `message_delta` streams "streaming…", `turn_completed`
settles to ConnIdle/"done" + `TurnDoneMsg` even with an attached orchestration
stream, plus near-misses: an in-progress loop still parks on "flow running…",
and an idle done loop at ConnWaiting still settles (CA-515 preserved).
Pre-existing settle tests (`tui_flow_done_settle_test.go`,
`run101411_focused_child_liveness_test.go`) pass unchanged.

## Verification

- `go test ./internal/tui/app/ -count=1` green except the two pre-existing
  environmental `TestCmdFocusAgent_*` network failures (confirmed unchanged).
- gofmt clean on all four edited files (LF-normalized temp copies); CRLF
  preserved on the three edited Go files, new test file CRLF.
- Runner side not modified; follow-up admission behavior (BUG-302/CA-382)
  untouched.

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CA-544",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-flow-chrome", "tui-statusline", "tui-chat-send"],
    "upstream_docs": ["SS-07", "CP-05"],
    "status": "verified"
  }
}
```
