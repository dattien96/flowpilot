# Task-264: Frozen Preflight Contract Model And Storage (CP-55 P-2)

## Metadata

- Document ID: `Task-264`
- Title: `Frozen Preflight Contract Model And Storage`
- Phase: `task`
- Status: `done` (2026-07-31 — implemented and self-verified in the current uncommitted worktree; not yet through a Codex review pass, see [CA-425](../../../change-audit/CA-425-frozen-preflight-contract-model-and-storage.md))
- Owner: `FlowPilot Architecture`
- Reviewers: `Codex` (pending — no review pass has run on this slice yet)
- Created: `2026-07-31`
- Last Updated: `2026-07-31`
- Feature Keys: `change-contract`
- Parent Documents: [CP-55: Flow-First Preflight Contract, Context Retrieval, and Canonical Acceptance](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) (P-2), [SD-21: Change Contract and Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-14: Code Context and Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `none`
- Related Documents: [Task-263: Explicit Flow Writer Semantics And Safety Topology](./Task-263-Explicit-Flow-Writer-Semantics-And-Safety-Topology.md) (P-1, done — this task is the next Flow-lifecycle slice), [Task-262: Shared Retrieval-Locus Builder](./Task-262-Shared-Retrieval-Locus-Builder.md) (CP-54 P-2 — supplies `buildRetrievalLocus`/`isConcreteCodeTarget`, both touched here), [CA-424](../../../change-audit/CA-424-explicit-flow-writer-semantics-and-safety-topology.md)
- Replaces: `None`
- Tags: `change-contract, agent-flow-engine, preflight-contract, frozen-store, additive`

## AI Quick View

### Summary

CP-55 §3.3 needs a Flow preflight contract lifecycle that is strict, immutable, and versioned — distinct from the legacy per-turn `Contract`/`Store` in `contract.go` (Task-184), which stays a tolerant, post-hoc, declared-or-inferred capture read by chat-mode context injection and the r-contract/r-scope gate. This task adds that second, independent lifecycle without touching the first: `PreflightContractDraft` (what a read-only planner proposes), `FrozenContractRecord` (the immutable, versioned freeze of it, bound to run/coder-step/baseline), `ContractStatusEvent` (append-only lifecycle transitions), and the strict parser/normalizer/store operations around them.

It also relocates the single "is this a concrete code file" predicate CP-54's `buildRetrievalLocus` already had into `changecontract.IsConcreteCodeTarget`, so the frozen-contract path normalization and the retrieval-locus builder cannot silently drift onto two different definitions of "concrete."

### Current Ask

Implement P-2 of CP-55 exactly: `PreflightContractDraft`, strict parser (no trailing prose, no unknown fields), shared declared-path normalization, `FrozenContractRecord`/`ContractStatusEvent`, an immutable append-only `FrozenStore`, and deterministic contract-id computation. **No runtime writer is wired in this slice** — nothing calls `FreezeContract`/`FrozenStore` from the actual Flow dispatch path yet; that begins at P-3 (read-only contract planner, freeze node, chain advancement).

### Key Decisions

- `D-1` **A second, independent lifecycle, not an extension of `Contract`.** `changecontract.Contract` is GitNexus-confirmed HIGH risk (13 impacted symbols across chat-mode injection and the legacy gate) — CP-55 §3.2/§3.3 already calls for a frozen artifact with different invariants (immutable, versioned, strict) than `Contract` was ever designed to have (tolerant, mutable-by-overwrite, last-wins). Reusing it would mean either weakening its tolerant load behavior for every existing caller, or bolting strict-mode fields onto a struct whose whole contract is "callers should treat a Save error as degraded, never blocking." Two separate types with two separate stores (`frozen_contracts.ndjson`, `frozen_contract_events.ndjson`, both new files under the existing `.flowpilot/contracts/` directory) avoids both.
- `D-2` **Payload and status are separate append-only records.** `FrozenContractRecord` never mutates after `SaveFrozen` persists it — an amendment is a new record (`Version+1`, `Supersedes` set to the prior `ContractID`), not an edit. `ContractStatusEvent` accumulates separately (`frozen` implicit at freeze time → `accepted` | `superseded` | `abandoned`), so "what was frozen" and "what happened to it since" never share one mutable slot.
- `D-3` **Idempotent same-id-same-payload, rejected same-id-different-payload.** `SaveFrozen` compares the marshaled JSON bytes of an existing record against the incoming one for the same `ContractID`; byte-identical is a no-op (safe to retry after a crash/recovery), anything else is an error — a frozen record's payload is never silently overwritten.
- `D-4` **`ComputeContractID` folds in the baseline fingerprint, not just `BaseSHA`.** The frozen design explicitly wanted dirty-worktree evidence (`BaselineWorktree map[string]string`, repo-relative path → content/status fingerprint), not only a commit SHA, since a Flow can start preflight against an uncommitted tree. The id hashes `runID`, `coderStepID`, `version`, the sorted declared paths, feature/intent/source-doc, `BaseSHA`, and the sorted `baseline` entries — map iteration order is neutralized by sorting before joining, matching the existing `ComputeSignature` pattern in `signature.go`.
- `D-5` **Two rejection modes in `NormalizeDeclaredCodePaths`, deliberately not unified.** Workspace escape (absolute path outside workspace, `../` traversal, a symlink that resolves outside workspace when the target already exists) is a security boundary — any one offending entry fails the whole call. "Not a concrete code target" (glob, directory bucket, doc/audit file, flag-like string) is noise, not an attack — each such entry is silently dropped via the same `IsConcreteCodeTarget` predicate `buildRetrievalLocus` already applies; the call only errors if *nothing* concrete survives filtering (an all-noise scope, e.g. all globs or all docs, reports the same "no concrete code path" error the empty-input case gets).
- `D-6` **Path-keyed mutex serializes writes across `FrozenStore` instances in one process, not just within one instance.** A package-level `map[string]*sync.Mutex` keyed by the store's absolute `frozen_contracts.ndjson` path means two independently-constructed `*FrozenStore` values rooted at the same workspace share one lock. Multi-process locking (two OS processes writing concurrently) is explicitly out of scope — documented as a P-3/P-4 prerequisite the runner process boundary must decide, not copied from runner's own flock implementation into this package.
- `D-7` **Strict reload: a corrupt line fails `NewFrozenStore` outright.** The legacy `Store.loadFromDisk` tolerates a corrupt/partial NDJSON line (skip and continue) because Task-184's Contract capture is explicitly best-effort (SS-14 AC-9). A Flow preflight guarantee cannot make the same tradeoff — silently dropping a torn frozen-contract record would mean a Flow could proceed as if no contract had ever been frozen for that step. `NewFrozenStore` returns an error naming the exact corrupt line instead.
- `D-8` **`GetFrozenForStep` performs the active-version reduction itself.** It returns the highest-`Version` record for `(runID, coderStepID)` whose latest `ContractStatusEvent` (if any) is neither `superseded` nor `abandoned` — i.e., the version currently governing that coder step — rather than requiring every caller to walk `ListVersionsForStep` plus the status log by hand. `ListVersionsForStep` still exists separately for audit (every version, including superseded ones).
- `D-9` **`isConcreteCodeTarget` (runner) is now a one-line wrapper over `changecontract.IsConcreteCodeTarget`.** Moved, not duplicated — GitNexus confirms exactly one caller (`buildRetrievalLocus`, LOW risk), and the wrapper preserves byte-identical behavior (verified: every pre-existing `TestIsConcreteCodeTarget*`/`TestBuildRetrievalLocus*` test in `retrieval_locus_test.go` still passes unmodified, plus a new test asserting the two functions agree on every case).

### Constraints

- Do not weaken or restructure `changecontract.Contract`/`Store` — GitNexus impact is HIGH (13 impacted); zero edits made to `contract.go`, `parse.go`, `infer.go`, `scope.go`, `head.go`, `drift.go`, `retire.go`, `update.go`, `decisions.go`, `signature.go`, `backfill.go`, `pack.go` in this task.
- Additive tests only, in new files (`paths_test.go`, `preflight_test.go`); the one existing test file touched (`internal/runner/retrieval_locus_test.go`) only gained one new test function, no existing test edited.
- No runtime wiring — nothing in the Flow dispatch path calls `FreezeContract`/`FrozenStore` yet (P-3).
- Provider-agnostic (Case 1) — no `providerKey`/`claude`/`codex`/`grok` branch anywhere in the new code.

### Open Questions

- None blocking P-2's own exit condition. P-3 will need to decide: where the planner step's raw text reaches `ParsePreflightDraft`, what supplies `knownFeatureKeys` to `ValidatePreflightDraft` (likely `featurecatalog.Catalog.All()` keys, read by the runner caller — `changecontract` does not import `featurecatalog`, preserving the `runner → featurecatalog → changeledger` / `runner → changecontract` dependency directions), and how `BaselineWorktree` gets captured (git status/diff snapshot at preflight time — CP-55 §3.3 assigns that capture to the runner in P-3; this task only stores and validates whatever value it is given).

## 1. Goal

Give CP-55's Flow lifecycle an immutable, versioned, strict preflight-contract artifact it can freeze before context/coder dispatch — without touching the existing tolerant per-turn `Contract` capture chat-mode and the legacy gate already depend on.

## 2. Parent Links

- coding plan: `CP-55` P-2
- tech design: `SD-21` (Change Contract), `SD-17` (context retrieval consumer of `DeclaredPaths`-shaped data)
- system spec: `SS-14`

## 3. Trigger

CP-55 P-1 (Task-263) landed the two behavior ids (`agent.code`, `contract.freeze`) and the static topology validator the Flow preflight lifecycle needs structurally. P-2 gives `contract.freeze` an actual data model and store to freeze into — P-3 wires the real handler.

## 4. Exact Change

- `internal/changecontract/paths.go` (**new**): `IsConcreteCodeTarget(p string) bool` (relocated from `runner`, byte-identical predicate) and `NormalizeDeclaredCodePaths(workspace string, paths []string) ([]string, error)`.
- `internal/changecontract/preflight.go` (**new**): `PreflightContractDraft`, `FrozenContractRecord`, `ContractStatusEvent`, `ContractStatus*` consts, `ParsePreflightDraft`, `ValidatePreflightDraft`, `ComputeContractID`, `FreezeContract`, and `FrozenStore` (`NewFrozenStore`, `SaveFrozen`, `GetFrozenForStep`, `ListVersionsForStep`, `AppendStatus`).
- `internal/runner/retrieval_locus.go` (**modified, minimal**): `isConcreteCodeTarget` now delegates to `changecontract.IsConcreteCodeTarget`; unused `flowgate` import removed. No other line changed.
- Test additive only: `internal/changecontract/paths_test.go`, `internal/changecontract/preflight_test.go` (both new), one new test function (`TestRetrievalLocusUsesSharedPathNormalization`) appended to the existing `internal/runner/retrieval_locus_test.go`.

## 5. Touched Areas

- files: 2 new production files + 1 minimally-modified production file in `changecontract`/`runner`; 2 new test files + 1 test file with one appended test
- modules: `changecontract` (new lifecycle), `runner` (one function body relocated to a wrapper)
- routes / tables / stores: two new local NDJSON files under `.flowpilot/contracts/` (`frozen_contracts.ndjson`, `frozen_contract_events.ndjson`) — additive, never read by the existing `Store`/`contracts.ndjson` path

## 6. Acceptance Check (DoD)

- [x] Strict JSON draft accepted; unknown fields, trailing prose, malformed JSON, and blank input all rejected — `TestParsePreflightDraftAcceptsStrictJSON`, `...RejectsUnknownFields`, `...RejectsTrailingProse`, `...RejectsMalformedJSON`, `...RejectsEmptyInput`, `...AcceptsSurroundingWhitespace`.
- [x] Draft structural validation: blank/unknown feature_key, blank intent, no-concrete-path all rejected; known feature_key and empty-allowlist both accepted — `TestValidatePreflightDraftRequiresFeatureKey`, `...RejectsUnknownFeatureKey`, `...AcceptsKnownFeatureKey`, `...SkipsKnownFeatureKeyCheckWhenListEmpty`, `...RequiresIntent`, `...RequiresConcretePath`, `...RequiresConcretePathEmptyList`.
- [x] Path normalization: separator normalization + case preservation, sort/dedupe, outside-workspace (relative `../` and absolute-outside) rejection, absolute-inside-workspace acceptance, glob-only/doc-only/bucket-only scope rejection, mixed noise+concrete keeps the concrete entry, symlink escape rejected (Windows-skipped, privilege-gated), a not-yet-created path does not trip the symlink check, blank workspace still filters noise — the full `TestNormalizeDeclaredCodePaths*` suite in `paths_test.go` (13 tests).
- [x] `ComputeContractID` deterministic across map iteration order, changes with version, changes with baseline fingerprint — `TestComputeContractIDIsDeterministic`, `...ChangesWithVersion`, `...ChangesWithBaseline`.
- [x] `FreezeContract` normalizes paths before hashing and propagates a normalization failure (all-noise declared paths) as an error — `TestFreezeContractNormalizesPathsAndComputesID`, `...PropagatesNormalizationError`.
- [x] `SaveFrozen`: same-id-same-payload idempotent, same-id-different-payload rejected, higher-version amendment allowed — `TestSaveFrozenRejectsMutationOfExistingContractID`, `...AllowsHigherAmendmentVersion`.
- [x] `GetFrozenForStep` returns the latest non-superseded/non-abandoned version, and reports not-ok when every version is inactive — `TestGetFrozenForStepReturnsLatestActiveVersion`, `...ReturnsNotOkWhenEveryVersionInactive`.
- [x] `AppendStatus` rejects an unknown contract id — `TestAppendStatusRejectsUnknownContractID`.
- [x] A fresh `FrozenStore` instance reloads previously persisted contracts + status events — `TestFrozenStoreReloadsAcrossInstances`.
- [x] A corrupt trailing line in either NDJSON file fails `NewFrozenStore` outright (fail-closed, unlike the legacy `Store`) — `TestFrozenStoreCorruptContractsLineFailsClosed`, `...CorruptEventsLineFailsClosed`.
- [x] Concurrent `FrozenStore` instances in one process do not interleave/tear writes — `TestFrozenStoreConcurrentInstancesDoNotInterleave` (20 goroutines, each a fresh store instance, distinct contract ids; final reload sees exactly 20 records).
- [x] The legacy `Contract`/`Store` lifecycle is provably untouched: it loads its own pre-existing records unaffected by a `FrozenStore` rooted at the same workspace, and a `FrozenStore` cannot see legacy records — `TestContractStoreLoadsLegacyRecordsWithoutNewFields`.
- [x] `isConcreteCodeTarget`(runner) and `changecontract.IsConcreteCodeTarget` agree on every case — `TestRetrievalLocusUsesSharedPathNormalization`; every pre-existing `retrieval_locus_test.go` test still passes unmodified.
- [x] Golden context-package fixtures unchanged — `TestBuildFlowContextPackage*`/`TestRenderFlowContextPackage*` (11 top-level, 17 including subtests) all green.
- [x] `gofmt -l` clean on the exact 6 touched/new files (not a package-wide `gofmt -w`, which CA-424 documents as a prior incident that reformatted ~300 unrelated files).
- [x] `go build ./...` clean; `go vet ./...` shows only one pre-existing, unrelated issue in `internal/structure/gitnexus.go` (an undischarged `context.CancelFunc`) that predates this task and was not touched here.
- [x] Cross-provider Case 1 agnostic — `grep -inE 'providerKey|claude|codex|grok'` over both new production files → zero matches.
- [x] GitNexus impact confirmed LOW for the one edited existing symbol (`isConcreteCodeTarget`, 1 direct caller, `buildRetrievalLocus`) before editing; `changecontract.Contract`/`Store` (HIGH, 13 impacted per CA-424's prior query) were not touched, by design (`D-1`).

## 7. Out of Scope

- Wiring `FreezeContract`/`FrozenStore` into the actual Flow dispatch path, the read-only contract planner node, or `contract.freeze`'s handler (CP-55 P-3).
- Scope-drift gate comparison against the frozen contract, context production after freeze (P-4).
- Terminal-acceptance Canonical mutation (P-5), disabling `change.contract` rendering (P-6), deterministic history ranking wiring (P-7), and P-8/P-9.
- Multi-process write locking for `FrozenStore` (explicitly deferred; documented as a P-3/P-4 prerequisite).
- Populating `BaselineWorktree` from a real git snapshot (the runtime capture logic is P-3; this task only stores/validates whatever map it is given).

## 8. Completion Notes

Implemented in the current uncommitted worktree by Claude (this session), following the P-2 scope frozen in CP-55 §3.3/§3.4 plus the additional architecture-review decisions captured in this task's Key Decisions (`FrozenContractRecord`/`ContractStatusEvent` as separate append-only records, workspace-aware `NormalizeDeclaredCodePaths`, path-keyed cross-instance store locking). No Codex review pass has run on this slice yet — unlike Task-263/CA-424, which records seven Codex review passes, this task and [CA-425](../../../change-audit/CA-425-frozen-preflight-contract-model-and-storage.md) describe only the original implementation-and-self-verification pass. Nothing in this slice is committed to git.

## 9. Cross-Provider Note

Provider-agnostic (Case 1): both new files parse/validate/store a contract draft and never receive or branch on a `providerKey`. Confirmed by `grep -inE 'providerKey|ProviderKey|claude|codex|grok'` over `paths.go`/`preflight.go` → zero matches.
