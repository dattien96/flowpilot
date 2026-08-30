# CA-683 — Opencode stats surface end-to-end: runner cache + desktop account card

## Problem

CA-682 populated real `opencode stats` usage lines runner-side, but:

1. Desktop `compactUsageLines` still returned `[]` for opencode (Task-302 T-6
   guard against fabricated 0% meters that no longer exist), so the pin card
   hid the real stats; the modal detail path did render them, but the pin card
   (the always-visible account card) showed only "Limit: N/A".
2. `loadOpencodeAccountMetadataInternal` ran the multi-second stats CLI on
   EVERY `/client/provider-accounts` request — the endpoint fans out per
   account on every TUI connect and desktop panel open.

## Change

- Desktop `ProviderAccountsPanel.tsx`: drop the opencode early-return in
  `compactUsageLines`; the opencode pin card now shows the "Limit: N/A (zen
  proxy)" chip plus up to 2 real stats lines as text subs (never percent
  meters). Gemini filter unchanged; modal text-row rendering unchanged.
- Runner `opencode_account_stats.go`: 60s in-memory cache per home (failures
  cached too, so a wedged CLI cannot stall every account refresh).

## Tests

- `TestOpencodeStatsCacheDedupesWithinTTL` — cache serves within TTL without
  re-running the CLI.
- `TestParseOpencodeStatsTable` (CA-682) still green; runner opencode/grok
  subsets green; desktop `npm run typecheck` — zero errors in touched files
  (the 8 pre-existing errors live in `store.run75035-timeline-isolation.test.ts`
  WIP on this branch, untouched by CA-683).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Opencode stats reach the desktop account card with a 60s runner-side cache; fabricated-meter guard replaced by real stats lines
# --->8---
