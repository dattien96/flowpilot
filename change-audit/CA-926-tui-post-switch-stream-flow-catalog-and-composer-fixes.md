---
id: CA-926
title: TUI fixes — post-switch orchestration stream reattach, /flow picker endpoint, drift-parked continue, provider model fallback, paste-token slash commands (BUG-428, 429, 430)
type: BugFix
feature: cli-tui
date: 2026-09-23
status: done
---

## Context

The live-verification wave exposed three TUI defects (four distinct behaviors):

- **BUG-428**: after `/provider <B>` on an active chat the server-side seed
  turn ran invisibly — tool events and approval requests never rendered and
  `/stop` was a no-op. Root cause was not the seed-envelope suppression (that
  guard intentionally hides only the seed's assistant text): `applyChatSwitched`
  reset `turnStream` but left `orchStream` attached to the OLD leg, so
  `cmdStartOrchestrationStream` bailed on `orchStream != nil` and the new leg's
  events had no consumer. A stale in-flight poll from the old stream could also
  deliver `orchStreamClosedMsg` after a new stream opened and kill it.
- **BUG-429**: `/flow` listed `(none)` for dev harnesses because
  `cmdFetchFlows` called `/client/chat/builtin-orchestration-options?subMode=…`
  — the chat-orchestration catalog — instead of `/client/flow-picker-options`,
  the user-startable catalog the resolver arms from.
- **BUG-430**: three polish defects:
  (a) `/continue` on a drift-parked *plain chat* flipped the loop to `running`
      but dispatched no turn — `resumeFlowWithFeedback` assumed every blocked
      loop had a hub/flow nodes; `autoOrchestrate=false` made
      `maybeAutoReinvokeHubWithNote` a no-op → composer soft-locked;
  (b) a provider switch that returned an empty model left the previous
      provider's model displayed (`devin · opencode/muse-…`);
  (c) a slash command typed after a collapsed `[Pasted N chars]` token was
      appended to the draft instead of parsed — command detection required the
      *raw* input to start with `/`.

## Changes

### BUG-428 — `internal/tui/app/`

- `turn_stream.go`: `orchStreamState` now carries its `ctx`; the stream loop
  tags every `orchStreamEventMsg`/`orchStreamClosedMsg` with the originating
  stream pointer. `stopOrchestrationStream` cancels the ctx (in addition to
  closing the channel), so a stale poll self-drops before delivering.
- `app.go`: `Update` drops orch msgs whose `st` tag does not match the live
  `m.orchStream` — a late old-leg event/close can no longer clobber the new
  stream.
- `chat_switch.go`: `applyChatSwitched` calls `stopOrchestrationStream()`
  before adopting the new handle so `cmdStartOrchestrationStream` starts a
  fresh stream on the new leg's event cursor. The seed-envelope suppression
  (`seedTurnActive`) is unchanged — operational events (tools, approvals,
  questions) flow through the orch stream as before.

### BUG-429 — `internal/tui/client` + `internal/tui/app`

- `client.go`: new `ListFlowPickerOptions(ctx, workingMode)` hitting
  `GET /client/flow-picker-options?workingMode=…` (served by
  `handleFlowPickerOptions`, which returns the five dev harnesses / two vibe
  harnesses).
- `app.go`: `cmdFetchFlows` calls `ListFlowPickerOptions`; the list render
  labels the section with the actual working mode instead of the hardcoded
  "(bug mode)".

### BUG-430 — `internal/runner` + `internal/tui/app`

- `interactive_service.go`: `resumeFlowWithFeedback` now detects
  `prevBlockReason == DriftPauseBlockReason` on a plain chat run
  (`parentRunID==""`, no flow nodes, `autoOrchestrate=false`) and calls the
  new `resumeDriftParkedChat`, which dispatches a real turn via `startTurn`
  (`durableResumeStepID` for the step, feedback or `"continue"` as the prompt).
  On dispatch failure the loop is re-parked with the reason
  (`continue failed to dispatch: …`) and the run stays actionable — it never
  sits `running` with nothing in flight.
- `chat_switch.go`: on `ChatSwitchResponse` with empty `Model`, the TUI falls
  back to the requested model, then the target provider's first catalog model,
  then `defaultModelForProvider` — a stale foreign model can never persist.
- `chat_paste.go` + `app.go`: new `pastedDraftSlashCommand()` strips collapsed
  paste tokens from the draft; if the remaining text is a `/cmd` line the
  command is dispatched directly (tokens stay in `pasteSegments`, the draft is
  preserved for the next Enter). Fires before expansion in the Enter path;
  ordinary pasted prompt text and slash-inside-text are unaffected.

## Tests (all added, none modified)

- `internal/tui/app/bug428_post_switch_stream_test.go` — switch reattaches
  orch stream; approval event on new leg renders; `/stop` reaches seed turn;
  stale `orchStreamClosedMsg`/`orchStreamEventMsg` from the old stream are
  dropped.
- `internal/tui/app/bug429_flow_catalog_test.go` — `cmdFetchFlows` uses the
  picker endpoint (httptest), dev harnesses listed and armable; mode label
  follows working mode.
- `internal/tui/app/bug430_model_paste_test.go` — empty switch-response model
  resolves to target-provider model; `/cmd` after `[Pasted N]` executes; token
  draft preserved; ordinary paste text unaffected.
- `internal/runner/bug430_drift_continue_test.go` — drift-parked chat continue
  dispatches a turn; pending-approval dispatch failure re-parks with the
  reason; in-process HTTP route test drives the real mux end-to-end.
- `internal/runner/bug429_flow_picker_endpoint_test.go` (in the runner file)
  — `/client/flow-picker-options?workingMode=dev` serves the five dev
  harnesses over the real mux.

## Live verification (bed: `/tmp/fp-live-k`, persistent runner :4466, fake
catalog via workspace supabase reset, pty driver)

- `/flow` lists `task-harness`, `bug-harness`, `bug-plan-harness`,
  `cp-harness`, `context-coding-review-synthesis`; `/flow task-harness` arms
  ("Flow armed: task-harness").
- Paste burst collapsed to `[Pasted 62 chars]`; typing `/status` executed the
  command (status card rendered) and the token stayed in the draft.
- Mid-run `/provider devin`: `⇄ switched to devin ·
  devin/claude-opus-5-5-medium` — footer carries the devin model, not the
  stale `google/gemini-2.5-flash`.
- The post-switch seed turn streamed (`▸ 2 tool calls`); `/stop` →
  `Stopped.` — the stop reached the seed leg.
- Drift park seeded into run-1's durable session (`loop_state
  {status:blocked, blockReason:drift}`), runner restarted — park rehydrated.
  `POST /agent-loop/continue` → `loopState: running` **and** a real provider
  turn dispatched (`turn-38` on opencode ACP); `/agent-loop/stop` →
  `stopped`.

## Regression delta

- `internal/tui/app`: 2 failures — `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle`
  (3 provider subtests) and `TestApprovalBarAndStopAreClickable` — identical on
  baseline (pre-existing).
- `internal/tui/client`: clean.
- `internal/runner`: Cluster K focused tests green; suite delta matches the
  previously-classified pre-existing set (no new failures attributable to this
  change).

## Caveats

- The Supabase project catalog was unreachable in the live bed (DNS for the
  demo ref); verification ran against the runner's offline fake catalog —
  the picker/builtin endpoints exercised are identical either way.
- The drift park was seeded into the durable session record rather than
  produced by an organic ≥80 drift score; the park → `/continue` → dispatch
  path itself ran end-to-end against a live provider turn.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-428
change_type: bugfix
summary: TUI post-switch stream isolation, flow picker catalog, composer fixes
# --->8---
