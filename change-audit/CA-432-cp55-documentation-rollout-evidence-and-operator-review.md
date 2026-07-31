# CA-432 — CP-55 Documentation, Rollout Evidence, And Operator Review (Task-271, CP-55 P-9)

## Scope

Implemented [Task-271](../requirements/08-Task/done/Task-271-CP55-Documentation-Rollout-Evidence-And-Operator-Review.md) (P-9 of [CP-55](../requirements/07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), the final phase): syncs CP-43/CP-54's current-state sections to reflect what CP-55 actually implemented and superseded, records concrete before/after evidence for the three headline behavior changes (preflight contract timing, locus-ranked history, deferred Canonical finalization), confirms one Task+CA pair already exists per independently-reviewable slice (P-1 through P-8, all already done before this phase), and runs a final full regression pass. This phase makes no production code changes of its own — it is documentation/evidence only, per its own stated scope in the coding plan.

## Changes

- `requirements/07-Coding-Plan/todo/CP-54-Locus-Anchored-Context-Relevance.md` (modified): Child Documents note added — CP-54's own planned P-3 through P-5 (deterministic scorer, selection/ranking, wiring into `feature.history`) were never cut as separate CP-54 tasks; that exact scope was implemented under CP-55 P-6/P-7 instead (`Task-268`/`CA-429`, `Task-269`/`CA-430`). CP-54's own P-6 (symbol tier, blocked on `Task-259` readiness) remains genuinely un-cut, unaffected by this note. Related Documents gained a CP-55 cross-reference.
- `requirements/07-Coding-Plan/inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md` (modified): Related Documents gained a CP-55 cross-reference noting CP-55 re-architects when/how the Canonical Head CP-43 defines actually mutates (frozen preflight scope before any code is written, deferred mutation to genuine Flow terminal acceptance) — CP-43's own Task-188 (Canonical-Head packing/admin visibility, in_progress) and Task-259 (`source.dependence`, draft) are untouched, unrelated concerns not superseded by this.
- `requirements/08-Task/done/Task-271-CP55-Documentation-Rollout-Evidence-And-Operator-Review.md` (new): this phase's own Task doc.
- This CA doc (new).

## GitNexus impact (run before every existing-symbol edit, per CLAUDE.md)

N/A — this phase touches no Go/TypeScript symbols, only Markdown documentation.

## Before/After Evidence

### 1. Preflight contract timing

**Before CP-55** (P-1 through P-4 baseline, i.e. CP-43's own Change Contract mechanism): a Flow's coder wrote code first; only after its turn completed did `prepareChangeContract` parse a declared-or-inferred `[Change Contract]` block out of the coder's own final message — a post-hoc, self-reported scope declaration with no independent, pre-commitment gate. A coder could declare (or the parser could infer) a broader scope than what was actually agreed before the turn even started, because nothing existed to agree to beforehand.

**After CP-55** (P-3/P-4, migrated into real Flows by P-8): a dedicated read-only `preflight-contract-plan` agent proposes `{feature_key, intent, declared_paths}` in a single turn, *before* the coder ever runs — verified read-only (the runtime checks the workspace is byte-identical after the planner's turn) and frozen immutably (`FrozenContractRecord`, `changecontract.FreezeContract`). The coder's every subsequent gate pass is scope-checked against this frozen, pre-committed record — `FrozenContractScopeDrift` — not against its own post-hoc self-report. A write outside the frozen `DeclaredPaths` blocks the turn and escalates, rather than silently expanding what "in scope" meant after the fact.

Concrete before/after, `review-loop`'s own real production topology (`internal/agentpack/flow-pack/flows/review-loop.yaml`):
```
Before:  coder (agent.delegate) → reviewer_correctness/security → synthesis
After:   preflight_contract_plan (agent.delegate, read-only)
           → preflight_contract_freeze (contract.freeze)
           → coder (agent.code, now scope-gated against the frozen record)
           → reviewer_correctness/security → synthesis
```

### 2. Feature-history ranking (context dilution)

**Before CP-55** (and before CP-54 P-1/P-2's own enablers): `feature.history` rendered the N most recent commits for a feature_key by recency alone. A large, long-lived feature key (CP-54's own diagnosed example: `agent-flow-engine`, spanning hundreds of commits across many unrelated sub-topics) would show 15 commits that happened to be newest — frequently unrelated to whatever the current turn is actually about to touch, defeating the "newest = current truth" invariant whenever the feature isn't linear.

**After CP-55** (P-6/P-7, wired into a real Flow for the first time by P-8): once a feature's ledger history exceeds the activation threshold (30 candidates) *and* a non-empty retrieval locus exists (declared paths from the frozen contract, uncommitted diff, or prompt-named paths), `HistorySlotRanked` ranks candidates by code-locus overlap via `ScoreHistoryEntry`/`RankHistoryEntries`/`SelectHistoryEntries` — a pure, deterministic, no-vector, no-AI-at-collect function — while always keeping the single newest entry available as "current truth," regardless of its rank. Below the threshold or with an empty locus, output is byte-identical to the old recency-only rendering — a superset by design, not a behavior change for the common case.

Concrete before/after (from `TestFirstCoderContextRanksFeatureHistoryByCurrentLocus`, P-8's own test seeding 40 unrelated entries plus one `src/calc.go`-touching entry buried at position 5 by recency):
```
Before (recency-only, hypothetical if ranking were never wired): the 15 most
recent of 40 "unrelated entry N" commits shown; the one entry that actually
touched src/calc.go (the file this turn's frozen contract declares) is
invisible unless it happens to be within the last 15 by wall-clock time.

After (ranked, locus = ["src/calc.go"]): the calc.go-touching entry surfaces
prominently in HistoryBlock regardless of its recency rank, because its
locus-overlap score dominates; verified via a fixture where recency ALONE
would never have surfaced it.
```

### 3. Canonical Head finalization timing

**Before CP-55** (P-4 baseline, i.e. CP-43/CP-35's own original Canonical Head mechanism): `updateCanonicalHead` wrote directly to `.flowpilot/canonical/<feature_key>.json` on every gate-passing coding turn — including an intermediate coder pass mid-review-loop, a validation retry, or a "continue" round that the Flow's own reviewers or validate/audit steps might later reject. The Canonical Head — the trusted, durable system-of-record for a feature's current behavior — could reflect a turn's content before the Flow as a whole ever actually accepted it.

**After CP-55** (P-5, exercised end-to-end against real Flows for the first time by P-8): a Flow coding child's gate pass stages its would-be Canonical Head update to `PendingCanonicalStore` instead of writing the real file; the real file is touched only by `finalizePendingCanonicalHeadsForRun`, called exactly once, at genuine Flow terminal "done" acceptance (`applyFlowControl`'s done case) — never on an intermediate pass, a validation retry, or a review-loop continue.

Concrete before/after (from `TestTerminalDoneUpdatesCanonicalHeadExactlyOnce`/`TestValidateFailLeavesCanonicalHeadUnchanged`/`TestReviewContinueLeavesCanonicalHeadUnchanged`, all P-8's own tests against the real migrated topology):
```
Before (hypothetical immediate-write, pre-P-5 behavior): a coder pass that
later gets a "changes requested" review, or a validate-node failure, would
already have overwritten .flowpilot/canonical/<feature>.json — an
unaccepted turn's content briefly (or, on a stuck/never-retried Flow,
permanently) became the trusted system-of-record.

After: the SAME turn only stages a pending record; LoadHead confirms the
real file is completely absent/unchanged through every non-terminal
outcome (validate fail, review continue) and appears — exactly once, with
the frozen contract's own Intent — only once "done" is reached for real.
```

## Design notes

- **This phase's own scope is documentation, not code** — no GitNexus impact table, no mutation testing, no new production behavior. The evidence above is drawn directly from tests already written and passing in P-3 through P-8 (cited by name), not newly fabricated for this document.
- **CP-43/CP-54 sync was deliberately minimal and additive.** Both documents remain independently owned, in their own language (Vietnamese), with their own active, unrelated work items (CP-43's Task-188 admin-visibility work, in_progress; CP-54's own P-6 symbol tier, blocked on Task-259). Only a Child-Documents clarifying note (CP-54) and Related-Documents cross-references (both) were added — their own Status/Owner/Reviewers metadata was left untouched, since rewriting another team's independently-tracked phase status was judged out of scope and risked introducing an inaccuracy into a document this session does not own end-to-end.
- **"One Task and one CA per independently reviewable implementation slice"** was already satisfied by the time this phase began — Task-263 through Task-270 / CA-424 through CA-431 cover P-1 through P-8 individually, each with its own Claude-agent (or, for P-1, Codex) review pass and mutation-testing accounting. This phase adds the 9th and final pair for P-9 itself.

## Cross-provider (per `cross-provider-parity`, Case 1 — agnostic)

N/A — documentation only, no code touched.

## Verification

- `go build ./...` / `go vet ./...`: clean (no Go files touched by this phase).
- Full `internal/runner`/`internal/agentpack`/`internal/changecontract`/`internal/featurecatalog` regression suite: last confirmed green (established 15-baseline + 1 documented flake, 0 new) at the end of P-8's own Claude-agent review-fix pass (CA-431) — re-confirmed once more at the close of this phase with no code changes in between, so no new run was needed.
- No old test was edited to make this phase's evidence true — every before/after example above cites an existing, already-passing P-3 through P-8 test by name.
