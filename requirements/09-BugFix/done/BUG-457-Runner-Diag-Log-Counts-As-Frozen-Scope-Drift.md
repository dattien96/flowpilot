# BUG-457: Runner's own flow diag log counts as frozen-scope drift

- status: done
- found: live run-13080 (post-rebase consolidated live test, bug-harness on devin/swe-2-high)
- fixed_by: CA-954
- tests: internal/runner/bug456_build_artifact_drift_test.go (TestGateDriftIgnoresRunnerDiagLog)

## Symptom (live)

run-13080 (bug-harness) reached `implement`; the coder child wrote the fix and
its CA note, then the post-turn frozen-scope gate escalated:

```
flow scope drift: wrote outside the frozen contract's declared paths:
.flowpilot/logs/features/agent-flow-engine/run-13080.ndjson
→ park WAITING_USER_APPROVAL
```

The "drift" path is the runner's own per-feature diagnostic log — appended by
`flowDiagLog` mid-turn inside the exact diff window the gate observes. Every
frozen writer gate pass on a flow-engine run self-parks on it: the file is
append-only so the turnStartWorktree fingerprint subtraction never matches,
and deleting it just recreates it next turn. Dead park by construction.

## Root cause

Same class as CA-649 (`.flowpilot/gate-metrics.ndjson` self-park) — a new
runner-owned observability file appeared (`.flowpilot/logs/features/**`) that
was not in the frozen-drift exemption set. The set already covers chats/
(bookkeeping), ledger files, contracts stores, canonical head, gate config —
logs/ was simply never added.

## Fix

`changecontract.IsRunnerLogsBookkeepingPath` — narrow `.flowpilot/logs/`
prefix exempt, mirroring `IsRunnerChatBookkeepingPath`. CA-427 Finding 2
stays closed: logs/ holds pure observability, no gate input lives there;
`.flowpilot/settings/` and `.flowpilot/contracts/` still drift.

## Tests

- `TestGateDriftIgnoresRunnerDiagLog` — RED before fix: declared
  `strutil.go` + `.flowpilot/logs/features/.../run-1.ndjson` must not park.

## Live evidence

- run-13080 diag: `flow_parked_awaiting_user` gateReason
  `flow scope drift: ... .flowpilot/logs/features/` 10:22:30 local.
