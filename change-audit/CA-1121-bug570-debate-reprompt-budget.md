# CA-1121 — BUG-570: debate-routed reprompts consume the reprompt budget

## Why

Live run-100368 (PrivateVault Task-026 tdd leg): a reprompt-class gate
violation on a vibe child routed to the owner-debate resolver inside
`applyVibeGateResolver` — which returned early, BEFORE the reprompt-budget
accounting that bounds the dev/child reprompt branches. `repromptAttempts`
stayed 0 forever: violation → debate → verdict orders rework → reprompted
child turn violates identically (zero-delta stubs already on disk) → fresh
debate → ∞. The live leg churned debate rounds 4/5+ for ~30 minutes.

## What changed

`apps/local-runner/internal/runner/vibe_gate.go`:

- In `applyVibeGateResolver`'s `vibeGateOwnerDebate` branch, when the routed
  result's action is `reprompt`, the gated run's `repromptAttempts` is
  incremented before mounting the debate. When the count reaches
  `maxFlowGateReprompts`, the gate escalates (stamps the escalated child
  node + `flow_control escalate` on the hub, or the run itself when it has
  no parent) instead of mounting yet another debate — mirroring the dev
  reprompt-exhausted contract: bounded loops, escalate to the operator.
- Block-class violations are untouched: owner debate remains the correct
  resolver for blocks, which are operator-decision territory, not retries.

## Invariant

Every remediation reprompt — whether issued directly by the gate or routed
through the owner debate — must consume the shared reprompt budget. Loops
are bounded; hitting the cap escalates with a structured status.

## Tests

`bug570_578_debate_reprompt_test.go` (red → green):

- `TestBUG570DebateRoutedRepromptConsumesBudget` — two reprompt-class
  violations mount debates and consume the budget; the third escalates
  instead of mounting a third debate
