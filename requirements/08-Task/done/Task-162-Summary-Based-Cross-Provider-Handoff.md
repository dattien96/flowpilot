# Task-162: Summary-Based Cross-Provider Handoff (Handoff Phase 2)

## Metadata

- Document ID: `Task-162`
- Title: `Summary-Based Cross-Provider Handoff`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `None`
- Related Documents: [CP-37: Prompt Context Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md), [Task-078: Cross-Provider Chat Handoff](Task-078-Cross-Provider-Chat-Handoff.md), [Task-161: Per-Feature Chat-Summary Timeline](Task-161-Per-Feature-Chat-Summary-Timeline.md), [CP-10: Integrations, Memory & Context Intelligence](../../07-Coding-Plan/done/CP-10-Integrations-Hardening.md)
- Replaces: `None`
- Tags: `desktop, chat, cross-provider, handoff, summary, summarizer, hybrid, phase-2`

## AI Quick View

### Summary

- **Fills the slot Task-078 leaves.** Task-078 ships the *raw* transcript handoff (deterministic, bounded, always works). This task adds the **AI-summary** layer on top — fixing Task-078's main weakness: raw newest-first truncation drops the *oldest, most important* framing (the original goal/decisions) on long chats.
- **Raw stays the floor; summary is an enhancement.** The switch must never fail (the user is often switching *because* they hit the provider's token limit and have zero budget left). So this task layers quality on a guaranteed base, with graceful degradation.
- **Hybrid layout.** Handoff content = `[AI summary of older turns]` + `[most-recent K turns verbatim]`. Preserves the arc (summary) *and* the precise current state (raw), within the same 64 KiB budget — strictly better than pure-truncation or pure-summary.
- **Reuses Task-161's summarizer.** The summarizer is built once in Task-161 (per-feature chat-summary timeline) and consumed here for the live handoff. No second summarizer. No RAG.
- **Division of labor with Task-157.** The handoff carries *what was discussed*; Task-157 auto-injects *what was done in code* on the target run's first turn (shared seam). So the summary stays focused on discussion, not code changes.

### Current Ask

- The summary-enhanced hybrid handoff is now delivered: raw transcript transfer remains the floor, and the older turns are condensed with Task-161's summarizer when available.

### Key Decisions

- `T-1` **Fallback ladder (pick best available, degrade gracefully):** (1) **Hybrid** = rolling summary + recent K raw; (2) **Target self-summary** = raw transcript + an envelope instruction for the target to summarize-then-continue (target has fresh budget); (3) **Raw floor** = Task-078's newest-first truncation. The switch never blocks on a model call.
- `T-2` **Rolling summary, not switch-time.** Maintain the summary *during* the chat (reuse Task-161's rolling summarizer) so it already exists at switch — surviving the token-limit case, where summarizing with the exhausted source provider is impossible.
- `T-3` **Cache by transcript state.** Key the summary on `state_key = hash(last_committed_turn_id + committed_turn_count)`. Cancel→reopen with no new turn → reuse (no regen); chat more → refresh; covers the retry/edit case that hashing only the last response would miss.
- `T-4` **Cheap, separate summarizer model.** So it does not drain the primary provider's scarce budget and is not blocked when the primary is rate-limited (same model as Task-161 `T-2`).
- `T-5` **Hybrid budget split.** Fill the most-recent K turns raw first (precise state), then prepend the rolling summary for everything older (the framing that would otherwise truncate). Slight summary/recent overlap is acceptable — the raw turns ground the lossy summary.
- `T-6` **Storage (extends Task-078 `T-9`, run-metadata JSON):** `handoff_summary`, `handoff_summary_state_key`, `handoff_summary_model`, `handoff_summary_updated_at`, and `handoff_mode: hybrid | target_summary | raw` for diagnostics.

### Constraints

- **Never block the switch on the summary.** Any summarizer failure/timeout degrades one tier; raw floor is always available.
- Reuse Task-078's bounded-block budgeting helper (UTF-8-safe truncation, `</previous_conversation>` escaping, omission markers) — do not reinvent.
- Reuse Task-161's summarizer — do not build a second one.
- Summarize **committed** turns only (never an in-flight turn).
- Same privacy posture as Task-078: the summary still crosses the vendor boundary (Task-078 `Q-2` review/redaction question applies — arguably more relevant since a summary may condense sensitive content).
- Do not inject feature/commit history here — Task-157 supplies it automatically on the target run's first turn via the shared seam.

### Open Questions

- `Q-1` Does the privacy/pre-send-review question (Task-078 `Q-2`) need resolving *before* shipping summaries, given a summary is condensed but still sensitive content crossing a vendor boundary?
- `Q-2` When no rolling summary exists (e.g. a chat from before this feature shipped), prefer **target self-summary** or **raw floor**? Proposal: target self-summary if the target has budget, else raw.
- `Q-3` Should the hybrid include the rolling summary of the *whole* conversation (simpler, mild overlap) or only the older-than-recent-K portion (less overlap, needs the split point known at summarize time)? Proposal: whole-conversation summary + recent-K raw (simpler, robust).

### Source Refs

- Builds on Task-078 (raw floor + slot, `T-5`/`T-6`/`T-7`/`T-9`) and Task-161 (summarizer). `SS-11 §5.1`/`§6`, `SD-12 §3.4`. CP-10 carries the reciprocal "AI-summary context pipeline" follow-up.
- Code: `apps/local-runner/internal/runner/{interactive_handlers.go,interactive_resume.go,transcript_loader.go}`, the Task-078 bounded-block helper, the Task-161 summarizer, `apps/desktop-flowpilot/src/state/store.ts` (handoff orchestration).

## 1. Goal

Upgrade the cross-provider handoff content from raw-only (Task-078) to a summary-enhanced hybrid that preserves the conversation's framing on long chats and survives the zero-budget token-limit case, by reusing Task-161's rolling summarizer — while keeping Task-078's raw transfer as the guaranteed floor.

## 2. Parent Links

- coding plan: `CP-18` §5.6/§5.7 (handoff); CP-10 (reciprocal AI-summary follow-up)
- tech design: `SD-12` §3.4 (provider-neutral transcript extraction + provenance)
- system spec: `SS-11` §5.1/§6
- specific upstream ids: phase-2 successor to Task-078; consumer of Task-161's summarizer

## 3. Trigger

Task-078's raw handoff truncates newest-first, dropping the oldest turns — which in a work chat are usually the original goal and the framing decisions the new provider most needs. A summary fixes this, but cannot replace raw transfer because the most common switch trigger (hitting the provider's token limit) leaves no budget to summarize with the source. Task-161 builds a rolling summarizer that already maintains a summary during the chat; this task consumes it for the handoff, with raw as the floor.

## 4. Exact Change

- `T-1` `runner` — implement the fallback ladder at handoff assembly: hybrid (Task-161 rolling summary + recent K raw) → target self-summary (raw + envelope instruction) → raw floor (Task-078). Record `handoff_mode`.
- `T-2` `runner` — wire Task-161's rolling summarizer + `state_key` cache into the source chat lifecycle so a current summary is available at switch.
- `T-3` `runner` — hybrid assembly: recent-K raw fill first, rolling summary prepended for older context, within the 64 KiB budget, reusing Task-078's bounded-block helper.
- `T-4` contract/metadata — add the `handoff_summary*` + `handoff_mode` fields (Task-078 `T-9` extension); surface `handoff_mode` in diagnostics.
- `T-5` desktop — no UX change required beyond Task-078; the handoff prompt still appears as the first user turn (now summary+recent instead of raw-truncated).
- `T-6` tests — hybrid layout assembled within budget; degrade to target-summary then raw on missing summary / model failure; cache reuse on cancel-reopen and refresh on new turn; committed-turns-only; `handoff_mode` recorded; switch never blocked by summarizer failure.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/{interactive_handlers.go,interactive_resume.go,transcript_loader.go}`, Task-078 bounded-block helper, Task-161 summarizer, `apps/desktop-flowpilot/src/state/store.ts`, focused runner/desktop tests
- modules: runner transcript/handoff assembly, summarizer (shared, from Task-161), desktop run state
- routes: reuses Task-078's `POST /client/workflow-runs/{runId}/handoff-context` (now returns hybrid content + `handoff_mode`)
- tables: none (run-metadata JSON, per Task-078 `T-9`)

## 6. Acceptance Check

- A handoff on a long chat includes a summary of earlier turns plus the most-recent K turns verbatim, within 64 KiB.
- When no rolling summary exists, the handoff degrades to target self-summary or raw floor without failing.
- A summarizer failure/timeout never blocks the switch; `handoff_mode` reflects the path taken.
- Cancel→reopen with no new turn reuses the cached summary; a new turn refreshes it; retry/edit invalidates via `state_key`.
- The summary carries discussion (goals/decisions), not a re-list of code changes (Task-157 supplies those on the target's first turn).
- The summary is generated by the cheap shared summarizer, not the (possibly exhausted) source provider.
- Targeted Go + desktop tests, typecheck, and builds pass.

### 6.1 Definition of Done (DOD)

All items are true for Task-162:

- [x] **DOD-1:** the handoff uses a summary-plus-raw hybrid when a cached summary exists.
- [x] **DOD-2:** when no cached summary exists, the handoff degrades to target self-summary or raw without blocking.
- [x] **DOD-3:** handoff mode is recorded for diagnostics and reflects the actual path taken.
- [x] **DOD-4:** the cached summary is keyed by transcript state and refreshes after new committed turns.
- [x] **DOD-5:** the shared summarizer is reused unchanged from Task-161 (the handoff reads the cached `chat_summary` entry produced by `Runner.SummarizeChatTranscript` — the cheap-tier model of the chat's own provider; it builds no second summarizer).
- [x] **DOD-6:** the target run still receives Task-157 feature history automatically on its first turn.
- [x] **DOD-7:** targeted Go and desktop tests, typecheck, and builds pass.
- [x] **DOD-8 (switch never blocked, three-tier ladder):** a summarizer failure/timeout, a missing cached summary, or the cheap model being unavailable never blocks the switch. `handoffMode` records which of the three tiers was used: `hybrid` (state-matched cached summary + recent raw), `target_summary` (no summary **and** the raw floor had to drop/truncate history → the envelope asks the target to self-summarize and mind the gaps), or `raw` (no summary but the whole conversation fit, so the complete raw transcript is sent with no extra instruction). The raw transcript is always the floor beneath all three.
- [x] **DOD-9 (run-scoped + state-keyed selection):** the hybrid summary is selected only from the source run's own cached summaries whose `state_key` matches the current transcript, so a newer summary recorded on another run for the same feature is never imported.

## 7. Out of Scope

- The summarizer implementation itself — built in Task-161 and reused here.
- RAG / semantic retrieval (CP-10).
- Pre-send context review/redaction UX (Task-078 `Q-2`) unless `Q-1` forces it.
- Feature/commit history injection (Task-157, automatic on the target's first turn).
- Gemini as a handoff source (Task-078 boundary).

## 8. Completion Notes

- result: done
- implementation notes: handoff assembly in [handoff_context.go](../../../apps/local-runner/internal/runner/handoff_context.go) packs the raw conversation first, then labels `handoffMode`: `hybrid` when a `state_key`-matched cached summary exists for the source run; else `target_summary` when packing dropped/truncated history (a self-summarize envelope instruction over the raw transcript); else `raw` (full conversation present, no summary and no instruction). The raw transcript packing (Task-078 `packConversationTurns`, 64 KiB, newest-first, `</previous_conversation>` escaping) is always the floor underneath all three. The summary itself comes from the Task-161 cheap-model summarizer, not the (possibly exhausted) source provider.
- follow-ups: privacy/pre-send review of the condensed summary crossing the vendor boundary (`Q-1` / Task-078 `Q-2`) remains an independent follow-up; the hybrid handoff slice itself is complete.
- upstream docs updated: `CP-18 §5.7` and the related handoff notes already describe the delivered cross-provider path and its summary follow-on.
