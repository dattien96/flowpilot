# BUG-064: Codex Approval Decision Value Rejected By App-Server

## Metadata

- Document ID: `BUG-064`
- Title: `Codex Approval Decision Value Rejected By App-Server`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- Child Documents: `none`
- Related Documents: [BUG-063: Desktop Chat Controls Not Applied To Provider Runs](./BUG-063-Desktop-Chat-Controls-Not-Applied-To-Provider-Runs.md), [BUG-061: Codex MCP Server Config Rejects on-request Approval Variant](./BUG-061-Codex-MCP-Config-On-Request-Invalid-Approval-Variant.md)
- Replaces: `none`
- Tags: `codex, approval, yolo, runner, provider, regression`

## AI Quick View

### Summary

- With YOLO=off, a Codex chat command surfaces an approval card; clicking **Approve** does not unblock the command — Codex reports "exec command rejected by user" / "the escalation request was rejected".
- The Codex adapter replied to the app-server approval request with `{"decision":"approve"}` / `{"decision":"deny"}`, but the real `codex app-server` deserializes the reply into a `ReviewDecision` enum whose serde values are `approved | approved_for_session | denied | abort`.
- `"approve"` is not a valid `ReviewDecision`, so Codex treats the response as a failed/declined approval.
- YOLO=on was unaffected: it runs `approvalMode: "never"`, so Codex never asks and the broken reply path is never exercised.

### Current Ask

- Done. The Codex adapter translates FlowPilot's internal approve/deny decision into the app-server `ReviewDecision` value before replying, so a human Approve actually completes the Codex turn.

### Key Decisions

- `V-1` Keep FlowPilot's internal vocabulary (`approve`/`deny`) stable across the desktop card, the runner approval bridge, and the policy engine; translate to Codex's wire enum only at the adapter boundary (`handleInbound`).
- `V-2` `ReviewDecision` serde values are `approved | approved_for_session | denied | abort`; the approval response struct carries a single `decision` field. The server→client approval methods are `execCommandApproval` and `applyPatchApproval` (the previously-assumed `approval/request` was a placeholder).
- `V-3` Unknown/empty decisions fail safe to `denied` so Codex never hangs and never silently runs an un-approved command.
- `V-4` This is a third, distinct Codex approval enum from the two in BUG-061: BUG-061 covered the MCP-server-config TOML (`auto|prompt|approve`) and the task-level `approvalMode` (`…|on-request|never`); this bug is the approval **response** decision enum (`ReviewDecision`).

### Constraints

- Do not change the runner approval bridge / policy engine vocabulary or the desktop card values.
- Do not change `yolo_resolver.go` sandbox/approval-mode mapping (correct per BUG-061).
- Claude's gating path is out of scope and unaffected (it uses the `--permission-prompt-tool` MCP contract with `{"behavior":"allow"|"deny"}`).

### Open Questions

- None for this slice.

### Source Refs

- User report on `2026-06-16`: YOLO=off, clicked Approve on the file-write approval card, Codex still reported "exec command rejected by user".
- Protocol literals extracted from the installed `codex.exe` (codex-cli `0.137.0`).
- Runner code: `apps/local-runner/internal/runner/codex_adapter.go` (`handleInbound`, `codexReviewDecision`).

## 1. Issue Summary

In direct chat mode with YOLO disabled, Codex runs in `workspace-write` sandbox with `approvalMode: "on-request"`. When Codex requests approval to run a command, FlowPilot shows the approval card. The user clicks **Approve**, the decision round-trips through the runner bridge to the Codex adapter, and the adapter replies to Codex — but Codex still aborts the command as if it were declined. YOLO=on (which never asks) works fine, which incorrectly suggested YOLO itself was the problem; the real defect is the approval **reply value**.

## 2. Parent Links

- impacted coding plan: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- impacted tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` chat mode + `apps/local-runner` with `FLOWPILOT_CODEX_APPSERVER` enabled and a real `codex` binary (codex-cli 0.137.0)
- reproduction steps:
  1. Open chat mode, select Codex, set YOLO **off**.
  2. Prompt: `Create a file called yolo-test.txt in the current directory with the text "yolo works"`.
  3. Wait for the approval card, click **Approve**.
  4. Observe Codex report "exec command rejected by user" and decline to create the file.
- frequency: always, for every YOLO=off Codex approval.

## 4. Expected vs Actual

- expected: clicking Approve replies with a valid `ReviewDecision` so Codex proceeds and completes the command.
- actual: the adapter replied `{"decision":"approve"}`; Codex could not deserialize `"approve"` into `ReviewDecision` and treated the approval as declined, aborting the command.

## 5. Impact

- users affected: all desktop chat users running Codex with YOLO off (the default, safe posture).
- workflows affected: every Codex command/patch that requires per-action approval.
- severity: high — YOLO=off is unusable for any action Codex gates.

## 6. Root Cause

- hypothesis: the assumed Codex approval reply shape was never verified against a real Codex build (flagged in `codex_event_mapper.go`: "verify against the installed build, 06 Part D").
- confirmed cause: `codexAdapter.handleInbound` replied `{"decision": <"approve"|"deny">}`. The real app-server expects `{"decision": <ReviewDecision>}` where `ReviewDecision` serializes to `approved | approved_for_session | denied | abort`. `"approve"`/`"deny"` are not members, so the deserialize fails and Codex declines.
- evidence:
  - serde struct names embedded in `codex.exe`: `ExecCommandApprovalResponse` / `ApplyPatchApprovalResponse` each "with 1 element" (the `decision` field); `ReviewDecision` enum values `approved`/`approved_for_session`/`denied`/`abort`; server→client request methods `execCommandApproval` / `applyPatchApproval`.
  - `apps/local-runner/internal/runner/codex_adapter.go` `handleInbound` previously replied the raw internal decision.

## 7. Fix Strategy

- `F-1` Add `codexReviewDecision(decision string) string` mapping internal values to the app-server enum: `approve`→`approved`, `deny`→`denied`, plus `approve_for_session`→`approved_for_session` and `abort`; unknown/empty → `denied` (fail safe).
- `F-2` In `handleInbound`, reply `{"decision": codexReviewDecision(decision)}` on both the success and the expiry/interrupt paths so every reply carries a valid `ReviewDecision`.
- `F-3` Keep the runner bridge, policy engine, and desktop card on the internal `approve`/`deny` vocabulary — translation happens only at the Codex adapter boundary.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'Test(CodexAdapterApprovalRoundTrip|CodexReviewDecisionMapsToAppServerEnum)' -count=1` — the round-trip now asserts the wire decision is `approved` (not `approve`); the table test covers every `ReviewDecision` value plus the fail-safe default.
- `V-2` `go build ./...` (runner) passes.
- `V-3` Manual: YOLO=on creates the file with no prompt (already verified); YOLO=off shows the card and clicking Approve now completes the Codex command instead of "exec command rejected by user".

## 9. Regression Guard

- tests: `TestCodexAdapterApprovalRoundTrip` (asserts wire value `approved`, fake server now uses the real `execCommandApproval` method), `TestCodexReviewDecisionMapsToAppServerEnum` (mapping table + fail-safe).
- alerts: none.
- audit checks: impact reviewed for `handleInbound` and the new `codexReviewDecision`; blast radius is the Codex approval-reply path only. GitNexus MCP query tools were not connected this session (only its hooks fired); verified via `go build` + targeted `go test`.

## 10. Follow-Up Document Updates

- upstream docs that must change: `none` — the `ReviewDecision` enum is a Codex wire detail, not a FlowPilot design rule.
- notes left unchanged on purpose:
  - `yolo_resolver.go` sandbox/approval-mode mapping (correct per BUG-061).
  - Claude's `--permission-prompt-tool` `behavior` contract (separate, working path).
