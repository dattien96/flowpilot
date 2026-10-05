# CA-1214 — Operator adjudications project to a durable ledger (live run-262417)

## Evidence
run-262417 (PrivateVault CP-04): the user answered the AC-4 requirement
escalation ("defer to CP-10 device gate"). The answer existed only as
freeform `pendingAgentContext` text — the spec-aligner re-verdicted
`blocked` at 12:54 after having `approved` at 12:52, because no durable,
citable waiver record existed. The task doc amendments (AM-1/2/3) that
finally landed were agent-mediated, not a mechanical projection.

## Fix
`appendVibeAdjudication` — when operator feedback resolves a
park/escalation on a vibe run, the answer is also appended to
`.flowpilot/adjudications.ndjson` in the workspace: sequential record id
(`adj-N`), run id, task doc id, feedback text, timestamp. The drained
pendingAgentContext note names the record
(`Operator adjudication adj-N (durable: .flowpilot/adjudications.ndjson)`)
so the re-driven leg cites a durable record instead of interpreting prose.

Fail-soft: missing workspace or write failure only logs — never blocks
the resume path. Non-vibe runs keep the plain feedback note.

## Tests
- `ca1214_adjudication_ledger_test.go`
  `TestCA1214_AdjudicationLandsInDurableLedger` — two adjudications get
  adj-1/adj-2 with correct fields.
  `TestCA1214_AdjudicationFailSoft` — no workspace / empty feedback no-ops.
