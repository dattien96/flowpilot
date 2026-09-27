# CA-1039 — Tournament, quota and durability review follow-ups

## Summary

The post-live branch review found nine remaining defects across tournament isolation, quota routing, durable projections and Codex account scoping. BUG-522 through BUG-530 are fixed as one cross-cutting runner batch.

## Changes

- Tournament candidates fail closed when an isolated worktree cannot be created; prompts and child cwd use the candidate worktree.
- Quota-respawned tournament candidates preserve workspace and cohort identity.
- Tournament joins accept terminal children only from the current attempt, and a stale prior-round sidecar stamp can no longer bypass a live current-attempt child.
- Only operator decision feedback may override a recorded winner patch; agent result prose cannot become merge input.
- Tournament escalation binds candidates to connected/selectable providers: two distinct providers when available, both candidates on the single provider when only one is connected; candidate dispatch still validates account connectivity. The tournament config parser accepts every registry provider (claude/codex/devin/gemini/grok/opencode).
- Persisted-only run snapshots include the durable worktree projection.
- Corrupt quota ledgers produce repair-required admission failure; legacy empty-object fields remain readable, active claims can repair a missing session pin during reconstruction, and a corrupt ledger is never overwritten by the live-block recorder.
- Tournament escalation children are persisted before asynchronous first-turn dispatch.
- Codex app servers are retained per account scope instead of killing another account's in-flight process.

## Red tests

Before production changes, these tests failed by assertion:

- `TestBug522_TournamentWorktreeCreateFailureDoesNotSpawnInMainWorkspace`
- `TestBug523_QuotaRespawnPreservesTournamentWorktreeAndCohort`
- `TestBug524_TournamentRetryJoinIgnoresTerminalChildrenFromPriorCohort`
- `TestBug524_SidecarStampDoesNotBypassLiveRetryChild`
- `TestBug525_AutomaticMergeIgnoresDiffInAgentResultMessage`
- `TestBug526_TournamentEscalationUsesAvailableProviders`
- `TestBug526_SingleConnectedProviderBindsBothCandidates`
- `TestBug527_DurableRunSnapshotIncludesWorktreeProjection`
- `TestBug528_CorruptQuotaLedgerFailsClosed`
- `TestBug529_TournamentEscalationPersistsChildBeforeDispatch`
- `TestBug530_CodexDifferentScopesDoNotTearDownEachOther`

## Verification

- Focused BUG-522…530 suite: pass.
- Related BUG-446/453/478/515/519/520/521, tournament escalation, quota routing and durable snapshot suite: pass.
- Race detector for the BUG-522…530 runner scope: pass.
- `go vet` on `internal/runner` + `internal/worktree`: pass.
- Full `go test ./internal/...`: environment-limited — `internal/runner` requires `codex`/`claude`/`opencode` binaries on PATH and `internal/tui/app` has two pre-existing environment-sensitive failures; every seam touched by this batch is green.
- Live drill (Devin-only, run-1269 via HTTP `POST /client/workflow-runs` + `/turns` + `/agent-loop/continue` + `/questions/{id}/answer` on `/tmp/fp-live-devin`):
  - `problem_scout`, `parallel_rollout` DONE on `devin/swe-2-high`.
  - Candidate worktrees created per candidate (`candidate-candidate-a`, `candidate-candidate-b`); children pinned to their own worktree cwd.
  - BUG-522 live: a stale worktree from a cancelled run caused a fail-closed `escalate` instead of a main-workspace fallback (observed on run-1).
  - Grok account was auto-rediscovered by `syncProviderAccounts` and stayed `connected`, so candidate-a still bound `grok-4.5`; a seeded `billing_required` ledger vetoed every dispatch — three `quota_route_required` cards (q-1851/q-2199/q-2729) offering `use_for_run|devin|...`; zero grok provider calls were ever made.
  - `use_for_run|devin` answers committed `route_committed` repins durably; the child respawn was refused while the parent was parked on the arbiter card ("parent loop is blocked (escalate)") — recorded as an ordering constraint, not a silent drop.
  - Arbiter tied 1.000/1.000 → parked tournament decision card; `{"feedback":"candidate-b"}` captured the choice.
  - A planted conflicting `thing.go` in the main workspace made the winner patch conflict → merge-escalate card ("reply with your own resolved diff"); `{"feedback":"candidate-b\n<diff>"}` applied the operator diff — `thing.go` lands `x + x + x` with the operator marker, not either candidate's version; `go test ./...` green; run reaches `completed`.
  - Kill -9 + restart: durable `GET /client/workflow-runs/run-1269` still returns `completed`; a candidate child GET exposes the worktree `workingDirectory` projection.

## Files

- `apps/local-runner/internal/runner/tournament_dispatch.go`
- `apps/local-runner/internal/runner/tournament_escalation.go`
- `apps/local-runner/internal/runner/quota_gate.go`
- `apps/local-runner/internal/runner/quota_claim.go`
- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/interactive_resume.go`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/local-runner/internal/runner/codex_appserver_process.go`
- `apps/local-runner/internal/runner/runner.go`
- `apps/local-runner/internal/runner/sessions.go`
- `apps/local-runner/internal/runner/review_followup_tournament_test.go`
- `apps/local-runner/internal/runner/review_followup_durability_test.go`
- `apps/local-runner/internal/runner/codex_scope_registry_test.go`
- `apps/local-runner/internal/tournament/config.go`
