# CA-383 — Transcript merge untagged-duplicate fix

## Summary

Fixed BUG-303, found while running a full-suite regression baseline to verify
an unrelated change (BUG-302): `mergeTurnLogAssistantsIntoTranscript`'s
text-based duplicate fallback only ran when the incoming turn-log entry
itself had no `TurnID` — never when the entry had a `TurnID` but the
historical event it should match was untagged (`ProviderTurnID == ""`,
common for older-format transcripts or synthesis turns never stamped). A
genuinely-already-present message got inserted a second time on restore.

Widened the fallback to run whenever the ID-based check finds nothing, but
narrowed its match target to only an untagged historical event
(`transcriptHasUntaggedAssistantText`) — never one already tagged with a
DIFFERENT turn id, which is a distinct, real event whose text happens to
coincide, not a duplicate.

Also fixed, in the same change-audit note as an aside (not worth a separate
BUG doc — a one-line stale constant): `TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted`
hardcoded `len(statuses) != 3`, predating Grok's addition as the 4th supported
provider (`resolveGoogleDriveMcpProviderStatuses` iterates
`["codex","gemini","claude","grok"]`). Updated the assertion to 4 — not
environment-dependent, would fail identically anywhere; user approved the
edit.

## Cross-provider parity

Classification: **Case 1, provider-agnostic** for the merge fix — confirmed
by reading (`mergeTurnLogAssistantsIntoTranscript` takes `[]ProviderEvent`/
`[]turnLogLine`, never `providerKey`) AND by the existing cross-provider test
`TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider`
(parameterized over Codex/Claude/Grok) passing with this fix.

## additive-tests-only compliance

Zero test files edited to fix the target test
(`TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis`) — it was
already red on unmodified code (a genuine pre-existing bug, not introduced by
any in-flight change) and is fixed purely via production code, per the
skill's own prescribed remedy. The one test edit in this note
(`TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted`'s constant) was
explicitly user-approved before editing.

## Verification

- An initial draft of this fix (unconditional text match, ignoring the
  historical event's own tag) made the target test pass but broke
  `TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider`
  (all 3 provider subtests) — caught immediately by running the targeted
  battery, confirmed via git-stash that this pre-existing test was never
  broken on unmodified code. Refined to only match untagged historical
  events; re-verified via git-stash both ways: target test fails without any
  fix / passes with the refined fix; Run20332 passes with both the refined
  fix and no fix.
- `go test ./internal/runner -run 'TestMergeTurnLogAssistants|TestRun20332|TestRun2334|TestRun1264|TestRun5695|TestRun9034|Restart|Resume|Replay|Transcript'` — 196 passed, only pre-existing environment failures (missing `codex` binary) remain.
- Full-suite `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/` — down to the known pre-existing environment-only failure set, alongside BUG-302 and BUG-304's fixes.
- `go build ./...` passes.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-303
change_type: bugfix
summary: Recognize an untagged historical duplicate for a turn-log entry that has a durable TurnID, without conflating it with a different, already-tagged turn that happens to share the same text; also correct a stale Google Drive provider-count test assertion (3 to 4, after Grok's addition).
# --->8---
