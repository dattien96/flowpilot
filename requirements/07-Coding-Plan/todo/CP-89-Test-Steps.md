# CP-89 Test Steps — Flow Launch: `immediate` vs `chat_then_forward`

- Document ID: `CP-89-Test-Steps`
- Title: `CP-89 Test Steps`
- Phase: `coding-plan`
- Status: `todo`
- Created: `2026-09-28`
- Parent Documents: `CP-89`
- Child Documents: ``
- Related Documents: `Task-451`, `Task-452`, `Task-453`, `CP-89-review.md`
- Tags: `verification, flow, forwardFlow, durability, restart`

## AI Quick View

### Summary

- Unit tests cover the latch, the forward seam, and the prompt pack. Live
  verification runs through real HTTP entry points on a provider that
  exists on the machine (Devin or Grok); other providers are declared
  skipped, never fake-verified.
- The critical matrix is `flowArm` value × turn shape × provider event:
  `immediate|pending|started × chat-turn|forward-turn × restart|provider-switch`.

### Current Ask

- Execute after Task-451 → Task-453. All §2 tests must pass before live
  verification. Existing suite regressions stop the task.

### Constraints

- Additive tests only; `immediate` default must be byte-identical — every
  existing flow test stays green unmodified.
- Live tests record provider + run IDs used; skipped providers are named
  with the reason.

## 1. Goal

Prove `chat_then_forward` is durable, fenced, bounded, and honest about
failures — and that `immediate` did not change.

## 2. Automated Verification

```bash
cd apps/local-runner && go test ./internal/runner/ -run 'TestTask45[123]' -count=1
cd apps/local-runner && go test -race ./internal/runner/ -run 'TestTask45[123]' -count=1
cd apps/local-runner && go test ./internal/runner/ -count=1   # full suite regression
```

| Group | Required tests | Pass criteria |
|---|---|---|
| Latch + durability | `TestTask451_*` (9 tests) | `pending` survives restart/switch; corrupt value fails closed; default unchanged |
| Forward seam | `TestTask452_*` (10 tests) | bare forward admitted; typed errors for no-pin/already-started/corrupt; fences run at forward time; `turnCount==0` guard untouched |
| Prompt pack | `TestTask453_*` (8 tests) | bounded, settled-only, role-filtered, deterministic, audited |

## 3. Live Test Cases (real runner HTTP entry points)

One live test file per campaign: `apps/local-runner/internal/runner/cp89_live_test.go`
guards the *harness* behind `LIVE=1` — the table below is executed manually
via curl/HTTP against a running binary, then mirrored as skipped-by-default
harness assertions like prior CPs.

| # | Live case | Steps | Expected evidence |
|---|---|---|---|
| L-1 | `immediate` unchanged | create run with `flowRef`, no `flowArm` → POST turn | entry child spawns; hub suppressed; identical to today |
| L-2 | Pending chat is chat | create `flowArm=pending` + `flowRef` → 3 plain turns | hub replies each turn; no child; `flowEngineDriven=false` in `sessions.ndjson` |
| L-3 | Forward starts flow | turn 4 `{"forwardFlow": true, "prompt": "Chốt: ..."}` | entry child spawn; child prompt contains forward text + a chat-transcript marker; `flowArm=started` in session row |
| L-4 | Bare forward | `{"forwardFlow": true}` no prompt | forwards fine — transcript-only package; NOT a 400 |
| L-5 | Double forward | second `forwardFlow` turn after start | 422 `flow_already_started`; no second entry child |
| L-6 | Forward with no pin | `forwardFlow` on a run created without flowRef | 422 `forward_requires_flow_pin` |
| L-7 | CP fence at forward | `vibe-cp-ingest` + `pending`; chat without source OK; forward without `SourceDocID` | chat turns 200; forward → 422 `invalid_cp_source`, `flowArm` stays `pending`; fix + forward again → starts |
| L-8 | Pinned SourceDocID | same but create carries `sourceDocID` | forward validates via pin — no re-paste needed |
| L-9 | Restart during pending | kill runner after turn 2; restart | run reconstructs `pending`; no `startResolvedFlow`; forward still works |
| L-10 | Restart after started | kill after forward; restart | run is a normal flow run; no re-arm; BUG-315 holds |
| L-11 | Provider switch mid-pending | switch provider leg on `pending`, then forward | `pending` rides the run; forward starts flow on the new leg |
| L-12 | Corrupt definition | pin flow, delete/corrupt pack file, then forward | 422 `invalid_flow_definition`; never a chat turn |
| L-13 | Never forwarded | create `pending`, chat, abandon/stop | run ends as a chat run — no nodes, no audit flow entries |

Skip matrix (record reason): providers without local binaries
(claude/codex/agy/opencode) — provider-parity claim limited to the executed
provider; `immediate` is provider-agnostic.

## 4. Exit Criteria

- §2 suite green + full `internal/runner` suite green.
- L-1 through L-13 executed or explicitly skipped with reason; each row has
  run IDs + evidence pointer.
- `CP-89-Test-Steps` updated to `done` alongside the task docs.
