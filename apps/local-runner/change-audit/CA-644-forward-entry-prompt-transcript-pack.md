# CA-644 — CP-89 Task-453: Forward Entry Prompt Transcript Pack

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: CP-89 / Task-453

---

## Problem

When a pending run forwards, the flow's entry child must see BOTH the
user's forward text and the settled chat transcript that preceded it —
under the entry node's hard context budget. A bare forward
(`{"forwardFlow":true}`) must still produce a valid transcript-only
package, and the transcript must degrade oldest-first instead of
evicting the pinned forward text.

## Change

- `ForwardPromptPackage{Prompt, TurnsIncluded, Bytes, Degraded}` +
  `buildForwardPromptPackage(rs, forwardText, budget)`:
  - transcript via `settledChatTurnsForRun` — the runner's own event
    store (`transcriptTurnsFromRun`), user+assistant pairs only;
    system-prompt frames (`isSystemPrompt`) excluded; a trailing
    in-flight `TurnStarted` is dropped (unsettled).
  - oldest-first degradation via `packConversationTurns` (the existing
    newest-kept/oldest-dropped packer with omission markers);
  - forward text as a `SectionCurrentTask` mandatory section —
    pinned intact, never dropped or truncated;
  - assembled + audited through `promptpacker.PackPrompt`
    (`TotalMaxTokens = budget`); deterministic for identical state.
- `forwardEntryBudgetTokens`: resolves the pinned flow's first
  spawnable entry node's `contextProfile.MaxEstPromptTokens`
  (Task-341 schema); falls back to 8000 tokens when unprofiled.
- `startTurn` forward branch: pack → (pack failure keeps pending +
  releases `turnInFlight`) → latch flip → emit `forward_prompt_packed`
  diag entry (`turns_included`, `bytes`, `degraded`) →
  `startResolvedFlow` receives `pkg.Prompt` as the entry prompt.
- Deviation note: `settledChatTurnsForRun` returns `[]transcriptTurn`
  (existing settled-turn shape) instead of a new `ChatTurn` type.

## Files

- `internal/runner/forward_prompt.go` — package + collector + budget
  resolver.
- `internal/runner/interactive_service.go` — forward branch wiring +
  audit emit.
- `internal/runner/task453_forwardprompt_test.go` — 8 additive tests.

## Test evidence

- `TestTask453_*` ×8 green: forward+transcript content, settled-only,
  role filtering, budget cap degrades oldest first, empty transcript →
  text-only, bare forward packs, deterministic replay, audit entry.
- `-race` on `TestTask45[123]` green.

## Risk / blast radius

- Read-mostly: the packer never mutates `rs`; the only new producer of
  `forward_prompt_packed` entries is the forward seam itself. No
  transcript store was added — rs.events remains the single source.
