# BUG-296: Claude flow coding child under YOLO fails every gated tool call (no permission-prompt-tool wired)

## Metadata

- Document ID: `BUG-296`
- Title: `Claude flow-engine coding child under YOLO=true launches --permission-mode default with no --permission-prompt-tool, so every gated tool call (Write/Edit/Bash) is denied headless with zero approval-request records`
- Phase: `bugfix`
- Status: `fixed`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-20`
- Last Updated: `2026-07-20` (fix applied: F-1 in `claude_permission_mcp.go`)
- Parent Documents: `-`
- Child Documents: `-`
- Related Documents: `BUG-288 (fix gate lifecycle re-entry — commit 968d210 introduced the regression here as a side effect)`, `BUG-294`, `BUG-295 (same run-12255/run-12260-class investigation session, independent root cause)`
- Replaces: `-`
- Tags: `agent-flow-engine, claude-provider, yolo, permission-mcp, flow-gate, regression, severity-high`

## AI Quick View

### Summary

- A Claude coder child (run-12480, spawned by flow-engine parent run-12475) had every write/edit attempt denied across 3 turns (turn-12485, turn-12512, turn-12536) despite the run's dispatch envelope recording `"yolo":true` on every turn.
- Root cause: the flow-coding-child + YOLO=true combination (V9-21 `forceShellBridge`) produces a posture of `ClaudePermissionMode="default"` (gated) + `RunnerAutoApprove=true`. `claudeArgs` only attaches `--permission-prompt-tool` `if !posture.RunnerAutoApprove` — false here — so Claude launches gated with no way to route its permission prompt anywhere. Headless Claude (no TTY) fails closed on every gated tool call.
- `approvals.ndjson` has zero entries for the run: FlowPilot's approve-MCP tool is never even invoked, because the CLI flag that would route requests to it was never attached.
- After the 2-attempt auto-reprompt budget (`maxGateFixCodeAutoReprompts`) was exhausted at turn-12536, no further dispatch activity ever occurred for the run (confirmed via `dispatch.ndjson`) — this part is correct, by-design behavior (stop auto-retrying, re-show the decision card), not itself a bug.
- Confirmed regression from commit `968d210` (2026-07-16, BUG-288), which introduced `resolveYoloPostureForTurn` (V9-21) and created this ClaudePermissionMode/RunnerAutoApprove decoupling without updating the pre-existing (and, until then, correct) `claudeArgs` gating in `claude_permission_mcp.go`.
- Confirmed Claude-only: Codex (`codex_adapter.go:300`) and Grok (`grok_adapter.go:487`) route every approval request unconditionally to the shared `bridge.RequestApproval` — no launch-time gate analogous to `--permission-prompt-tool` exists for them, so the shared bridge's `RunnerAutoApprove` auto-approve logic is reached correctly in both cases.
- Test-coverage gap identified: two existing tests each validate one half of the pipeline in isolation and were never composed, so neither could catch this. See "6. Root Cause" and "9. Regression Guard" for detail.

### Current Ask

- None — `F-1` implemented, additive-tested across Claude/Codex/Grok, and verified. See "7. Fix Strategy" and "8. Validation".

### Key Decisions

- `V-1` `claudeArgs` attaches `--permission-prompt-tool` whenever `ClaudePermissionMode != "bypassPermissions"` (gated), independent of `RunnerAutoApprove` — restores the invariant the flag actually needs.

### Constraints

- The fix touches `claudeArgs` (`claude_permission_mcp.go`), the single function that assembles the Claude CLI invocation for every Claude turn (chat and flow) — even though the bug only manifests for one specific posture combination, the fix function is shared, so validation must cover all four `(yolo, forceShellBridge)` combinations, not just the broken one.
- additive-tests-only: any future fix must add new dedicated tests only; no existing test may be edited or weakened, per this repo's standing rule.
- `-race` cannot currently be run on this machine (no gcc/CGO).
- GitNexus impact analysis unavailable on this machine (`%1 is not a valid Win32 application`) — performed localized tracing instead (see Root Cause evidence).

### Open Questions

- Gemini was not deeply investigated (the user's 3 questions were scoped to Claude/Codex/Grok). Gemini also sets `forceShellBridge=true` unconditionally (`interactive_service.go:5588`, `rs.providerKey == ProviderKeyGemini`), but its adapter always passes `--dangerously-skip-permissions` (`gemini_adapter.go:252`) regardless of posture — a structurally different, always-bypassed design — so it is very unlikely to share this exact defect, but this was not exhaustively traced and is left open.
- Whether the decision-card re-surfacing after auto-reprompt-budget exhaustion reliably reaches the desktop client when the user is viewing a CHILD transcript (not main/parent) was raised as a secondary open question in the same investigation session but is NOT part of this bug's confirmed root cause — flagged here only for continuity, not required for this fix.

### Source Refs

- run-12475 (parent, project `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`), child run-12480 (coder), turns turn-12485 / turn-12512 / turn-12536.
- `.flowpilot/chats/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/dispatch.ndjson` — envelope records `"yolo":true` for all 3 turns; last dispatch record for run-12480 is `turn-12536 terminal_completed` at `2026-07-20T06:06:37.29Z`, nothing afterward.
- `.flowpilot/chats/approvals.ndjson` — zero entries for run-12480 across its entire lifetime.
- `.flowpilot/chats/sessions.ndjson` — `pending_flow_gate_final_msg` on all 3 turns shows the model reporting a denied write/edit tool call in natural language.
- Screenshots: FlowPilot Desktop coder transcript showing repeated "write denied" / "requires write permission" text with a live spinner and no actionable decision UI.

## 1. Issue Summary

A Claude coding child spawned inside a flow-engine review-loop, running with YOLO=true, could never write to disk: every attempt to edit the production file was denied, across the initial turn and two auto-reprompt retries, until the retry budget was exhausted and the run went idle with no further activity — reading, to the user, as a hang. This happens even though YOLO was genuinely on for this run from its first turn (verified in the durable dispatch envelope), which excludes the (separate, previously suspected) possibility of a stale/frozen YOLO flag on the child.

## 2. Parent Links

- impacted coding plan: `CP-51-PhaseAB-Timeline-And-Verification-Log` (review-loop remediation flow that spawned the affected run)
- impacted tech design: Claude provider CLI arg assembly (`claude_permission_mcp.go: claudeArgs`), YOLO posture resolution (`yolo_resolver.go: resolveYoloPostureForTurn`, V9-21), shared approval bridge (`interactive_service.go: turnBridge.RequestApproval`)
- impacted system spec: YOLO/approval posture contract (04-04), V9-21 flow-coding-child shell-bridge design (introduced in BUG-288 / commit `968d210`)

## 3. Environment and Reproduction

- environment: local runner `127.0.0.1:4318`, desktop FlowPilot, Claude provider (claude-sonnet), workspace `D:\working\gate-sandbox`, flow-engine review-loop with a coder child node.
- reproduction steps:
  1. Start a flow (review-loop or similar) whose entry/coding node resolves to `agent.delegate` with a non-reviewer role (e.g. `coder`), on the Claude provider, with YOLO=true.
  2. Let the flow spawn the coding child; it attempts to write/edit the target file to apply a fix.
  3. Observe: the child's turn completes with assistant text reporting the edit was denied / requires write permission — never actually applying the change.
  4. Check `.flowpilot/chats/approvals.ndjson`: no entry was created for this denial.
  5. Retry (flow-gate auto-reprompt) repeats the identical denial up to the configured budget (`maxGateFixCodeAutoReprompts = 2`), then stops.
- frequency: deterministic — every turn of every Claude coding child under this exact posture combination is affected, not intermittent.

## 4. Expected vs Actual

- expected: under YOLO=true, a flow coding child's ordinary file writes are auto-approved by the runner bridge (only git-commit-shaped shell commands are denied, reserved for the Audit step) — matching the explicit intent documented in `resolveYoloPostureForTurn`'s own comment ("keep auto-approve on the bridge for ordinary commands").
- actual: the write/edit tool call never reaches the runner's approval bridge at all; Claude's own headless process fails it closed before FlowPilot ever sees a request, because the CLI was launched in a gated permission mode with no configured way to answer that gate.

## 5. Impact

- users affected: any user running a Claude-provider coding child (coder, tester, or similar `agent.delegate` non-reviewer role) inside a flow-engine workflow with YOLO=true.
- workflows affected: every YOLO-enabled flow-engine coding step on Claude — this is not limited to the review-loop remediation path; any `agent.delegate` coding node hits `forceShellBridge=true` under YOLO the same way.
- severity: high — the affected child can make no code changes at all (100% denial rate on gated tools), the flow's stated auto-approve intent is silently defeated, and the user has no way to grant/see the blocked request (no approval-card, since no approval request is ever created).

## 6. Root Cause

- confirmed cause: `resolveYoloPostureForTurn(yolo=true, forceShellBridge=true)` (`yolo_resolver.go:66-75`, V9-21, introduced in commit `968d210`) produces `ClaudePermissionMode="default"` (gated) while explicitly keeping `RunnerAutoApprove=true`. `claudeArgs` (`claude_permission_mcp.go:65-79`) attaches the Claude CLI flag `--permission-prompt-tool` (the only mechanism by which a headless, no-TTY Claude process can route a permission request anywhere) only `if !posture.RunnerAutoApprove`. Since `RunnerAutoApprove` is `true` in this combination, the flag is omitted — Claude runs `--permission-mode default` (which still gates every tool call) with literally no configured way to get an answer. Claude fails the tool call closed (denies it) entirely on its own side; FlowPilot's `turnBridge.RequestApproval` (`interactive_service.go:4509`), the shared approval bridge that WOULD correctly auto-approve under `RunnerAutoApprove=true`, is never even called.
- why `approvals.ndjson` is empty: an approval record is only ever created inside `RequestApproval` (`interactive_service.go:4559-4577`), which requires the adapter to have actually received and forwarded a permission/approval request. Since Claude's own internal gate denies the call before any such request is ever surfaced (no `--permission-prompt-tool` to call), `RequestApproval` — and therefore `approvals.ndjson` — is never touched.
- why this is a regression, not a pre-existing gap: before commit `968d210`, `ClaudePermissionMode` and `RunnerAutoApprove` were always set together by the single function `resolveYoloPosture(yolo)` — `default`+`false` (YOLO off) or `bypassPermissions`+`true` (YOLO on). Under `bypassPermissions`, Claude does not gate tool calls at all internally, so omitting `--permission-prompt-tool` was harmless. `claudeArgs`'s `if !posture.RunnerAutoApprove` condition (unchanged since the very first commit that introduced it, `f85de5e`) was therefore a CORRECT proxy for "is Claude's permission mode gated" — until `968d210` introduced a third combination (`default` + `RunnerAutoApprove=true`) that breaks that proxy's correctness. `968d210`'s diff never touched `claude_permission_mcp.go` (confirmed via `git show --stat`), so the proxy was never revisited.
- why this is Claude-only, confirmed:
  - Codex: `codex_adapter.go:277-306` (`handleInbound`) forwards every inbound approval-shaped request (`exec_approval_request`/`patch_approval_request`) to `bridge.RequestApproval` unconditionally — there is no Codex-side launch flag gating whether this channel exists at all. Codex's own `CodexApprovalMode` ("untrusted" under `forceShellBridge`) governs whether CODEX ITSELF decides to ask, but whenever it does ask, FlowPilot always hears about it and always answers via the shared bridge, correctly reaching the `RunnerAutoApprove=true` auto-approve branch.
  - Grok: `grok_adapter.go:450-490` (`handleInbound`) forwards every `session/request_permission` inbound call to `bridge.RequestApproval` unconditionally, same shape as Codex — no launch-time gate.
  - Claude alone has an extra, launch-time-only decision (whether to attach `--permission-prompt-tool` at all) that is not present for the other two providers, and that decision is the one that went stale relative to V9-21.
- evidence: `yolo_resolver.go:39-75`; `claude_permission_mcp.go:48-84` (`claudeArgs`); `interactive_service.go:5583-5594` (`forceShellBridge` computation for a flow coding child), `:4509-4526` (`RequestApproval`, confirming the shared bridge itself is correct and reachable); `codex_adapter.go:277-306`; `grok_adapter.go:450-490`; `git log -L` on both `claude_permission_mcp.go:74,79` (showing the `!RunnerAutoApprove` condition present since `f85de5e`, the file's origin commit, cosmetically reshuffled but logically unchanged by `2dbd9d4`) and `yolo_resolver.go:58,76` (showing `resolveYoloPostureForTurn`/V9-21 introduced whole-cloth by `968d210`, 2026-07-16, 88 commits before HEAD); `git show --stat 968d210` confirming `claude_permission_mcp.go` was not among the files that commit touched; `dispatch.ndjson` envelope `"yolo":true` for all 3 turns of run-12480 (ruling out a stale-yolo explanation); `approvals.ndjson` empty for run-12480.

## 7. Fix Strategy (APPLIED)

- `F-1` (applied) — in `claudeArgs` (`claude_permission_mcp.go`), stopped gating `--permission-prompt-tool` on `RunnerAutoApprove` and gated it instead on whether `ClaudePermissionMode` is actually gated (i.e., attach it whenever the mode is NOT `"bypassPermissions"`). Changed `if !posture.RunnerAutoApprove { ... }` to `if posture.ClaudePermissionMode != "bypassPermissions" { ... }`. This restores the invariant the function actually needs ("attach the prompt-tool whenever Claude's own permission mode will ask"), independent of the separate, now-decoupled `RunnerAutoApprove` runner-side auto-decide flag. Under this fix: `default`+`RunnerAutoApprove=false` (plain YOLO=false) still attaches the tool (unchanged behavior); `bypassPermissions`+`RunnerAutoApprove=true` (plain YOLO=true, no forceShellBridge) still omits it (unchanged behavior, still harmless since nothing gates); `default`+`RunnerAutoApprove=true` (the V9-21 combination, previously broken) now correctly attaches the tool, so Claude's permission prompts route to FlowPilot's approve-MCP, which reaches `RequestApproval` and auto-approves under `RunnerAutoApprove=true` (denying only commit-shaped commands first, per the existing, correct, provider-neutral bridge logic).
- Considered and rejected: gating on `!RunnerAutoApprove || posture.ClaudePermissionMode != "bypassPermissions"` (an OR of both conditions) — equivalent in every case the codebase currently produces, but strictly checking the mode alone is simpler and directly matches what the flag is FOR, so it was preferred.
- Codex and Grok: confirmed unaffected, no fix needed. Codex's `handleInbound` (`codex_adapter.go:277-306`) routes every inbound approval-shaped request to `bridge.RequestApproval` unconditionally — no launch-time gate analogous to Claude's exists. Grok's `handleInbound` (`grok_adapter.go:457-494`) auto-approves directly via its own `if yolo { ... }` branch before ever consulting `GrokPermissionMode`/`forceShellBridge` at all, so the V9-21 combination changes nothing about Grok's behavior.
- additive-tests-only: existing `TestClaudeArgsIncludesStrictMcpConfig` / `TestClaudeArgsDisablesBuiltinAskUserQuestion` (`claude_permission_mcp_test.go`), `TestV9MatrixForceShellBridgePosture` (`v9_matrix_test.go`), `TestCodexAdapterYoloApprovalMatrix` (`codex_appserver_test.go`), and `TestGrokAdapterYoloOnAutoApprovesViaRunnerPolicyNotBridge` (`grok_adapter_test.go`) were left unmodified. New coverage added in `bug296_yolo_permission_prompt_tool_test.go` (see Validation).

## 8. Validation

- New additive test file `apps/local-runner/internal/runner/bug296_yolo_permission_prompt_tool_test.go`, 4 tests, all pass:
  - `TestBug296ClaudeArgsAttachesPermissionPromptToolUnderForceShellBridge` — the fix itself: `claudeArgs(resolveYoloPostureForTurn(true, true), ...)` now includes `--permission-prompt-tool`.
  - `TestBug296ClaudeArgsOmitsPermissionPromptToolUnderPlainYolo` — non-regression: `claudeArgs(resolveYoloPostureForTurn(true, false), ...)` still omits it (plain YOLO, unchanged).
  - `TestBug296RequestApprovalAutoApprovesOrdinaryWriteUnderYolo` — provider-neutral: `turnBridge.RequestApproval` (the shared decision point every provider relies on) auto-approves an ordinary write under `yolo=true`, independent of which provider is asking.
  - `TestBug296CodexForceShellBridgeYoloStillRoutesApprovalRequest` — closes an equivalent, previously-untested gap for Codex: under the same `ForceShellBridge=true, YoloMode=true` combination, Codex's real adapter (via `startFakeCodex`/`newCodexAdapter`/`handleInbound`) still sends the file-change approval request (count=1, not silently dropped) and it comes back `"accept"`.
  - Grok: no new test added — `TestGrokAdapterYoloOnAutoApprovesViaRunnerPolicyNotBridge` (pre-existing, unmodified) already fully covers this exact class for Grok, since Grok's decision does not depend on `forceShellBridge`/`GrokPermissionMode` at all.
- `go test ./internal/runner -run TestBug296 -count=1`: 4 passed.
- Regression battery `go test ./internal/runner -run 'TestClaudeArgs|TestV9Matrix|TestClaudeAdapter|TestCodexAdapter|TestGrokAdapter|TestRequestApproval|TestBug296|TestClaudeSendTurn|TestCodexPermissions|TestCodexMcp|TestBug294|TestBug295' -count=2`: 172 passed (run twice — fully deterministic).
- Full-suite `go test ./internal/runner -count=1` was run twice — once on this fix, once on a stashed baseline (fix reverted) for comparison. Both runs show a DIFFERENT set of ~19-24 failures each time (e.g. `TestCodexResumeCommandUsesYoloDerivedSandboxAndApproval`, `TestNextAccountHomePathGrokUsesGrokHomePrefix`, `TestSkillsMergeCodexProjectAgentsAndProviderHomeWithPrecedence`, `TestGitCommitGuardBlocksCommit`, home-path-dependent tests) — proving the full suite is independently flaky/environment-dependent on this machine (no Codex CLI binary, machine-specific home paths, Grok account slot state, git guard shim executability) regardless of this fix. None of the flaky failures are in a file this fix touches (`claude_permission_mcp.go`) or call `claudeArgs`/reference `ClaudePermissionMode`/`RunnerAutoApprove`.
- `-race` not run: this machine lacks gcc/CGO.
- GitNexus impact analysis unavailable (`%1 is not a valid Win32 application`); localized tracing performed instead (see Root Cause evidence) — enumerated every caller/consumer of `ClaudePermissionMode`/`RunnerAutoApprove` and both other providers' inbound-approval handlers before changing the single shared line.

## 9. Regression Guard

- why existing tests did not catch this (test-coverage gap, confirmed):
  - `TestV9MatrixForceShellBridgePosture` (`v9_matrix_test.go:142-159`) calls `resolveYoloPostureForTurn(true, true)` directly and asserts the resulting `YoloPosture` struct fields (`ClaudePermissionMode == "default"`, `RunnerAutoApprove == true`) — correctly validating that POSTURE CONSTRUCTION is right. It never passes that posture into `claudeArgs` to check the resulting CLI args.
  - `TestClaudeArgsIncludesStrictMcpConfig` and `TestClaudeArgsDisablesBuiltinAskUserQuestion` (`claude_permission_mcp_test.go:88-106`) — the only two tests that call `claudeArgs` directly — both pass ONLY `resolveYoloPosture(false)` or `resolveYoloPosture(true)` (the plain, pre-V9-21 constructor), never a posture built via `resolveYoloPostureForTurn(true, true)`. Both plain postures keep `ClaudePermissionMode` and `RunnerAutoApprove` in the old, always-correlated lockstep, so neither test could ever exercise the new decoupled combination.
  - Net effect: one test suite validates the posture VALUE for the new combination in isolation; a different test suite validates `claudeArgs`'s ARGS OUTPUT but only ever for the two OLD combinations. No test composes "the V9-21 posture" with "claudeArgs's output" — the exact seam where this bug lives — so both suites pass while the integration between them is broken. A classic unit-tested-in-isolation / never-composed gap, not a logic error either test suite got wrong on its own terms.
- tests: added, all pass (see Validation). `bug296_yolo_permission_prompt_tool_test.go` covers exactly the composed check identified as missing: (1) `resolveYoloPostureForTurn(true, true)` → `claudeArgs` DOES include `--permission-prompt-tool` (regression guard for `F-1`), (2) `resolveYoloPostureForTurn(true, false)` → `claudeArgs` still OMITS it (non-regression), (3) the shared `turnBridge.RequestApproval` auto-approves under `yolo=true` regardless of provider, (4) Codex's real adapter wiring still routes the approval request under the same `forceShellBridge+yolo` combination (closing an equivalent, previously-untested gap). Grok's existing `TestGrokAdapterYoloOnAutoApprovesViaRunnerPolicyNotBridge` was left unmodified and already covers Grok. Existing `TestClaudeArgsIncludesStrictMcpConfig` / `TestClaudeArgsDisablesBuiltinAskUserQuestion` / `TestV9MatrixForceShellBridgePosture` / `TestCodexAdapterYoloApprovalMatrix` continue to pass unmodified.
- alerts: consider a runtime warning/log line when a Claude turn's resolved `ClaudePermissionMode` is gated (not `bypassPermissions`) but the launch args end up without `--permission-prompt-tool` — would have caught this class of drift immediately instead of requiring manual dispatch-ledger forensics. Not implemented in this fix (out of scope; noted as a future observability improvement, same as BUG-295's equivalent note).
- audit checks: CA note `CA-374` (feature_key `agent-flow-engine`) accompanies this fix.

## 10. Follow-Up Document Updates

- upstream docs that must change: none identified — the V9-21 design intent (`resolveYoloPostureForTurn`'s own code comment) is unchanged by this fix; this bug corrected a downstream consumer (`claudeArgs`) that had fallen out of sync with it, not the design itself.
- notes left unchanged on purpose: the Gemini open question (see Open Questions) was deliberately left untraced rather than guessed — Gemini's `--dangerously-skip-permissions` design is structurally different enough that it was out of scope for this fix's verification, which was scoped to Claude/Codex/Grok per the user's explicit ask.
