## Review pass 1 — Codex reviewer — A1 hub park

Request: Complete CP-51 A1–A3 under additive-tests-only; this pass reviewed the A1 residual hub-park implementation.

FAIL

Important finding: same-turn `continue` suppression is only in memory. `PendingGateReprompt*` remains persisted while `hubContinueDelegatedTurnID` is not part of session state, so a crash/restart can revive the forbidden root hub gate reprompt and reproduce the run-9437 concurrent writer race. Persist the suppress/reroute atomically and add a new restart/reconstruction regression that proves resume/idle flush cannot auto-reprompt the hub after a continue-delegated writer.

Verification: focused live-path tests passed; `-race` could not run because `CGO_ENABLED=0` and `gcc` is unavailable.

## Review pass 2 — Codex reviewer — A1 durable suppression

Request: Re-review the A1 durable suppression/restart fix under additive-tests-only.

FAIL

Critical finding: `hubContinueDelegatedTurnID` is persisted but never cleared. The restart/idle-flush suppress path therefore can erase a legitimate later, unrelated hub gate reprompt, despite the CP-51 requirement being scoped to the same transfer. Make the marker truly turn-scoped or consume it durably once the transfer finishes, and add a new regression proving a later hub reprompt is not suppressed. The new tests currently cover only live, same-turn, and restart suppression.

Independent regression: unchanged legacy `TestHubNotifyDeferredReinvokeRetriesWithStashedPromptNotGenericOne` now fails; fix production behavior only.

## Review pass 3 — Codex reviewer — Claude hub.notify repair

Request: Repair the unchanged legacy hub.notify deferred reinvoke regression without violating CP-51 A1 or additive-tests-only.

OK

No Critical, Important, or Minor findings. The root post-turn gate success paths clear `postTurnGateCancel` before tail `notifyTurnIdle`, restoring the deferred `pendingHubReinvokePrompt` drain. The durable continue-delegate suppression remains turn-scoped and is consumed so a later N+1 reprompt is not suppressed. Legacy tests are unchanged; runner additions are dedicated `run9437_*` files.

Focused validation: `go test ./internal/runner -run "TestHubNotify|TestRun9437|TestRun1618|TestHubParked" -count=1` passed 10 tests. `-race` remains unavailable because CGO requires missing `gcc`.

## Review pass 4 — Codex reviewer — BUG-290 Context Produce model visibility

Request: Review the Claude Sonnet medium fix that suppresses inherited run-model metadata for explicit non-agent behavior cards, under additive-tests-only.

OK

No Critical, Important, or Minor code findings. `FlowStepTimeline` now renders model metadata only for `agent.delegate` and legacy steps with no `behaviorId`; explicit non-agent behaviors, including `context.produce`, do not inherit and display the run model. The regression coverage is a new, dedicated test file and existing tests were untouched. Review follow-up corrected the BUG/CA record's task link and test-evidence command.

Focused validation: `npx tsx --test apps/desktop-flowpilot/src/components/FlowStepTimeline.model-visibility.test.ts` passed 4 tests; `npm --prefix apps/desktop-flowpilot run build` passed.

## Review pass 5 — Codex reviewer — BUG-290 Settings list renderer

Request: Review the Settings > Step Definitions follow-up after the live screenshot proved the original Flow Timeline fix targeted the wrong surface.

OK

No Critical, Important, or Minor findings. The Settings form and list now share `stepDefinitionRequiresModel`; `stepDefinitionListSubtitle` renders `stepType` alone for explicit non-agent behaviors and preserves `stepType / model` for `agent.delegate` and legacy rows. The dedicated test file is additive-only, and BUG-290/CA-368 record the actual runnable verification.

Focused validation: `npx tsx --test apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.step-list-model-visibility.test.ts` passed 4 tests; the earlier Flow Timeline test passed 4 tests; `npm --prefix apps/desktop-flowpilot run build` passed.

## Review pass 6 — Codex reviewer — BUG-291 regression-gate dismissal

Request: Review the A1 live regression where locally dismissing an r-reg decision card orphaned a pending coder gate.

OK

No Critical, Important, or Minor findings. Option-bearing regression cards now use `Stop flow` and the existing cascade-stop action rather than `dismissGateBlock`; plain informational gate cards remain locally acknowledgeable. The regression-decision overlay has no outside-click dismissal. The new test is additive-only and BUG-291/CA-369 accurately retain the live-retest limitation.

Focused validation: `npx tsx --test apps/desktop-flowpilot/src/components/gateBlockActions.test.ts` passed 2 tests; `npm --prefix apps/desktop-flowpilot run build` passed.

## Review pass 7 â€” Codex reviewer â€” BUG-292 Stop flow snapshot reconciliation

Request: Review the `run-11679` child-focused Stop regression where runner state was terminal but the desktop main card restored a cached `running` snapshot.

OK

No Critical, Important, or Minor findings after the partial durable-checkpoint failure path was included. `reconcileStoppedRunSnapshots` now covers successful Stop, a `RunnerApiError` carrying an embedded stopped graph, and the no-snapshot fallback. Completed/failed child snapshots are preserved; stale active parent/child snapshots are terminalized.

Focused validation: `npx tsx --tsconfig apps/desktop-flowpilot/tsconfig.json --test apps/desktop-flowpilot/src/state/stop_parent_snapshot_reconciliation.test.ts` passed 2 tests; runner stop/interrupt/A1 probes passed 3 tests; `npm --prefix apps/desktop-flowpilot run build` passed.

## Review pass 5 — Independent reviewer — BUG-293 joined-result-note lost after restart

Request: Review the fix that redacts internal flow-engine/system prompts from the live `turn_started` bubble so the live transcript matches replay, under additive-tests-only.

OK

No Critical, Important, or blocking findings. `liveTurnStartedDisplayPrompt` (interactive_resume.go, co-located above `isInternalTranscriptEvent`) returns "" for `isSystemPrompt` prompts; the single live emit site (interactive_service.go:6866) uses it for the DISPLAY prompt only. `in.Prompt` still reaches the provider (`runTurn`), the turn log (`AppendTurnLog`), and `rs.lastPrompt` unchanged — the LLM turn and history title are unaffected. `EventTurnStarted` is not a flow-sidecar type, so persisted state and reconstruction are untouched (reconstruction rebuilds from the turn log, then filters via `userFacingTranscriptEvents`). Desktop `timelineReducer.ts:198` renders a bubble only when `e.prompt` is truthy, so an empty prompt yields no bubble while the turn still opens for the assistant response. The change hides ALL system-prompt classes (gate reprompt, handoff, flow-engine) consistently; no existing test asserts a system prompt is user-facing live — `run1264_settle_and_restore_test.go` already requires the opposite on replay, so live now matches. Bonus alignment: the handoff-context path (`handoff_context.go`) also stops injecting system prompts as user turns, matching replay. Additive-tests-only respected: only `bug293_joined_note_live_replay_symmetry_test.go` added; no existing test modified.

Focused validation: `go test ./internal/runner -run "TestBug293" -count=1` → 9 passed; blast-radius battery (`TestBug293|TestRun2334|TestHubNotify|TestRun9437|TestRun1618|TestHubParked|Reprompt|Transcript|FeatureBucket|JoinedNote|Synthesis|Reinvoke|Handoff`) → 107 passed. `-race` unavailable (no gcc/CGO). GitNexus unavailable on this machine; localized inspection confirmed 6866 is the only live turn_started-with-prompt emit.

## Review pass 6 — Independent reviewer — BUG-294 resumed agent card completed suffix

Request: Review the fix that gates the resumed child result annotation on `session.Status == RunStatusCompleted` so a restart-killed child no longer shows a false "— completed", under additive-tests-only.

OK

No Critical, Important, or Minor findings. The resume gate (`interactive_resume.go`: `if child.completed && child.lastMessage != ""`, `completed` from `session.Status == RunStatusCompleted`) exactly mirrors the LIVE emit condition (`interactive_service.go:5155-5172`: `EventAgentResultInjected` fires only for `completion.status == RunStatusCompleted` with non-empty FinalMessage; a failed child returns early and never emits). `ListAllProviderSessions` returns the raw persisted `RunStatus`, so a mid-turn-killed child correctly reads "running" here (the Running→Cancelled normalization applies to the reconstructed run, not this lookup). No regression: every completion path persists `status=completed` before the annotation emit, so genuinely-completed children (incl. cohort members) keep "— completed". `resumedParentAgentAnnotations` is the ONLY replay source of a child result annotation (desktop `timelineReducer.ts` sets `finalMessage` solely from `agent_result_injected`; `Timeline.tsx:544` renders "— completed" purely from it), so cutting it at the source fully removes the false suffix while the card's independent `status` segment shows the true "cancelled". additive-tests-only respected: only `bug294_resumed_agent_card_completed_suffix_test.go` added; no existing test modified.

Focused validation: `go test ./internal/runner -run "TestBug294" -count=1` → 3 passed; restore/annotation/replay battery (`TestBug294|TestRun1264Restore|TestRun1264Settle|TestRun2334RestartReplayKeepsAssistantResponsesForEveryProvider`) → 12 passed. `-race` unavailable (no gcc/CGO). GitNexus unavailable; localized inspection confirmed both `EventAgentResultInjected` emit sites now agree.

## run-63960 blocked restart — Codex review pass 1 (2026-07-23)
Reviewer: gpt-5.6-terra high
Result: Not OK — 7 findings (critical gen reset, graph race, unlocked persist, 409 false-positive, stopped/done clear, R3 gaps, CA incomplete)
Grok addressed findings; re-review scheduled as pass 2.

## run-63960 blocked restart — Codex review pass 2 (2026-07-23)
Reviewer: gpt-5.6-terra high
Result: OK — no remaining blocking issues
Validated: gen high-water, locked persist, stale HTTP guard, 409 restriction, terminal settle clear, provider matrix, CA-414
Waiver: store.history-replay-order pre-existing red (not this diff)

# Review Results

## Pass 1 — Codex 5.6 Terra — CP-55 P-1

**Request summary:** Implement CP-55 P-1 explicit Flow writer semantics and safety-topology foundation with additive tests, Task-263, and CA-424. Normal chat must remain unrestricted.

**Result:** NOT OK

1. **Critical:** `flow_safety_topology.go:140` returns `true` when the target is absent or unreachable. It can accept a writer/freeze forward-cycle disconnected from every entry. Require declared IDs and entry reachability; add absent-ID, unreachable-cycle, no-entry-cycle, and multiple-entry tests.
2. **Critical:** Task-263 is phase-format noncompliant. Its Parent Documents metadata omits linked SD-21/SS-14, and required `## 8. Completion Notes` was replaced by `## 8. Cross-Provider Note`.
3. **Important:** The acceptance check only rejects direct `writer -> done`. A non-acceptance intermediate node passes, so the claim that every terminal path crosses acceptance is false. Model a declared acceptance boundary and traverse paths, or narrow the P-1 claim strictly to a non-direct-done foundation check.
4. **Important:** Verification claims conflict. Task-263 calls the full runner suite green while CA-424 records failures; the current environment produces broader prerequisite failures. Record only reproducible focused/package/build evidence and correct the new-test count.
5. **Important:** Task-263 links a nonexistent CA-328 filename. Use `CA-328-bug288-flow-mode-three-tier-gate-and-change-contract-reentry.md`.
6. **Important:** CP-55 metadata uses invalid phase/status values for its reference format.
7. **Medium:** CA-424's change-ledger fence omits the SS-13 `entries:` records and uses unsupported `change_type`/`summary` fields.

**Confirmed positives:** no pre-existing test file is modified; focused registry tests pass; the full `internal/agentpack` suite passes; Normal chat behavior was not changed; `contract.freeze` placeholder fails closed.

## Pass 2 — Codex 5.6 Terra — CP-55 P-1 Review Fix

**Request summary:** Re-review the fail-closed dominance and explicit `acceptance_nodes` fixes from pass 1.

**Result:** NOT OK

1. **Important:** writer paths with no reachable terminal `done` still pass vacuously. Require at least one reachable `done` per writer and add no-successor plus accepted-cycle-without-done rejection tests.
2. **Important:** `AcceptanceNodes` is parsed from embedded YAML but is not persisted/restored by the Supabase-backed user-flow store. Add durable storage/migration, read/write/clone/mirror coverage, or the P-8 migration will fail after reload.
3. **Minor:** no test exercises actual root-YAML `acceptance_nodes` parsing.
4. **Minor:** blank/duplicate acceptance IDs are not explicitly rejected.
5. **Minor:** CA-424's targeted-runner test count is internally inconsistent and conflicts with the current reproducible focused command.

**Confirmed fixed:** declared-node/entry reachability now makes dominance fail closed; stateful `(node, crossedAcceptance)` traversal catches mixed branches and terminates on cycles; Task-263 now has compliant metadata, upstream links, and `Completion Notes`; Normal chat remains unchanged; no pre-existing test file is modified.

## Pass 3 — Codex — CP-55 P-1 Persistence Review

**Request summary:** Re-review the P-1 topology and durable `acceptance_nodes` implementation after the pass-2 fixes.

**Result:** NOT OK

1. **Important:** The actual TypeScript Settings clone path drops `acceptance_nodes_json`. The Go store is correct, but `adminModels.ts` has no `acceptanceNodes`; `mapWorkflow`, `saveWorkflow`, and `cloneWorkflow` in `supabaseAdminRepository.ts` omit the field; the Settings draft/save path cannot carry it. A cloned migrated built-in therefore receives the database default `[]` and loses its safety boundary.
2. **Important:** Task-263 is stale after pass 2. It omits the Supabase store, migration, new tests, and table scope; retains old verification counts; and describes only the pass-1 production files.
3. **Minor:** No focused test reads `20260730121000_add_workflow_acceptance_nodes.sql` and asserts the table, column, `jsonb NOT NULL`, and default `[]` contract.
4. **Minor:** CA-424 overstates direct clone/update/mirror proof. The new acceptance-bearing test calls a conversion helper, while the existing clone/update tests use empty lists.
5. **Minor:** A topology comment claims every forward path reaches `done`, while the implementation intentionally requires only at least one reachable `done` and checks every reached `done` path crosses acceptance.

**Verified green:** agentpack 57 tests; focused AcceptanceNodes runner 6 tests; existing Supabase/mirror/clone/migration selection 50 tests; `go build ./...`; reached-done, blank/duplicate ID, YAML parsing, and Go store round-trip behavior.

## Pass 4 — Codex — CP-55 P-1 Final Persistence Review

**Request summary:** Re-review the complete P-1 implementation after the TypeScript Settings persistence, migration-test, and documentation fixes.

**Result:** NOT OK

1. **Minor:** `flow_safety_topology.go` uses the same diagnostic for both “no reachable terminal `done`” and “a reachable `done` path bypasses acceptance.” The former diagnostic falsely says such a path exists. Distinguish the cases or use one truthful combined message.
2. **Minor:** CA-424 says `CloneBuiltin`, `UpdateUserFlow`, and `SyncBuiltins` acceptance-bearing preservation are “proven directly,” but the cited test directly executes only `builtinRecordFromFlow`. Narrow the claim to direct helper coverage plus code-path inspection, or add direct acceptance-bearing tests.
3. **Minor:** Stale wording remains: CP-55 says the roster grew after two review-fix passes although there are three; Task-263 and CA-424 refer to “four touched/new production files” without qualifying that as the original pass-1 scope.

**Verified green:** TypeScript repository and draft round-trip tests 7+3; existing TypeScript regression tests 5+8+5+4; Go AcceptanceNodes tests 7; agentpack tests 57; Go build/vet clean; focused migration SQL test green; no tracked pre-existing Go or TypeScript test modified.


## Pass 5 — Codex — CP-55 P-1 Review Ledger Integrity and Final-Tree Count Audit

**Request summary:** Re-review the CP-55 P-1 implementation after the pass-4 diagnostic-message, claim-narrowing, and stale-wording fixes, including the integrity of this review_result.md ledger and the Task-263 final-tree verification counts.

**Result:** NOT OK

1. **Important:** review_result.md overwrote 80 historical lines instead of appending; fixed by restoring the full HEAD ledger and appending CP-55 passes.
2. **Minor:** Task-263 final-tree verification count said 57 but the fresh agentpack suite is 60.

**Confirmed fixed:** all Pass-4 findings were otherwise fixed. Pass 6 remains outstanding; this is not a clean final review.

## Pass 6 — Codex — CP-55 P-1 Finalization Review

**Request summary:** Verify the append-only ledger repair, the corrected final-tree test count, all prior P-1 findings, and final repository hygiene.

**Result:** NOT OK

1. **Minor:** CP-55, Task-263, and CA-424 still described Pass 5 as outstanding even though Pass 5 existed and both findings were fixed.
2. **Minor:** Twelve untracked `.rr_*.tmp` ledger-repair artifacts remained at the repository root.

**Verified clean otherwise:** `review_result.md` preserves the exact HEAD prefix and is append-only; CP-55 Passes 1–5 occur once and in order; Task-263 records 60; agentpack, runner persistence, Go build/vet, TypeScript repository/Settings selections, and `git diff --check` pass; no tracked pre-existing test changed; the user's CP-54 diff and untracked CP-43-52-53-54 note are preserved. Pass 7 remains outstanding; this is not a clean final review.

## Pass 7 — Codex — CP-55 P-1 Final Clean Gate

**Result:** OK

No Critical, Important, Medium, or Minor findings.

Verified: `review_result.md` remains exact append-only history; current CP-55/Task-263/CA-424 status is truthful; all twelve repair artifacts are gone; Task-263 records 60; agentpack and runner persistence tests, Go build/vet, TypeScript repository/Settings selections, and `git diff --check` pass; no tracked pre-existing test changed; the user's CP-54 diff and untracked CP-43-52-53-54 note are preserved.
