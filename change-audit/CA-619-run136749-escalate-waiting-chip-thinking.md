# CA-619: run-136749 escalate park showed no Continue/Stop and kept Thinking

## What

Rag Harness run-136749 parked `blocked: escalate` with step `implement` `WAITING_USER_APPROVAL` (Grok). Banner said `/continue`, but no `[Continue]/[Stop]` chip and status kept `Thinking 1m17s` + `[stop]`. User typed `next` (no unpark) — flow appeared stuck.

## Why

- Runner stamp: escalate/cap parks via `setFlowStepAwaitingUser` to `WAITING_USER_APPROVAL` (same string as YOLO/ask_user). Also stamps rag-harness `implement` when hub.inline absent.
- TUI bug: `flowHasActiveAgents()` treated `waiting_user_approval` as live. `flowLoopBlocked()` is `blocked && !flowHasActiveAgents()` → with implement WAITING it returned false → `renderBlockedBar()` empty. `workIsLive()`/`turnIsActive()` checked `flowHasActiveAgents()` before `flowLoopBlocked()` → Thinking + `[stop]` stayed on (screenshot).
- Contract test `TestBlockedBar_NoChipsWhenRunningChild` (child `running` must win) is correct and must not be weakened; `waiting_user_approval` is the mis-classified park stamp.
- Banner for `escalate`/`cap` did not surface `GateReason`.

## Fix

- **agents_focus.go** `hasLiveWorkingChild()`: `running`/`spawned` = live; `waiting_approval`/`waiting_question` = live only when `approval`/`question`/`gate` not yet mounted (so YOLO gate still wins); `waiting_user_approval` = park stamp, never live.
- **step_runtime.go** `flowLoopBlocked()`: `blocked` && no `approval`/`question`/`gate` && `!hasLiveWorkingChild()` && no `turnStream`/`focusedChildLive` → true. Previously used `flowHasActiveAgents()` → hid Continue.
- **app.go** `turnIsActive()`/`workIsLive()`: check `hasUnresolvedAttention()` then `flowLoopBlocked()` before `flowHasActiveAgents()` — blocked park is not live even though `flowHasActiveAgents()` is true for WAITING (run-136749).
- **step_runtime.go** `showBlockedBanner()`: append `GateReason` for `escalate`/`cap` (as for `delegate_failed`) via `truncateRunes(gate,120)` (rune-safe). Keeps `delegate_failed` path.

Will not undo: BUG-231 park contract, CA-616/617/618 delegate_failed chips, `TestBlockedBar_NoChipsWhenRunningChild`, YOLO Approve/Deny, ask_user `renderQuestionBar`.

## Tests (additive)

- `tui/app/run136749_escalate_waiting_chips_test.go` (agnostic, 1 representative): `TestRun136749_EscalateWAITINGHasContinueStopNoApprove` (WAITING park → Continue/Stop, no Approve, blocked true, !workIsLive, !turnIsActive), `TestRun136749_YOLOApprovalStillWinsOverPark` (approval present → no Continue), `TestRun136749_QuestionWinsOverPark`, `TestRun136749_RunningChildStillHidesChips` (running wins, contract lock), `TestRun136749_CapAndDelegateFailedAlsoPark` (cap/delegate_failed + WAITING → blocked), `TestRun136749_BannerIncludesGateReason` (escalate gate in banner + rune-safe truncate).
- Old: `go vet ./internal/tui/app`, `go test ./internal/tui/app` (full suite 21s) pass, including `TestBlockedBar_*`, `TestRun127174_SSEAgentGraphSnapshotShowsBlockedChips`, `TestStatusSpinner_*` (CA-537).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-43
change_type: bugfix
summary: escalate park WAITING chip + Thinking off — treat WAITING_USER_APPROVAL as park stamp not live work (run-136749)
# --->8---
