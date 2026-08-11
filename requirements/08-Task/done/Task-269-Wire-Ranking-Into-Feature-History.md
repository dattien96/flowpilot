# Task-269: Wire Ranking Into Feature.History (CP-55 P-7)

## Metadata

- Document ID: `Task-269`
- Title: `Wire Ranking Into Feature.History`
- Phase: `task`
- Status: `done` (2026-07-31 — implemented, then reviewed by a dedicated Claude reviewer agent: 0 Critical, 8 Important, 9 Minor findings — a CONDITIONAL PASS. The core mechanism (activation boundary, empty-locus fallback, the disabling guarantee, contract-store independence) was verified correct, but the review found real defects at the edges the invariants exist to protect — the "current truth" entry could silently lose its excerpt and its enforcement instruction in ranked mode, and a zero-value Limit silently disabled the output cap. All 8 Important findings fixed and re-verified via mutation testing; all 9 Minor findings addressed inline or explicitly accepted/deferred. Full `internal/runner` regression re-run twice: 16 failures both times — the same 15 pre-existing failures plus 1 confirmed load-dependent flake, 0 new deterministic regressions. See [CA-430](../../../change-audit/CA-430-wire-ranking-into-feature-history.md) for the complete findings/fix accounting.)
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude (agent review, complete — see CA-430)`
- Created: `2026-07-31`
- Last Updated: `2026-07-31`
- Feature Keys: `context-regression-engine`, `agent-flow-engine`
- Parent Documents: [CP-55: Flow-First Preflight Contract, Context Retrieval, and Canonical Acceptance](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) (P-7), [Task-268](./Task-268-Deterministic-History-Relevance-Scorer.md) (P-6 — `ScoreHistoryEntry`/`RankHistoryEntries`, the primitives this task's `SelectHistoryEntries` builds on), [Task-262](./Task-262-Shared-Retrieval-Locus-Builder.md) (`buildRetrievalLocus`, wired into production for the first time by this task)
- Child Documents: `none`
- Related Documents: [CA-429](../../../change-audit/CA-429-deterministic-history-relevance-scorer.md), [CA-423](../../../change-audit/CA-423-shared-retrieval-locus-builder.md), `BUG-266`, `BUG-323`
- Replaces: `None`
- Tags: `context-regression-engine, change-ledger, retrieval-locus, ranking, feature-history, additive`

## AI Quick View

### Summary

Adds `HistoryRankingConfig`/`SelectHistoryEntries` (the "which entries to show" decision on top of CP-55 P-6's pure scoring) and `HistorySlotRanked` (the renderer), then wires both into `featureHistorySource.Fetch` — the live `feature.history` context source. `Fetch` now builds a `RetrievalLocus` via the previously-unwired `buildRetrievalLocus` (declared Contract + uncommitted diff + prompt paths) and ranks a feature's change-ledger history by code-locus overlap whenever it has more candidate entries than `DefaultHistoryRankingConfig`'s activation threshold (30), falling back to the pre-existing recency-only rendering otherwise — byte-identical to today's output for every feature below that threshold or with no locus signal at all.

### Current Ask

Implement P-7 exactly: add `HistoryRankingConfig`/selection/ranked renderer, preserve exact fallback output, keep the absolute newest feature entry available as current truth, build locus in `featureHistorySource.Fetch`, ensure the Contract store still feeds locus even when `change.contract`'s own rendering is disabled, and leave `chat.summary` untouched.

### Key Decisions

- `D-1` **The context-source "disabled" contract needs no new code in `Fetch` at all — it is entirely enforced one layer up, in `ContextSourceRegistry.Collect`.** `Collect` only calls `Fetch` for IDs present in its `enabledIDs` argument; a disabled source's `Fetch` is simply never invoked, so ranking work cannot run for a disabled `feature.history`. Verified directly (not assumed) by reading `Collect`'s implementation, and pinned post-review by a call-counting decorator test (`TestFeatureHistoryProducesNoOutputWhenSourceIsDisabled`, strengthened per Claude-agent review Finding I-8) that proves zero `Fetch` invocations, not merely zero rendered output.
- `D-2` **"The Contract store feeds locus even when `change.contract` rendering is disabled" required no fix — it already held by construction.** `buildRetrievalLocus` opens the Contract store directly (`changecontract.OpenStoreReadOnly`), with no cache, no singleton, and no dependency on the `changeContractSource` type or its registry entry. Confirmed by Claude-agent review reading the actual store-open code, and pinned by `TestFeatureHistoryRanksWhenChangeContractRenderingIsDisabled`.
- `D-3` **CORRECTION (Claude-agent review pass 1, Important — Finding I-1): `HistorySlotRanked`'s fallback path no longer double-reads the ledger.** The original implementation read the ledger once to decide fallback-vs-ranked, then called `HistorySlot`, which read it again to render — doubling the cost of the hottest, most common path (every feature below the activation threshold) and making the "preserve exact fallback output" guarantee depend on two separate reads agreeing, which the ledger interface never required. Fixed: the fallback path now renders directly from the already-read entries.
- `D-4` **CORRECTION (Claude-agent review pass 1, Important — Finding I-2/I-3): the ranked-mode output no longer silently weakens the "newest = current truth" invariant relative to the legacy renderer.** The ranked header now explicitly states the list is not chronological and names the current-truth marker as the one to build on (the legacy header's own enforcement instruction, previously dropped); the current-truth entry's `CAExcerpt` now always renders regardless of its rank position; and a backfilled current-truth entry (one that would not otherwise make the rank cut) is now placed at the FRONT of the shown list rather than the back, matching the reviewer's own recommendation that "current truth buried at the bottom of a relevance list is the worst of both orderings."
- `D-5` **CORRECTION (Claude-agent review pass 1, Important — Finding I-4): a `HistoryRankingConfig{Limit: 0}` no longer disables the output cap.** The original clamp conflated "Limit not set" with "Limit exceeds candidate count," resolving both to unlimited — silently defeating the one guarantee the legacy renderer enforced unconditionally. Fixed: a non-positive `Limit` now falls back to the package default before the len-clamp applies.
- `D-6` **CORRECTION (Claude-agent review pass 1, Important — Finding I-5): `DefaultHistoryRankingConfig` is now a function, not an exported mutable `var`.** The coding plan's own illustrative snippet used `var`, but an exported mutable global read at production call time is a real global-state hazard (any test in any package could reassign it and silently change ranking behavior process-wide) — deliberately deviated from the doc's illustrative shape for this reason; no test signature was anchored to it being a var specifically.

### Constraints

- Existing behavior preserved exactly for every case below the activation threshold or with an empty locus — confirmed via the full pre-existing `TestBuildFlowContextPackage*`/`TestRenderFlowContextPackage*`/`TestComposeFeatureBlocks*` golden-fixture suites, unmodified and green.
- `chat.summary` deliberately untouched — confirmed by `TestChatSummaryRemainsRecencyBased`, which seeds a Contract capable of anchoring a locus and confirms `chatSummarySource` never picks it up.
- Full `internal/runner` regression suite run twice (original pass and again after the review-fix pass), mandatory per the same risk classification established for prior CP-55 phases that touch live `runner` wiring (unlike P-6, which touched only `featurecatalog`).
- Provider-agnostic (Case 1).

### Open Questions

- **Carried forward from Claude-agent review (CA-430 Finding M-9):** the non-Flow per-turn injection path (`composeFeatureBlocks` → `HistorySlot`) remains recency-only; only the Flow context-package path (`featureHistorySource.Fetch`) is ranked. This is consistent with this task's own stated scope, but is recorded explicitly as a known asymmetry, candidate scope for a later phase.
- **Carried forward from Claude-agent review (CA-430 Finding M-4, originally raised in CA-429):** `SelectHistoryEntries`' `CommitUnix`-based recency ordering can, in principle, diverge from `changeledger.GetFeatureHistory`'s own raw-RFC3339-string ordering for a `CommittedAt` value carrying a non-UTC offset. This is `changeledger` ownership territory (fixing it means either normalizing `Ledger.Upsert`'s input or formally accepting the divergence), not resolved by this task.
- **Carried forward from Claude-agent review (CA-430 Finding M-2):** `SelectHistoryEntries`' newest-entry identity check compares `CommitHash` strings, which is safe for every real `changeledger.Ledger` (keyed by hash, duplicates including duplicate empty hashes cannot occur) but not formally guaranteed for an arbitrary caller-supplied slice. A fully robust fix would require threading original-index identity through `RankHistoryEntries`' public return shape — judged disproportionate for a currently-unreachable Minor finding.

## 1. Goal

Make a large feature's `feature.history` context section surface the commits most relevant to what a Flow is actually about to touch, instead of only the most recent ones — without changing anything for the common case where a feature's history is still small or the turn carries no locus signal.

## 2. Parent Links

- coding plan: `CP-55` P-7

## 3. Trigger

CP-55 P-6 (Task-268) added `ScoreHistoryEntry`/`RankHistoryEntries` but left them completely unwired — `feature.history` still rendered purely by recency via `featurecatalog.HistorySlot`, and `buildRetrievalLocus` (CP-54/CP-55 P-2, Task-262) had zero production callers.

## 4. Exact Change

- `internal/featurecatalog/relevance.go` (**modified, appended**): `HistoryRankingConfig`, `DefaultHistoryRankingConfig()`, `HistorySelection`, `SelectHistoryEntries`.
- `internal/featurecatalog/slots.go` (**modified**): `historyLedger`, `renderHistoryEntryLine`, `renderHistorySlotChronological` (extracted from `HistorySlot`), `HistorySlotRanked`.
- `internal/runner/context_sources_builtin.go` (**modified**): `featureHistorySource.Fetch` rewritten to build a locus and call `HistorySlotRanked`.
- `internal/runner/retrieval_locus.go` (**modified, comment-only**): stale "unconsumed" comment corrected.
- `internal/featurecatalog/history_selection_test.go` (**new**): 12 tests.
- `internal/runner/context_source_feature_history_ranking_test.go` (**new**): 5 tests.

## 5. Touched Areas

- files: 2 modified production files (1 comment-only), 1 modified production file with new exported API, 2 new test files
- modules: `featurecatalog`, `runner` (context sources)
- routes / tables: none (reuses the existing `.flowpilot/ledger/feature_history.ndjson` and `.flowpilot/contracts/contracts.ndjson`)

## 6. Acceptance Check (DoD)

- [x] Fallback preserves current output bytes exactly — `TestSelectHistoryEntriesFallsBackWhenLocusEmpty`, `TestSelectHistoryEntriesFallsBackAtThreshold`, `TestHistorySlotRankedPreservesLegacyBytesOnFallback` (strengthened post-review to also cover the legacy cap-omission line, Finding M-3).
- [x] Ranking activates above the threshold with a non-empty locus — `TestSelectHistoryEntriesRanksAboveThreshold`.
- [x] **The absolute newest entry remains available as current truth — verified both at the selection level (exact length, exactly-once presence, front placement) and at the rendered-output level** (corrected/strengthened after Claude-agent review Findings I-3/I-7) — `TestSelectHistoryEntriesKeepsNewestAbsoluteTruth`, `TestHistorySlotRankedShowsCurrentTruthMarkerInRankedMode` (new), `TestHistorySlotRankedAlwaysShowsCurrentTruthExcerpt` (new).
- [x] Ranked selection is capped at the configured Limit — `TestSelectHistoryEntriesCapsAtFifteen`; **a non-positive Limit uses the default cap rather than disabling capping entirely** (corrected after Finding I-4) — `TestSelectHistoryEntriesNonPositiveLimitUsesDefaultCap` (new).
- [x] Fewer-than-limit candidates are all present, just reordered — `TestSelectHistoryEntriesHandlesFewerThanLimit`.
- [x] Ranked output reports its selected/candidate counts — `TestHistorySlotRankedReportsRankedCandidateCount`.
- [x] `HistorySlotRanked` returns empty output on no entries, matching `HistorySlot`'s own contract (new, Finding M-7(3)) — `TestHistorySlotRankedReturnsEmptyOnNoEntries`.
- [x] `featureHistorySource.Fetch` builds its locus from the frozen/declared Contract — `TestFeatureHistorySourceBuildsLocusFromFrozenContract` (fixture corrected post-review to genuinely discriminate ranked from fallback output, Finding M-1).
- [x] The locus merges Contract-declared paths, uncommitted-diff paths, and prompt-named paths — `TestFeatureHistorySourceMergesContractDiffAndPromptPaths`.
- [x] **Ranking still activates when `change.contract`'s own rendering is disabled** (already true by construction, now proven, not merely asserted) — `TestFeatureHistoryRanksWhenChangeContractRenderingIsDisabled`.
- [x] **Disabling `feature.history` itself means ranking work never runs, not merely that no output is shown** (strengthened after Finding I-8 with a call-counting decorator plus a positive control) — `TestFeatureHistoryProducesNoOutputWhenSourceIsDisabled`.
- [x] `chat.summary` remains recency-based and untouched by ranking, even with an anchoring Contract present — `TestChatSummaryRemainsRecencyBased`.
- [x] `gofmt -l`/`go build`/`go vet` clean on every touched/new file (package-wide `gofmt -l` on `context_sources_builtin.go` flagged only by the same pre-existing Windows-checkout CRLF churn CA-424 through CA-429 already documented; `git diff --stat` confirms a pure 12-line change).
- [x] **Full `internal/runner` regression suite run twice** (original pass and again after the review-fix pass): 16 failures on both runs — the same 15 pre-existing/environment failures established across CP-55 P-3 through P-6, plus 1 confirmed load-dependent flake — zero new deterministic regressions either run.
- [x] Cross-provider Case 1 agnostic.
- [x] **Claude-agent adversarial review performed and acted on**: 0 Critical + 8 Important + 9 Minor findings; all 8 Important findings fixed and re-verified via mutation testing; all 9 Minor findings fixed, folded into another fix, or explicitly accepted/deferred with reasoning in CA-430 — none silently dropped.

## 7. Out of Scope

- Ranking the non-Flow per-turn injection path (`composeFeatureBlocks`/`HistorySlot`'s remaining direct caller) — this task's own stated scope is the `feature.history` context source specifically (CA-430 Finding M-9).
- Normalizing `changeledger.Ledger.Upsert`'s `CommittedAt` input to close the residual string-vs-instant ordering divergence with `GetFeatureHistory` (CA-430 Finding M-4) — `changeledger` ownership, not this task's.
- Threading original-index identity through `RankHistoryEntries`'s return shape to close the currently-unreachable newest-entry hash-collision edge case (CA-430 Finding M-2).
- CP-55 P-8/P-9.

## 8. Cross-Provider Note

Provider-agnostic (Case 1): `grep -inE 'providerKey|claude|codex|grok'` over every file this task touches returns zero matches — the mechanism reads only `changeledger.Entry`/`RetrievalLocus`/`FlowContextHints` fields, with no provider dimension.
