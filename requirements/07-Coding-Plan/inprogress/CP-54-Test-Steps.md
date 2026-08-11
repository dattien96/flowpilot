# CP-54 — Step Test Guide (Locus-Anchored Relevance Retrieval)

## Metadata

- Document ID: `CP-54-TEST-STEPS`
- Title: `CP-54 Verification Steps By Phase`
- Phase: `verification`
- Status: `active`
- Owner: `FlowPilot`
- Created: `2026-08-11`
- Parent Documents: [CP-54: Locus-Anchored Context Relevance](./CP-54-Locus-Anchored-Context-Relevance.md), [CP-43-Test-Steps](./CP-43-Test-Steps.md)
- Related Documents: [Task-261](../done/Task-261-Persist-Changed-Paths-In-Change-Ledger.md) (P-1), [Task-262](../done/Task-262-Shared-Retrieval-Locus-Builder.md) (P-2), [Task-268](../done/Task-268-Deterministic-History-Relevance-Scorer.md) (P-3 via CP-55), [Task-269](../done/Task-269-Wire-Ranking-Into-Feature-History.md) (P-4 via CP-55), [CP-55](../done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [BUG-323](../../09-BugFix/done/BUG-323-GitNexus-Structure-Provider-Always-Returns-Empty.md), [CA-436](../../change-audit/CA-436-cp54-p6-symbol-overlap-and-rank-telemetry.md)
- Tags: `context-regression-engine, locus, relevance-ranking, verification`

## AI Quick View

- **What:** Checklist test từng phase CP-54 — enabler ledger/locus (P-1/P-2) + ranking path+symbol (P-3/P-4 ship qua CP-55) + P-6 symbol tier + rank telemetry.
- **Why:** Xác nhận retrieval đảo trục sang locus hoạt động và superset an toàn (locus rỗng → recency cũ).
- **Working dir:** `cd apps/local-runner` cho mọi lệnh `go test`.

## Map phase → trạng thái

| Phase | Nội dung | Task | Trạng thái | Verify ở |
|-------|----------|------|------------|----------|
| **P-1** | Ghi `ChangedPaths` vào ledger | Task-261 / CA-422 | ✅ done | Phần 1 |
| **P-2** | `RetrievalLocus` builder + `IsConcreteCodeTarget` | Task-262 / CA-423 | ✅ done | Phần 2 |
| **P-3** | Scorer deterministic | Task-268 / CA-429 | ✅ done (CP-55 P-6) | Phần 3 |
| **P-4** | Rank/select `feature.history` có điều kiện | Task-269 / CA-430 | ✅ done (CP-55 P-7) | Phần 4 |
| **P-5** | `chat.summary` ranking | Q-5 | ✅ chốt **giữ recency** (không đổi) | Phần 5 |
| **P-6** | Symbol tier + telemetry | — | ✅ **code done** (2026-08-11 — [CA-436](../../change-audit/CA-436-cp54-p6-symbol-overlap-and-rank-telemetry.md)) | Phần 6 |

**Ghi chú kiến trúc:** type `RetrievalLocus` ở `internal/featurecatalog`; builder `buildRetrievalLocus` ở `internal/runner`; scorer ở `internal/featurecatalog/relevance.go`.

---

## Phần 1 — ChangedPaths trong ledger (P-1)

**Mục tiêu:** Mỗi commit entry có `changed_paths` (forward-slash, repo-relative); entry cũ thiếu field → `nil` → degrade recency, không lỗi.

### Automated

```bash
go test ./internal/changeledger/... -count=1 -run 'TestParseChangedPaths|TestAttachChangedPaths|TestEntryWithoutChangedPaths|TestChangedPathsByCommit|TestParseRepoAttaches|TestBuildBackfills' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 1.1 | `TestParseChangedPathsChunkReadsHashThenFiles` | Parse `git show --name-only` đúng |
| 1.2 | `TestEntryWithoutChangedPathsFieldLoadsAsNil` | Legacy NDJSON load OK |
| 1.3 | `TestParseRepoAttachesChangedPaths` | Enrich gắn paths khi build ledger |
| 1.4 | `TestBuildBackfillsChangedPathsOnLegacyEntries` | Backfill entry thiếu field |
| 1.5 | `TestChangedPathsByCommitNonFatalWithoutGitRepo` | Không git → non-fatal |

### Manual

| Step | Hành động | Pass khi |
|------|-----------|----------|
| 1.M1 | Mở `.flowpilot/ledger/feature_history.ndjson` sau commit mới | Entry mới có `changed_paths: ["path/to/file.go", ...]` |
| 1.M2 | Entry cũ (trước P-1) | Không có key hoặc `null` — ranking vẫn chạy recency cho entry đó |

---

## Phần 2 — RetrievalLocus builder (P-2)

**Mục tiêu:** Một hàm hợp nhất locus từ contract + uncommitted diff + prompt paths; loại glob/bucket/doc; rỗng → degrade.

### Automated

```bash
go test ./internal/runner/ -count=1 -run 'TestBuildRetrievalLocus|TestIsConcreteCodeTarget|TestRetrievalLocusUsesSharedPathNormalization' -v
go test ./internal/featurecatalog/... -count=1 -run 'TestRetrievalLocus' -v
go test ./internal/changecontract/... -count=1 -run 'TestNormalizeDeclaredCodePaths|TestIsConcreteCodeTarget' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 2.1 | `TestBuildRetrievalLocusUsesDeclaredContractPaths` | Contract declared → paths trong locus |
| 2.2 | `TestBuildRetrievalLocusDropsInferredDirectoryBuckets` | `"apps"` bucket bị loại |
| 2.3 | `TestBuildRetrievalLocusMergesDiffAndPromptPaths` | Diff + prompt merge |
| 2.4 | `TestBuildRetrievalLocusUsesExplicitPathsForFreshlyFrozenContract` | Frozen paths (CP-55) ưu tiên cao nhất |
| 2.5 | `TestBuildRetrievalLocusEmptyWhenNothingToAnchorOn` | Locus rỗng an toàn |
| 2.6 | `TestRetrievalLocusUsesSharedPathNormalization` | `changecontract.IsConcreteCodeTarget` = runner predicate |

### Manual

| Step | Hành động | Pass khi |
|------|-----------|----------|
| 2.M1 | Flow có frozen contract paths | Locus = declared paths (+ diff nếu có) |
| 2.M2 | Normal chat, không contract | Locus từ diff + prompt only |
| 2.M3 | Không diff, không path trong prompt | Locus rỗng → ranking tắt |

---

## Phần 3 — Relevance scorer (P-3, ship CP-55 P-6)

**Mục tiêu:** `ScoreHistoryEntry` / `RankHistoryEntries` — path-overlap trội recency; tie-break `CommitHash`; deterministic.

### Automated

```bash
go test ./internal/featurecatalog/... -count=1 -run 'TestScoreHistoryEntry|TestRankHistoryEntries' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 3.1 | `TestScoreHistoryEntryReturnsZeroForNilChangedPaths` | Entry cũ không paths → score 0 |
| 3.2 | `TestRankHistoryEntriesOverlapDominatesRecency` | Overlap cao thắng commit mới hơn nhưng xa locus |
| 3.3 | `TestRankHistoryEntriesRecencyBreaksEqualOverlap` | Cùng overlap → mới hơn thắng |
| 3.4 | `TestRankHistoryEntriesCommitHashBreaksCompleteTie` | Tie-break ổn định |
| 3.5 | `TestRankHistoryEntriesIsDeterministicAcrossRuns` | 2 lần chạy → giống hệt |
| 3.6 | `TestRankHistoryEntriesSymbolOverlapPreservesRecencyWhenEqual` | Symbol tier không phá path-only tie-break |

---

## Phần 4 — Wire `feature.history` ranked (P-4, ship CP-55 P-7)

**Mục tiêu:** Chỉ rank khi `len(candidates) > threshold` (~30) **và** locus ≠ rỗng; ngược lại byte-compatible recency.

### Automated

```bash
go test ./internal/featurecatalog/... -count=1 -run 'TestSelectHistoryEntries|TestHistorySlotRanked' -v
go test ./internal/runner/ -count=1 -run 'TestFeatureHistorySourceBuildsLocus|TestFeatureHistoryRanksWhenChangeContractRenderingIsDisabled|TestFirstCoderContextRanksFeatureHistoryByCurrentLocus' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 4.1 | `TestSelectHistoryEntriesFallsBackWhenLocusEmpty` | Locus rỗng → recency |
| 4.2 | `TestSelectHistoryEntriesRanksAboveThreshold` | M>30 + locus → ranked top-N |
| 4.3 | `TestHistorySlotRankedPreservesLegacyBytesOnFallback` | Feature nhỏ → output cũ |
| 4.4 | `TestHistorySlotRankedAlwaysShowsCurrentTruthExcerpt` | Newest vẫn có marker "current truth" |
| 4.5 | `TestFeatureHistorySourceBuildsLocusFromFrozenContract` | Flow: locus từ frozen contract |
| 4.6 | `TestFirstCoderContextRanksFeatureHistoryByCurrentLocus` | E2E Flow context package ranked |

### Regression (bắt buộc xanh, không sửa golden)

```bash
go test ./internal/runner/ -count=1 -run 'TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor|TestRenderFlowContextPackageStableSections' -v
```

### Manual

| Step | Hành động | Pass khi |
|------|-----------|----------|
| 4.M1 | Feature BIG (`agent-flow-engine`), locus = vùng dispatch | "Prior work" ưu tiên commit chạm cùng file, không phải 15 commit mới nhất bất kỳ |
| 4.M2 | Feature nhỏ (<30 entries) | Prompt history **không đổi** vs trước CP-54 |
| 4.M3 | Tắt `change.contract` trong flow sources | Ranking vẫn chạy từ locus khác (frozen explicit paths / diff) |

---

## Phần 5 — `chat.summary` (P-5, Q-5)

**Mục tiêu:** v1 **không** locus-rank chat — giữ recency (Phương án A).

### Automated

```bash
go test ./internal/runner/ -count=1 -run 'TestBuildFlowContextPackageIncludesChatSummary' -v
```

| Step | Pass khi |
|------|----------|
| 5.1 | `chat.summary` slot vẫn recency-based; không có test ranked chat mới |
| 5.2 | Doc CP-54 ghi rõ lý do (số summary/run nhỏ, anchor yếu) |

**Không cần manual riêng** trừ spot-check prompt vẫn có chat summary như cũ.

---

## Phần 6 — Symbol tier + rank telemetry (P-6) ✅ code done

**Mục tiêu:** `SymbolOverlap` trong scorer (basename heuristic, cùng `GitNexusImpactTargets`); locus symbols từ contract paths + `symbols:`; log `[context-rank]` auditable.

### Automated

```bash
go test ./internal/featurecatalog/... -count=1 -run 'TestScoreHistoryEntrySymbol|TestRankHistoryEntriesSymbol|TestRankHistoryEntriesSymbolOverlap' -v
go test ./internal/runner/ -count=1 -run 'TestBuildRetrievalLocusPopulatesSymbols|TestBuildRetrievalLocusMerges|TestBuildRetrievalLocusDerives' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 6.1 | `TestScoreHistoryEntrySymbolOverlapFromDerivedBasename` | `gate_hook.go` → `GateHook` match |
| 6.2 | `TestRankHistoryEntriesSymbolOverlapDominatesRecency` | Symbol overlap thắng recency khi path overlap = 0 |
| 6.3 | `TestRankHistoryEntriesPathOverlapStillBeatsSymbolOverlap` | Path vẫn ưu tiên hơn symbol |
| 6.4 | `TestBuildRetrievalLocusPopulatesSymbolsFromDeclaredPaths` | Locus symbols từ declared paths |
| 6.5 | `TestBuildRetrievalLocusMergesDeclaredSymbolsAndPathDerived` | Merge `symbols:` + derived |
| 6.6 | `TestRankHistoryEntriesSymbolOverlapPreservesRecencyWhenEqual` | Equal symbol → recency tie-break |

### Manual

| Step | Hành động | Pass khi |
|------|-----------|----------|
| 6.M1 | Feature BIG, contract `symbols:` hoặc path-derived symbol | Commit cùng symbol lên điểm dù khác file |
| 6.M2 | Runner log khi ranking kích hoạt | Dòng `[context-rank]` có `path_overlap` / `symbol_overlap` / `selected=` |
| 6.M3 | `RetrievalLocus.Symbols` populated | Frozen/declared contract → symbols trong locus |

**Giới hạn v1:** symbol-overlap = heuristic basename (không gọi GitNexus lúc rank); blast-radius GitNexus vẫn ở `source.dependence` / `r-scope`.

---

## Full-loop E2E (CP-54 path-only v1)

| # | Bước | Phase |
|---|------|-------|
| 1 | Ledger có `changed_paths` cho commit feature BIG | P-1 |
| 2 | Start Flow / chat với contract hoặc diff rõ vùng X | P-2 |
| 3 | Context package `feature.history` — commit vùng X lên top | P-4 |
| 4 | Đổi locus sang vùng Y (contract amend / prompt khác) | P-4 |
| 5 | Feature nhỏ — prompt không đổi | P-4 fallback |
| 6 | `chat.summary` vẫn recency | P-5 |

### One-shot bundle

```bash
go test ./internal/changeledger/... ./internal/featurecatalog/... -count=1 -timeout 5m
go test ./internal/runner/ -count=1 -timeout 8m \
  -run 'TestBuildRetrievalLocus|TestSelectHistoryEntries|TestHistorySlotRanked|TestFeatureHistorySource|TestFirstCoderContextRanks|TestBuildFlowContextPackageOutputUnchanged'
```

---

## Checklist tổng (operator)

| Phần | Automated | Manual | Ghi chú |
|------|-----------|--------|---------|
| P-1 ChangedPaths | ☐ | ☐ | Done |
| P-2 Locus builder | ☐ | ☐ | Done |
| P-3 Scorer | ☐ | — | Done via CP-55 |
| P-4 Ranked history | ☐ | ☐ 4.M1–M3 | Done via CP-55 P-7/P-8 |
| P-5 chat.summary | ☐ | ☐ spot | No change by design |
| P-6 Symbol tier | ☐ | ☐ 6.M1–M3 | **Code done** — manual tick in doc |

**CP-54 code complete (2026-08-11).** Verification-complete = P-1→P-6 bundle xanh + manual 4.M1–M2 + 6.M1–M2 tick trong doc.
