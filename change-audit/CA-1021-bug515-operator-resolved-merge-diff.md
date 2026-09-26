# CA-1021 — BUG-515: operator-resolved diff path for conflicted merges

## Context

Deep review B-5 second half: re-picking a conflicted tournament winner
re-applied the same stored patch and re-parked forever (live run-3688).
The decision feedback channel reached `runTournamentMergeNode` but was
never read.

## Changes

- `tournament_dispatch.go`: new `extractOperatorPatch(feedback)` — narrow
  detection (`diff --git` header or leading `--- `/`+++ ` pair), strips a
  trailing markdown fence, restores a missing trailing newline (git apply
  requires it — round-2 review found TrimSpace stripping it produced
  "corrupt patch"). `runTournamentMergeNode` applies the parsed diff in
  place of the recorded snapshot; clean → done + loser sweep, conflict →
  re-park with the operator's patch in the payload. Card option
  consequence documents the escape hatch.

## Tests

New `bug515_merge_operator_diff_test.go` — drifted-workspace conflict →
re-park → re-pick with resolved diff applies it and settles done;
prose-only feedback keeps the stored-patch path.
