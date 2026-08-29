# CA-687 — TUI /provider refresh: the "Detect models" equivalent

## Problem

Operator report: the TUI model list showed only grok-4.5 while the runner
catalog already carried grok-4.6. The TUI fetches the provider catalog once at
session start (8s budget, CA-535) and keeps it for the whole session — a model
detected later (new grok release, first opencode sync) never appears until a
full restart. Desktop has a "Detect models" button for exactly this; the TUI
had no equivalent.

## Change

- `/provider refresh` (alias `reload`): re-fetches GET /providers via the
  existing `cmdLoadProvidersCatalog` (20s budget) and flows through the
  normal `ProvidersCatalogMsg` handler — catalog replaced in place, current
  provider/model selection kept, model-aware reasoning lists re-derive
  automatically (CA-686), skills reload. Safe during an active run (read-only).
- Hints added: `/provider` list footer and `/model` list footer; the
  "No provider catalog loaded yet" hint now points at `/provider refresh`.

## Tests

- `TestProviderRefreshCommandDispatchesCatalogLoad` — dispatches the catalog
  load, selection untouched, refreshed catalog lands (grok-4.6 appears).
- Full tui/app suite: only the 7 documented pre-existing render failures.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-304
change_type: feature
summary: TUI provider refresh command reloads the model catalog without a restart, matching the Desktop detect-models button
# --->8---
