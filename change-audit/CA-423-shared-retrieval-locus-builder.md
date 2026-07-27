# CA-423 — Shared Retrieval-Locus Builder (Task-262, CP-54 P-2)

## Scope

Implemented [Task-262](../requirements/08-Task/todo/Task-262-Shared-Retrieval-Locus-Builder.md) (P-2 of [CP-54](../requirements/07-Coding-Plan/todo/CP-54-Locus-Anchored-Context-Relevance.md)): one place that answers "what code region is this turn touching?", so history can be ranked against it. [Task-261](../requirements/08-Task/done/Task-261-Persist-Changed-Paths-In-Change-Ledger.md) recorded the other half ("what did each commit touch"); this is the query-time side.

Nothing consumes it yet — wiring is P-4 — so runtime behavior is byte-identical after this change.

## Changes

- `featurecatalog/locus.go` (new): `RetrievalLocus{Paths, Symbols, RunID}` + `IsEmpty()`.
- `runner/retrieval_locus.go` (new): `isConcreteCodeTarget` (the shared normalizer) and `buildRetrievalLocus(workspace, runID, prompt)`, merging three sources in descending confidence — the run's declared `change.contract`, the uncommitted diff, then paths named in the prompt.

## Design notes

- **Why the type lives in `featurecatalog` and the builder in `runner`.** CP-54 P-3 puts the scorer in `featurecatalog/relevance.go` taking a `RetrievalLocus`. Import direction is `runner → featurecatalog → changeledger`, verified: `featurecatalog` does not import `runner`. Had the type been declared in `runner` as CP-54 §7 sketched, P-3 would not compile. The builder stays in `runner` because it needs `flowgate.IsDocOrAuditFile` and the two Task-246 producers that already live there.
- **Reuse over reimplementation.** `uncommittedChangedPaths` and `extractPromptSourcePaths` (Task-246) already handle NUL-delimited git output, separator normalization and doc filtering. They are called, not copied, and not modified — `source.excerpt` depends on them.
- **The normalizer is the load-bearing part.** Real contracts are mostly *inferred*, and inference records top-level directory buckets (`"apps"`, `"internal"`) rather than files. Letting those through would match nearly every commit and recreate the exact dilution CP-54 exists to remove. So `isConcreteCodeTarget` drops directory buckets (no extension), globs (`*?[`), doc/audit files, and anything starting with `-` (so a downstream tool runner can never parse a target as a CLI flag). This is the same normalization Task-259 T-3 specifies, now written once.
- **`Symbols` is honestly documented as dead.** Nothing populates `Contract.DeclaredSymbols` — `ParseDeclaration` reads only `feature:`/`intent:`/`files:`, and inference sets paths only. The field and its dedupe path exist as the extension point, and both the type comment and the helper say plainly that it stays nil until BUG-323 `Q-2` decides where symbols come from. Recorded so no one later reads the field as working.
- **Determinism.** Paths are deduped then sorted; source order only decides which duplicate wins, since consumers treat the locus as a set. Two runs over the same state produce identical output (asserted) — the rule BUG-266 set for history ordering.
- **Empty is a signal, not a failure.** No contract, no git repo, blank workspace, or empty prompt all yield an empty locus, which is what tells P-4 to keep today's recency ordering. This is what makes the ranking a safe superset (CP-54 QĐ-4).

## Cross-provider (per `cross-provider-parity`, Case 1 — agnostic)

`grep -n 'providerKey|ProviderKey'` over both new files returns **no matches**. The builder reads git and the contract store; it takes no `providerKey` and never branches on one. No Claude/Codex/Grok matrix applies; evidence stated rather than assumed.

## GitNexus impact

Not applicable in the usual sense: this change adds two new files and **modifies no existing symbol**, so there is no blast radius to assess. (CLAUDE.md's rule is about editing existing functions.) The index was refreshed with `npx gitnexus analyze` after the previous commits.

## Verification

- `go build ./...` clean; `go vet ./internal/featurecatalog/... ./internal/runner/...` clean.
- 22 new tests, all passing, none skipped: locus semantics (zero value empty, paths or symbols make it non-empty, a run id alone does **not** — it cannot rank commits); normalizer (real files accepted incl. root `go.mod`; directory buckets, globs, docs/audit, and `-`-prefixed targets rejected); symbol dedupe (sort + dedupe, blank-only → nil); and builder behavior against real temp git repos (declared contract paths, inferred dir-buckets dropped, globs/docs/flags dropped, diff + prompt merged, dedupe across all three sources, sorted + deterministic across two runs, empty run id skips the contract, nothing-to-anchor-on → empty, outside a git repo the prompt still works, blank workspace safe).
- Full `go test ./internal/featurecatalog/` green. Runner context + golden battery green (`TestBuildFlowContextPackage|TestRenderFlowContextPackage|TestFlowContextPackage|TestContextSource|TestFeatureHistorySource|TestCanonicalHead|TestChangeContract|TestComposeFeatureBlocks|TestResolveEnabledContext|TestExtractPromptSourcePaths|TestUncommittedChangedPaths`), confirming the golden fixture output is unchanged.
- **Zero edits to existing files.** `git status --short` shows only new files — no pre-existing source or test was touched (`additive-tests-only`). One collision was handled by renaming *my* helper (`containsString` → `locusHasPath`) rather than touching the existing declaration.

## Prior CA claims left intact

Task-246's producers and Task-247's contract-store read pattern are reused unchanged. Task-261's `ChangedPaths` is the intended match target for P-3 but is not read yet. No context source, slot, priority or ordering was altered.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-262
change_type: feature
summary: add the shared RetrievalLocus type plus the builder that unifies a run's declared change contract, the uncommitted diff and prompt-named paths into one deterministic set of concrete code files, with the shared normalizer that drops directory buckets, globs, docs and flag-like targets; nothing consumes it yet so runtime output is unchanged, and Symbols is documented as inert until BUG-323 Q-2 supplies a symbol source
# --->8---
