# BUG-343: Grok scan posture does not gate exec — bash runs via self-resolved `pending_interaction`

## Metadata

- Document ID: `BUG-343`
- Title: `Grok scan posture does not gate exec — bash runs via self-resolved pending_interaction; --always-approve forces the same bypass under YOLO`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-02`
- Last Updated: `2026-09-02`
- Feature Keys: `ai-providers`
- Parent Documents: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [BUG-331](BUG-331-Opencode-Yolo-Off-Does-Not-Gate-Writes-Default-Allow-All.md) (same class, opencode edition)
- Child Documents: `none`
- Related Documents: [CA-714](../../../change-audit/CA-714-grok-always-ask-exec-gate.md), CA-706/CA-712/CA-713 (BUG-341 opencode deny family), Task-208/Task-218 (grok permission channel + YOLO SSOT)
- Replaces: `none`
- Tags: `grok, acp, approval-gate, scan-posture, read-only, yolo, severity-high`

## AI Quick View

### Summary

- Live verification of BUG-341 parity (2026-09-01/02, runner API + `cli-runner.log`/`runner.log`): with `chatPosture=scan`, a Grok chat turn executed a real bash command (`ls -la .flowpilot/ | head`) — real output in the tool result — with **zero permission round-trip**, under YOLO-on AND YOLO-off.
- Wire evidence: Grok never sends `session/request_permission` for exec tools. Instead it emits `_x.ai/session_notification{update.sessionUpdate:"pending_interaction", kind:"permission", tool_call_id}` and **self-resolves it** (`interaction_resolved`) with no client reply — the runner has no handler for that protocol.
- `--always-approve` (Task-218's YOLO spawn flag) forces the identical bypass even when the ask-mode env is present (live-probed: flag wins over env).
- Contrast: Grok DOES send the standard `session/request_permission` for **file edits** — that path gates correctly (37 historical allows; scan-deny verified live 2026-09-01: `reject-once` → tool failed, model's pre-tool text still streams, no BUG-341-style blank).

### Current Ask

- Make scan/plan read-only postures (and the YOLO-off approval card) actually gate Grok exec tools, using the same "always-ask + runner decides" discipline BUG-331 installed for opencode.

### Key Decisions

- `V-1` Launch every Grok process with `GROK_DEFAULT_PERMISSION_MODE=ask` (env) and **never** pass `--always-approve` (live-probed: the flag overrides the env; the env overrides a user config.toml `permission_mode="always-approve"` bypass).
- `V-2` With ask-mode, exec emits the standard `session/request_permission` and blocks — the existing `grokAdapter.handleInbound` decision layer applies unchanged: YOLO-on → auto-approve, scan/plan → read-only policy (exec/write denied, reads allowed), YOLO-off → approval card. No new adapter protocol code.
- `V-3` `handle.alwaysApprove` and the process-key dimension stay (flip-respawn contract, old tests untouched except the one args assertion amended below).

### Constraints

- One legacy test asserted the old launch contract (`["agent","--always-approve","stdio"]`, grok_registry_test.go `TestEnsureGrokProcessPassesAlwaysApproveFlagWhenRequested`). That assertion encodes the exact bypass being fixed; it is amended to `["agent","stdio"]` with a rationale comment (R1 disclosure in CA-714).
- YOLO-on exec now round-trips one extra RPC per tool (grok asks → runner auto-approves). Live probes show negligible latency.

### Open Questions

- Whether Grok's `pending_interaction` channel carries other interaction kinds that could hang under always-ask (none observed; prompts completed normally in every probe). Monitor live usage.

### Source Refs

- Live probe scripts: `/tmp/grok_perm_probe.py`, `/tmp/grok_perm_probe2.py` (grok 1.0.13, 2026-09-02), transcripts in CA-714.
- Runner-API repro runs: `run-456291` (yolo-on scan bypass), `run-459828` (yolo-off bypass), `run-460253` (edit deny → non-blank), BUG-341 family runs 421135/424302/433929/437116/440305.
- Task-208 Open Question Q-1 ("the bridge only helps if Grok happens to ask at all") — answered: with ask-mode, Grok asks.

## 1. Issue Summary

Scan/plan chat postures promise read-only behavior: every tool reaches the runner's approval bridge and the read-only policy auto-approves reads and silently denies writes/exec (`chat_posture_policy.go`). For Grok this never happened for exec tools: Grok's ACP dialect gates `run_terminal_command` through its own `pending_interaction`/`interaction_resolved` notification channel (not the standard `session/request_permission` request), self-resolves it when the client does not answer, and runs the command. Additionally, YOLO handles were spawned with `--always-approve`, which forces the same self-resolve even against an ask-mode override. Result: a "read-only" Grok chat could execute arbitrary bash.

## 2. Parent Links

- impacted coding plan: CP-46 (Grok provider), Task-208/Task-218 permission channel + YOLO SSOT
- impacted tech design: SD-06 AI Provider Integration (approval gating per provider)
- impacted system spec: SS read-only chat posture (scan/plan silent deny)

## 3. Environment and Reproduction

- environment: macOS, runner served from `apps/local-runner`, grok 1.0.13, active account `/Users/tiendat/.grok`
- reproduction steps:
  1. Start the runner, create a chat run `providerKey=grok, yoloMode=true, chatMode=normal_chat, cwd=<repo>`.
  2. Send a turn with `chatPosture=scan` and a prompt asking to run `ls -la .flowpilot/ | head -n 20`.
  3. Observe `tool_completed` with real directory listing output — no `permission_required`, no deny.
- frequency: deterministic (both YOLO-on and YOLO-off).

## 4. Root Cause

1. Grok's exec approval does not use the ACP-standard `session/request_permission` (edits only). Exec uses `_x.ai/session_notification → pending_interaction` and auto-resolves when the client stays silent — the runner's entire decision layer (YOLO auto-approve, YOLO-off card, read-only deny) is unreachable for bash.
2. Task-218 spawned YOLO handles with `--always-approve`, which live-probes show overrides `GROK_DEFAULT_PERMISSION_MODE=ask` — the same self-resolve bypass, by construction, for YOLO accounts.
3. `chat_posture_policy` deny paths were therefore dead code for Grok exec, the same class BUG-331 fixed for opencode (allow-all by default, runner never asked).

## 5. Fix

1. `grok_process.go` `grokProcessEnv`: append `GROK_DEFAULT_PERMISSION_MODE=ask` unconditionally (choke point, shared by every spawn; env beats a user config.toml `always-approve` bypass — live-probed).
2. `grok_process.go` `ensureGrokProcessSegmented`: never append `--always-approve`. `alwaysApprove` remains on the handle/process key so the flip-respawn contract is unchanged.
3. `grok_adapter.go` `handleInbound` unchanged: it already answers `session/request_permission` per `yoloModes` (YOLO-on auto-approve; read-only postures force yoloModes=false → bridge → `readOnlyApprovalDecision` denies exec/writes, allows reads; YOLO-off → card).
4. Legacy test amendment (disclosed): `TestEnsureGrokProcessPassesAlwaysApproveFlagWhenRequested` now asserts `["agent","stdio"]` — the old assertion locked the bypass itself.
5. Additive tests (`bug343_grok_always_ask_env_test.go`): env carries the ask mode; YOLO spawn args never carry `--always-approve` (handle key preserved); non-YOLO args unchanged; exec option kinds (`allow_always/allow_once/reject_once/reject_always`) map to `allow-once`/`reject-once`.

## 6. Validation

- Red-before: with pre-fix `grok_process.go` stashed, `TestGrokProcessEnvForcesAlwaysAskPermissionMode` FAIL (env missing) and `TestGrokSpawnArgsNeverCarryAlwaysApprove` FAIL (`[agent --always-approve stdio]`).
- Live probe matrix (real binary): default mode → exec self-runs (bug); env ask → `session/request_permission` blocks; reject reply → command not run, `stopReason=cancelled`; allow reply → command runs, `end_turn`; env ask + `--always-approve` → self-resolves (flag wins); env ask + config `always-approve` → still asks (env wins).
- Live runner verification post-fix: see CA-714.
- Claude/Codex/opencode: unaffected — no shared code path (grok-only spawn args/env; opencode has its own BUG-331 overlay; claude/codex gate via their own CLI/app-server channels).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-343
change_type: bugfix
summary: force grok always-ask via GROK_DEFAULT_PERMISSION_MODE=ask and drop --always-approve so scan/plan postures and the YOLO-off card gate exec through the runner bridge
# --->8---
