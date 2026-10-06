# CA-1227 — runner: BUG-642 scaffold pre-existing check attributes writes to the current turn

- **Area**: apps/local-runner/internal/runner (gate_hook.go)
- **Evidence**: live run-306526 (vibe-tasks CP-04, Task-045), 2026-10-06 —
  `r-scaffold-red` re-fired `vibe owner-debate mount cap` ~5× in 30 min on
  `core/security-rasp/src/main/cpp/CMakeLists.txt`. The file's content was
  the *sanctioned* Task-044 split committed by coder remediation
  (`23781fd`+`d99145c`, adjudicated adj-22/31, declared under contract
  v7) — but the check diffs the workspace against the contract's
  `BaseSHA`, so committed remediation diffs forever and every later
  scaffold turn re-parked. Operator discharges cleared the park only;
  nothing in the diff changed, so the next pass re-fired.
- **Bug**: `preLockWritten` (snapshot of the fingerprint-filtered
  cumulative writes) was classified against the whole `BaseSHA..now`
  window with no turn attribution — a historical sanctioned write is
  indistinguishable from a fresh scaffold write.
- **Fix**: new helper `unchangedSinceTurnStart(cwd, path, rs)` exempts a
  flagged path only when it is provably untouched THIS turn:
  1. its content fingerprint matches the turn-start dirty snapshot, or
  2. for a tracked path absent from the snapshot,
     `git diff --quiet turnStartGitHead -- path` exits 0 (worktree+index
     identical to the turn-start commit — covers remediation committed
     before the turn).
  Fail-closed: no snapshot entry AND (untracked | empty head | git error)
  reports "changed". A this-turn write — uncommitted, staged, or
  committed mid-turn — still flags; mid-turn commits additionally hit the
  commit-reserved guard (Task-242 D-7) which blocks the turn outright.
  Scope is deliberately the `scaffoldPreExisting` classification only —
  the general scope-drift gate keeps cumulative-vs-base semantics (an
  undeclared committed path is drift regardless of when it landed).
- **Deliberately deferred**: the doc's second root-cause arm (durable
  resolved-violation ledger so an adjudicated cap discharge suppresses an
  identical re-detection). With turn attribution the observed wedge cannot
  recur — the adjudicated content either lands as a commit (exempt on
  later turns) or stays uncommitted (still a genuine violation). A ledger
  would only matter for adjudicated-but-never-committed sanctioned
  writes; tracked in the BUG-642 doc.
- **Provider parity**: provider-agnostic — gate-side git/diff logic; no
  adapter, event stream, or session code touched.
- **Tests (additive, reproduce-first)**:
  `bug642_scaffold_preexisting_cumulative_test.go` —
  - `TestScaffoldGateIgnoresRemediationCommittedBeforeTurn`: live repro —
    locked declared file rewritten by a pre-turn commit must not flag
    (red before fix).
  - `TestScaffoldGateStillFlagsLockedFileWrittenThisTurn`: uncommitted
    this-turn write to the locked file still flags (guard).
  - `TestScaffoldGateFlagsMidTurnCommitOnLockedFile`: mid-turn committed
    write still blocks — commit cannot evade attribution (guard).
- **Verification**: `go test -count=1 -v -run 'Scaffold|scaffold'
  ./internal/runner/` — all green including
  `TestScaffoldBoundedStubs_*`, `TestVibeScaffoldRedWaived_*`,
  `TestRun2062497_*`. Full `./internal/runner/` suite run in the same
  pass — only `TestBug334_ProbeSegmentIsolatedFromChatScope` fails, and it
  fails identically on the stashed pre-change tree (`opencode` binary not
  on PATH — environmental).
- **Worktree**: n/a (runner-only change).
