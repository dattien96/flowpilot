# BUG-430: TUI polish batch — `/continue` on drift-parked chat soft-locks composer; `/provider` keeps stale foreign model; slash commands swallowed by paste-token draft

## Metadata

- Document ID: `BUG-430`
- Title: `Three TUI UX defects: composer soft-lock after /continue on drift-parked chat; stale cross-provider model after /provider switch; /-commands appended to collapsed paste drafts`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: evidence `~/fp-beds/lt-evidence/ui/RESULT.md` (BUG-LIVE-UI-1, BUG-LIVE-UI-4, BUG-LIVE-UI-3)
- Feature Keys: `tui`, `composer`, `provider-switch`, `drift-park`, `slash-commands`

## AI Quick View

### Summary

- **(a) `/continue` soft-lock:** resuming a drift-parked *chat* run flips status to `running` but spawns no turn (`autoOrchestrate=false` → `maybeAutoReinvokeHubWithNote` no-ops); TUI shows `[stop] · in progress — Enter disabled` indefinitely (`turnActive=true`, `flowBlocked=false`) — composer soft-locked >2 min, recovery only via Ctrl+C.
- **(b) Stale foreign model:** after `/provider devin`, `/status` + footer showed `devin · opencode/muse-spark-1.2-contributor-free` — runner returned empty `Resp.Model` and the guard `if msg.Resp.Model != ""` lets the previous provider's model id persist; `routeProviderSwitch` is invoked with `model=""`.
- **(c) Paste-token swallow:** when a long prompt has collapsed to a `[Pasted N chars]` token, a subsequently typed `/…` is appended to the draft instead of parsed as a command (input no longer starts with `/`).

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

Three independent TUI polish defects observed in the same live wave:

1. `/continue` on a drift-parked chat run leaves the composer soft-locked with a permanent "in progress" status line and no turn actually running.
2. `/provider <key>` on a live chat keeps the previous provider's model id, producing an invalid `provider · foreign-model` pair in `/status`/footer.
3. Typing a slash command while a collapsed `[Pasted N chars]` token occupies the draft appends the text to the draft rather than executing the command.

### Expected

1. `/continue` either re-drives a turn or reports why nothing was dispatched; the composer never soft-locks on a dead run.
2. A provider switch clears or replaces the model when the runner returns no model (or validates the pair against the new provider's catalog).
3. Slash commands are recognized regardless of draft content (or the draft state is surfaced clearly).

### Actual

1. `loopState.status=running`, no turn spawned; UI stuck `turnActive=true` >2 min; only Ctrl+C recovered. Evidence run: `run-9` drift-parked chat.
2. `devin · opencode/muse-spark-1.2-contributor-free` shown; worked around via `/model devin/swe-1-6`; devin leg's `chat_provider_switch` record carried `fromModel:""`.
3. `/…` text merged into the paste-token draft — contributed to wedge confusion during the session.

### Impact

(a) Dead-looking UI requiring Ctrl+C; (b) invalid provider/model pair shown and possibly submitted on the next turn; (c) commands silently become prompt text — all erode trust in TUI state.

## Reproduction

1. Drive a chat run to drift-park (`drift_pause_required`, `loopState.blocked=drift`); `/continue` → status `running`, no turn, composer locked.
2. On a live chat with provider A model m: `/provider B` → `/status` shows `B · m` (m not in B's catalog).
3. Paste a long prompt (collapses to `[Pasted N chars]`), then type `/status` → appended to draft, not executed.

## Root cause

- (a) `autoOrchestrate=false` on drift-parked chat runs → `maybeAutoReinvokeHubWithNote` no-ops after `/continue` flips status to `running`; TUI sets `turnActive=true` with nothing to track (`internal/tui/app`, continue/continue-path; agent-graph `loopState.status=running`).
- (b) `apps/local-runner/internal/tui/app/chat_switch.go:322-323` — `if msg.Resp.Model != "" { m.model = msg.Resp.Model }` lets a stale foreign model persist when the switch response carries no model; `apps/local-runner/internal/tui/app/app.go:4694` — `routeProviderSwitch(<provider>, "")` called with `model=""`. (Evidence cited `app.go:322`; verified location at HEAD `435e336b` is `chat_switch.go:322`.)
- (c) Composer input gate requires the input to *start with* `/`; a collapsed paste token in the draft breaks that prefix check.

## Evidence

- `~/fp-beds/lt-evidence/ui/RESULT.md` — BUG-LIVE-UI-1 (`ui-stuck-check.txt`, `ui-stall-later.txt`, `~/.flowpilot/tui.log`, agent-graph `loopState.status=running`); BUG-LIVE-UI-4 (`ui3-status-devin.txt`, `ui3-model-devin.txt`, devin-leg `chat_provider_switch` `fromModel:""`); BUG-LIVE-UI-3 (driving notes + `ui-stuck-check.txt` sequence).

## Severity

- `low` — three polish defects; (a) is the most disruptive (soft-lock), all recoverable without data loss.

## Completion Notes (implemented 2026-09-23, CA-926b)

- (a) `resumeFlowWithFeedback` detects `blockReason=="drift"` on a plain chat
  run and calls the new `resumeDriftParkedChat`, which dispatches a real turn
  (`startTurn` with `durableResumeStepID` + feedback/"continue" prompt). On
  dispatch failure the loop re-parks with the reason — never a dead
  `running` state. Tests: `internal/runner/bug430_drift_continue_test.go`
  (dispatch, repark-on-failure, real-mux route e2e). Live: seeded a drift
  park into run-1's durable session, restarted, `POST /agent-loop/continue`
  → `running` + real opencode turn `turn-38`.
- (b) `applyChatSwitched` now falls back to requested model → target
  provider's first catalog model → `defaultModelForProvider` when the
  switch response carries an empty model. Live: `/provider devin` →
  `devin/claude-opus-5-5-medium` in footer (was stale `google/gemini-…`).
- (c) `pastedDraftSlashCommand()` strips collapsed paste tokens; a remaining
  `/cmd` line dispatches as a command while the token draft is preserved.
  Live: `[Pasted 62 chars]` + `/status` → status card rendered, token kept.
