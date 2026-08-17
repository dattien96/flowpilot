# BUG-326: Flow reprompt gate replayed into a resumed chat transcript as a user query (run-104296)

## Metadata

- Document ID: `BUG-326`
- Title: `A flow gate reprompt is stamped with a user `<user_query>` role by the Grok provider, so on restart the shared overlay replays it as a genuine user question instead of keeping it internal`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-17`
- Last Updated: `2026-08-17`
- Parent Documents: [CP-51: Durable Chat Resume](../../07-Coding-Plan/inprogress/CP-51-Durable-Chat-Resume.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Child Documents: `none`
- Related Documents: [CA-538](../../../change-audit/CA-538-system-prompt-tag-replay.md), [CA-386](../../../change-audit/CA-386-post-flow-followup-transcript-order.md), [CA-371](../../../change-audit/CA-371-live-replay-symmetry-keyed-on-isSystemPrompt.md)
- Replaces: `none`
- Tags: `chat-history, flow-gate, replay, system-prompt, regression, severity-medium`

## AI Quick View

### Summary

- run-104296: a flow gate reprompt that stopped the run mid-question was persisted by Grok wrapped inside a `## History …` preamble plus a `grokAskUserReinforcement` `<user_query>` block; on restart the shared overlay matched it by role and replayed it as the user's own next question — the operator's genuine follow-up was dropped from the restored transcript.
- The provider (Grok) **wraps** internal system prompts in `<user_query>` blocks with surrounding reinforcement text; `isSystemPrompt` only detected bare prompt text via `HasPrefix`, so a wrapped gate reprompt classified as a user turn and was overlaid into a history slot.
- Legacy `HasPrefix` detection of gate/flow-engine/handoff prompts is inherently fragile: it only matches prompts the runner itself emits verbatim, and misses any provider wrapping.

### Current Ask

- Stamp every internal system prompt (gate reprompt, handoff envelope, flow-engine/hub orchestration) with a durable `[SYSTEM_PROMPT]` tag at the wire boundary, classify system prompts by tag first, and keep the legacy detectors for old runs that predate the tag.

### Key Decisions

- `D-1` New shared constant `systemPromptTag = "[SYSTEM_PROMPT]"` in `feature_history.go`; `isSystemPrompt` checks the tag first, then falls back to legacy `isGateReprompt || isHandoffPrompt || isFlowEnginePrompt` (backward compatibility with runs recorded before the tag).
- `D-2` `isGateReprompt` switches from `HasPrefix` to `Contains` so a gate sentence wrapped mid-block by a provider is still recognized when no tag is present.
- `D-3` `isFlowEnginePrompt` becomes tag-aware (tag prefix short-circuits to system) so direct callers like `skipHistory` classify consistently.
- `D-4` The tag is stamped last-mile in `startTurn`, immediately before the turn is emitted / written to the turn log / handed to `runTurn` — the tag therefore lands on exactly what the provider receives and what the turn log records, keeping live and replay symmetric; the stamp is idempotent (never double-tagged) and **never applied to user prompts**.

### Constraints

- additive-tests-only: only a new test file was added (`run104296_system_prompt_tag_replay_test.go`); no existing test file modified.
- `isSystemPrompt` is Case 1 (shared, provider-agnostic) — no per-provider branch was added; parity for Claude/Codex/Grok verified by the shared matrix tests.
- Prior contract preserved: CA-101 raw-prompt overlay, CA-371 live/replay symmetry keyed on `isSystemPrompt`, CA-380/CA-386 overlay slot math, CA-501 foreign-slot guard.
- User prompts (including `ask_user`/`spawn_agent` reinforcement) must never be tagged.

### Open Questions

- `Q-1` None. Known unrelated flake re-confirmed: `TestGeminiAdapterPromptPrepAndEnv` passes in isolation; its `agy`-binary timeout is environmental.

### Source Refs

- run-104296 (operator report: `Question?` gate reprompt replayed as user question after restart).
- Code: `apps/local-runner/internal/runner/feature_history.go` (`systemPromptTag`, `isSystemPrompt`, `isGateReprompt`, `isFlowEnginePrompt`), `interactive_service.go` (`startTurn` last-mile stamp), `grok_transcript_loader.go` (`grokUserQueryText`).
- Tests: `apps/local-runner/internal/runner/run104296_system_prompt_tag_replay_test.go`.

## 1. Issue Summary

When a flow gate raised a reprompt (`flowgate.GateRepromptPrefix`, "Question?"-style) and the run was stopped mid-question, Grok's persisted transcript wrapped that system prompt inside a `## History …` preamble and a `grokAskUserReinforcement` `<user_query>` block. On restart, `overlayRawTurnPrompts` / `overlayRawGrokTurnPrompts` could not classify the wrapped prompt as internal (it is not a bare gate sentence, so `HasPrefix` failed), treated it as the user's next question, and overlaid it into a history slot — dropping the operator's actual follow-up.

## 2. Parent Links

- impacted coding plan: CP-51 (durable chat resume / transcript replay)
- impacted tech design: SD-19 (flow gate / agent loop), shared transcript overlay in runner
- impacted system spec: none directly (runner layer)

## 3. Environment and Reproduction

- environment: runner local mode, flow mode, gate reprompt, Grok provider; restart via `resumeRun`.
- reproduction steps: run a flow whose gate asks a question, stop the run mid-question, restart the runner, resume the chat; the restored transcript shows the gate reprompt as the user's own question and drops the real follow-up.
- frequency: deterministic for Grok (wrapped `<user_query>`); latent for any provider that may wrap internal prompts.

## 4. Expected vs Actual

- expected: the gate reprompt stays internal (never shown as a user question); the operator's genuine follow-up is restored in order.
- actual: the wrapped gate reprompt replayed as a user question; the genuine follow-up was dropped.

## 5. Impact

- users affected: anyone resuming a flow chat that stopped at a gate question (Grok in this report; the tag fixes the class for all providers).
- workflows affected: chat-history resume/replay for flow mode.
- severity: medium (data-loss-of-input on resume; no corruption of persisted files).

## 6. Root Cause

- hypothesis: `isSystemPrompt` failed to classify a provider-wrapped gate reprompt because detection relied on `HasPrefix` against the bare gate sentence.
- confirmed cause: Grok embeds internal prompts inside a history preamble + reinforcement `<user_query>` block; `HasPrefix` (and role-blind matching) then misclassifies the wrapped prompt as a user turn.
- evidence: `run-104296-turns.ndjson` / Grok `chat_history.jsonl` show the wrapped block; the new regression test reproduces the replay order (`[Q1, Q2, Q3]` vs `[Q1, gate, Q2, Q3]`).

## 7. Fix Strategy

- `F-1` Introduce the `[SYSTEM_PROMPT]` tag as the single source of truth for "this prompt was generated by the system": stamped once, last-mile, for every internal prompt before it reaches the provider and the turn log.
- `F-2` `isSystemPrompt` returns `true` on tag presence, then falls back to legacy detectors; `isGateReprompt`/`isFlowEnginePrompt` are hardened (`Contains`, tag-aware) so pre-tag runs still replay correctly.
- `F-3` Additive regression tests: wrapped-gate replay keeps QA pairing, tag-stamped prompts are system, legacy detector parity, and overlay slot math with a wrapped gate.

## 8. Validation

- `V-1` New tests: `TestRun104296WrappedGateReplayKeepsQAPairing`, `TestSystemPromptTagStampedPromptsAreSystem`, `TestLegacySystemPromptDetectorsProviderParity`, `TestOverlayRawTurnPromptsWrappedGateSlotsDirect` — all green (`ok flowpilot-runner/internal/runner 1.921s`).
- `V-2` Prior overlay/replay/gate/feature battery still green (`-run 'TestRun2334|TestBug083|TestBug293|TestBug306|TestRun1264|TestBug295|TestBug289|TestGate|…'` → ok 12.055s); full runner package fails only on the unrelated `TestGeminiAdapterPromptPrepAndEnv` env flake (passes in isolation).
- `V-3` `go build ./...` and `go vet ./internal/runner/` clean; `gofmt` clean for all changed files.

## 9. Regression Guard

- tests: `run104296_system_prompt_tag_replay_test.go` (new); existing run2334/bug293/bug306/run1264/durable-resume/run24377 suites unchanged and green.
- alerts: none.
- audit checks: `feature_key: chat-history`; do not regress CA-101/371/380/386/501.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `isSystemPrompt` legacy detectors intentionally retained for backward compatibility; future runs can rely on the tag alone.
