# CA-578: Pin Grok 7d / Claude 5h quota to collapsed F4 status line (without Team UUID)

## What

Expanded `▾ F4` showed quota on the account row; collapsed `▸ F4` hid it, so `7d:NN%` disappeared on F4. Added `collapsedQuotaChip` that pins compact quota (`5h:NN%` dim, `7d:NN%` highlight) to the always-visible line0 right after the fold chip, while suppressing the Grok `UsageSummary` `"Team <uuid>"` / `"Personal"` which is a team marker, not a quota (Desktop never renders it as quota).

## Why

- CA-448 pinned remaining as first chip so `fitStatusWidth` truncation doesn't hide it; multi-row split moved quota to the last chip of the account row, re-introducing truncation.
- Grok `auth.json` sets `UsageSummary` to `Team <id>`; TUI fallback to `UsageSummary` rendered the UUID as if it were quota on both expanded and collapsed, as seen in the screenshot (`Team 54b113be-a4be…` on both `▾ F4` and account row). Desktop parity is to not render that string as quota.

## Fix

- `apps/local-runner/internal/tui/app/status_bar.go:99` — `renderStatusLine0` prepends `collapsedQuotaChip()` after fold chip.
- `status_bar.go:138` — `collapsedQuotaChip()`: `Remaining5hPercent`/`Remaining7dPercent` → compact chips; else weekly/7d `UsageDetailLines`; else first detail line (e.g. Team Credits); never `UsageSummary` Team UUID. No `· resets` suffix on collapsed; expanded keeps full detail.
- `status_bar.go:259` + `helpers.go:501` — guard `UsageSummary` fallback: ignore when `Team ` prefix or `Personal`, preserving real plan summaries (Claude "Pro").
- `internal/tui/app/f4_quota_pin_test.go` (additive): collapsed Grok 7d, Claude 5h, both, weekly fallback, Team Credits fallback, Team UUID not shown (collapsed+expanded), expanded+collapsed both show 7d.

## Tests

- `go test ./internal/tui/app -run TestF4 -count=1` 7/7 pass; full `go test ./internal/tui/app -count=1` 13.5s green.
- Screenshot repro (`Team 54b113be…` on both rows) no longer renders UUID as quota; expanded shows `grok | email` only when quota missing.

## Residual

- When Grok billing `GET /v1/billing?format=credits` returns no `currentPeriod` and no `weeklyLimit`/`monthlyLimit`, quota is genuinely absent; collapsed correctly shows no chip (not a fake Team UUID). Investigate `loadGrokQuota` HTTP status if `7d` stays absent after re-login.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: pin Grok 7d/Claude 5h quota to collapsed F4 line0 and suppress Team UUID UsageSummary as fake quota
# --->8---
