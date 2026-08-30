# CA-690 — Opencode acp always spawns ask-gated so YOLO/posture decisions fire in the runner

## Problem

Operator test E1 (CP-57-Test-Steps section E): with YOLO **OFF**, opencode
created `approval-test.txt` in the project with **no permission_required
card** — the write went straight through.

Root cause: opencode's default permission config is allow-all ("By default,
opencode allows all operations"), and neither the user's opencode.json/jsonc
nor FlowPilot's account config pinned a `permission` block. So live opencode
never emitted `session/request_permission` and the complete runner-side
decision layer stayed dead code:

- `handleInbound` (opencode_adapter.go:462-497): YOLO-on auto-approves tool
  permissions; YOLO-off routes to `bridge.RequestApproval` (card); read-only
  postures force `yoloModes=false` (opencode_adapter.go:241) so the bridge
  denies writes — all correct, all never triggered.
- `opencode acp` has NO `--auto`/`--permission` launch flag (verified `--help`
  1.18.25), so `ApplyOpencodeYoloPosture`'s `opencodeDesiredAuto` flip + process
  kill changes nothing observable; `OpencodePermissionMode` was a blank
  identifier (opencode_adapter.go:242, provider_registry.go:588).
- Scan/plan for opencode was prompt-only (CP-57 H2 gap) — same root cause.

This closes CP-57 OC-04/P-5/R-3 ("do not trust the opencode.json allowlist").

## Live probe evidence (opencode 1.18.25, 2026-08-30)

Scripts `/tmp/fp_perm_probe*.py`, raw in session log; results recorded in
`requirements/09-BugFix/todo/BUG-331-…md`:

- **A baseline**: default config → file created with **0 permission requests**.
- **B/C/D overlay**: `OPENCODE_CONFIG_CONTENT={"permission":{"edit":"ask","bash":"ask"}}`
  → real `session/request_permission` (kind=edit, with diff) for an in-project
  write; reply allow → file created, turn `end_turn`; client replying ERROR to
  opencode's `fs/write_text_file` (FlowPilot behavior) → opencode falls back to
  its own write, turn still clean.
- **G/H/I merge semantics**: file-config permission applies (G); the overlay
  WINS on conflict (H) and DEEP-MERGES without wiping file keys (I: file
  edit=ask + content bash=ask → edit request still fires) — account isolation
  (OPENCODE_CONFIG file, auth.json, mcpServers) intact.
- **J deny**: reply `reject` → **file NOT created**, turn ends `end_turn` —
  the gate really blocks (pre-verifies guide E2).

## Fix (OpenCode-only, always-ask + runner decides)

- `opencodeLaunchEnv()` (opencode_model_variants.go — the single shared env
  choke point for both the turn adapter factory and the variants prober) sets
  `OPENCODE_CONFIG_CONTENT = {"permission":{"edit":"ask","bash":"ask"}}` before
  both branches (account-resolved and env-HOME fallback), so same-scope reuse
  can never hand a chat turn an ungated process. Env-only → identical on
  Windows/macOS/Linux; no paths touched (CA-679 preserved).
- YOLO/posture decisions stay per-turn runner-side (`yoloModes` + bridge).
  **No respawn on toggle** — respawning would re-orphan sessions (BUG-329
  mechanism); `opencodeProcessKey` and same-scope reuse untouched; the `auto`
  key dimension is constant dead weight kept for old-test stability.
- Scan/plan gating comes free: read-only posture forces `yoloModes=false` →
  requests hit `bridge.RequestApproval` → existing read-allow/write-deny
  policy (chat_posture_store_test.go).
- `webfetch` deliberately NOT pinned: Codex/Claude/Grok gate file/exec only.
- TUI/Desktop unchanged — `YoloMode` already rides every TurnRequest; the
  CA-681 `/provider-accounts/opencode-yolo-posture` endpoint stays as-is with
  no caller needed.

## Parity classification (R2)

Per-provider fix. The only modified production function is
`opencodeLaunchEnv()`, called exclusively from opencode paths
(provider_registry.go:585 turn factory + opencode_model_variants.go:92
variants prober). Claude/Codex/Grok launch envs, adapters, and approval
machinery untouched: codex gates via per-turn sandbox/approval flags +
`exec_approval_request`/`applyPatchApproval`; claude via `--permission-prompt-tool`
+ permission MCP; grok via native `session/request_permission` — all three
CLIs ask by default, which is exactly the behavior the overlay gives opencode.

## Tests

- `bug331_opencode_permission_ask_env_test.go` (5, additive): overlay present
  in the env-HOME fallback branch AND the account-resolved branch (alongside
  untouched OPENCODE_CONFIG/HOME/XDG_DATA_HOME); overlay JSON pins exactly
  edit+bash=ask, nothing else; `opencodeProcessEnv` passes overlay + config
  file as distinct keys (CA-679 coexistence); BUG-329 same-scope reuse across
  model/variant/auto unaffected by the overlay.
- R1: baseline captured BEFORE the change (`Opencode|CA679|Bug329|ChatPosture`
  suites green); after the change the same suites stay green; full runner
  package failures (5) reproduce identically with the fix stashed →
  pre-existing, unrelated (firebase/flow/drive).

## Review hardening (post-review, same day)

- **Overlay must win (review Important #1)**: `opencodeLaunchEnv` previously
  set the overlay before `account.ExtraEnv` was spread over the map — a
  custom/stale account env carrying `OPENCODE_CONFIG_CONTENT` could ungate the
  process. The overlay is now assigned LAST in both branches, and
  `opencodeProcessEnv` strips a host-inherited `OPENCODE_CONFIG_CONTENT` from
  `os.Environ` (same class as `OPENCODE_API_KEY`) so the gate can only travel
  via extraEnv. Tests: `TestBug331OverlaySurvivesAccountExtraEnv`,
  `TestBug331ProcessEnvStripsHostOverlayContent`.
- **SUPERSEDED (BUG-334)**: the "sessions are independent" claim below held
  for prompts but NOT for MCP — opencode keys its MCP clients by server NAME
  per PROCESS, so any session/new with a different/empty mcpServers block on
  the shared process (child turn, variants probe) replaced the connection and
  killed in-flight parent MCP calls ("MCP -32000 Connection closed", guide
  section F). BUG-334 isolates child runs (`|child:<runID>`) and probes
  (`probe`) on their own process segments with base-only reclaim, and closes
  child processes on their terminal event. Original note kept for history:
  the variants probe rode the shared ACP process (CA-689c); a separate probe
  process was then judged unsafe because the old reclaim closed other
  scopes — the segmented reclaim removes that objection.
- **Accepted minor (no change)**: `handleGetOpencodeModelVariants` returns raw
  `err.Error()` with 502 — local-only endpoint, desktop is the sole client;
  message text is diagnostic, not a security boundary.
- Guide/commit wording corrected: E is pre-verified by probes but NOT yet
  ✅-marked — re-test E1–E5 after the runner restart (commit `4ba11d77`
  amended to `15ce6d6b` says "pending re-test").

## Residual notes

- Opencode processes already running from before this fix keep the old env —
  restart the TUI/runner (or toggle the account) to spawn gated processes.
- Guide E re-test expectations: E1 card, E2 deny→no file + clean end (probe J),
  E3 approve→file (probe B/C/D), E4 YOLO-on auto-approve no card (existing
  handleInbound branch), E5 question channel never auto-approved
  (`isToolPermission` restricts auto-approve to `session/request_permission`).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Inject OPENCODE_CONFIG_CONTENT permission edit/bash=ask overlay into every opencode acp spawn so approval/YOLO/posture gating actually fires
# --->8---
