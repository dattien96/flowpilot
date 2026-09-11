# CA-806 — turn-2: Failed guards, completed-child evidence, real validate tests

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Failed runs never park/resume; restart-lost DONE falls back to persisted completed-child evidence; Retry/OK proven against real command.validate
# --->8---

## Why

Turn-2 debate (RealValidate 2x P2 test-only, RestartLost 2x P1 + 1x P2, RegressSafe pass):
- Failed run with DONE→pending edge could park + OK resurrect terminal run.
- Restart reseeds rows PENDING: park silent no-op (no gate, idle open); Retry refused PENDING pred.
- CA-803/804 tests used hub.notify stand-in, never exercised command.validate.

## Change

- `maybeParkVibeResumeConfirm` + pause-gate OK: Failed status returns early (OK clears card, no loop flip, no dispatch).
- New `persistedCompletedChildExists`: DONE-or-completed-child counts as finished in `pendingVibeResumeFromNode` and `maybeAdvance` predecessor check (strict: unknown rows without evidence refuse, no heal).
- Production path confirmed: healed Running + fresh ctx reaches `runValidateNode`.

## Tests

New `ca806_turn2_failed_evidence_realvalidate_test.go` (6 pass): Failed no-park, Failed OK no-flip, Retry→real validate escalate, OK→real validate escalate, no-evidence no-heal, child-evidence advances. Old suites untouched, full pause-chain regression green.

## Providers

Agnostic Case 1.

## Will not undo

BUG-234, BUG-288 (Failed/Cancelled-stopped terminal), CA-801→805 chain.
