# BUG-303 — Transcript merge misses a duplicate when the historical event is untagged

## Metadata

- Document ID: `BUG-303`
- Title: Transcript merge misses a duplicate when the historical event has no ProviderTurnID
- Phase: `bugfix`
- Status: `done`
- Owner: local-runner
- Reviewers: n/a
- Created: 2026-07-21
- Last Updated: 2026-07-21
- Parent Documents: CA-379 (run-20332 history replay parity), BUG-300 (flow-hub first prompt lost on restart)
- Child Documents: none
- Related Documents: BUG-304 (companion — the test-only race discovered alongside this one during the same full-suite verification pass)
- Replaces: none
- Tags: chat-history, agent-flow-engine, regression

## AI Quick View

### Summary

- Found while running a full-suite regression baseline to verify BUG-302 — a pre-existing, already-red test (`TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis`), unrelated to BUG-302 itself.
- `mergeTurnLogAssistantsIntoTranscript` (the shared restore/replay dedup pass used across Codex, Claude, and Grok) only falls back to text-based duplicate detection when the **turn-log entry itself** has no `TurnID`. It never falls back to text matching when the entry DOES have a durable `TurnID` but the historical event it should match is untagged (no `ProviderTurnID` of its own) — which happens for older-format transcripts or synthesis turns that were never stamped.
- Net effect: a message that is genuinely already present in the replayed transcript (just untagged) gets inserted a **second time** on restore, because neither the ID-based nor the (too-narrowly-gated) text-based check recognizes it.

### Current Ask

- Recognize an untagged historical duplicate for a turn-log entry that DOES carry a durable `TurnID`, without breaking the existing, deliberate behavior that two DIFFERENT, already-tagged turns are allowed to share identical text (never conflate them just because their content coincides).

### Key Decisions

- `V-1` Widen the text-based fallback to run whenever the ID-based check finds nothing — but only match it against a historical event that is **itself untagged** (`ProviderTurnID == ""`). A historical event already tagged with a DIFFERENT turn id is a distinct, real event whose text happens to coincide — never treated as this entry's duplicate.

### Constraints

- Must not regress `TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider` (Codex/Claude/Grok), which specifically guards that two distinct, already-tagged turns sharing identical response text are BOTH preserved, not collapsed.
- additive-tests-only: the target test (`TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis`) is pre-existing and already red before this fix — fixed via production code per the skill's own prescribed workflow ("prefer: fix production code"), never edited.
- cross-provider-parity: verify the shared merge function and its cross-provider test coverage.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_resume.go` (`mergeTurnLogAssistantsIntoTranscript`, `transcriptHasAssistantForTurn`, `transcriptHasUntaggedAssistantText`)
- `apps/local-runner/internal/runner/run5695_resume_agent_timeline_order_test.go` (`TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis`)
- `apps/local-runner/internal/runner/run20332_multi_turn_hub_resume_test.go` (`TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider` — the regression guard this fix must not break)

## 1. Issue Summary

While capturing a full-suite regression baseline (unrelated to the change being verified), `TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis` was found already failing on the current `develop`-derived branch, independent of any in-flight change. Root-caused as a real, pre-existing gap in the durable-transcript merge/dedup logic used by every restart/resume/history-reopen path.

## 2. Parent Links

- impacted coding plan: CA-379's durable replay contract (Codex/Claude/Grok parity)
- impacted tech design: n/a
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: pure in-memory Go logic, no OS/provider/network dependency
- reproduction: `go test ./internal/runner -run TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis -v` — fails on unmodified code
- frequency: deterministic whenever a historical (provider-file-loaded) transcript event has no `ProviderTurnID` and the durable turn log's matching entry does have a `TurnID`

## 4. Expected vs Actual

- expected: merging turn-log entries into a replayed transcript inserts only genuinely missing responses; an already-present (if untagged) response is recognized and skipped.
- actual: the already-present untagged response is inserted a second time, producing a duplicate message in the restored transcript.

## 5. Impact

- users affected: anyone reopening/resuming a run whose historical transcript mixes untagged (older-format or synthesis) events with newer, `TurnID`-stamped turn-log entries
- workflows affected: restart/resume/history-reopen across Codex, Claude, and Grok (shared merge function)
- severity: low-medium (cosmetic duplicate message on replay; does not affect the live run or its actual outcome)

## 6. Root Cause

- hypothesis: the text-based dedup fallback in `mergeTurnLogAssistantsIntoTranscript` is gated on the wrong condition.
- confirmed cause (`apps/local-runner/internal/runner/interactive_resume.go`):
  ```go
  turnID := strings.TrimSpace(e.TurnID)
  if turnID != "" && transcriptHasAssistantForTurn(out, turnID) {
      continue
  }
  if turnID == "" && transcriptHasAssistantText(out, text) {   // only when e.TurnID is empty
      continue
  }
  ```
  The text-based fallback only ran when the **incoming turn-log entry** (`e.TurnID`) had no id — never when the entry had an id but the **historical event it should match** was the untagged one. A turn-log entry with `TurnID: "turn-1"` whose matching historical event has no `ProviderTurnID` at all therefore falls through both checks and gets inserted as a duplicate.
- evidence: `TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis` reproduces exactly this shape (historical `EventMessageCompleted` with no `ProviderTurnID`, turn-log entry `TurnID: "turn-1"` with matching text) and fails on unmodified code with a duplicate "Round 0: changes requested" message.

## 7. Fix Strategy

- `F-1` Widened the fallback to run whenever the ID-based check finds nothing (not gated to `e.TurnID == ""`), but matching only against a historical event that is **itself untagged** (`transcriptHasUntaggedAssistantText`, replacing the old unconditional `transcriptHasAssistantText`). A historical event already tagged with a different turn id is never treated as a match, regardless of text — preserving the existing "two distinct tagged turns can share identical text" guarantee.

## 8. Validation

- `V-1` **cross-provider-parity (Case 1, provider-agnostic — confirmed by reading):** `mergeTurnLogAssistantsIntoTranscript` and its helpers take `[]ProviderEvent`/`[]turnLogLine`, never `providerKey`, and never branch on one. Additionally, this specific fix is directly exercised by the existing `TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider`, which is already parameterized over Codex, Claude, and Grok — all three subtests pass with the fix (and would have failed had the fix been too broad, which an earlier draft of this fix — matching only on text, unconditionally — in fact did; see below).
- `V-2` **additive-tests-only:** zero test files edited to make the TARGET test pass — `TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis` was fixed purely via production code, per the skill's own prescribed remedy for an old test failing on unmodified code.
- `V-3` **Regression discipline on the fix itself:** an initial draft of `F-1` (falling back to `transcriptHasAssistantText`, matching ANY historical event's text regardless of its own tag) made the target test pass but broke `TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider` (all 3 provider subtests) — confirmed via git-stash that this pre-existing test passed on unmodified code and failed only with the overly-broad draft. Refined to `transcriptHasUntaggedAssistantText` (matches only an untagged historical event), confirmed via git-stash: the target test still fails without any fix and passes with the refined fix; `TestRun20332...` passes with both the refined fix and no fix (never broken).
- `V-4` Full-suite regression: `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1 -timeout 10m` and a targeted 200+-test restart/resume/replay/transcript battery — both green with this fix plus BUG-302's and BUG-304's fixes applied together; failure list reduces to (and does not exceed) the known pre-existing environment-only set.
- `go build ./...` passes.

## 9. Regression Guard

- tests: the now-passing pre-existing `TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis`; the already-existing `TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider` as the cross-provider anti-regression guard for this exact fix.
- alerts: n/a
- audit checks: none beyond the existing restart/replay battery.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: the ID-based dedup path, the causal insertion-index logic, and every other branch of `mergeTurnLogAssistantsIntoTranscript` are untouched — this fix only widens the text-fallback's trigger condition and narrows its match target.
