# BUG-465: Timeline collapseRepeatedFinals drops a second turn's identical answer

- status: done
- found: live cht_10a27db90766 / leg run-49042 (B-59-1 provider-switch drill)
- fixed_by: CA-962
- tests: internal/runner/bug465_collapse_cross_turn_test.go

## Symptom (live)

Devin → Codex provider-switch drill:

1. Devin leg answered turn-49033 ("marker 7f3a").
2. Switch → Codex leg (legSeq 1, raw handoff, includedTurnCount 1).
3. Codex turn-49044 answered with canned final
   "Done. The change is implemented and the step is complete."
4. Codex turn-49051 answered the marker question with the **same** canned
   final text.

Rendered timeline then showed `turn_started` (seq 10) with **no answer** —
looked like a silently lost response. Provider events for turn-49051
(message_delta → message_completed → turn_completed) and the raw
workflow_chat_events rows (seqs 11/12) were all present; the loss was in
the read model only.

## Root cause

`collapseRepeatedFinals` exists to dedupe the capture-mapper echo pair:
EventMessageCompleted and EventTurnCompleted both carry the final text of
the *same* turn, producing two adjacent message_completed records. Its
predicate was "text equals the last message_completed text **on the leg**"
with no turn boundary — so turn N's answer, when textually identical to
turn N−1's, was collapsed against the previous turn's record.

## Fix

In `collapseRepeatedFinals`: a `turn_started` record resets the per-leg
baseline (`delete(lastTextByLeg, rec.LegRunID)`). Dedup still collapses
the same-turn echo pair; a new turn's identical answer renders.

## Provider parity

Provider-agnostic: pure read-model fix over durable transcript records.
Live repro was on codex; identical canned finals happen on any provider.

## Notes

- Symptom class: "user asked a question, saw no answer" — same shape a
  real response-loss bug would have. Distinguishing evidence was the raw
  record store (seqs 11/12 present) vs rendered output.
