# BUG-638 — `FlowAwaitingUserCard` only exposes Allow/amend on drift-format parks; RESCOPE escalates that explicitly name paths to declare render Stop/Retry-only

- **ID:** BUG-638
- **Severity:** High UX — the one operator action the escalation asks for
  ("declare CMakeLists.txt in scope") is unreachable from the card; the
  only offered actions are Retry-with-old-scope (re-runs the proven-failed
  loop) and Stop.
- **Status:** FIXED — CA-1231 (2026-10-06): the card exposes a
  declare-paths amend field on every amendable park (canAmend), not
  only drift-marker parks. Pairs with BUG-637/CA-1230 — the endpoint
  now reaches the run's contracts mid-debate.
  the parent sprint contract while parked inside the nested debate flow);
  fixing the affordance alone still 404s until BUG-637 lands.
- **Found:** run-306526 (`vibe-tasks` CP-04, Task-044), 2026-10-06 ~04:40.
  Owner-debate escalate card shows "DECISION REQUESTED (rescope/contract
  amend): (a) declare CMakeLists.txt in scope — required by the contract's
  own wiring intent…" with buttons `Stop`, `Retry — run again with old
  scope`. No path input, no Allow.

## Defect

`awaitingUserDriftState` (flowAwaitingUserDrift.ts) derives `isDrift`
solely from `parseDriftedPaths`, which requires the literal marker
`"wrote outside the frozen contract's declared paths:"` — the shape a
coder-step drift park emits. A debate-synthesis escalate whose verdict is
"the contract was wrong, declare X" has no marker, so `isDrift=false` and
the Allow button (`FlowAwaitingUserCard.tsx:149-159`) never renders even
though the decision text literally names the missing path.

This makes RESCOPE verdicts a dead-end in the UI: the escalation *asks*
for an amend, the amend path exists (`amendFlow` → `agent-loop/amend`),
but the card offers no way to reach it. Operator must hand-edit
`.flowpilot/contracts/frozen_contracts.ndjson` — the live workaround used
on run-306526 (minted v7 for coder+tdd steps, `CMakeLists.txt` unioned
into `declared_paths`, then `flow-control:continue`).

## Fix direction

- Render the Allow/amend affordance whenever the block is amendable, not
  only when the reason matches the drift marker. Practically: a small
  free-form "declare path(s)" input on escalate cards, or a structured
  `amendPaths` field on the decision payload the escalate can populate
  (the debate verdict already knows the path — it wrote it in prose).
- If a structured field lands server-side, `parseDriftedPaths` gains a
  second source (payload.amendPaths ?? parsed marker) so prose never has
  to be machine-parsed.
- Pairs with BUG-637: amend must enumerate contracts by run, not by
  currently-mounted flow nodes, or the new button still 404s mid-debate.
