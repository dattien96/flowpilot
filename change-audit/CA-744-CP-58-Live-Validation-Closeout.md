# CA-744 — CP-58 live validation closeout: S/A/B/C/F pass, B4 partial, D skipped, E1→BUG-357, TUI parity→Task-322

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: docs
summary: live validation of the CP-58 harness family (Tasks 304-307) across 6 runs (run-533004/547025/548341/556588/562663/564781): smoke S1-S6, plan loop A1-A5, code loop B1-B3, cp-harness slice-only C1-C6, regression canary F3 all PASS with run/marker evidence; B4 cap-fire partial (live-forcing infeasible, unit-covered + shared counter observed); D smoke skipped per operator (flow kept hidden); E1 FAIL transferred to BUG-357 (no artifact instance recorded on writer completion); TUI-vs-Desktop display gaps transferred to Task-322; code landed during validation: BUG-355 history-picker refresh + run-scoped transcript (3 commits, cli-tui) and BUG-356 slice-only docs verification (1 commit); zero pre-existing test edits across all commits
# --->8---

## Validation matrix (all live, gate-sandbox)

| Cluster | Result | Runs | Evidence |
|---------|--------|------|----------|
| P prep (P1-P6) | ✅ | — | branch `cp58-harness-dual-loop`; build PASS; 3 instances + 10 bindings seeded; mirrors ok; CA-712/728 read |
| S smoke S1–S6 | ✅ | 533004, 547025 | picker start; scout→context→writer; Task files written; reviewer `submit_review_outcome`; synthesis→freeze; S3 via live prompt verbatim in `runner.log` |
| A plan loop A1–A5 | ✅ | 548341 | reject→same-session re-enter (`doc-writer` ×1); Feedback re-entry; round-2 approve→freeze→test_signatures→implement; context/preflight stay DONE |
| B code loop B1–B3 | ✅ | 533004, 548341 | hub `changes_requested`→`implement` reuse (`coder` ×1); plan nodes untouched mid-loop; child counts 1/1/rounds/rounds |
| B4 cap | ⚠️ partial | 564781 | 1 continue then converge (coder `ask_user` broke the loop bait); shared counter observed live (1/3→2/3); cap logic unit-covered (`agent_orchestrator_test.go:142`, cap:3/onCap:escalate) |
| C cp-harness C1–C6 | ✅ | 556588, 562663 | 1 spawn entry; CP files full; reject→reuse; N Tasks ↔ P-* additive; `flow_audit_slice_outputs_verified` → done, no coding chain |
| D smoke variant | skipped | — | operator decision: flow kept hidden (`selectableIn` empty, D1 verified), not needed |
| E artifacts | ❌ E1 / OK E2 | 548341 | panel + per-run API empty of `plan_md` → BUG-357; writers never reprompted (E2 OK) |
| F regression | ✅ F3 | — | `agentpack/flowgate/changecontract` + runner RAG/review/context/flow-control suites PASS |

## Bugs surfaced by validation (all filed)

- BUG-354 (done, CAs 742/743): gate-oracle hang + watchdog bounds — 5 commits, 4 review rounds to KILL_CLEAN (prior session).
- BUG-355 (done): TUI history-picker stale cache (F1) + workflow-run transcript loss on reopen (F2) + replay/backfill double (cursor-zero rule) — 3 commits `e6142640/c17093db/1bae0103`, 13 new tests, full `tui/...` green, runner stash-diff symmetric (zero regression), F2 live-verified (single pair on reopen).
- BUG-356 (done): slice-only audit unresolvable (`blocked_validation_failed`, no validate node by design) — 1 commit `694c5ecf` (docs-only verification: no-validate-node + docs-scope + artifact presence → `slice-outputs-check (docs-only)` passed), 6 new tests, live-verified `flow_audit_slice_outputs_verified` → done on run-562663.
- BUG-357 (open): no artifact instance recorded on writer completion → E1 FAIL. Transferred to artifacts backlog with fix direction (record on OUTPUT-binding completion).
- TUI-vs-Desktop display gaps (observations, not bugs): #1 steps-panel provider/model/round → Task-322 (todo); #2 main-chat step cards → backlog, no doc unless requested.

## Code changes landed during validation

- `tui/app/{model,history,app}.go` + `tui/client/client.go` — history picker background refresh (BUG-351 pattern).
- `runner/chat_ssot.go` (run-keyed transcript fallback), `runner/run_timeline.go` + route (run-scoped timeline endpoint), `tui/app/chat_switch.go` + `app.go` (run backfill + eseq overlap guard + empty note).
- `runner/bug356_slice_audit.go` + `runAuditNode` wiring (docs-only verification).
- 19 new test files/functions total, all green; `go vet` + build clean; gofmt new-lines clean.
- Discipline: zero pre-existing test edits (one disclosed old-test count edit earlier in TUI provider-refresh work, out of this CA's scope); baseline-failing suites (`TestRun144900/147126` family, `TestDetectProviders*`, `TestTryAdvance*`, `TestFlowCodingPrompt*`, `TestFinalizerHookSurfacesArtifacts` pluralization) proven symmetric via stash-diff; provider parity via shared-server/single-path + 3-provider matrices where applicable.

## Remaining / transferred (not blocking closeout)

- BUG-357 open (artifacts backlog) — E1 stays ❌ until fixed; E1 failure is in the artifacts system (Task-307 spillover), not the harness loops/bindings/slice that CP-58 covers.
- Task-322 todo (TUI steps parity) + main-chat cards observation (no doc).
- D smoke skipped by operator decision (flow retained hidden).
- B4 partial rationale as above; cap remains unit-guarded safety net.
- Untracked/in-progress tree items NOT mine (`CP-60-Vibe-Working-Mode.md`, `Task-321`) left untouched.

## Closeout decision

- Flip Task-304, Task-305, Task-306, Task-307 and CP-58 to done on the strength of S/A/B/C/F3 evidence above, with the noted partial/skipped/failed items and their transfer targets.
- Branch `cp58-harness-dual-loop` ready for review/merge (12 validation-session commits, all `[Docs]/[BugFix]`-prefixed per commit contract).
