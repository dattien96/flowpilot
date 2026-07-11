# Task-267: Move Canonical Mutation To Terminal Flow Acceptance (CP-55 P-5)

## Metadata

- Document ID: `Task-267`
- Title: `Move Canonical Mutation To Terminal Flow Acceptance`
- Phase: `task`
- Status: `done` (2026-07-31 — implemented, then reviewed by a dedicated Claude reviewer agent: 3 Critical, 8 Important, 8 Minor findings (a FAIL verdict), including a silent-data-loss fail-open bug (a `(run, feature)` key that was ever finalized/abandoned could never become pending again after a legitimate redrive), a partial-commit bug (finalizing several features one at a time could permanently mutate an earlier feature's real Canonical Head even though the Flow's terminal acceptance as a whole was refused), and a hang-inducing bug (a finalize failure burned the turn's one-decision slot with no operator-actionable path forward — the same recurring class as BUG-288 #9 / Task-242 T-9). All 3 Critical + 6 of 8 Important findings fixed and re-verified via mutation testing (3 Critical fixes independently proven by temporarily reverting each and confirming the corresponding test fails, then restoring it). Full `internal/runner` regression re-run after fixes: 16 failures — the same 15 pre-existing failures plus 1 confirmed load-dependent flake unrelated to this task, 0 new deterministic regressions. See [CA-428](../../../change-audit/CA-428-move-canonical-mutation-to-terminal-flow-acceptance.md) for the complete findings/fix accounting.)
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude (agent review, complete — see CA-428)`
- Created: `2026-07-31`
- Last Updated: `2026-07-31`
- Feature Keys: `agent-flow-engine`, `change-contract`
- Parent Documents: [CP-55: Flow-First Preflight Contract, Context Retrieval, and Canonical Acceptance](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) (P-5), [Task-266](./Task-266-Enforce-Frozen-Scope-At-Coder-Gate-And-Amendments.md) (P-4 — the coder-gate this task's `canonicalPendingRoute` routes alongside)
- Child Documents: `none`
- Related Documents: [CA-425](../../../change-audit/CA-425-frozen-preflight-contract-model-and-storage.md), [CA-426](../../../change-audit/CA-426-contract-planner-freeze-node-and-inline-chain-advancement.md), [CA-427](../../../change-audit/CA-427-enforce-frozen-scope-at-coder-gate-and-amendments.md)
- Replaces: `None`
- Tags: `agent-flow-engine, change-contract, canonical-head, terminal-acceptance, additive`

## AI Quick View

### Summary

Adds a `PendingCanonicalStore` (mirroring `FrozenStore`'s established append-only shape) that lets a Flow coder child's gate pass STAGE a would-be Canonical Head update instead of writing the real `.flowpilot/canonical/<feature_key>.json` file. `commitChangeContract` gains a `canonicalPendingRoute` parameter that routes a Flow coding child to staging while leaving Normal chat and a Flow's own root/hub turn on the pre-existing immediate-write path. The real Head is finalized in exactly one place — `applyFlowControl`'s `"done"` case, genuine terminal Flow acceptance, using the LATEST staged value and aborting the whole "done" transition if finalization fails. Every other terminal-ish outcome (Stop/Cancel, failure) abandons the staged update instead; `"continue"`/`"escalate"` leave it untouched. Explicit rebaseline/retire/merge Canonical Head operations are completely unaffected.

### Current Ask

Implement P-5 exactly: pending Canonical store, split `commitChangeContract` effects, coder gate stages only, terminal `"done"` finalizes before publishing done, failure/stopped/cancelled outcomes abandon, explicit rebaseline/retire/merge preserved, Normal chat's legacy immediate-write behavior preserved.

### Key Decisions

- `D-1` **Mirrors `FrozenStore`'s proven shape deliberately**, not a new persistence idiom: two append-only NDJSON files, a path-keyed mutex serializing writers across in-process instances, strict reload that fails closed on a corrupt trailing line, "logical last-wins status" for deciding pending/finalized/abandoned.
- `D-2` **The routing decision is one disjoint boolean at the existing gate call site**, not a rewrite of `commitChangeContract`'s own logic: `canonicalRoute := canonicalPendingRoute{}` unless `isCodingChild && s.isFlowEngineDriven(parentID)`. Root/hub gate calls always pass the zero value, so Normal chat and a Flow's own hub turn are byte-for-byte unaffected.
- `D-3` **`PendingCanonicalRecord` carries the fully-computed Head update at stage time; finalizing writes it byte-for-byte, never recomputing spec/code drift at finalize time.** Drift is evaluated once, when the coder's gate pass stages the record — matching the spec's own framing that a Flow's terminal acceptance is of *what was already computed and agreed at each step*, not a fresh recomputation against whatever the Head happens to look like at the moment "done" fires.
- `D-4` **CORRECTION (Claude-agent review pass 1, Critical — Finding C-1): a `(run, feature)` key that was ever finalized/abandoned could never become pending again.** The original `isActiveLocked` trusted only the *Status* of the latest recorded status event for a key — once terminal, every later `Stage` for that same key was silently invisible to `GetPending`/`ListPendingForRun` forever, even though this codebase explicitly supports a follow-up turn on an already-failed/stopped run (`bug308_stopped_run_followup_allowed_test.go`, `bug302_chat_followup_after_flow_done_test.go`). Concretely: a Flow fails (its pending update abandoned), gets redriven on the same run, the coder stages a corrected value, the hub reaches "done" — the original code would see nothing pending and silently succeed, publishing `done` with the Canonical Head never actually updated to the redrive's own value. Fixed: `PendingCanonicalRecord.Seq` (monotonic per store) and `PendingCanonicalStatusEvent.RecordSeq` let `isActiveLocked` distinguish "resolved-and-still-terminal" from "re-staged since the last resolution" by comparing sequence numbers rather than reading `Status` alone. Verified via mutation testing (see Acceptance Check).
- `D-5` **CORRECTION (Claude-agent review pass 1, Critical — Finding C-2): finalize across a run's several staged features was not atomic.** The original implementation wrote+marked one feature's real Head at a time; if a later feature in the same "done" batch failed to write (disk full, permission denied, a Drive-sync lock — `.flowpilot/canonical/` is Drive-synced), an earlier feature's real Head was already permanently mutated even though the Flow's terminal acceptance of ALL its features together was refused as a whole. Fixed: `changecontract.SaveHead` split into `StageHeadWrite`/`CommitHeadWrite`/`DiscardHeadWrite`; finalize now stages every feature's Head to a tmp file FIRST, and only once every stage in the batch succeeds does phase two commit (rename) and mark any of them finalized. Verified via mutation testing with feature keys deliberately ordered so the write-failing one sorts after the succeeding one, forcing the exact scenario the finding describes.
- `D-6` **CORRECTION (Claude-agent review pass 1, Critical — Finding C-3): a finalize failure burned the turn's one-decision slot with no operator-actionable path forward.** `lastFlowControlTurnID` is stamped before the status switch, so a finalize failure inside `"done"` returned an error without ever unstamping it — a hub retrying `flow_control(done)` on the same turn was rejected as a duplicate, and the loop was left silently `"running"` with nothing ever escalated — the same recurring unanswerable-hang class this codebase has regressed on before (BUG-288 #9 / Task-242 T-9). Fixed: on finalize failure, the turn stamp is cleared and `applyFlowControl(escalate)` is called (mirroring `gate_hook.go`'s own commit-failure-escalates precedent) before the error returns, so the loop reaches `Status="blocked"`/`BlockReason="escalate"`. Verified via mutation testing.
- `D-7` **Six of eight Important findings fixed in the same pass** (fresh-reload-before-decide for finalize/abandon, a lock-safety fix so `applyFlowControl` never reads a shared field outside `s.mu`, abandoning for a stopped/failed run's direct children too — not only the run itself, a `Stage`-time `FeatureKey`/`Head.FeatureKey` mismatch guard, three vacuous tests replaced with discriminating ones, and two previously-uncovered paths given direct tests). The remaining two (a reaper for a pending record left dangling by a hard process kill with no graceful Stop/failure event at all, and the escalate-forever case folded into it) are explicitly accepted/deferred — see Open Questions.

### Constraints

- Additive-only diff to `gate_hook.go`/`interactive_service.go`: `updateCanonicalHead` split into three functions (one pure, one unchanged-behavior, one new) rather than any existing external behavior changing; `commitChangeContract`'s new parameter is threaded through all 5 existing call sites with the zero value everywhere except the two Flow-coding-child sites.
- Full `internal/runner` regression suite run twice (original pass and again after the review-fix pass), mandatory per the same CRITICAL-risk classification established for P-3/P-4 — GitNexus again misreported `applyFlowControl`'s callers/`stopAgentLoop`/`commitChangeContract` as LOW/0-impacted for two of the three.
- `-race` could not be run in this environment (no cgo toolchain) — disclosed; the one finding it would most directly have surfaced (a lock-safety read outside `s.mu`) was verified by code inspection instead.
- Provider-agnostic (Case 1).

### Open Questions

- **Carried forward from Claude-agent review (CA-428 Finding I-4):** there is no reaper for a pending record left dangling by a hard kill (no Stop, no `EventTurnFailed`, no graceful terminal path at all) or by an escalate that is never resumed. This is inert (never causes an incorrect Head write, only accumulates clutter), not unsafe, but fixing it properly means a startup/reconstruction-time reconciliation pass across all runs — a broader change to the runner's recovery path than this task's own scope. Left for a later housekeeping task.
- **Carried forward from Claude-agent review (CA-428 Finding I-3's scoping):** Stop/failure abandon is deliberately one level deep (the failed/stopped run plus its direct children), matching every other Stop-cascade mechanism in this codebase today (gateEpoch bump, turn cancellation, etc. are all one-level-deep). A sub-hub-of-sub-hub topology beyond one level would need a broader, separate fix to the Stop cascade itself, not scoped here.
- **Carried forward from Claude-agent review (CA-428 Finding M-1):** a doc-less new feature now finalizes with `SpecConfidence=spec_less` rather than the pre-P-5 `current`, since the real Head is never written mid-flow for `BuildHead`'s birth-path special case to apply. Arguably more accurate, but an undocumented behavior change nonetheless.

## 1. Goal

Make "a Flow's Canonical Head can never represent code the Flow has not finally accepted" an enforced, tested property — Canonical Head mutation happens exactly once per Flow, at genuine terminal `"done"`, never at an intermediate coder pass, a validation retry, or a review-loop continue.

## 2. Parent Links

- coding plan: `CP-55` P-5

## 3. Trigger

CP-55 P-4 (Task-266) enforced that a Flow coder writes only within its frozen scope, but every gate-passing turn still committed the real Canonical Head immediately (`commitChangeContract` → `updateCanonicalHead`) — a multi-turn Flow's Head could reflect superseded intermediate code, and a Stop/Cancel/failure partway through a Flow had already permanently mutated shared feature state with nothing to undo it.

## 4. Exact Change

- `internal/changecontract/pending_head.go` (**new**): `PendingCanonicalStore` and its full API (`Stage`, `GetPending`, `ListPendingForRun`, `ListPendingForRunFresh`, `AppendStatus`).
- `internal/changecontract/head.go` (**modified, additive**): `SaveHead` refactored into `StageHeadWrite`/`CommitHeadWrite`/`DiscardHeadWrite`; `SaveHead` itself now a thin wrapper.
- `internal/runner/gate_hook.go` (**modified**): `updateCanonicalHead` split 3 ways; `commitChangeContract` gains `canonicalPendingRoute`; new `finalizePendingCanonicalHeadsForRun`/`abandonPendingCanonicalHeadsForRun`.
- `internal/runner/interactive_service.go` (**modified**): three hook points — `applyFlowControl`'s `"done"` case, `stopAgentLoop`, the `EventTurnFailed` handler.
- `internal/changecontract/pending_head_test.go` (**new**): 13 unit tests for the store itself (11 original + 2 added post-review).
- `internal/runner/flow_pending_canonical_test.go` (**new**): 27 runner-level integration tests (19 original spec signatures + 8 added post-review).

## 5. Touched Areas

- files: 1 new production file (`pending_head.go`), 2 modified production files, 2 new test files
- modules: `changecontract`, `runner` (gate + flow control)
- routes / tables: new `.flowpilot/canonical-pending/{pending_canonical,pending_canonical_events}.ndjson`; reuses the existing `.flowpilot/canonical/<feature_key>.json` per-feature Head files

## 6. Acceptance Check (DoD)

- [x] A coder child's gate pass never writes the real Canonical Head — `TestCoderGateDoesNotSaveCanonicalHeadForFlow`; it stages instead — `TestCoderGateStagesPendingCanonicalUpdate`.
- [x] Terminal `"done"` finalizes the Canonical Head — `TestFlowDoneFinalizesCanonicalHead`; using the LATEST staged version across multiple gate passes — `TestFlowDoneFinalizesLatestAcceptedVersionOnly`.
- [x] A finalization failure does not publish `"done"` — `TestFlowDoneFinalizationFailureDoesNotPublishDone` — **and now also escalates instead of silently wedging the loop** (corrected after CA-428 Finding C-3), verified via mutation testing — `TestFlowDoneFinalizationFailureEscalatesLoopForOperatorAction` (new).
- [x] Finalize is idempotent / crash-retry-safe — `TestFinalizeAcceptedCanonicalHeadsIsIdempotent`, `TestRestartRetriesInterruptedCanonicalFinalization`.
- [x] **Finalize across several staged features is all-or-nothing** (corrected after CA-428 Finding C-2, a real partial-commit bug), verified via mutation testing — `TestFinalizePartialFailureCommitsNoHeadInTheBatch` (new).
- [x] A stale/duplicate `"done"` on an already-terminal loop is rejected without re-finalizing — `TestDuplicateDoneIsRejectedWithoutRefinalizing` (renamed from the original, misnamed `TestStaleEpochCannotFinalizeCanonicalHead` — CA-428 Finding I-8); a gate pass evaluated at a genuinely stale `gateEpoch` does not stage — `TestStaleEpochGatePassDoesNotStagePendingCanonicalUpdate` (new, closes the actual epoch gap the renamed test never covered).
- [x] Every non-`"done"` terminal outcome abandons the staged update: failure — `TestFlowFailureAbandonsPendingCanonicalUpdate`; Stop — `TestFlowStopAbandonsPendingCanonicalUpdate`; Cancel (the same code path as Stop in this codebase) — `TestFlowCancelAbandonsPendingCanonicalUpdate`; escalate leaves it untouched (neither finalized nor abandoned) — `TestFlowEscalationDoesNotFinalizeCanonicalHead`.
- [x] **A `(run, feature)` key can become pending again after being abandoned, once genuinely re-staged** (corrected after CA-428 Finding C-1, a real silent-data-loss bug), verified via mutation testing — `TestPendingCanonicalStoreRestageAfterAbandonIsPendingAgain` (unit-level, new) and `TestFlowFailureThenRedriveThenDoneFinalizesTheNewHead` (runner-level, new).
- [x] `"continue"` (plain, validation-failure retry, and review-loop retry — all the same code path in this codebase) never finalizes or abandons — `TestFlowContinueDoesNotFinalizeCanonicalHead`, `TestValidationFailureDoesNotFinalizeCanonicalHead`, `TestReviewRetryDoesNotFinalizeCanonicalHead`.
- [x] A spec-drifted turn does not auto-finalize — `TestSpecDriftDoesNotAutoFinalizeCanonicalHead`.
- [x] Explicit rebaseline/retire/merge remain immediate and untouched by staging — `TestExplicitRebaselineBehaviorIsUnchanged`, `TestExplicitRetireAndMergeBehaviorIsUnchanged` (both corrected to assert the pending store file never exists, replacing a vacuous `GetPending("", ...)` check — CA-428 Finding I-6).
- [x] Normal chat's legacy immediate-write behavior is unaffected — `TestNormalChatCanonicalPathRemainsBackwardCompatible`.
- [x] **A Flow's own root/hub turn is also unaffected** (previously safe by construction but unpinned — CA-428 Finding I-7) — `TestFlowRootHubTurnWritesCanonicalHeadImmediately` (new); **an ad hoc coding child outside a Flow keeps immediate-write behavior** (also previously unpinned) — `TestAdHocCodingChildOutsideFlowEngineWritesImmediately` (new).
- [x] `gofmt -l`/`go build`/`go vet` clean on every touched/new file.
- [x] **Full `internal/runner` regression suite run twice** (original pass and again after the review-fix pass): 16 failures on the second run — the identical 15 pre-existing/environment failures from P-3/P-4's own runs, plus one confirmed load-dependent flake (verified to pass in isolation, verified unrelated to any file this task touches) — zero new deterministic regressions either run.
- [x] Cross-provider Case 1 agnostic.
- [x] GitNexus CLI impact query run on `applyFlowControl`, `stopAgentLoop`, `commitChangeContract`, `updateCanonicalHead` before editing; two of the four (`stopAgentLoop`, `commitChangeContract`) reported LOW/0-impacted, contradicted by direct grep of their real call sites — same disclosed-and-proceeded pattern as P-3/P-4.
- [x] **Claude-agent adversarial review performed and acted on**: 3 Critical + 8 Important + 8 Minor findings (a FAIL verdict); all 3 Critical findings and 6 of the 8 Important findings fixed and re-verified via mutation testing; the remaining Important finding (I-4, a reaper for hard-kill-orphaned records — inert, not unsafe) and all 8 Minor findings are explicitly accepted/deferred with reasoning in CA-428, not silently dropped.

## 7. Out of Scope

- A reconciliation/reaper pass for a pending record orphaned by a process kill with no graceful terminal event at all (CA-428 Finding I-4).
- Recursive (more than one level deep) Stop/failure cascade for a nested sub-hub-of-sub-hub topology (CA-428 Finding I-3's scoping note) — matches every other Stop-cascade mechanism in this codebase today, all of which are one-level-deep.
- Re-deriving `SpecConfidence`/drift semantics to exactly match the pre-P-5 immediate-write timing for a brand-new, doc-less feature (CA-428 Finding M-1) — the small observable difference is a direct, accepted consequence of deferring the real write to terminal acceptance.
- CP-55 P-6 (deterministic history relevance scorer) through P-9.

## 8. Cross-Provider Note

Provider-agnostic (Case 1): `grep -inE 'providerKey|claude|codex|grok'` over `pending_head.go`, the `head.go` additions, and the new/modified functions in `gate_hook.go`/`interactive_service.go` returns zero matches — the mechanism reads only `CanonicalHead`/`Contract`/run-state fields, with no provider dimension.
