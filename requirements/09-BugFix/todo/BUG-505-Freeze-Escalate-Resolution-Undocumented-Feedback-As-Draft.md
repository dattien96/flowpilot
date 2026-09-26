# BUG-505 — `preflight_contract_freeze` escalate resolves by treating operator continue-feedback AS the planner draft; undocumented and brittle

## Status
FIXED — unit-verified, 2026-09-26, fixed build.

- Fix (CA-1007): in `resumeFlowWithFeedback`, a `contract.freeze`
  escalated node no longer feeds raw Continue feedback into
  `runContractFreezeNode` as the planner draft. Operator prose (or a bare
  Retry) with no retrievable draft retries the canonical planner delegate
  (`preflight_contract_plan`); a parseable operator-supplied draft or a
  draft retrievable from a child event/parent cache still feeds the
  freeze directly — the live escape hatch is preserved.
- Unit: `bug505_freeze_feedback_retry_test.go` (green).

## Live-found during
`run-15708` (bug-harness, grok planner), 2026-09-26, build 8c95a5bb.

Sequence:
1. Grok `contract-planner` ended its single turn mid-exploration without
   emitting a draft → freeze escalated `invalid planner proposal:
   changecontract: empty preflight draft`.
2. Operator `continue` with prose feedback ("planner ended without emitting
   a draft, re-run it…") → freeze re-evaluated with **the feedback text as
   plannerResult** → new escalate `invalid character 'c' looking for
   beginning of value` (feedback starts with 'c').
3. Operator `continue` whose feedback was a raw JSON draft
   `{"feature_key":…,"intent":…,"declared_paths":[…]}` → freeze accepted it
   and minted contract v2 → flow advanced.

## Problem

- The escalate card/loop never tells the operator that the expected input is
  a **raw preflight draft JSON**. The generic affordance ("continue with
  feedback") invites prose, which fails strict parse and re-parks with a
  parse error that reveals nothing about the required shape.
- There is no "re-run the planner" affordance at this gate: feedback is
  consumed as the node's output, so guidance text can never reach a new
  planner turn.
- Compounds BUG-503: the operator-supplied draft is also the only way to
  un-park, but a correct draft still can't fix a wrong workspace.

## Suggested fix directions

- When the freeze escalate reason is `invalid planner proposal`, the park
  card should expose a structured option: "retry planner" (re-dispatch the
  planner node with feedback) vs "supply draft" (paste JSON), instead of
  silently parsing any feedback as a draft.
- At minimum: on `ParsePreflightDraft` failure during a resume, include the
  expected schema fields in the escalate detail so the next feedback has a
  chance.

## Related
- BUG-503 (run-15708's deeper failure was wrong-workspace, this gate masked
  it for two rounds).
