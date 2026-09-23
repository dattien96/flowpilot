---
id: CA-924
title: Context engine fixes — contract-scoped mid-flow produce, bare declared paths, glob-amplified catalog noise, GitNexus repo-name resolution + surfaced warnings, contextProfile wiring (BUG-417, 418, 419, 420, 421)
type: BugFix
feature: agent-flow-engine
date: 2026-09-23
status: done
---

## Context

The CP-66/70 live-verification wave showed the runtime context-package paths
diverged from the contract model in five ways:

- An auto-cataloged mega-glob feature out-scored real registered features in
  `ResolveFeature` — every prompt token matched hundreds of ledger-derived
  globs, and r-fk suggestions emitted unregistered keys (`claude`, `calc`).
- Mid-flow `context.produce` (task-harness scout → context → plan_writer)
  re-resolved the feature from the planner's `resultMessage` prose instead of
  trusting the run's declared/frozen contract — so the produced package's
  feature key was whatever lexical noise won that day, not the declared
  `calc-core`. The same path also accepted malformed legacy contract rows
  where prose had been concatenated into a filename.
- Contract `declared_paths` that were bare root-level filenames (`calc.go`)
  were dropped from `ExplicitSourcePaths` by the prose-only extractor, so the
  `source.excerpt` section never carried the file the contract declared.
- `source.dependence` resolved the GitNexus repo name from the *current
  directory basename*; on beds cloned/renamed after indexing (dir
  `lt-cpNN` indexed as `gate-sandbox`) every `gitnexus impact` call returned
  `Repository "<dir>" not found`, and per-target failures were swallowed by
  `continue` — the whole section vanished with zero observable signal.
- `contextProfiles.*.candidateSources` was only consulted on the inline-entry
  chain (`startInlineEntryChain`); the mid-flow produce node and the
  freeze-chain package builds used the producing node's own source list, so
  `knowledge.flow` never reached plan_writer/coder packages even when the
  consumer's profile declared it.

## Changes

### BUG-417 — cap glob-substring amplification + registered-only r-fk suggestions

`internal/featurecatalog/resolve.go`: glob-derived substring hits now
contribute a bounded score per query token instead of scaling with glob-set
size — a 200-entry auto-derived glob list can no longer out-score a
registered feature on volume alone. `internal/runner/gate_hook.go`:
`suggestFeatureKeys` filters candidate keys through `loadKnownFeatureKeys`
(`change-audit/FEATURE-KEYS.md`) so r-fk reprompts only ever offer registered
keys.
Tests: `internal/featurecatalog/bug417_resolve_test.go`,
`internal/runner/bug_cluster_i_context_test.go`
(`TestBug417_RFKSuggestionsOnlyRegisteredKeys`).

### BUG-418 — mid-flow produce trusts the contract, and malformed rows are sanitized

`internal/runner/flow_validate_audit_dispatch.go` (`runContextProduceNode`):
loads `latestContractForRun` (legacy `contracts.ndjson` → frozen store — the
same unified seam the context sources use) and seeds
`FlowContextHints.ResolvedFeatureKey = contract.FeatureKey`, bypassing prose
re-resolution exactly like the freeze-chain path already did.
`internal/changecontract/parse.go`: `splitAndTrim` rejects declaration rows
that carry prose-concatenated values (whitespace-joined filename+text), so a
malformed legacy row can no longer smuggle junk paths into the contract.
Tests: `internal/changecontract/bug418_parse_test.go`,
`TestBug418_ProduceUsesContractFeatureKey`.

### BUG-419 — bare declared filenames reach source.excerpt

`internal/runner/flow_context_hint_paths.go`: new `mergeDeclaredSourcePaths`
merges `contract.DeclaredPaths` (verbatim, bare filenames included) with the
bounded `extractPromptSourcePaths` prose parse; the contract list wins and
prose hits are additive only. `runContextProduceNode` uses it, matching the
freeze chain which already seeded `DeclaredPaths` directly.
Tests: `TestBug419_BareDeclaredFilenameProducesExcerpt`,
`TestBug419_MergedDeclaredPathsRetainBareFilename`.

### BUG-420 — index-derived repo name + observable degradation

`internal/structure/gitnexus.go`: `repoNameFromDir` reads
`<dir>/.gitnexus/meta.json` `repoPath` first — a cloned/renamed bed resolves
the name the index was registered under — and falls back to the directory
basename when metadata is absent/corrupt. `internal/runner/
context_source_dependence.go` (`renderDependenceBody`): per-target
`Dependents` errors are collected into `section.Warnings`
(`gitnexus impact failed for N/M target(s): <err>`) instead of being
swallowed — the warnings merge into `FlowContextPackage.Warnings`, so a full
name-mismatch run degrades visibly rather than returning an empty section.
`gitNexusProvider.Available()` is unchanged: pre-existing tests
(`structure_test.go`) pin `Available()==true` unconditionally, and the
oracle rule forbids weakening them; observability is delivered through
warnings, which carry the real error text (stronger signal than the generic
guidance the bug report asked for).
Tests: `internal/structure/gitnexus_bug420_test.go`
(meta.json resolution, basename fallback, warning propagation),
`TestBug420_*` in the runner file.

### BUG-421 — contextProfiles wired into runtime produce paths

`internal/runner/context_profile.go` + `context_sources_builtin.go`: new
`producedContextSourceIDs(flowDef, produceNode, consumerNode)` resolves the
source set for the node that will *consume* the package — artifact binding →
consumer profile `candidateSources` → node `ContextSources` → defaults —
mirroring the precedence `resolveEnabledContextSourceIDs` already
implemented for the entry chain. `runContextProduceNode` resolves its
forward target first and builds the package with the consumer's sources;
`advanceFlowThroughFreezeChain`'s `buildAndStorePackage` calls do the same
for each hop's consumer (writer/first-path-node).
Tests: `TestBug421_ProduceResolvesConsumerProfileSources`,
`TestBug421_FreezeChainPackageUsesConsumerProfile`.

## Safe-fix-contract compliance

- Reproduce-first: all 10 Cluster I tests failed by assertion on the pre-fix
  tree (catalog noise win, prose re-resolution, dropped bare path, silent
  dependence, missing profile sources) before any production change.
- Additive tests only; no existing test touched. `Available()` semantics
  were left byte-compatible specifically because `structure_test.go` pins
  them — the conflict was resolved by delivering the report's observable
  requirement (error text in warnings) through a different channel.
- Provider-agnostic: all touched code runs in-process in the runner before
  any provider call; the produce path emits the same package regardless of
  provider. Live evidence was collected on the devin provider.

## Verification

- `go test -count=1 ./internal/featurecatalog/ ./internal/changecontract/
  ./internal/structure/ ./internal/runner/ -run '<Cluster I tests>'` — all
  green post-fix.
- Full touched-package suites: `featurecatalog`, `changecontract` clean;
  `structure` has two pre-existing env failures identical on baseline
  (`TestRepoNameFromDirUsesBasename` — Windows path on POSIX;
  `TestGitNexusDependentsSmokeScopeDiff` — `Repository "fp-baseline" not
  found`, the checkout dir is not registered in the local index);
  `runner` has 23 failures, all identical on the baseline worktree
  (`d191004f`) or flaky-pass on isolated re-run
  (`TestPlanApprovalPark_ApproveAdvancesToFreeze`,
  `TestRun63960FullMatrix_EscalateParkThenRestart`,
  `TestFinalizerHookSurfacesArtifacts` — the last fails deterministically
  on baseline in isolation; racy wait predicate accepts the seeded fake
  `diff_snapshot` artifact before the real finalizer artifact lands).
  Zero Cluster I regressions.
- `gofmt` clean on all touched files; `go vet` clean on all four packages.
- Live (`/tmp/fp-live-i`, serve on `runner-ws` symlink → `ws`, devin/swe-2,
  task-harness):
  - BUG-417/418: three runs show the feature-resolution spectrum in the
    produced package's `## Context - feature:` line — `unresolved` (no
    cwd), `mega-noise` (contract absent — the bug), then `calc-core` once
    the declared contract row existed. The plan_writer prompt embeds
    `### change.contract` with `Scope: calc.go`, `Confidence: declared`.
  - BUG-419: `source.excerpt` section carries the bare-filename `calc.go`
    excerpt (full function body) into the plan_writer prompt.
  - BUG-420: from the renamed bed, `npx gitnexus impact Percent --repo
    runner-ws` reproduces `Repository "runner-ws" not found` (the reported
    silent-empty cause); `--repo ws` (the name `meta.json repoPath`
    resolves to) reaches the index (`Target 'Percent' not found` — repo
    resolved, target miss). Per-target errors now surface in
    `section.Warnings` (unit-covered).
  - BUG-421: the produced package's section set is exactly the
    `plan_writer` profile's `candidateSources`
    (`conventions, knowledge.flow, canonical.head, feature.history,
    change.contract, source.excerpt`) — profile wiring reached the mid-flow
    produce path.

## Known caveats

- A run created via raw `POST /client/workflow-runs` without `cwd` produces
  an empty-workspace package (`feature: unresolved`, relative-path catalog
  warning). That matches every other cwd consumer — the desktop client
  always sends `cwd`; not a new gap.
- Prompt-declared `[Change Contract]` blocks persist at the parent's
  post-turn gate; for task-harness the pre-freeze `context` node runs before
  the parent turn finalizes, so a prompt-block contract can arrive after the
  mid-flow produce. Frozen contracts (the canonical flow path) and directly
  stored rows are unaffected; if prompt-block contracts must reach pre-freeze
  produces, that's a separate seam worth its own bug.
- `source.dependence` legitimately does not appear for nodes whose profile
  omits it (plan_writer); it fires on the coder profile via the freeze chain.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-421
change_type: bugfix
summary: Cluster I context-engine fixes — contract-scoped mid-flow produce + bare declared paths, glob-amplification cap + registered-only r-fk suggestions, GitNexus meta.json repo-name + dependence warnings, consumer-profile source wiring on produce/freeze paths
# --->8---
