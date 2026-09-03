# CA-716 — Grok no-reply notice after denied-permission turn (BUG-342)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-342
change_type: bugfix
summary: emit an honest [no reply text] notice when a Grok turn is cancelled after a FlowPilot-denied permission and no answer text exists, so scan/plan denies no longer render as a blank bubble (parity with opencode CA-713)
# --->8---

## Problem

In a scan/plan chat the runner auto-denies any write/unknown Grok tool via the
posture bridge. Grok treats the denial as a hard stop: it cancels the whole
prompt (`session/prompt_complete` with `agentResult:null`,
`stopReason:"cancelled"`, `cancellationCategory:"PermissionRejected"` — live
run-464841) and produces NO answer text. `grokAdapter.emitTerminal` only
special-cased `refusal`/`error`; `cancelled` fell through to a blank
`EventTurnCompleted` — a blank bubble indistinguishable from a broken turn.
Same class as opencode BUG-341, which CA-713 fixed with a no-reply notice.

## What changed (`grok_adapter.go`, `grok_permission.go`)

- `grokAdapter.permissionDenied map[string]string` (sessionID → blocked
  tool/command excerpt), reset per new turn (`SendTurn`) and cleaned up with
  the other per-session state.
- `handleInbound` marks a deny on every deny outcome (bridge-less fallback,
  expiry/interrupt error, and explicit deny decision) — mirroring opencode's
  `markOpencodePermissionDenied`. YOLO auto-approve and approvals never mark.
- `emitTerminal`: when `stopReason=cancelled` (case-insensitive) AND
  `permissionDenied` this turn AND final text is empty → emit one
  `EventMessageDelta` + `EventTurnCompleted` with
  `[no reply text] Grok aborted the turn because a tool permission was denied
  (<blocked tool/command>) … Switch posture (plan/code) or approve the tool to
  continue.` plus a `[grok-acp] WARN … permissionDenied=true` log line.
  `refusal`/`error`, user-initiated cancels (no deny), and turns that streamed
  text are unchanged.

## R1 — old-suite regression evidence

- New additive tests pass; all legacy grok adapter / YOLO / decision-vocabulary
  tests pass (`TestGrokAdapterYolo*`, `TestGrokAdapterApproveRoundTrip*`,
  `TestGrokEncodePermissionDecision*`, posture suite).
- Full `./internal/runner` run vs post-344 state: no new failure in any
  grok/approval/posture code path. Three unrelated flow-engine harness tests
  flaked under full-suite load (`TestAdvanceHubDoneThroughEdgeDispatchesHubNotify`,
  `TestRun75035_SeedChildIgnoresSiblingCodexSessionPollution`,
  `TestMultiWorkspaceRunsIndependent` — the last flip-flopped fail→pass→fail
  across three full runs) and each passes in isolation (`ok 0.459s`); none
  touch the grok adapter/notice code. Not regressions from this change.
- No legacy test was edited (additive only).

## R2 — cross-provider parity (Q-3 answered)

Per-provider denied-permission shape, verified against adapter code:

- **Claude:** a denied tool is answered per-tool with `{"behavior":"deny"}`
  (claude_adapter.go handleInbound) — the model receives the blocked-tool
  result and CONTINUES the turn, so an answer text can still be produced. No
  blank-after-deny shape → no notice needed.
- **Codex:** a denied permission is answered with an empty permission profile
  "so Codex can continue without the extra permission"
  (codex_adapter.go:529-530) — Codex continues and can produce a final answer.
  No blank-after-deny shape → no notice needed.
- **opencode:** aborts the agent loop after a denied tool with no text —
  fixed by CA-712 (replay recovery) + CA-713 (notice). Already shipped.
- **Grok:** cancels the whole prompt with `agentResult:null` — this CA adds the
  notice (no `session/load` replay exists for Grok; `agentResult:null` is
  ground truth the model never wrote an answer).

Only Grok needed the change; the classifier and the deny decision are
untouched (the notice is informational only, D-1).

## R3 — new coverage (`bug342_grok_no_reply_notice_test.go`, additive)

- **Positive:** deny + `cancelled` + empty text → `[no reply text]` delta
  naming the blocked tool + FinalMessage == notice, no `EventTurnFailed`.
- **Negative — user cancel:** `cancelled` with no deny → no notice, blank turn
  preserved.
- **Negative — text wins:** deny + streamed `lastText` (and deny + result text)
  → no notice, text is the final message.
- **Negative — refusal/error:** unchanged `EventTurnFailed`, no notice.
- **Mark/query lifecycle:** not denied → mark → denied with tool excerpt;
  session isolation.

## Honest gaps

- Post-fix LIVE runner E2E still pending (rerun run-464841 in scan: denied
  `git commit`/`rm` → notice renders in TUI/Desktop + transcript; scan compound
  reads now approve per CA-715 so the notice is only for genuinely-write
  denies). Unit tests are the R1–R3 evidence for this commit.
- The notice is provider-facing English text rendered as assistant text; the
  terminal carries `FinalMessage` for TUI and Desktop.
- The blocked-tool excerpt is truncated to 60 chars; stored per session only
  during the live turn.
- If a future Grok build lets the model reply after a denial, the notice
  becomes dead code on that path (text wins) — harmless.

## Prior CA not undone

- CA-714 (always-ask env / no `--always-approve`, BUG-343) intact — this
  notice keys on the deny outcome the gate produces.
- CA-715 (compositional read-only exec, BUG-344) intact — genuinely-write
  denies still occur and now surface a notice.
- CA-713 (opencode notice) and the opencode `permissionDenied` pattern are
  untouched; Grok mirrors the pattern without altering opencode.
