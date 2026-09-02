# CA-714 — Grok exec gated via always-ask env; `--always-approve` removed (BUG-343)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-343
change_type: bugfix
summary: force GROK_DEFAULT_PERMISSION_MODE=ask on every grok spawn (appended last so account ExtraEnv cannot defeat it) and never pass --always-approve, so scan/plan read-only postures and the YOLO-off card gate exec through the runner bridge
# --->8---

## Problem

A Grok chat turn in `chatPosture=scan` executed real bash with zero permission
round-trip (run-456291 yolo-on, run-459828 yolo-off; live-probed grok 1.0.13,
probe scripts `/tmp/grok_perm_probe*.py`). Two compounding causes:

1. Grok's default permission mode gates exec (`run_terminal_command`) through a
   `pending_interaction` notification channel it self-resolves when the client
   stays silent — the runner's decision layer (YOLO auto-approve / read-only
   deny / YOLO-off card) is unreachable for bash. File edits DO use the
   standard `session/request_permission` and were already gated (37 historical
   allows; scan-deny live-verified run-460253).
2. Task-218 spawned YOLO handles with `--always-approve`, which live-probes
   showed overrides `GROK_DEFAULT_PERMISSION_MODE=ask` — the same
   self-resolve bypass, by construction, for YOLO accounts.

## What changed

- `grok_process.go` `grokProcessEnv`: append `GROK_DEFAULT_PERMISSION_MODE=ask`
  **after** the extraEnv loop (last-occurrence-wins in the child env) so the
  gate beats BOTH a host `os.Environ()` value and an account.ExtraEnv entry
  carrying the key — the same overlay discipline BUG-331/CA-690 installed for
  opencode. `handleInbound` then sees a standard `session/request_permission`
  for exec and decides: YOLO-on auto-allow, scan/plan deny writes+exec via
  `readOnlyApprovalDecision`, YOLO-off approval card. Env also wins over a
  user config.toml `permission_mode="always-approve"` (live-probed).
- `grok_process.go` `ensureGrokProcess`: `--always-approve` is NEVER passed,
  even for YOLO handles. `handle.alwaysApprove` and the process key
  (grokProcessKey) are unchanged, so the flip-respawn contract holds.
- Stale comments updated: YOLO=true's config.toml non-rewrite is now justified
  by the ask-env override (env > config), not by the removed flag.
- Legacy test amended (R1 disclosure, owner-approved before commit):
  `TestEnsureGrokProcessPassesAlwaysApproveFlagWhenRequested` asserted the
  exact bypass being fixed (`["agent","--always-approve","stdio"]`); it now
  asserts `["agent","stdio"]` and keeps `handle.alwaysApprove=true`. No other
  legacy test changed.

## R1 — old-suite regression evidence

- Targeted re-run (env/args/ensure/apply/yolo-adapter/permission-policy tests)
  PASS with the change; `internal/tui/app` `TestTask291*` PASS.
- Full `./internal/runner` run with the change: FAILs are confined to
  pre-existing, unrelated tests (flow-engine residual/gate harness,
  skills-merge scope-drift, provider inventory/detect, supabase/firebase) —
  **none touch grok spawn env/args**, and the harness failures reproduce
  identically for claude/codex provider variants.
- Baseline proof: with the BUG-343 diff stashed AND the new test file removed,
  the sampled failing subset still fails with identical messages
  (`TestSkillsMergeClaudeProjectAndProviderHomeWithPrecedence`,
  `TestRootFlowEngineDefersCompletedUntilGate`,
  `TestDetectProvidersPopulatesInventoryShape`,
  `TestMultiWorkspaceRunsIndependent`, …) → not caused by this change.

## R2 — provider classification

Per-provider, grok-only: the change is confined to `grok_process.go`
(env/args of the `grok agent stdio` spawn) and its registry tests. Claude,
Codex, and opencode have no shared code path (opencode keeps its own
BUG-331/CA-690 `OPENCODE_CONFIG_CONTENT` overlay in `opencodeLaunchEnv`;
Claude/Codex gate via their own CLI channels). No Claude/Codex test file
touched.

## R3 — new test coverage (`bug343_grok_always_ask_env_test.go`, additive)

- `TestGrokProcessEnvForcesAlwaysAskPermissionMode` — every spawn carries the
  ask env (choke point).
- `TestGrokProcessEnvExtraEnvCannotDefeatAskGate` — account.ExtraEnv carrying
  `GROK_DEFAULT_PERMISSION_MODE=always-approve` still ends with the ask value
  (last-wins; red before the env-order fix).
- `TestGrokSpawnArgsNeverCarryAlwaysApprove` — YOLO spawn args have no
  `--always-approve`; handle key still records alwaysApprove.
- `TestGrokSpawnArgsNonYoloStillBare` — non-YOLO launch unchanged
  (`agent --model … --reasoning-effort … stdio`).
- `TestGrokEncodePermissionDecisionCoversExecOptions` — real exec option kinds
  (`allow_always/allow_once/reject_once/reject_always`) map to
  `allow-once`/`reject-once` through the standard channel.
- YOLO-on auto-approve path itself is locked by the unchanged legacy adapter
  tests (`TestGrokAdapterYoloOnAutoApprovesViaRunnerPolicyNotBridge`,
  `TestGrokAdapterYoloOffBlocksOnBridgeAndDeniesWithoutApproval`) — PASS.

## Honest gaps

- Post-fix LIVE runner E2E (real binary, runner API) is still pending — the
  pre-fix live probe matrix (default→self-runs; env ask→request_permission
  blocks; reject→not run/stopReason=cancelled; allow→runs; flag wins over env;
  env wins over config) is recorded in BUG-343 but no post-fix run-* transcript
  exists yet in this CA.
- `pending_interaction` still has no runner handler; if a future grok version
  self-resolves some ask-mode interaction kind again, exec could re-bypass —
  monitored live (none observed in probes).
- YOLO-on exec now round-trips one extra RPC per tool (grok asks → runner
  auto-approves); live probes showed negligible latency.
- Full runner suite has pre-existing unrelated failures on this branch
  (flow-harness/skills/supabase/firebase class) — tracked separately, not
  introduced here.

## Prior CA not undone

- Task-218/CA YOLO=false config.toml rewrite (auto-enforce gating) intact —
  kept as defense in depth for YOLO-off spawns.
- BUG-331/CA-690 opencode ask-gate overlay untouched.
- Flip-respawn contract (model/effort/alwaysApprove key) unchanged
  (`TestEnsureGrokProcessRespawnsWhenAlwaysApproveFlips` PASS).
