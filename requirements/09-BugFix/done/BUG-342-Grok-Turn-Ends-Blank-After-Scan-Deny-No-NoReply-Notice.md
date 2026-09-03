# BUG-342 — Grok chat turn ends blank after scan/plan read-only deny: no no-reply notice (parity with opencode BUG-341/CA-713)

## Metadata

- Document ID: `BUG-342`
- Title: `Grok chat turn ends blank after scan/plan read-only deny — no no-reply notice (opencode already fixed by BUG-341/CA-713)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-02`
- Last Updated: `2026-09-02`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP](../../07-Coding-Plan/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/CP-56-Terminal-TUI-Chat-And-Flow-Client.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- Child Documents: `none`
- Related Documents: `BUG-341` (same class, opencode — fixed via CA-712 replay recovery + CA-713 no-reply notice; no BugFix doc file, tracked in CA notes), [CA-712](../../../change-audit/CA-712-opencode-replay-recovery-denied-permission.md), [CA-713](../../../change-audit/CA-713-opencode-no-reply-notice-denied-turn.md), run-464841 (`.flowpilot/cli-runner.log` 2026-09-01 17:36–17:38 UTC), `grok_adapter.go`
- Replaces: `none`
- Tags: `ai-providers, grok, scan-plan-code, read-only-posture, approval-gate, blank-turn, severity-medium`

## AI Quick View

### Summary

- Live `run-464841` (gate-sandbox, Grok `grok-4.5`, scan posture, prompt `test B4-inplace, toi la Nam`): the read-only posture auto-denied Grok's `run_terminal_command` (`posture_read_only_scan` → `reject-once`). Grok then **aborted the whole prompt**: `session/prompt_complete` arrives with `agentResult: null`, `stopReason: "cancelled"`, `cancellationCategory: "PermissionRejected"` — no answer text exists anywhere (log lines 81163–81515).
- The Grok adapter's terminal handler (`grok_adapter.go` `emitTerminal`, lines 542–571) only special-cases `stopReason == "refusal" | "error"`; `cancelled` falls through to `EventTurnCompleted` with an **empty FinalMessage** → TUI/Desktop show a blank bubble, no notification — exactly the BUG-341 blank shape that opencode already fixed via CA-713's no-reply notice.
- Unlike opencode, Grok has no `session/load` replay store to recover an answer from (`agentResult: null` is ground truth — the model never wrote one). The fix is **only** the notice layer: emit an honest `[no reply text] …` delta + FinalMessage when a denied-permission turn ends cancelled and empty.
- This is the reported-provider half of a cross-provider parity gap: shared turn-shape (denied permission → provider aborts → no answer) but per-provider adapters; Claude/Codex must be checked for the same blank shape (R2 parity matrix in Validation).

### Current Ask

- Emit a no-reply notice (mirroring opencode's CA-713 text) when a Grok turn: (a) had a FlowPilot **denied** permission this turn, (b) ends `stopReason="cancelled"` / `PermissionRejected`, and (c) produced no final text. The notice must name the blocked tool/command and the posture so the user understands why the turn ended and what to do (switch posture / approve).
- Verify (or explicitly prove absent) the same blank-shape for **Claude and Codex** before closing (cross-provider-parity R2).

### Key Decisions

- `D-1` The notice is **informational only** — it must never change the deny decision, the optionId sent back, or the permission outcome (read-only posture discipline stays runner-side, unchanged).
- `D-2` Detection keys off the **terminal result**, not only the deny flag: emit the notice only when `cancelled` + deny-this-turn + empty text all hold. A user-initiated cancel (no FlowPilot deny) is NOT this bug — it must keep today's behavior (open question Q-2 whether it should also get a lighter notice).
- `D-3` Reuse the existing `permissionDenied` per-session tracking pattern from `opencode_adapter.go` (`markOpencodePermissionDenied` / `permissionDeniedFor`, lines 473–492) — Grok gets the same mark-on-deny hook at its `session/request_permission` deny path.
- `D-4` Notice wording mirrors CA-713's proven phrasing (`[no reply text] … aborted the turn right after a tool permission was denied … Switch posture (plan/code) or approve the tool to continue.`) with Grok's tool name + command interpolated from `ApprovalDetails` for correlation.

### Constraints

- additive-tests-only: new tests only (`grok_*_test.go` / new `bug342_*_test.go`); do not edit existing adapter suites.
- oracle-rule: failing old tests → fix production code, never the assertion.
- Read-only posture semantics are NOT in scope to relax (that is BUG-344). BUG-342 only surfaces the outcome honestly.
- The `reject-once` optionId choice (`grokEncodePermissionDecision`) is untouched — it already sends the correct deny (live-verified in run-464841 log: `send {"result":{"outcome":{"optionId":"reject-once",...}}}`).
- Parity: the notice layer must work for both `flowpilot chat` (TUI) and Desktop — one `EventMessageDelta` + `FinalMessage`, same as opencode.

### Open Questions

- `Q-1` Should the notice also fire when the turn is cancelled WITHOUT a FlowPilot deny (genuine user stop / provider-side cancel with empty text)? Today that path also renders blank. Recommended: separate lighter notice only if cheap; otherwise keep out of scope (opencode CA-713 also keys on `permissionDenied`).
- `Q-2` Does Grok ever emit a usable `agentResult` on a LATER prompt in the same session (recovery-by-next-turn like opencode's session/load replay)? If live evidence shows the answer can surface on the next prompt, a replay attempt may be worth adding before the notice — but run-464841 shows `agentResult: null`, so the notice is the primary fix.
- `Q-3` Claude and Codex: do their denied-permission turns also end with an empty answer + no notice today? Parity matrix (V-4) must answer this before closing — do not assume.

### Source Refs

- `run-464841` forensic (`.flowpilot/cli-runner.log`, lines 81163–81515; `.flowpilot/chats/run-464841-turns.ndjson`):
  - `turn-464843` provider=grok model=grok-4.5 yolo=true, posture scan, prompt `test B4-inplace, toi la Nam`.
  - `00:37:01` `session/request_permission` for `run_terminal_command` (compound bash, see BUG-344) → bridge auto-denied `posture_read_only_scan` → `send {"result":{"outcome":{"optionId":"reject-once","outcome":"selected"}}}`.
  - `00:37:11` `session/update` ToolCallUpdate status `Failed` ("User rejected the execution for tool `run_terminal_command`"), then `_x.ai/session_notification` `turn_completed stop_reason=cancelled`, then `_x.ai/session/prompt_complete` `{"agentResult":null,"stopReason":"cancelled","cancellationCategory":"PermissionRejected","cancellationContext":{"tool_name":"run_terminal_command"}}`.
  - No `EventMessageDelta` after the deny; `emitTerminal` got `stopReason="cancelled"` → blank `EventTurnCompleted`.
- Code: `apps/local-runner/internal/runner/grok_adapter.go:542-571` (`emitTerminal`), `grok_permission.go` (`grokApprovalDetailsFromRequest`, `grokEncodePermissionDecision`), `chat_posture_policy.go` (`readOnlyApprovalDecision`), `opencode_adapter.go:473-492` (deny-tracking pattern to mirror).
- Prior same-class fix (opencode): commits `c7f1fa2f` (CA-712 replay recovery) and `aec0dbfc` (CA-713 no-reply notice).

## 1. Issue Summary

In a scan (read-only) chat, the runner auto-denies any write/unknown Grok tool via the posture bridge. Grok treats the denial as a hard stop: it cancels the entire prompt (`PermissionRejected`) and produces **no answer at all** (`agentResult: null`). The Grok adapter's terminal handler does not special-case `cancelled`, so the turn completes with an empty final message. The user sees a chat that just "stops" with zero explanation — the same blank shape BUG-341 fixed for opencode (CA-713), but Grok still has no equivalent.

## 2. Parent Links

- impacted coding plan: [CP-46: Grok Build Controlled Adapter Over ACP](../../07-Coding-Plan/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- impacted tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- impacted system spec: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)

## 3. Environment and Reproduction

- environment: local-runner `serve` + Desktop or TUI chat; provider grok (`grok-4.5`); scan posture active; cwd gate-sandbox (any repo).
- reproduction steps:
  1. Start a chat in **scan** posture with Grok.
  2. Ask a question that makes Grok call any tool the read-only policy denies (e.g. a compound `run_terminal_command` — see BUG-344 — or a write tool).
  3. Watch: the bridge auto-denies (`posture_read_only_scan`), Grok replies `reject-once`, then the prompt completes `cancelled` with `agentResult: null`.
  4. The chat bubble is **blank** — no error, no notice.
- frequency: every denied Grok tool in scan/plan posture (100% in run-464841's first turn).

## 4. Expected vs Actual

- expected: after a denied permission, the turn ends with an honest notice — `[no reply text] … permission was denied … Switch posture (plan/code) or approve the tool to continue.` — exactly as opencode does since CA-713. Optionally name the blocked tool/command.
- actual: `EventTurnCompleted` with empty `FinalMessage`; blank bubble; no `[grok-acp]` WARN with `permissionDenied=` for correlation (unlike opencode's `WARN turn_completed with EMPTY finalMessage … permissionDenied=true`).

## 5. Impact

- users affected: anyone using scan/plan chat posture with Grok (the reported provider; Claude/Codex parity unknown — Q-3).
- workflows affected: chat-mode scan/plan exploration; the operator cannot tell a "denied tool" stop from a broken turn; scan looks broken.
- severity: medium (UX + diagnosability; no data/security impact — the deny itself is correct).

## 6. Root Cause

- hypothesis: Grok adapter misses the provider-specific "denied permission → aborted prompt" terminal shape.
- confirmed cause: `emitTerminal` (grok_adapter.go:560-568) handles only `refusal`/`error`; `stopReason="cancelled"` (with `cancellationCategory=PermissionRejected`) falls through to `EventTurnCompleted` with `finalText == ""` (line 562 falls back to `lastText`, which is also empty because Grok streamed no assistant text after the tool-call).
- evidence: run-464841 wire (Source Refs): `session/prompt_complete` `agentResult: null, stopReason: cancelled, PermissionRejected`; transcript `.flowpilot/chats/run-464841-turns.ndjson` shows the assistant text for turn-464843 is only the pre-tool intent line — no final answer; `grok_adapter.go` has no `cancelled` branch and no deny tracking (grep `permissionDenied` matches only opencode_adapter.go).

## 7. Fix Strategy

- `F-1` Track per-turn permission denials on the Grok adapter (mirror opencode): add `permissionDenied map[string]bool` + `markGrokPermissionDenied(sessionID)` / `grokPermissionDeniedFor(sessionID)`, called on every deny path of the Grok `session/request_permission` handler (both posture auto-deny and any explicit runner deny). Reset per new prompt/turn.
- `F-2` Extend `emitTerminal`: when `stopReason` is `cancelled` (case-insensitive) **and** `grokPermissionDeniedFor(sessionID)` **and** final text is empty → emit `EventMessageDelta` + `EventTurnCompleted(FinalMessage=notice)` with:
  `[no reply text] Grok aborted the turn because a tool permission was denied (scan/plan read-only posture). Blocked tool: <tool name> — <command excerpt>. Switch posture (plan/code) or approve the tool to continue.`
  Keep `refusal`/`error` behavior unchanged.
- `F-3` Log a WARN with `permissionDenied=true` + `stopReason` + `tool_name` (correlation with future logs, mirroring opencode's WARN line).
- `F-4` Parity pass (cross-provider-parity): check Claude and Codex adapters for the same empty-turn-after-deny shape; add notices where the same shape exists (or record explicit evidence they stream a final answer after denial — then no change needed).

## 8. Validation

- `V-1` Unit (additive): deny a Grok permission via the bridge (scan posture) → synthetic `session/prompt_complete` with `stopReason="cancelled"`, `agentResult=null` → assert exactly one `EventMessageDelta` containing `[no reply text]` + blocked tool name, and `FinalMessage` equals the notice (red-before proven by mutation: without F-1/F-2 the test fails with empty FinalMessage).
- `V-2` Unit negative: `cancelled` with **no** deny (user stop) → no notice, today's behavior preserved; `cancelled` + deny + **non-empty** streamed text → no notice (text wins); `refusal`/`error` → `EventTurnFailed` unchanged.
- `V-3` Live: rerun run-464841 scenario headlessly (scan posture, Grok, prompt `test B4-inplace, toi la Nam`) → terminal renders the notice; `.flowpilot/chats/*.ndjson` transcript contains the notice text; log shows the new WARN.
- `V-4` Parity: same headless scenario for Claude and Codex (posture scan, denied tool) → either each shows a notice/reply, or explicit evidence records their post-deny behavior (Q-3). Record in the CA note.

## 9. Regression Guard

- tests: new `TestGrokNoReplyNoticeWhenCancelledAfterDeniedPermission` (+ negatives in `V-2`) in a new `grok_no_reply_notice_test.go`; existing `TestOpencode*` CA-712/CA-713 suites must stay green and untouched.
- alerts: `[grok-acp] WARN … permissionDenied=true` when F-3 lands; `agentResult: null` + `stopReason=cancelled` greppable in cli-runner.log.
- audit checks: feature_key `ai-providers`; prior claims CA-712/CA-713 (opencode) untouched; BUG-344 policy change does not alter the deny path this bug keys on.

## 10. Follow-Up Document Updates

- upstream docs that must change: none required (bugfix delta); CP-46 test steps may gain a scan-deny notice check.
- notes left unchanged on purpose: read-only posture policy (BUG-344's scope), `grokEncodePermissionDecision` deny optionId selection, opencode CA-712/CA-713 recovery paths.