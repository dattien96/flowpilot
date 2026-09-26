# BUG-518 — Worktree Diff fails when `.flowpilot/` is repo-ignored, killing every candidate patch snapshot

Status: FIXED (unit + live re-drive pending on run-49109)
Filed: 2026-09-26 (round-5 live test, run-49109)
CA: CA-1026

## Symptom

On the `lt-full` bed the runner installs `.flowpilot/` into
`.git/info/exclude` (runtime state stays out of `git status`). The
tournament arbiter's per-candidate patch snapshot then failed:

```
tournament.arbiter: patch snapshot for candidate-a: worktree:
git add -N -- . :(exclude).flowpilot: exit status 1:
The following paths are ignored by one of your .gitignore files:
.flowpilot
```

Fail-closed (correct) consequences, but the whole tournament was lost:

- `behaviorTournamentArbiter` errored → verdict-time `Cleanup` wiped BOTH
  candidate worktrees (their real patches went with them),
- the run parked `tournament_arbiter_blocked` with a prose escalate —
  no decision card,
- every `agent-loop/continue` re-ran the arbiter, hit the same error,
  re-parked. Run-49109 showed the loop twice in the flow diag log.

## Root cause

`Manager.Diff` intent-to-add'ed untracked files via a pathspec sweep:

```go
git add -N -- . ':(exclude).flowpilot'
```

When any ignore source (`.gitignore` OR `.git/info/exclude`) covers
`.flowpilot`, `git add` names the ignored directory during pathspec
expansion and exits 1 *before* the exclude element filters it. The
exclude magic cannot rescue a pathspec sweep once an ignored path is
named.

## Fix (CA-1026)

`internal/worktree/manager.go` — `Diff` now enumerates the intent-to-add
set explicitly instead of sweeping:

```
git ls-files -z --modified --others --exclude-standard
  | git add -N --pathspec-from-file=- --pathspec-file-nul
```

`--exclude-standard` honors `.gitignore`, `.git/info/exclude`, and
`core.excludesFile`, so ignored runtime dirs are never named. Modified
paths are included so the index write still runs whenever any change
exists — preserving the BUG-453 fail-closed contract (an index fault
still breaks Diff; verified by `TestBug453SnapshotFailureFailsClosed`,
unchanged). Clean worktrees skip the write entirely. The `git diff`
exclude pathspec on `.flowpilot` is retained as the second guard.

### Second vector (live run-69516, round-5 re-drive)

The first fix still died live: beds TRACK files under `.flowpilot/`
(canonical/catalog metadata), and `ls-files --modified` reports tracked
paths regardless of `--exclude-standard` — so modified tracked
`.flowpilot/...` entries reached `git add -N`, which rejects ignored
paths outright. The enumerated set is now filtered to drop `.flowpilot`
prefixes before feeding `add -N` (they're excluded from the patch
anyway).

## Tests

`internal/worktree/bug518_diff_ignored_flowpilot_test.go` —

- `TestBUG518_DiffToleratesIgnoredFlowpilotDir`: repo with
  `.git/info/exclude` → `.flowpilot/`; candidate worktree carries a
  `.flowpilot/logs/x.log` runtime file + real output. Diff must return
  the candidate patch (no error), must not contain `.flowpilot` paths.
- `TestBUG518_DiffToleratesTrackedFlowpilotFiles`: repo TRACKS
  `.flowpilot/catalog/features.ndjson` then ignores the dir via
  info/exclude; candidate modifies it. Diff must succeed and exclude
  `.flowpilot` (the exact run-69516 shape).

Regression coverage: `internal/worktree`, `internal/tournament`,
runner tournament suites (`TestBug453SnapshotFailureFailsClosed` keeps
the same assertion — the lock-poisoned fault still fails closed).

## Live evidence

- Found: run-49109 (devin hub + grok/devin candidates) — flow diag
  `tournament_arbiter_blocked` ×2 with the exact `git add -N` error;
  `git worktree list` confirmed both candidate dirs wiped.
- Reproduced standalone in `/tmp/repro-wt`: the two-command pipeline
  exits 0 where the pathspec sweep exits 1.
- Post-fix re-drive (r9, `run-82594` + `run-76075`): three arbiter
  verdicts routed across rounds — **zero** `patch snapshot` errors on
  the tracked-`.flowpilot` bed; merge_and_audit DONE and run completed
  after the restart leg.
- Second vector found live on `run-69516` (r8): tracked `.flowpilot`
  files enumerated by `--modified` → `git add -N` refused → same kill.
  Filter added + `TestBUG518_DiffToleratesTrackedFlowpilotFiles`.
