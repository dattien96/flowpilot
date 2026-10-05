# CA-1212 — Commit gate attributes authorship via per-leg tool telemetry (live run-262417)

## Evidence
run-262417 (PrivateVault CP-04): the rework leg created premature commit
`eb27e87` at 12:54:51. The same second, the spec-aligner leg completed its
turn → `flow_gate_violation: "flow coding step created a git commit"` →
blocked + parked `waiting_user_approval` with no decision surface. A
read-only leg was blamed for a sibling's commit.

## Root cause
Two workspace-global observations fed the gate:
- `collectCommitSubjectsSince(cwd, baseSHA)` counts commits since turn
  base — a sibling's commit lands inside EVERY live leg's window.
- `observeTurnScopedDiff` = `ObserveGitDiffSince(baseSHA)` — includes
  committed-since-turn-start content, so the sibling's commit also
  inflated the innocent leg's `hasDiff` → `isCodingChild=true`.

Parallel legs share cwd; a workspace scan fundamentally cannot attribute
a commit to a leg. The only authoritative evidence is the leg's own tool
telemetry.

## Fix
- `finalizeInput` gains `ExecCommands []string` + `ToolCalls int`,
  populated by `finalizeInputLocked` scanning the turn's own
  `EventToolStarted` events — already recorded in `rs.events`, no
  provider pipeline changes.
- `execCommandFromToolInput` extracts command strings across provider
  Input shapes (bare string, map keys command/cmd/shell/script/input/
  argv). Non-exec tools carry file/content keys — never command keys —
  so scanning every tool is safe.
- `commitGateBlocksLeg`: block when (a) the leg provably ran a
  commit-shaped command, or (b) zero tool telemetry (degraded) AND a
  commit exists — fail-closed fallback preserving Task-242's invariant.
  A leg with telemetry and no commit command is provably innocent.

## Tests
- `ca1212_commit_gate_attribution_test.go`
  `TestCA1212_CommitGateAttribution` — innocent-with-telemetry skipped,
  own-commit blocked, degraded-telemetry blocks (fail-closed), Write-tool
  content with "git commit" inside not misread as a command.
  `TestCA1212_ExecCommandFromToolInput` — all provider Input shapes.
  `TestCA1212_FinalizeInputCollectsTurnTelemetry` — per-turn scoping.
