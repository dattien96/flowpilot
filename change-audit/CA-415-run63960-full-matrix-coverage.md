# CA-415: full matrix coverage for blocked restart (run-63960)

## Why

Operator asked for full coverage after partial tests left Continue/Stop and
disk-shape gaps. Full R3 matrix + two production fixes found by the new tests.

## Production fixes found while writing full tests

1. **`stop()` did not bump `_agentGraphLoadSeq`** after `stopAgentLoop` —
   a late blocked HTTP graph refresh could restore Continue/Stop over Stop.
2. **`stop()` skipped `stopAgentLoop` when `status===blocked` but graph not
   yet loaded** (409 path sets blocked before refresh resolves) — Stop had no
   place to seal the loop. Now `status === "blocked"` forces loop stop.

## Test matrix (additive files)

### Runner — `run63960_blocked_restart_full_matrix_test.go`

| Axis | Values |
|---|---|
| Provider | codex, claude, grok |
| blockReason | cap, escalate, member_stalled |
| Disk shape | crash settle+reprompt, settle-only, reprompt-only, clean blocked |
| Lifecycle | no auto-turn, freeform 409, graph card data, Continue, Stop, second restart |
| Extra | child startTurn under blocked parent; real escalate park→restart; normalize status; gen high-water |

Also retains prior focused file `run63960_blocked_restart_no_gate_reprompt_test.go`.

### Desktop

| File | Coverage |
|---|---|
| `store.flow-awaiting-user-full-matrix.test.ts` | provider × err shape × blockReason; history open; Continue/Stop; graph fail; switch-run discard; late Stop race; false-positive 409/500; BUG-231 running child |
| `store.flow-awaiting-user-409.test.ts` | original focused cases |
| `store.flow-blocked-terminal.test.ts` | timeline settle blocked |

## Explicit residual (not unit-testable here)

- Electron live smoke (operator): restart runner+desktop, open real run
- Playwright UI click Continue/Stop
- Drive restore of blocked session (separate sync path)

## Prior claims preserved

CA-414, CA-413, run-23820 gen high-water, BUG-231, BUG-248 Stop→cancelled.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-231
change_type: test
summary: Full provider×blockReason×disk×lifecycle matrix for blocked restart; fix Stop load-seq race and Stop-when-blocked-without-graph
# --->8---
