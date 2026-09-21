# CA-898 — CP-67 P-6 closeout: test-matrix gaps, evidence table, live runbook

# ---8<--- flowpilot:change-ledger
feature_key: contract-first-tdd
source_doc_id: CP-67
change_type: feature
summary: Closeout slice for Contract-First Scaffold TDD — adds the remaining matrix tests (batch accumulation, multi-round negotiation redispatch, claude/codex/grok transport parity), records the 46-signature evidence table with the three renamed-test mappings, documents the in-memory batch-buffer limitation, adds the §11 live/manual provider verification runbook, ticks the upstream-sync checklist incl. the CP-64 flag retire note, and marks the plan DoD complete
# --->8---

## Why

The implementation slices (CA-895/896/897) were committed but the closeout
was incomplete: CP-67-Test-Steps.md still showed all 46 signatures as `todo`,
three documented test names had no exact match (legitimate renames never
recorded), two matrix gaps remained (multi-round negotiation, batch
accumulation), provider parity rested only on the shared-transport design
argument, the in-memory batch buffer was at risk of being silently treated as
restart-durable, and there was no manual/live verification runbook.

## Change

- `runner/cp67_coder_transport_test.go` (additive only):
  - `TestCoderBatchAccumulatesAcrossSubmissions` — two record-only
    submissions accumulate under the parent stepID (2 rows), and the buffer
    is per-run scoped (sibling run sees 0).
  - `TestNegotiationMultiRoundRedispatchesHub` — round 1 batch → hub
    dispatch → continue (NegotiationRound=1), then a fresh batch re-buffers,
    re-dispatches with the new rows in the prompt, and continue lands
    NegotiationRound=2.
  - `TestCoderOutcomeTransportProviderParity` — identical coder batch through
    `turnBridge.SubmitFlowControl` for claude/codex/grok parent runs returns
    `renegotiation_recorded` and buffers the same row (uses the CP-55
    per-provider fake-registry fixture, since the claude controlled runtime
    is not creatable via `createRun` in tests).
- `CP-67-Test-Steps.md`:
  - Metadata `todo` → `done` (2026-09-20, with pending-live caveat).
  - §8 evidence table filled: 43/46 exact-name pass; 3 mapped rows —
    `TestCoderOutcomeChildCallIsRecordOnlyAndBuffered` →
    `TestSubmitFlowControlCoderBatchIsRecordOnly` + HTTP variant;
    `TestExtractCanonicalSignaturesCppViaTreeSitter` → `...ViaLSP`;
    `TestValidateStubBodiesCppViaTreeSitter` → `TestValidateStubBodiesCpp`
    (default no-cgo build exercises the LSP adapter, not tree-sitter).
  - §8.1 lists the 17 transport/dispatch tests beyond the spec signatures.
  - §8.2 records the in-memory batch-buffer limitation (no
    ProviderSessionState persistence → graceful degradation on restart, NOT
    claimed durable) and the four verified pre-existing HEAD failures.
  - §9 lists CA-895..898; §10 checklist ticked with commit evidence.
  - §11 new: manual/live verification runbook — setup, Scenario A
    (contract-first happy path A1–A6 incl. the back-edge regression pin),
    Scenario B (renegotiation loop B1–B7 incl. cap and vibe-sprint parity),
    §11.4 provider parity matrix left `pending` (no live runs executed in
    this environment), §11.5 restart-loss observation note.
- `CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md`: 12 DoD
  checkboxes ticked (each verified against committed code/tests); Status
  `ready` → `implemented` with live-runs-pending caveat; Last Updated
  2026-09-20.
- `CP-64-Reproduce-First-TDD-Gate.md`: §8 retire note for
  `FLOWPILOT_ENABLE_REPRODUCE_GATE` (always-on since B-9; rollback = revert,
  not flag flip).

## safe-fix-contract reconciliation

- R1 (old tests): no pre-existing test edited in this slice; the four known
  failures reproduce identically on clean HEAD `eb2e07f6` — reported, not
  hidden.
- R2 (parity): `TestCoderOutcomeTransportProviderParity` now pins identical
  transport behavior for all three provider keys at the shared bridge; live
  provider runs remain pending per §11.4 — recorded, not claimed.
- R3 (matrix): accumulation + multi-round tests added; restart durability
  explicitly documented as out-of-contract limitation (§8.2, §11.5).

## Verification

- `go test ./internal/runner/ -run 'TestCoderBatchAccumulates|
  TestNegotiationMultiRound|TestCoderOutcomeTransportProviderParity'` — 3/3 pass.
- Focused CP-67 suite (Scaffold|Cp67|Signature|Negotiation|CoderOutcome|…):
  101 pass / 1 fail — the single fail is the pre-existing
  `TestIsFlowPlannerExcludedPathCoversSkillpackScaffold`.
- `go test ./internal/agentpack/ ./internal/flowgate/
  ./internal/changecontract/ ./internal/lsp/` — all green.
- `go vet` on the four touched packages — clean.
