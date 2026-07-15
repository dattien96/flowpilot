# Task-268: Deterministic History Relevance Scorer (CP-55 P-6)

## Metadata

- Document ID: `Task-268`
- Title: `Deterministic History Relevance Scorer`
- Phase: `task`
- Status: `done` (2026-07-31 — implemented, then reviewed by a dedicated Claude reviewer agent: 0 Critical, 5 Important, 7 Minor findings — a CONDITIONAL PASS, since the production logic was judged functionally correct but 3 of the 9 originally-mandated tests were shallow enough that the reviewer constructed specific passing mutants that violated the phase's own invariants, including one that silently reintroduces the BUG-266 class of nondeterminism. All 5 Important findings fixed and re-verified via mutation testing (each temporarily reverted/mutated, the corresponding test confirmed to fail, then restored to pass); all 7 Minor findings addressed inline. See [CA-429](../../../change-audit/CA-429-deterministic-history-relevance-scorer.md) for the complete findings/fix accounting.)
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude (agent review, complete — see CA-429)`
- Created: `2026-07-31`
- Last Updated: `2026-07-31`
- Feature Keys: `context-regression-engine`
- Parent Documents: [CP-55: Flow-First Preflight Contract, Context Retrieval, and Canonical Acceptance](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) (P-6), [Task-261](./Task-261-Persist-Changed-Paths-In-Change-Ledger.md) (`changeledger.Entry.ChangedPaths`, the field this scorer reads), [Task-262](./Task-262-Shared-Retrieval-Locus-Builder.md) (`featurecatalog.RetrievalLocus`, the type this scorer compares against)
- Child Documents: `none`
- Related Documents: [CA-422](../../../change-audit/CA-422-persist-changed-paths-in-change-ledger.md), [CA-423](../../../change-audit/CA-423-shared-retrieval-locus-builder.md), `BUG-266`, `BUG-323`
- Replaces: `None`
- Tags: `context-regression-engine, change-ledger, retrieval-locus, ranking, additive`

## AI Quick View

### Summary

Adds `ScoreHistoryEntry`/`RankHistoryEntries` to a new `internal/featurecatalog/relevance.go`: pure, stateless functions that rank a slice of `changeledger.Entry` against a `RetrievalLocus` by (1) exact normalized path overlap, (2) commit recency, (3) a commit-hash tie-break — mirroring `changeledger.GetFeatureHistory`'s own established tie-break discipline (BUG-266). This phase deliberately does not wire the scorer into anything user-visible; `feature.history` continues to render by recency alone until CP-55 P-7 adds the selection/rendering layer on top of these primitives. `RetrievalLocus.Symbols` is deliberately never read (BUG-323 Q-2 — nothing populates it yet).

### Current Ask

Implement P-6 exactly: add `featurecatalog/relevance.go`, implement exact normalized path overlap, add explicit deterministic tie-breakers, keep symbols unused — while leaving every other part of the codebase byte-for-byte unaffected (no caller wired yet).

### Key Decisions

- `D-1` **Path overlap is decided by exact string equality after independent canonicalization on both sides, not fuzzy/prefix/substring matching.** `canonicalPathForOverlap` (forward-slash, `path.Clean`, trimmed) is applied to both `entry.ChangedPaths` and `locus.Paths` at comparison time rather than trusting upstream producers to already agree — Claude-agent review (Finding IMP-1) traced the actual producers and found they do not: a locus path built from a user prompt can retain a leading `./` (`flow_context_hint_paths.go` deliberately keeps it), while the ledger's own `ChangedPaths` never carries one. Re-canonicalizing inside this package, on both sides, closes that gap regardless of which producer drifts.
- `D-2` **`ScoreHistoryEntry`'s locus-set construction is factored out (`buildLocusPathSet`) so `RankHistoryEntries` builds it once and reuses it across every entry**, rather than rebuilding a fresh map per entry — a perf detail (Claude-agent review Finding MIN-3) that fell out naturally from the D-1 refactor.
- `D-3` **`CommitUnix` is `entry.CommittedAt` parsed to Unix seconds, with 0 as the fallback for an unparseable timestamp — documented as a floor by convention, not by construction.** Claude-agent review (Finding IMP-5) caught that the original comment claimed 0 was "the oldest possible value," which is false (a genuine pre-1970 timestamp parses negative); the comment now states the true behavior and documents that this ordering can diverge from `changeledger.GetFeatureHistory`'s own raw-string `CommittedAt` comparison for a timestamp carrying a non-UTC offset — a fact P-7 must not assume away.
- `D-4` **The three-level ordering (overlap, then recency, then hash) is a plain lexicographic comparator on `(-PathOverlap, -CommitUnix, CommitHash)`**, which is what makes it a valid strict weak ordering — Claude-agent review verified this rigorously by constructing an asymmetric variant (a `>`-guarded shortcut instead of the correct `!=`-guarded direction check) and confirming it violates strict weak ordering and passes the original (too-weak) determinism test; the review-fix pass strengthened that test specifically to catch this class of comparator bug (see D-5).
- `D-5` **`TestRankHistoryEntriesIsDeterministicAcrossRuns` now permutes a purpose-built fixture across 7 input orderings and asserts one pinned expected order, rather than re-running one fixed input order 20 times.** The original version could not distinguish a correct implementation from a deterministic-but-wrong one, since re-feeding identical input to a pure function trivially reproduces identical output regardless of comparator correctness (Claude-agent review Finding IMP-3). The new fixture includes an entry with equal path-overlap to its peers but an older commit time and an adversarially-chosen smaller hash, specifically to expose the asymmetric-comparator failure mode from D-4.
- `D-6` **`RetrievalLocus.Symbols` is verified unread by a test that actually exercises it, not merely a vacuous zero-overlap comparison.** The original `TestRankHistoryEntriesDoesNotUseSymbolsBeforeBug323` set `Symbols` non-empty while leaving `Paths` empty — both hit the same early-return regardless of whether `Symbols` was ever read, so a Paths-gated-Symbols-reading mutant would have passed it (Claude-agent review Finding IMP-4). The review-fix pass added a second case with both `Paths` and `Symbols` populated and colliding, which does discriminate.

### Constraints

- Zero wiring: no existing file outside `internal/featurecatalog` was touched; grep for `ScoreHistoryEntry|RankHistoryEntries|RankedHistoryEntry|HistoryRelevance` across `apps/local-runner` returns matches only in the two new files this task adds.
- `go build ./...`/`go vet ./...` clean across the whole module; no full `internal/runner` regression run performed (see Verification — this task touches nothing that suite exercises).
- Provider-agnostic (Case 1).

### Open Questions

- **Carried forward implicitly for CP-55 P-7:** `HistoryRankingConfig`, `SelectHistoryEntries`, `HistorySlotRanked`, and the `feature.history` source wiring are P-7's own scope per the coding plan's own phase split — not an omission here.
- **Carried forward from Claude-agent review (CA-429 Finding IMP-5):** P-7's `SelectHistoryEntries` must not assume `RankHistoryEntries`'s `CommitUnix`-based recency ordering agrees byte-for-byte with `changeledger.GetFeatureHistory`'s own raw-RFC3339-string ordering — they can diverge for a timestamp carrying a non-UTC offset. Whichever phase wires the two together should decide explicitly which ordering "current truth" means, rather than assume they coincide.

## 1. Goal

Add deterministic, testable path-locus-based ranking primitives for feature-history entries, so a later phase can select the most relevant history for a Flow's context instead of relying on recency alone — without touching anything that renders context today.

## 2. Parent Links

- coding plan: `CP-55` P-6

## 3. Trigger

CP-54/CP-55 established `changeledger.Entry.ChangedPaths` (Task-261) and `featurecatalog.RetrievalLocus` (Task-262) as the two halves of a locus-based relevance signal, but nothing yet computes a score or ranking from them — `feature.history` still renders purely by recency.

## 4. Exact Change

- `internal/featurecatalog/relevance.go` (**new**): `HistoryRelevance`, `RankedHistoryEntry`, `ScoreHistoryEntry`, `RankHistoryEntries`, plus unexported helpers `canonicalPathForOverlap`, `buildLocusPathSet`, `scoreAgainstLocusSet`.
- `internal/featurecatalog/relevance_test.go` (**new**): 16 tests — 9 matching the CP-55 P-6 test signatures (3 expanded during the review-fix pass) plus 4 added directly against Claude-agent review findings.

## 5. Touched Areas

- files: 2 new files, 0 modified files
- modules: `featurecatalog` (reads `changeledger.Entry`, no changes to `changeledger` itself)
- routes / tables: none

## 6. Acceptance Check (DoD)

- [x] `ScoreHistoryEntry` counts exact path overlap, rejecting prefix/substring near-misses — `TestScoreHistoryEntryCountsExactPathOverlap` (expanded post-review, Finding IMP-2).
- [x] Duplicate paths (either side) do not inflate the score — `TestScoreHistoryEntryDeduplicatesPaths` (expanded post-review, Finding MIN-4).
- [x] Nil `ChangedPaths` scores zero overlap but remains eligible — `TestScoreHistoryEntryReturnsZeroForNilChangedPaths`.
- [x] Empty locus scores zero overlap — `TestScoreHistoryEntryReturnsZeroForEmptyLocus`.
- [x] **Paths are canonicalized before comparison, not trusted as pre-normalized** (new invariant added post-review, Finding IMP-1) — `TestScoreHistoryEntryCanonicalizesPathsBeforeComparing`.
- [x] **An unparseable `CommittedAt` falls back to 0 without disturbing other signals, and a parseable one is pinned to an exact Unix-seconds value** (new coverage added post-review, Finding IMP-5) — `TestScoreHistoryEntryHandlesUnparseableCommittedAt`, `TestScoreHistoryEntryParsesCommittedAtAsUnixSeconds`.
- [x] Overlap dominates recency — `TestRankHistoryEntriesOverlapDominatesRecency`.
- [x] Recency breaks an equal-overlap tie — `TestRankHistoryEntriesRecencyBreaksEqualOverlap`.
- [x] Commit hash breaks a complete tie, in both input-order directions — `TestRankHistoryEntriesCommitHashBreaksCompleteTie` (strengthened post-review to check both orderings, folding in Finding MIN-7).
- [x] **Ranking is genuinely independent of input order, verified via permutation, not merely repeated identical runs** (corrected after Claude-agent review Finding IMP-3, which showed the original test could not distinguish correct from deterministic-but-wrong) — `TestRankHistoryEntriesIsDeterministicAcrossRuns`.
- [x] **`RetrievalLocus.Symbols` is never read, verified with a fixture that actually exercises it** (corrected after Claude-agent review Finding IMP-4, which showed the original fixture was vacuous) — `TestRankHistoryEntriesDoesNotUseSymbolsBeforeBug323`.
- [x] Empty input returns `nil`, matching `changeledger.GetFeatureHistory`'s own convention (new, Finding MIN-6) — `TestRankHistoryEntriesReturnsNilForEmptyInput`.
- [x] `gofmt -l`/`go build`/`go vet` clean on every touched/new file; `go vet ./...` clean module-wide except one pre-existing, unrelated warning.
- [x] Cross-provider Case 1 agnostic.
- [x] GitNexus impact check performed; not meaningfully applicable (brand-new symbols with zero callers by design) — disclosed rather than skipped silently.
- [x] **Claude-agent adversarial review performed and acted on**: 0 Critical + 5 Important + 7 Minor findings; all 5 Important findings fixed and re-verified via mutation testing; all 7 Minor findings addressed (fixed, documented, or explicitly folded into another fix) — none silently dropped.

## 7. Out of Scope

- `HistoryRankingConfig`, `DefaultHistoryRankingConfig`, `HistorySelection`, `SelectHistoryEntries`, `HistorySlotRanked` — all explicitly CP-55 P-7's own production changes and test signatures.
- Reconciling `RankHistoryEntries`'s `CommitUnix`-based ordering with `changeledger.GetFeatureHistory`'s raw-string ordering when they diverge (CA-429 Finding IMP-5) — recorded as an open question for whichever phase wires the two together.
- CP-55 P-7 through P-9.

## 8. Cross-Provider Note

Provider-agnostic (Case 1): `grep -inE 'providerKey|claude|codex|grok'` over `relevance.go` returns zero matches — the code reads only `changeledger.Entry`/`RetrievalLocus` fields, with no provider dimension.
