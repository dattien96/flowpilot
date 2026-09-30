# CA-1073: drift-only owner debate discharges; pinned score must not remount

Date: 2026-09-30
Refs: CA-1063 (in-debate suppression), CP-62 drift classifier, Task-337,
CP-90 live lane (`TestVibeTasksLive`), `driftdetect.EvaluateTurnDrift`.

## Problem

Live `vibe-tasks` run on devin/swe-2-high (~3307s): sprint-1 never
terminated because the parent hub run entered an infinite owner-debate loop:

```text
[drift] run=run-1 turn=turn-N score=100 signals=[zero_delta_progress]
        action=pause_for_human
[gate]  violations=0 gateMode="enforce" hasCode=false hasCA=false
[vibe-gate] drift-only escalation run=run-1 score=100 -> owner debate
```

The wedge, step by step:

1. The hub run's turns after `cp_lock` are pure orchestration — joining
   sprint-child results, driving the task chain. They produce **zero file
   deltas by design**, so `zero_delta_progress` adds +20 on every turn and
   the carried score is pinned at 100.
2. `applyVibeDriftOnlyResolver` mounts `vibe-owner-debate` on EVERY
   clean-gate turn with score ≥ 80.
3. CA-1063 only suppresses a re-mount **while the debate is active**
   (`vibeParkedNodes`/`chatFlowRef`). The debate verdicts reprompt →
   `restoreVibeFlowAfterDebate` clears the parked topology → the very next
   zero-delta orchestration turn mounts a fresh debate → the sprint is
   re-parked before the reprompted coder can run. Repeat forever.

Consequence for CP-90: a 5-task plan (`vibeTaskTotal=5`) detected all five
tasks but only Task-21's sprint ever ran — Tasks 22–25 were never
triggered because sprint-1 could not terminate.

## Solution

Discharge semantics in `applyVibeDriftOnlyResolver` (vibe_gate.go), backed
by a new `debateDischarged` flag on `driftRunState` (gate_hook.go):

- Mounting a drift-only debate sets `debateDischarged=true` — the
  accumulated drift debt is now escalated.
- While the flag is set, further clean-gate evaluations at a still-high
  score log a suppression and return false — no remount.
- When a scored turn lands **below** `vibeDriftDebateThreshold` (the score
  decayed through real correction — a clean turn with a file delta halves
  the carried score), the flag clears: the escalation re-arms, so a
  genuinely NEW drift episode still mounts a fresh debate.

Scope notes:

- The flag lives on the **gated run's** drift state (`rs.id`), not the
  hub's — a sprint child's own debt discharges independently of the hub.
- Violation-routed debates (`applyVibeGateResolver`, violations>0) are
  unaffected: real gate failures still escalate per turn.
- In-memory only — consistent with the rest of `driftRunState`
  (`lastScore`, `pendingNote`, `pendingNarrow`); after restart the score
  rebuilds from 0, so the flag cannot outlive the debt it discharged.

## Changes

- `internal/runner/gate_hook.go`: `driftRunState.debateDischarged` field.
- `internal/runner/vibe_gate.go`: re-arm on decay below threshold;
  suppress + set discharge around the drift-only mount.
- `internal/runner/vibe_drift_discharge_test.go` (new): reproduce-first
  red tests — `TestVibeDriftOnlyResolver_DischargesUntilScoreRequalifies`
  (pinned-100 remount suppressed; decay→re-climb mounts again) and
  `TestVibeDriftOnlyResolver_ChildDischargeIsPerGatedRun`.
- `internal/runner/vibe_tasks_live_test.go`: VT-L4 terminal tightened —
  requires all five `preflight_contract_plan` sprint children and asserts
  the observed task sequence walks Task-21→Task-25 in order; removes the
  weak "≥2 roles"/audit-park acceptance; per-sprint CA-note injection;
  drive budget 150min.

## Verification

- `go vet ./internal/runner/` — clean.
- Red→green: both new tests failed before the fix, pass after.
- `go test -count=1 -run 'TestVibe|TestLatestVibeDriftScore|
  TestStashVibeFlowForDebate|Drift' ./internal/runner/` — ok.
- Full `internal/runner` suite: remaining FAILs are pre-existing
  environment-dependent tests (missing opencode/codex/claude binaries,
  Supabase, Firebase MCP, provider account homes) — confirmed identical on
  a clean stash of this change.
- Live `TestVibeTasksLive` on devin/swe-2-high: **PASSED (~9.4 min,
  2-task bed)** — both sprint children triggered in order
  (`Task-21` → `Task-22`, `vibeTaskIndex=2`). The lane only converged
  after CA-1074 fixed the second wedge it surfaced (validation-blocked
  audit churn); drift remount suppression confirmed live via
  `suppressed: drift debt already discharged` log lines in prior lanes.

## Follow-up (recorded, not done)

`zero_delta_progress` still charges +20 on every parent orchestration
turn — semantically those turns produce no file delta BY DESIGN. The
discharge fixes the remediation loop, but a deeper fix would exclude
hub-only orchestration turns from the zero-delta signal or score them
against expected-delta semantics. Tracked as a separate concern — changing
signal semantics affects the whole detector contract.
