---
id: CA-933
title: TUI stream-open run identity + resolved-model propagation (BUG-450, BUG-452)
type: BugFix
feature: cli-tui
date: 2026-09-23
status: done
---

## Context

- BUG-450: BUG-428's fix guarded stale stream events/close but not a stale
  `orchStreamOpenedMsg` — a late open from an old run/leg could cancel or
  displace the active stream of the newer leg after a provider switch.
- BUG-452: the runner's switch response echoed the *requested* model
  (`Model: req.Model`) and the TUI filled an empty response from catalog order
  (`catalog[0]`) — neither reflected the resolved leg model, so the footer
  could display a model the new leg was not running.

## Change

`internal/tui/app/turn_stream.go` + `internal/tui/app/app.go`:

- `orchStreamOpenedMsg` carries the originating run/leg identity; stream-open
  application is guarded so only the current run/leg can open/displace stream
  state — a stale open from the previous leg self-drops.

`internal/runner/chat_switch.go`:

- Switch responses return the resolved leg model (`newLeg.modelName`,
  falling back to `defaultModelForProvider`) instead of echoing the request.

`internal/tui/app/chat_switch.go`:

- Switch handling prefers `TargetModel`/the response's resolved model before
  catalog ordering; an empty response no longer displays `catalog[0]`.

## Tests (added only)

- `internal/tui/app` — `TestBug450_StaleOrchOpenDoesNotDisplaceNewLegStream`
  (red before: stale open cancelled the new leg's stream);
  `TestBug452_EmptyResponsePrefersRequestedOverCatalogOrder`.
- `internal/runner` — `TestBug452_SwitchResponseReturnsResolvedLegModel` in
  `bug451_452_model_truth_test.go`.

## Result

- `go test -count=1 ./internal/tui/app` focused tests green;
  `./internal/runner` model-truth tests green.
- Provider-agnostic: stream identity is transport-level, model truth flows
  through the shared switch path.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-450
change_type: bugfix
summary: Stale stream-open can no longer displace a new leg's stream; switch responses carry resolved model truth preferred over catalog order
# --->8---
