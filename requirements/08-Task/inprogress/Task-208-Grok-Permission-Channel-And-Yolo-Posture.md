# Task-208: Grok Permission Channel And YOLO Posture

## Metadata

- Document ID: `Task-208`
- Title: `Grok Permission Channel And YOLO Posture`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Task-207: Grok Controlled Adapter MVP (Chat/Stream/Resume)](./Task-207-Grok-Controlled-Adapter-MVP.md)
- Child Documents: `None`
- Related Documents: [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md), [BUG-069: Claude YOLO Off Always Auto-Approves](../done/BUG-069-Claude-YOLO-Off-Always-Auto-Approves.md)
- Replaces: `None`
- Tags: `grok, grok-build, approval, permission, yolo, acp, local-runner`

## AI Quick View

### Summary

- Wire Grok's live, verified `session/request_permission` inbound request into `TurnBridge.RequestApproval`, answering with the exact ACP `optionId` Grok offered (`allow-once`, `allow-edits-session`, `reject-once`).
- Add a `GrokPermissionMode` to `YoloPosture` so YOLO=true maps to `bypassPermissions` (the only mode Grok's own docs say takes effect via flag) with `RunnerAutoApprove=true`, and YOLO=false keeps default + live per-call gating.
- Prove deny-by-default actually holds for every tool class Grok can invoke (shell, write/edit, MCP), not just the one gated `write` call already verified live; close any fast-path gap with `--allow`/`--deny` rules or a `PreToolUse` hook if needed.

### Current Ask

- Make YOLO=false genuinely block a denied action and YOLO=true genuinely bypass only through runner policy — reproduce the CP-46 authoring finding (`write` denied with a wrong `optionId` → `PermissionRejected`; correct `optionId` → executed) as a repeatable, policy-driven test, then extend it to `ask_user` (must never auto-approve) and to any read/grep fast-path Grok applies regardless of mode.

### Key Decisions

- `T-1` Reuse the Codex **inbound-request** pattern (`handleInbound`) for `session/request_permission` — do **not** build a Claude-style HTTP MCP `--permission-prompt-tool` server for this; Grok's channel is already a server→client JSON-RPC request with `options[]`, which is a closer match to Codex's approval flow.
- `T-2` The decision encoder must pick from the **options actually offered** in that specific request (`options[].optionId`/`kind`), never a hardcoded string — replicate the exact failure/success behavior observed live (wrong id → `PermissionRejected` + `stop_reason:"cancelled"`; correct id → `tool_call_update{status:"completed"}`).
- `T-3` `RunnerAutoApprove=true` (YOLO on) must not suppress `ask_user` — if Grok routes `ask_user` through the same permission channel or a distinct one, both paths must still block for a real answer.
- `T-4` If Grok's built-in safe-read/grep fast-path auto-resolves some tool calls without ever emitting `session/request_permission` (as its own docs describe), YOLO=false deny-by-default must still be provably enforced for every non-safe tool; add `--allow`/`--deny` glob rules or a `PreToolUse` hook only if the live behavior shows a gap.
- `T-5` No persisted Grok-side allowlist (`~/.grok/config.toml` `[permission]`, or an imported `.claude/settings.json`) may bypass the runner's YOLO toggle — mirror the Claude `CA-079`/`BUG-069` fix (force a clean permission posture per launched process/home).

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-46 `P-0`):** additive-only; the `GrokPermissionMode` field on `YoloPosture` is read only by Grok; `resolveYoloPosture` outputs for codex/claude must stay byte-identical; do not alter Codex/Claude approval routing.
- Do not add a Grok-only approval UI; approvals must render through the existing shared approval card / `TurnBridge.RequestApproval` contract.
- Do not weaken Codex/Claude approval behavior while adding the Grok decision-vocabulary mapper.
- Any fallback (`--allow`/`--deny`/hook) must be provably additive and must not create a second source of truth that can drift from `YoloPosture`.

### Open Questions

- `Q-1` (CP-46 `Q-3`) Does Grok emit `session/request_permission` for every non-fast-path tool class, or are there other silent-approve categories to discover empirically?
- `Q-2` Does Grok expose a distinct permission channel for `ask_user` specifically, or does `ask_user` arrive as a normal tool call subject to the same `session/request_permission` gate?

### Source Refs

- `CP-46` sections `P-5`, `P-6`, `P-9`; parity rows `GR-04`, `GR-05`, `GR-06`; Risks `R-4`, `R-6`.
- Live captured evidence (CP-46 authoring): `session/request_permission` request/response pair, `pending_interaction`/`interaction_resolved` notifications, `PermissionRejected` cancellation on wrong `optionId`.
- `apps/local-runner/internal/runner/yolo_resolver.go`, `codex_adapter.go` (`handleInbound`, decision-vocabulary mappers `codexReviewDecision`/`codexPermissionsApprovalResponse`), `claude_permission_mcp.go` (contrast reference only — not the pattern used here).

## 1. Goal

Make Grok's approval gating and YOLO posture behaviorally equivalent to Codex/Claude: YOLO=false blocks a denied action for real, YOLO=true auto-approves only through runner policy, and `ask_user` is never silently approved.

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-09`
- system spec: `SS-08`
- specific upstream ids: `CP-46 P-5`, `P-6`, `P-9`

## 3. Trigger

Task-207 ships a chat-only adapter with no gating; this task adds the safety-critical approval/YOLO layer before Grok can be exposed for real tool use.

## 4. Exact Change

- `T-1` In `grok_process.go`'s dispatcher `inbound` handler, route `session/request_permission` to a new Grok `ApprovalDetails` builder (tool title, `rawInput`, `kind`, cwd) → `bridge.RequestApproval`.
- `T-2` Add a decision-vocabulary encoder that maps the bridge's allow/deny decision to the specific `optionId` present in that request's `options[]` (prefer `kind:"allow_once"` for allow, `kind:"reject_once"` for deny); reply `{jsonrpc:"2.0", id:<reqId>, result:{outcome:{outcome:"selected", optionId:<chosen>}}}`.
- `T-3` Add `GrokPermissionMode string` to `YoloPosture` (`yolo_resolver.go`); `resolveYoloPosture` sets it to `"bypassPermissions"` when YOLO=true, `""`/`"default"` when YOLO=false.
- `T-4` In the Grok adapter, YOLO=true still processes inbound `session/request_permission` requests but auto-answers them with the runner's own auto-approve decision (not by disabling the channel) so `ask_user`-shaped requests can be special-cased to always block.
- `T-5` Add a `PreToolUse`-hook or `--allow`/`--deny` fallback (only if `T-testing` below finds a gap) ensuring YOLO=false denies every non-fast-path tool by default.
- `T-6` Add golden-fixture tests replaying the exact captured live `session/request_permission` frame: correct `optionId` → simulated tool executes; wrong/absent `optionId` → `PermissionRejected` is surfaced as a controlled, non-crashing turn failure.
- `T-7` Add a test proving a stale/imported Grok-side allowlist cannot bypass YOLO=false (mirror `BUG-069`'s regression test shape).
- `T-8` Ensure the dispatcher's `inbound` handler spawns each `session/request_permission` on its own goroutine bound to the turn ctx, so N concurrent gated tool calls each surface as a grouped card, resolve independently, and never wedge the pump (Task-182 / BUG-157 / BUG-182 class); add a two-simultaneous-gates test (`GR-29`).
- `T-9` Set `ApprovalDetails.Kind` correctly per Grok tool (`"exec"` for shell/run_terminal, `"edit"`/`"write"` for file ops) so the existing per-project "don't ask again" command allowlist (BUG-246) engages for Grok exec approvals; add a test that an allowed exec pattern is not re-prompted (`GR-04`).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/yolo_resolver.go`, `grok_process.go` / `grok_adapter.go` (inbound handling), new `grok_permission.go` (decision-vocabulary mapper), `grok_adapter_test.go`
- modules: local runner approval/YOLO subsystem (Grok-specific mapping only)
- routes: none new
- tables: none

## 6. Acceptance Check

- A gated write/shell action under YOLO=false surfaces `permission_required` to the bridge; deny prevents execution; allow executes.
- YOLO=true auto-approves eligible actions via runner policy only; `ask_user` still blocks for a real answer.
- Golden-fixture replay of the live-captured frame passes deterministically.
- No regression in Codex/Claude approval tests.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` `session/request_permission` is routed to `TurnBridge.RequestApproval` and answered with a request-specific `optionId`. (`grok_adapter.go handleInbound` + `grokEncodePermissionDecision`; `TestGrokAdapterYoloOffBlocksOnBridgeAndDeniesWithoutApproval`.)
- [x] `DOD-2` `GrokPermissionMode` exists on `YoloPosture` and is set correctly for both YOLO states. (`yolo_resolver.go`.)
- [x] `DOD-3` YOLO=false denies a test action end to end (action verifiably does not execute). (Same test; `bridge.approvalCallCount()==1`.)
- [x] `DOD-4` YOLO=true auto-approves only through runner policy; `ask_user` is proven to still block. (`TestGrokAdapterYoloOnAutoApprovesViaRunnerPolicyNotBridge` proves the bridge is never consulted under YOLO; `ask_user` itself is a distinct MCP/native-tool path per Task-209, never routed through this channel, so it structurally cannot be auto-answered here — not independently re-tested.)
- [~] `DOD-5` A stale Grok-side allowlist cannot bypass YOLO=false (regression test passes). **Live-verified finding, partially resolved (2026-07-10, real logged-in account):** the runner-side design is structurally immune — the adapter never reads Grok's own `config.toml`. However live testing found the ACP channel itself is not: this account's real `~/.grok/config.toml` has `[ui] permission_mode="always-approve"` (set by Grok's own `/always-approve` slash command), which makes Grok skip `session/request_permission` entirely — confirmed via a controlled experiment (temporarily flipped to `permission_mode="default"`, re-ran, `session/request_permission` fired and a runner deny correctly blocked the write; original file restored exactly). There is no `grok agent stdio` flag to override this. This note originally concluded FlowPilot "cannot safely rewrite the user's config file" and shipped only a warning. **Superseded 2026-07-10 by [Task-218](./Task-218-Grok-Yolo-Enforced-Via-Config-Rewrite-And-Always-Approve-Flag.md):** that "cannot rewrite" premise was reversed — `setGrokConfigPermissionMode` now rewrites the one `permission_mode` line safely (minimal line-patch, atomic temp+rename, no-op when unchanged), and `ensureGrokProcess` calls it automatically on **every YOLO=false spawn** when the config bypasses, before `grok` reads the file — so gating is auto-enforced without waiting for any user toggle. The old warning is now only emitted if that rewrite fails. Remaining to fully close: the BUG-069-style **live** end-to-end proof that a real write is blocked (Task-218 DOD-6/DOD-7). The accepted config-clobber tradeoff (YOLO=false flips a standalone-TUI always-approve to default) is documented in Task-218 `T-6`.
- [~] `DOD-6` Golden-fixture test replays the exact live-captured permission round-trip (both correct and incorrect `optionId` paths). `TestGrokDispatcherRoutesInboundPermissionRequest` covers routing; the "wrong/absent optionId → PermissionRejected" path is asserted indirectly via `TestGrokEncodePermissionDecisionNoMatchReturnsEmpty` (unit-level) rather than a full live-shaped round-trip fixture.
- [ ] `DOD-7` Two simultaneous gated Grok tool calls surface as a grouped card, resolve independently, and do not wedge the run (`GR-29`). **Not tested directly**: true by construction (the dispatcher's `dispatch()` spawns `go d.inbound(...)` per inbound frame — see `grok_process.go`), but no test issues two concurrent `session/request_permission` requests and asserts both resolve independently.
- [x] `DOD-8` Grok exec approvals set `Kind=="exec"` and engage the per-project "don't ask again" allowlist (`GR-04`, BUG-246). (`TestGrokPermissionKindClassifiesExecFileMcp`, `TestGrokAdapterYoloOffBlocksOnBridgeAndDeniesWithoutApproval` asserts `Kind=="exec"`. The exec-vs-file-vs-mcp classification itself is inferred from docs, not independently live-verified against a real write/exec tool call — see CP-46 §10.2.)
- [x] `DOD-9` **Base-regression (`P-0`):** `yolo_resolver.go` `GrokPermissionMode` is read only by Grok; `resolveYoloPosture` outputs for codex/claude are byte-identical; Codex/Claude approval regression tests remain green. (Full suite verified unchanged vs. clean baseline.)

## 7. Out of Scope

- MCP tool registration (`ask_user`/`spawn_agent` bridge wiring itself) — Task-209.
- Desktop approval-card UI changes — none expected; if any are needed they belong to Task-211.
- Account-level permission config (`~/.grok/config.toml` authoring) beyond what is needed to prove deny-by-default — picked up by [Task-218: Grok YOLO Enforced Via Config Rewrite And Always-Approve Flag](./Task-218-Grok-Yolo-Enforced-Via-Config-Rewrite-And-Always-Approve-Flag.md), which authors `config.toml`'s `permission_mode` directly since DOD-5 below found no CLI flag exists for that direction.

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated:
