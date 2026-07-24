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
