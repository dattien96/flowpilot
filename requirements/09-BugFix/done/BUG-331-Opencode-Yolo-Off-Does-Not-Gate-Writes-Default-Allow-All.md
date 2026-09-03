# BUG-331: Opencode YOLO-off does not gate writes — acp default allow-all never emits session/request_permission

## Metadata

- Document ID: `BUG-331`
- Title: `Opencode YOLO-off does not gate writes — acp default allow-all never emits session/request_permission`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-09-02`
- Feature Keys: `ai-providers`
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md), [CP-57-Test-Steps](../../07-Coding-Plan/done/CP-57-Test-Steps.md) (section E), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- Child Documents: `none`
- Related Documents: [CA-679](../../../change-audit/CA-679-opencode-config-file-env-and-tui-model-restore.md), [CA-680](../../../change-audit/CA-680-opencode-midchat-model-session-load.md) (BUG-329), [CA-681](../../../change-audit/CA-681-opencode-yolo-posture-endpoint.md), CP-57 OC-04/P-5/R-3 (do-not-trust-allowlist requirements)
- Replaces: `none`
- Tags: `opencode, acp, approval-gate, yolo, posture, severity-high`

## AI Quick View

### Summary

- Operator test E1 (CP-57-Test-Steps): with YOLO OFF, opencode created `approval-test.txt` in the project with **no permission_required card** — write went straight through.
- Client side is complete and was never triggered: `handleInbound` (opencode_adapter.go:462-497) auto-approves tool permissions when YOLO-on, routes to `bridge.RequestApproval` (card) when YOLO-off, and read-only postures force `yoloModes=false` (opencode_adapter.go:241) so the bridge denies writes. Tests fake the inbound request; live opencode never sends one.
- Live probes (opencode 1.18.25, 2026-08-30, scripts `/tmp/fp_perm_probe*.py`):
  - **A (baseline)**: default config → file created with **0 permission requests** (allow-all).
  - **B/C/D**: `OPENCODE_CONFIG_CONTENT='{"permission":{"edit":"ask","bash":"ask"}}'` → real `session/request_permission` (kind=edit, with diff) for an in-project write; allow → file created, turn `end_turn`; client replying **error** to `fs/write_text_file` (FlowPilot behavior) → opencode falls back to its own write, turn still clean.
  - **G/H/I**: config merge semantics — file config `permission` applies (G); `OPENCODE_CONFIG_CONTENT` **wins on conflict** (H) and **deep-merges** without wiping file keys (I: file edit=ask + content bash=ask → edit request still fires).
  - **J (deny)**: reply `reject` → **file_created=False**, turn ends `end_turn` — the gate really blocks.

### Root Cause

1. Opencode's default permission config is allow-all ("By default, opencode allows all operations"). The user's `~/.config/opencode/opencode.json`/`opencode.jsonc` and FlowPilot's account config pin no `permission` block, so every opencode write/bash is executed silently.
2. `opencode acp` has **no** `--auto`/`--permission` launch flag (verified `--help` 1.18.25), so `ApplyOpencodeYoloPosture` (opencode_process.go:550-565) flips `opencodeDesiredAuto` + kills processes, but `auto` only feeds `opencodeProcessKey`/handle metadata — opencode never reads it. `OpencodePermissionMode` is a blank identifier at opencode_adapter.go:242-243 and provider_registry.go:588.
3. Because opencode never asks, the whole runner-side decision layer (YOLO auto-approve, YOLO-off card, scan/plan deny) stays dead code for live turns.

### Fix (OpenCode-only, always-ask + runner decides)

1. `opencodeLaunchEnv()` (opencode_model_variants.go — the single shared env choke point for both the turn adapter factory and the variants prober) adds `OPENCODE_CONFIG_CONTENT = {"permission":{"edit":"ask","bash":"ask"}}`. Deep-merge overlay applied last (probe H/I): account config file, auth.json, mcpServers all intact; `OPENCODE_CONFIG` file path unchanged (CA-679 preserved).
2. YOLO/posture decisions stay per-turn at the runner via the existing `yoloModes` map + bridge policy. No process respawn on YOLO/posture change (would re-introduce BUG-329 session orphaning). `opencodeProcessKey` and the BUG-329 same-scope reuse are untouched; the `auto` key dimension becomes constant dead weight (kept for old-test stability).
3. Scan/plan gating comes free: read-only posture forces `yoloModes=false` → every request hits `bridge.RequestApproval` → existing read-allow/write-deny policy (chat_posture_store_test.go). This closes the CP-57 H2 gap (scan was prompt-only for opencode).
4. Webfetch stays at opencode's default (allow) for parity with Codex/Claude/Grok, which gate file/exec only.
5. TUI/Desktop unchanged: `YoloMode` already rides every TurnRequest. The CA-681 `/provider-accounts/opencode-yolo-posture` endpoint stays as-is (no caller needed).

Claude/Codex/Grok untouched — their CLIs gate by default and YOLO-off already produces cards (codex `codexYoloDeriveForChatPosture` sandbox/approval flags + `exec_approval_request`/`applyPatchApproval`; claude `--permission-prompt-tool` + permission MCP; grok native `session/request_permission`).

### Validation

- Additive tests (new file `bug331_opencode_permission_ask_env_test.go`): launch env carries the overlay in both account-resolved and env-HOME fallback branches; `opencodeProcessEnv` passes it through; overlay JSON pins exactly edit+bash ask; same-scope reuse (BUG-329) unaffected by the overlay.
- Old opencode suites (`ca679_opencode_config_env_test.go`, `opencode_process_test.go`, `opencode_yolo_approval_test.go`, `bug329_*`) must stay green (R1 baseline captured before the change).
- Live probe evidence recorded above (A/B/C/D/G/H/I/J) pre-verifies guide E1 (card), E2 (deny → no file, clean end), E3 (approve → file), E4 (YOLO on auto-approve, no card), E5 (question channel never auto-approved — `handleInbound` restricts auto-approve to `session/request_permission`).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Force opencode acp to always ask via OPENCODE_CONFIG_CONTENT permission overlay so YOLO/posture decisions fire in the runner
# --->8---
