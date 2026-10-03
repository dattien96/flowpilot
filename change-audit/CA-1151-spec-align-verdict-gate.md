# CA-1151 — spec_align: green tests are not proof of intent

Owner rule (verbatim): a task is not done when the code is written and the
suite is green, because the TDD leg itself may have encoded the wrong
assertions. `pass` requires tests green **and** tests+code re-verified
aligned with the requirement chain — SS → SD → CP → Task. Before this change
no vibe-sprint step re-read the requirement documents after green: the
frozen contract was distilled once at `preflight_contract_freeze`, and every
downstream check (reviewer AC verdicts, audit, boundary) verified against
that contract — a contract frozen wrong, or TDD written to a drifted
contract, passed end-to-end.

## Change (pack data only — zero Go code)

New review-cohort member `spec_align` in `vibe-sprint`:

```
validate --done--> spec_align ─┐ cohort: review, join: all
validate --done--> reviewer  ──┴─> synthesis --done--> audit
```

- `validate` now has two `done` edges; the engine fans both out into the
  same `flow-auto-validate-round-N` cohort (existing
  `forwardDoneTargets` fan-out — verified live by the CA-1092 spawn test,
  which logs both child spawns on one `tryAdvanceFlowFromNode` call).
- `spec_align` declares `cohort: review`, so `synthesisDoneVerdictError`
  automatically requires a recorded `spec_align` verdict before synthesis
  done — the gate is the existing machine-verdict machinery, not prose.
- A cohort member's settle feeds the cohort join, not a per-node done edge,
  so `spec_align` intentionally declares **no** done edge (regression-pinned
  by test).
- `changes_requested` blocks done → existing `synthesis → coder` loop and
  `renegotiate_signatures → synthesis_negotiation → tdd` path fix the
  drifted test or code. `blocked` (ambiguous/contradictory spec chain)
  reaches `ask_user` through the synthesis escalate edge — requirement
  ambiguity is a human question, not an AI-silently-decides one.
- Agent def grants `tools: [Read, Grep, Glob]` only + `posture: read_only`
  on the node — the verifier cannot touch the tree. Verdict travels through
  `submit_review_outcome`; no file artifact is required from it.

## New files

- `flow-pack/agents/spec-aligner.md` — role contract: re-derive requirements
  Task → CP → SD → SS via `Parent Documents:`/`Parent Links`, classify each
  SYNCED / OUTDATED / MISSING / CONTRADICTS, verdict-first.
- `flow-pack/prompts/spec-align-check.md` — the turn prompt (same contract
  spelled as steps; both registered in `manifest.yaml`).

## Tests

- `ca1151_spec_align_gate_test.go` (new): node declared with
  `cohort: review` + `read_only` + `agent.delegate`; edges
  `validate→{spec_align,reviewer}` both present, no `spec_align→*` done
  edge; `synthesisDoneVerdictError` blocks on (a) missing spec_align
  verdict with reviewer approved, (b) spec_align `changes_requested`,
  passes only with both approved.
- `ca1092_vibe_sprint_reviewer_test.go`: updated to track the intentional
  topology change — validate done now fans out both cohort members; the
  synthesis-gate fixture records both verdicts.
- `ca1087_sprint_handoff_stale_done_test.go`: legit-dispatch fixture
  records the now-required `spec_align` approved verdict (same sanctioned
  fixture-tracking as its existing CA-1092/1093/1096 annotations).
- `task323_vibe_sprint_v2_test.go`, `task326_vibe_pack_inventory_test.go`:
  pack topology/inventory pins updated for the added node+agent (the
  inventory test's own comment documents this update mechanism — CP-67 P-3
  did the same for `scaffold-architect`).

## Provider parity

No adapter/event/session/gate code touched — the verdict tool
(`submit_review_outcome`) is provider-agnostic engine surface already
exercised for the reviewer leg on all three providers. The leg prompt uses
the same deferred-tool ordering instruction (verdict FIRST) that the
reviewer prompt carries for Grok.

## Known residual

`spec_align`'s own node still settles into the cohort join — it does not
block reviewer from starting (parallel). That is deliberate: the two checks
are independent; ordering cost outweighs nothing since both verdicts are
required at the gate anyway.
