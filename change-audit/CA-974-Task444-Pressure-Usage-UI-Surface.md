# CA-974 — Task-444: pressure & usage UI surface (Desktop + TUI)

## Summary

The CP-86 signals were invisible without rendering — and the old surface
would mislabel a prompt-length estimate as tokens. Users now see two honestly
labeled figures (`prompt ~Nk est` vs `usage Nk`, `—` when absent), an inline
banner at aware pressure, and a pinned compaction notice — with interruption
reserved for real decision cards via the existing question path.

## What changed

- `internal/runner/provider_event.go` — `TokenUsageSnapshot.EstPromptTokens`
  (`estPromptTokens`), the runner's own len(prompt)/4 estimate.
- `internal/runner/interactive_service.go` — emitLocked stamps the est on
  every `token_usage_updated` (pure in-memory).
- `apps/desktop-flowpilot/src/types/contract.ts` — `ContextPressurePayload`
  + `context_pressure`/`provider_compacted` union members + `estPromptTokens`.
- `apps/desktop-flowpilot/src/state/store.ts` — `contextNotice` state (on
  `AppState` + `RunSnapshot` so focus switches keep it); `applyEvent`
  (exported for tests) sets/clears it: aware clears when a later usage event
  drops below tier, compacted stays pinned, leg change auto-clears via the
  providerSessionId pin; turn_started does NOT clear (pressure persists).
- `src/components/ContextUsageNotice.tsx` — new `UsageFigures` +
  `ContextNotice` (inline `role="status"` lines, no buttons/dialog).
- `src/components/ChatInput.tsx` — mounts the notice + figures beside the
  existing usage line.
- TUI: `client.go` `ContextPressurePayload`/`EstPromptTokens`/
  `ProviderSessionID` on ProviderEvent; `app.go` tracks `ctxStatus` pinned to
  `ctxStatusLegID` (leg change resets), aware clears below 0.8, compaction
  posts an inline system line; `helpers.go` `formatContextStatus` composes
  `formatContextLimits` (signature unchanged — spec's 4-arg signature would
  have broken existing callers; deviation documented in Task-444 §11) adding
  `prompt ~Nk est` + `!ctx ~NN%` + `compacted prev→cur`; session_panel uses
  the wrapper.

## Tests

- New `ContextUsageNotice.render.test.tsx` — 4 tests (labeled figures,
  dash-not-zero, banner-not-card, compacted pinned notice).
- New `store.context-notice.test.ts` — 4 ingest tests (aware lands, compacted
  pinned with prev→cur, below-tier clear, leg-rotation clear).
- New `task444_ui_surface_test.go` (runner) + `task444_context_status_test.go`
  (TUI) — est field + JSON key, formatter table, handleEvent tracking,
  clear-on-drop, leg reset.
- `tsc --noEmit` clean; go build clean.

## Honest gaps

- Ask-tier banner shares the aware marker (the decision card itself is the
  differentiator); a distinct tint is a cosmetic follow-up.
- Awareness notices intentionally never create attention-inbox entries
  (CP-84 semantics preserved).

# ---8<--- flowpilot:change-ledger
feature_key: token-usage
source_doc_id: Task-444
change_type: feature
summary: est-vs-usage honest labels + inline pressure/compaction notices on desktop (ContextNotice/UsageFigures + store ingest) and TUI (formatContextStatus + ctxStatus leg-pinned marks); decisions stay on user_question_required
# --->8---
