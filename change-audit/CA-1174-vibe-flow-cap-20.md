# CA-1174 — vibe flow family: policy.cap raised to 20

## What changed

`apps/local-runner/internal/agentpack/flow-pack/flows/`:

- `vibe-owner-debate.yaml` — `policy.cap` 5 → 20 (the owner-debate
  remediation sub-flow that resolves non-r-requirement gate fails)
- `vibe-tasks.yaml` — `policy.cap` 3 → 20 (the CP-level vibe-tasks
  parent flow)
- `vibe-ingest.yaml` — `policy.cap` 3 → 20
- `vibe-cp-ingest.yaml` — `policy.cap` 3 → 20

`extendBy`/`extendMax`/`onCap` untouched. `vibe-sprint.yaml` already
declares `cap: 20`.

Non-vibe harnesses (`bug-harness` 3, `bug-plan-harness`/`task-harness` 5,
`rag-harness`/`cp-harness`/`review-loop` 3) intentionally left at their
designed cost envelopes — the `dual_loop_cap_test.go` assertions pin
those exact values.

## Why (live evidence, run-183756)

- The owner-debate leg burned all 5 remediation rounds on grok owner
  agents returning truncated investigation logs with no verdict, then
  escalated `awaiting_user`. Each round costs minutes; cap 5 gives a
  flaky owner too little headroom to self-resolve before asking the
  human.
- `flow_executor` seeds `st.Cap/st.RoundCap` on the parent's shared
  loop state from whichever flow mounted last — so the debate leg's
  cap 5 overwrote the sprint's cap 20 on the shared loop, and the
  sprint's own submit_review_outcome round counter read "round 0/5"
  against the DEBATE leg's budget. Raising debate cap to 20 removes the
  immediate symptom; the shared-loop overwrite itself stays a latent
  bug (captured in the live bug ledger).

## Invariant

Remediation loops bound by a cap should get enough headroom to actually
remediate before escalating; a cap sized for a flaky sub-agent must not
starve the whole sprint on its failures.

## Tests

`go test -count=1 ./internal/agentpack/...` — green (pack load + cap
assertions unaffected; no test pins the vibe-family caps).
