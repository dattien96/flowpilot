# CA-422 — Persist `ChangedPaths` In The Change Ledger (Task-261, CP-54 P-1)

## Scope

Implemented [Task-261](../requirements/08-Task/todo/Task-261-Persist-Changed-Paths-In-Change-Ledger.md) (P-1 of [CP-54](../requirements/07-Coding-Plan/todo/CP-54-Locus-Anchored-Context-Relevance.md)): record which files each commit touched directly in `changeledger.Entry`, so the retrieval layer can rank feature history by **code-locus overlap** instead of by `feature_key` + recency alone. This is the enabler CP-54 P-2…P-5 build on; without it there is nothing to match a turn's locus against.

## Changes

- `changeledger/ledger.go`: new `Entry.ChangedPaths []string` (`json:"changed_paths,omitempty"`) — repo-relative, forward-slash, sorted. Nil on entries written before this landed; readers treat nil as "no locus signal" and fall back to recency, so nothing about today's ordering changes until a consumer opts in.
- `changeledger/changed_paths.go` (new): `changedPathsByCommit(repoDir, cursor)` runs **one** `git log --no-merges --name-only --pretty=format:"\x1e%H"` over the same range `ParseRepo` already walks and returns `hash → paths`. `parseChangedPathsChunk` splits on `0x1e` (placed *before* the hash so each chunk is exactly one commit) and sorts the file list. `attachChangedPaths` fills matching entries. `(*Ledger).backfillChangedPaths` fills pre-existing entries and `Compact()`s when it changed anything.
- `changeledger/parse.go`: `ParseRepo` attaches the paths after assigning `OrderIndex`. `gitLogFormat` and `parseRecord` are **untouched**.
- `changeledger/ledger.go`: `Build` now calls `backfillChangedPaths` on both branches — including the "no new commits" early return, which is precisely the case that matters on an existing repo.

## Design notes

- **The doc's proposed approach was wrong and was not followed.** CP-54 §7 P-1 said to reuse the `git show --name-only` call that `enrich.go`'s `pathFeatureKey` already makes. That call only happens at **Priority 3** of `enrichEntry`, and the majority of entries return earlier at Priority 0/1 and never reach it — so filling the field there would have meant **one subprocess per commit**. A separate single `git log` pass costs **one subprocess for the whole build**, independent of history size. Recorded as `T-1` in Task-261.
- **Why not fold `--name-only` into the existing `git log`:** it interleaves the file list with the `0x1e`-delimited records and changes what `parseRecord` receives, which would have forced edits to the pre-existing parse tests. A second pass keeps the legacy suite untouched (`additive-tests-only`).
- **Backfill is not optional.** Every real ledger predates this field; without backfill the feature would only cover commits made from now on and CP-54 would have nothing to score. Known residual: an entry whose commit is unreachable from HEAD (rebased away) or that touched no files can never be filled, so the single `git log` repeats per engine init — bounded, non-fatal, noted in the code.
- **Determinism (BUG-266):** paths are sorted, never left in git's emission order; two runs over the same repo produce byte-identical results (asserted).

## Cross-provider (per `cross-provider-parity`, Case 1 — agnostic)

`internal/changeledger/` is **provider-agnostic**: `grep -n 'providerKey|ProviderKey|Provider'` over the package returns **no matches**. It reads git and change-audit only, takes no `providerKey`, and never branches on one. No Claude/Codex/Grok matrix is required for this slice; evidence stated here rather than assumed.

## GitNexus impact (run before editing, per CLAUDE.md)

`npx gitnexus impact <sym> --repo flowpilot` (index up-to-date @ `59451ff`):

| Symbol | Risk | Impacted | Processes | Direct caller |
|---|---|---|---|---|
| `ParseRepo` | **LOW** | 3 | 0 | `Build` → `runEngineInit` (indirect) |
| `EnrichAll` | **LOW** | 3 | 0 | `Build` → `runEngineInit` (indirect) |

Note: the MCP GitNexus tools are unavailable in this session, so the CLI was used — with `--repo flowpilot` and **without** `--json`, which is the correct invocation. The production `structure` package uses the wrong one; see **BUG-323** below.

## Verification

- `go build ./...` clean; `go vet ./internal/changeledger/...` clean.
- `go test ./internal/changeledger/... -count=1` — **42 passed**, including 15 new cases in `changed_paths_test.go`, none skipped (git present).
- New coverage matrix: chunk parse (hash+files, sort-normalization, empty chunk, commit with no files), attach (partial match, empty map is a no-op), backward compat (legacy NDJSON line → nil; `omitempty` drops the key), git-backed (per-commit file lists, multi-file sorting, determinism across two runs, not-a-repo, cursor not an ancestor), `ParseRepo` integration, and `Build` backfill of a legacy entry + skip-when-nothing-missing.
- Downstream: `go test ./internal/runner/ -run 'TestBuildFlowContextPackage|TestFeatureHistorySource|TestComposeFeatureBlocks|TestCanonicalHead|TestChangeContract' -count=1` — ok.
- **Pre-existing tests: zero edits.** `git status --short` shows only `ledger.go` + `parse.go` modified and two new files added; no test file was touched (`additive-tests-only`, `oracle-rule`).
- Real-repo format evidence: `git log --no-merges --name-only --pretty=format:$'\x1e%H' -3` on flowpilot emits exactly the chunk shape `parseChangedPathsChunk` consumes.

## Prior CA claims left intact

Builds on `CA-293`/`CA-294`/`CA-295`/`CA-296`/`CA-297` (CP-43 P-1…P-5) without altering any of them: no change to contract capture, scope-drift rules, canonical head, decision records, or head-first packing. `feature_key` assignment, `ResolveFeature`, its threshold, and `GetFeatureHistory`'s ordering/tie-break are all untouched.

## Related finding — BUG-323 (opened, not fixed here)

While verifying the GitNexus CLI contract that Task-259 depends on, found `structure.gitNexusProvider.Dependents` is **broken in production**: it runs `impact <target> --json` (flag does not exist → non-zero exit → error swallowed into an empty summary), omits the now-required `--repo`, expects a JSON schema (`{dependents,nearest,flows}`) the tool never emits (so it would fail *silently* even once the flag is fixed), and passes file paths that the CLI rejects outright — it resolves symbols only. Consequence: `HighSeverity` can never be true, so Task-185's `r-scope` never escalates to block, and Task-259 is unimplementable as specified (its `DeclaredSymbols` input is never populated either). Filed as [BUG-323](../requirements/09-BugFix/todo/BUG-323-GitNexus-Structure-Provider-Always-Returns-Empty.md); Task-259 moved to `blocked`. **CP-54 P-1 (this change) is unaffected** — it is pure git, no GitNexus.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-261
change_type: feature
summary: change ledger entries now record the repo-relative sorted file list each commit touched (ChangedPaths), filled by a single extra git log pass over the range ParseRepo already walks plus a one-time backfill of pre-existing entries, so retrieval can rank feature history by code-locus overlap instead of feature_key plus recency; backward-compatible (legacy entries load as nil and degrade to today's recency behavior) and provider-agnostic
# --->8---
