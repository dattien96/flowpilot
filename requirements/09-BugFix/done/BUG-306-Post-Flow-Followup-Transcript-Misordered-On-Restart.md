# BUG-306 — Post-flow follow-up transcript is misordered on restart (prompt-1 sinks to the bottom)

## Metadata

- Document ID: `BUG-306`
- Title: Restart reconstruction misaligns overlay when a post-flow follow-up carries the agent-context note prefix and the hub turn-1 was suppressed
- Phase: `bugfix`
- Status: `done`
- Owner: local-runner
- Reviewers: n/a
- Created: 2026-07-21
- Last Updated: 2026-07-21
- Parent Documents: BUG-302 (enabled post-"done" follow-ups), BUG-300 (suppressed hub turn-1 / prepend), BUG-083 + Task-076 (raw-prompt overlay), BUG-122 (agent-context note prefix), CP-51 A10
- Child Documents: none
- Related Documents: BUG-305 (companion — the live-hang half of the same A10 scenario), BUG-293/295/272, CA-366/CA-379 (prior transcript-order fixes)
- Replaces: none
- Tags: chat-history, agent-flow-engine, resume, transcript, regression

## AI Quick View

### Summary

- Found live on run-18371 (Workflow mode) while verifying CP-51 A10: after a flow finished, sending follow-up turns and restarting the server produced a scrambled restored transcript — follow-up prompts rendered at the TOP and the original "fix bug 1+1 != 2" prompt sank to the BOTTOM (screenshots image 1 and image 5).
- Two compounding causes in the shared restart reconstruction (`overlayRawTurnPrompts`):
  1. The FIRST post-flow follow-up is sent to the provider wrapped in `composeAgentContextBlock`'s `"[FlowPilot system note — sub-agents …]"` prefix (BUG-122), so the provider session file persists the wrapped text as that user turn. `isSystemPrompt` (via `isFlowEnginePrompt`) treats the wrapped slot as a pure system prompt, so `overlayRawTurnPrompts` **skipped** it.
  2. `overlayRawTurnPrompts` matched turn-log raw prompts to historical slots **front-to-back**, but a flow hub's turn-1 is suppressed (spawns children without ever calling the provider, BUG-300) so the leading prompt has **no slot** — the positional match is off by one from the start.
- Together: prompt-1's text is overlaid onto the LAST follow-up's slot (rendered at the bottom), and the follow-ups are prepended to the top by `prependMissingPromptOnlyEvents`. The model reproduces both observed screenshots exactly.

### Current Ask

- Restore the correct transcript order on restart — original prompt first, then flow output, then each follow-up in its place — without regressing any prior transcript-order fix (this area has many: BUG-083, BUG-300, BUG-293/295, BUG-272, CA-366, CA-379, run1264, run2334, run20332).

### Key Decisions

- `V-1` **Do NOT change `isSystemPrompt` / `isFlowEnginePrompt`.** BUG-300 and run1264 deliberately keep an agent-context-WRAPPED join note internal, and `TestRun1264ComposedSubAgentPromptIsInternal` asserts composed sub-agent prompts stay system. All transcript-hiding invariants must be preserved, so the new logic lives locally in `overlayRawTurnPrompts`.
- `V-2` Add `stripAgentContextBlock` (shares the `agentContextBlockOpen/Close` constants with `composeAgentContextBlock` so they cannot drift) and `isOverlayableUserTurn`: a slot is a genuine user turn when it is not a system prompt, OR it carries the agent-context prefix AND the text after the note is itself not a system prompt. This distinguishes a wrapped user follow-up (overlay it) from a wrapped join note (keep internal — the exact BUG-300/run1264 case).
- `V-3` Align the overlay from the END: `promptIndex = len(rawPrompts) − (overlay-able slot count)`. The only prompts missing a slot are leading suppressed hub turn-1(s), so the last N raw prompts map to the N slots and the leading prompt falls through to `prependMissingPromptOnlyEvents`. When every prompt has a slot (all normal chats, non-suppressed flows) the offset is 0 — behavior is identical to before.

### Constraints

- additive-tests-only: only `bug306_post_flow_followup_transcript_order_test.go` is added; no existing test modified.
- cross-provider-parity: `overlayRawTurnPrompts` is the shared overlay for Claude and Codex; Grok's `overlayRawGrokTurnPrompts` delegates directly to it, so one fix covers all three.
- Must not regress the extensive prior transcript-order test suite.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_resume.go` (`overlayRawTurnPrompts`, `stripAgentContextBlock`, `isOverlayableUserTurn`, `overlayRawGrokTurnPrompts`, `prependMissingPromptOnlyEvents`)
- `apps/local-runner/internal/runner/interactive_service.go` (`composeAgentContextBlock`, `agentContextBlockOpen/Close` constants)
- `apps/local-runner/internal/runner/bug306_post_flow_followup_transcript_order_test.go`

## 1. Issue Summary

A restart reconstructs the main-chat transcript from the provider session file plus the durable turn log. When a post-flow follow-up carried the agent-context note prefix and the hub's turn-1 was suppressed, the shared positional overlay mis-mapped the raw prompts onto the historical slots, sinking the original prompt to the bottom and floating the follow-ups to the top.

## 2. Parent Links

- impacted coding plan: CP-51 A10
- impacted tech design: BUG-083/Task-076 raw-prompt overlay contract; BUG-300 suppressed-turn-1 prepend
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: any; requires a flow-hub run (turn-1 suppressed) with ≥1 post-flow follow-up whose first turn carried the agent-context note prefix, then a server restart (in_memory=false reconstruction).
- reproduction (live): run-18371 — flow finished, sent "hello …" then "Ý là …", restarted twice; restored transcript showed follow-ups first and "fix bug 1+1 != 2" last.
- reproduction (test): `go test ./internal/runner -run TestPostFlowFollowUpTranscriptKeepsPromptOrderOnRestart` — fails without the fix with `prompt order[0] = "hello …"`.

## 4. Expected vs Actual

- expected: restored order = original prompt, flow output, follow-up-1, follow-up-2 (matching the live pre-restart view).
- actual: follow-ups prepended to the top; original prompt overlaid onto the last follow-up's slot and rendered at the bottom.

## 5. Impact

- users affected: anyone reopening a flow run after sending post-flow follow-ups (the CP-51 A10 scenario opened by BUG-302), across Codex/Claude/Grok.
- workflows affected: restart/history-reopen transcript reconstruction.
- severity: medium (cosmetic-but-confusing misordering of a restored transcript; no data loss — durable content is intact, only its replay order is wrong).

## 6. Root Cause

- confirmed cause: `overlayRawTurnPrompts` (1) skipped the agent-context-wrapped follow-up slot because `isSystemPrompt` flags the wrapper, and (2) matched raw prompts front-to-back with no accounting for the suppressed leading hub turn-1. `loadClaudeTranscriptEvents` assigns synthetic `replay-prompt-N` ids, so there is no turn-id correlation to fall back on — the overlay is purely positional.
- evidence: run-18371 Claude `.jsonl` line 63 stores the wrapped follow-up; the reconstruction model (offset shift + skipped slot) reproduces both screenshots; the regression test fails identically without the fix.

## 7. Fix Strategy

- `F-1` `stripAgentContextBlock` + `isOverlayableUserTurn` (local, `isSystemPrompt` untouched) so a wrapped genuine user turn is overlay-able while a wrapped join note stays internal.
- `F-2` Offset the overlay start by `len(rawPrompts) − overlayableSlots` so suppressed leading prompts don't shift the alignment; they fall through to `prependMissingPromptOnlyEvents`.
- `F-3` Share `agentContextBlockOpen/Close` constants between `composeAgentContextBlock` and `stripAgentContextBlock` so the wrapper text cannot drift from the strip logic.

## 8. Validation

- `V-1` **cross-provider-parity:** `overlayRawGrokTurnPrompts` delegates to `overlayRawTurnPrompts`; the shared function serves Claude/Codex/Grok. The prior cross-provider suites `TestRun20332FlowHubHistoryParityForEveryProvider` and `TestRun2334…AnchorsSidecarsByDurableTurnID` (both parameterized over all three) pass with the fix.
- `V-2` **additive-tests-only:** only the new BUG-306 test added; no existing test modified.
- `V-3` **git-stash regression discipline:** with the fix stashed, BOTH the Claude and Grok integration tests (`TestPostFlowFollowUpTranscriptKeepsPromptOrderOnRestart` and `…Grok`) fail with `prompt order[0] = "hello …"` (original prompt not first) — the scramble reproduced on both the shared Claude/Codex seed path and Grok's separate `seedGrokTranscriptFromDisk` path; with the fix both pass. A focused unit test `TestIsOverlayableUserTurn` additionally pins the discriminator (bare / pure-orchestration / agent-context-wrapped-user / agent-context-wrapped-join-note / malformed-wrapper) so the BUG-300 collision cannot be reintroduced by a future simplification.
- `V-4` **prior-fix invariants preserved:** the full order/replay guarding battery passes — `TestClaudeRestartRestoresFirstPromptWhenHubNeverCalledProviderOnTurnOne` (BUG-300), `TestRun1264Restore*` (internal join note / composed sub-agent stay hidden, Claude + Grok), `TestSeedTranscriptUsesRawPromptFromTurnLog` (BUG-083), `TestSeedTranscriptFromDiskMovesSidecarPrefix…`, run2334/run20332 (all 35 in the targeted battery green). Every existing scenario resolves to offset 0 with inert wrapper logic — byte-identical behavior; only the new post-flow-follow-up shape changes.
- `V-5` **full-suite regression:** `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1` — failure set does not grow beyond the known pre-existing environment-only set plus the known parallel-load flakes.
- `go build ./...` and `gofmt` clean.

## 9. Regression Guard

- tests: `TestPostFlowFollowUpTranscriptKeepsPromptOrderOnRestart` (Claude), `…Grok` (Grok separate seed path), and `TestIsOverlayableUserTurn` (discriminator unit test); the entire prior transcript-order battery continues to guard the untouched invariants.
- alerts: n/a
- audit checks: none.

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-51 A10 note updated to reference this follow-on restart-order fix.
- notes left unchanged on purpose: `isSystemPrompt`/`isFlowEnginePrompt` and every transcript-hiding path are intentionally untouched; the fix is confined to the overlay alignment + a local overlay-ability predicate.
