# BUG-394: Scope-drift gate counts runner/provider harness self-writes as agent violations

## Metadata

- Document ID: `BUG-394`
- Title: `Frozen-contract scope-drift check flags runner/provider bookkeeping files (.flowpilot/guard/test_baseline.json, .flowpilot/settings/gate-config.json, .devin/mcp_config.local.json) as coder drift`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-44-Pluggable-Context-Source-Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [CP-58-Test-Steps](../../07-Coding-Plan/done/CP-58-Test-Steps.md), [CP-62-Test-Steps](../../07-Coding-Plan/done/CP-62-Test-Steps.md)
- Feature Keys: `change-contract`

## AI Quick View

### Summary

- The frozen-contract scope-drift check counts files the agent never wrote — the gate's own `.flowpilot/guard/test_baseline.json` (baseline capture write during the gate pass itself), the runner's first-init `.flowpilot/settings/gate-config.json`, and the Devin provider session's `.devin/mcp_config.local.json` — as coder scope violations.
- Confirmed 3× live: CP-44 run-11/run-4472 (`… .devin/mcp_config.local.json, .flowpilot/guard/test_baseline.json`), CP-58 run-2284 (`… .flowpilot/settings/gate-config.json`), CP-62 run-2737 (`… .flowpilot/guard/test_baseline.json`).
- The coder prompt explicitly forbids touching `.flowpilot/` — the gate punishes writes the agent neither made nor can prevent; unwinnable without `agent-loop/amend`, which pollutes the frozen contract with runner-internal paths.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** Mid-flow `flow_gate_violation / status=block` with `flow scope drift: wrote outside the frozen contract's declared paths: <runner-internal file>`; step parks `WAITING_USER_APPROVAL` / loop `blocked escalate`.
- **Expected:** Runner-internal state under `.flowpilot/` (guard baselines, settings, contracts, ledger) and provider-session files (`.devin/`) are excluded from the declared-paths drift check — consistent with CA-427's carve-out that keeps `flow-rules.json` drifting.
- **Actual:** `ObserveGitDiffSince` lists the bookkeeping files as added/modified and `FrozenContractScopeDrift` reports them as coder drift → `applyFlowControl(escalate)` → park. A custom gate-decision re-prompt reproduced the identical block (CP-44) → unwinnable loop.
- **Impact:** First `task-harness`/`bug-harness` run in any workspace without these files gitignored parks mid-flow on a false positive; recovery (`amend`) widens the contract with paths that aren't product scope. Note: real drift detection in the same sessions was correct (CP-62: `mathutil/fib.go` vs declared `stringutil/*`) — the defect is only the non-exempted internal paths.

## Reproduction

1. Fresh workspace without `.flowpilot/settings/gate-config.json` / `.flowpilot/guard/` gitignored; run `task-harness` or `bug-harness` to the first post-freeze writer gate pass.
2. The runner's own gate init writes `gate-config.json`; the gate's baseline capture writes `test_baseline.json`; a devin session writes `.devin/mcp_config.local.json`.
3. Next gate pass flags them as coder drift → park.
- runIds: `run-4472` parent `run-11` (CP-44, event seq 1212; earlier seq 769 was `exit status 128` pre-git-init), `run-2284` (CP-58, park at `test_signatures` ~03:37–03:47), `run-2737` (CP-62, blocked/escalate 04:45:36+ until manual `changes_requested` 05:05).

## Root cause

- `apps/local-runner/internal/runner/gate_hook.go:931-958` — the drift exemption list covers frozen-store, pending-canonical, ledger, chat bookkeeping, change-audit, canonical-head, legacy-contracts, tool-owned scaffold (`IsToolOwnedScaffoldPath`), and markdown (`IsMarkdownDocPath`) — but NOT `.flowpilot/guard/test_baseline.json`, `.flowpilot/settings/gate-config.json`, or `.devin/mcp_config.local.json`.
- The files are written by the runner/gate itself: `writeDefaultGateConfig` (`engine_gate_config.go:60-67`, called from `engine_setup.go:391`) on first init; baseline snapshot capture inside the same gate pass; provider MCP config by the Devin session.
- CA-427 deliberately avoided a `.flowpilot/**`-wide exemption (`flow-rules.json` must still drift) — the fix needs exact-path additions for runner-owned bookkeeping, not a broad glob.

## Evidence

- `~/fp-beds/lt-evidence/cp44/RESULT.md` (§8 BUG-LIVE-3 — `run-4472` event seq 1212; `flow-reproduce-*`/`flow-steps-runtime.json`)
- `~/fp-beds/lt-evidence/cp58/BUG-LIVE-3-gate-config-self-drift.md` + `~/fp-beds/lt-evidence/cp58/RESULT.md` (run-2284 gateReason; `runner.log` gate pass 03:37:47)
- `~/fp-beds/lt-evidence/cp62/BUG-LIVE-2.md` + `~/fp-beds/lt-evidence/cp62/RESULT.md` (run-2737; `l62-3-run2737-agentgraph.json`, `monitor.log`, `runner.log` ~7107 agent-side `git status` showing ` M .flowpilot/guard/test_baseline.json`)
- Bed files: `/Users/tiendat/fp-beds/lt-cp58/.flowpilot/settings/gate-config.json` (mtime 03:37, runner-written)

## Severity

- `high` — deterministic false-positive park on first run in non-gitignored workspaces; systematically blocks the implement→validate handoff and forces contract pollution to recover.

## Completion Notes (implemented 2026-09-23, CA-920)

- New `changecontract.IsRunnerOwnedConfigPath` exempts exact runner/provider bookkeeping paths (`.flowpilot/guard/test_baseline.json`, `.flowpilot/settings/gate-config.json`, `.devin/mcp_config.local.json`) from frozen-contract scope drift — wired into the gate_hook exemption chain.
- Exact-match only: the rest of `.flowpilot/**` stays covered (CA-427 boundary preserved).
- Unit: `TestBug394_RunnerOwnedGateBookkeepingPathsExempt` — green.
