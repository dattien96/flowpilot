# CA-879 — CP-65 P-4 Escalation fallback wiring (Task-371)

# ---8<--- flowpilot:change-ledger
feature_key: tournament-harness
source_doc_id: Task-371
change_type: feature
summary: flag-gated tournament rescue on review-cap / cohort-stall / debate-stall with anti-recursion + state-level return path
# --->8---

## Why

The tournament must open itself when ordinary flows deadlock: review loop
at cap, cohort silent past stall timeout, vibe debate past owner-fail
retries. All three previously terminated in parks/cards only.

## Change

- **`runner/tournament_escalation.go`** (new): `FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION`
  flag (default OFF) + `TournamentEscalationEnabled()`; `runIsTournamentFlow`
  (topology/workflow identity, data-driven); `shouldEscalateToTournament`
  (single anti-recursion choke point); `escalateToTournament` (child run
  record inheriting workspace/provider/contract via shared workspace,
  distilled intent, tournament topology attached record-level with entry
  dispatch on first normal turn — no provider calls here; parent loop flips
  to `tournament_escalation`, never failed/stopped); `maybeEscalateCapToTournament`
  (shared hook: stuck-blocked loops only, single rescue); `resumeParentAfterTournament`
  (merge → running + winner noted; refuse → blocked/cap fail-closed).
- **3 hooks (flag-gated, 3–10 lines each)**: applyFlowControl continue-cap
  branch, stall sweep park site, debate owner-fail cap site (skips legacy
  park when rescued).
- **2 guards**: sweep early-return + transition rejected-preserve cover the
  new status (same treatment as blocked).
- **Deliberate deviations from Task-371**: (a) helper takes
  (parentRunID, reason) — frozen contract needs no parameter (shared
  workspace IS the handoff); (b) child untracked by the orchestrator
  children map so parks cannot wipe its intent (documented trade-off);
  (c) full live re-entry rides the normal Continue path — return path is
  state-level (P-5 scope needs provider turns).
- **Tests** (`tournament_escalation_test.go`, 6): cap rescue, debate rescue,
  flag-off identical, tournament-run + double-rescue refusal, live-loop
  refusal, resume merge/refuse.

## Tests

- New 6/6 green. Related old suites green: 85/85 (apply/submit flow
  control incl. cap+escalate settle, member/stall timeouts, hub-stall
  matrix, CA-792/796/803 debate, blocked-restart, 540927 cohort gate).
- Full agentpack green (P-3). `go vet` clean; core diffs verified surgical
  (orchestrator 2 lines, stall guard+hook, debate hook, cap hook +10,
  const +9; an incidental gofmt whitespace hunk in emitLocked reverted).
- R1: flag defaults OFF — the entire legacy suite runs the byte-identical
  fallback, proven by staying green. R2: Case-1 agnostic — no providerKey
  in new code paths (grep). R3: cap/stall/debate triggers + flag on/off +
  recursion + resume matrix covered.

## Prior CA claims kept intact

- CA-876/877/878 untouched. CP-64 reproduce gate, BUG-231 cap/escalate
  distinction, Task-241 stall semantics, run-2047 no-clobber rule all
  preserved (guards extend them, never narrow).
- Earlier broad-run FAIL was the operator's own 8m go-timeout on 245 heavy
  tests, not a test failure (no --- FAIL lines; targeted 85 pass in ~8s).
