---
name: BUG-089-Remote-Restore-UX-And-Provider-Model-Auto-Select
description: Multiple UI/UX defects in the Remote Chats restore section, provider model auto-selection, sync icon semantics, and active account card reset-time display.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-089`
- Title: Remote Restore UX Gaps, Provider Model Auto-Select Missing, Sync Icon Wrong, Active Card Missing Reset Time
- Phase: `bugfix`
- Status: `done`
- Owner: DatNguyen
- Reviewers: —
- Created: 2026-06-19
- Last Updated: 2026-06-19
- Parent Documents: `Task-018-Auto-Switch-Account.md`
- Child Documents: —
- Related Documents: `BUG-081-Desktop-Sidebar-Sync-Button-Shows-Reload-Icon.md`, `BUG-088-Desktop-History-Sync-Actions-Missing-In-Progress-Indicator.md`
- Replaces: —
- Tags: ui, navigator, accounts, model-selection, restore, severity-medium

## AI Quick View

### Summary

- The Remote Chats section had text-only "Restore" buttons with no icon, no per-item loading state, and no bulk restore action.
- After a successful restore the button remained visible until the list reloaded — no immediate feedback.
- Selecting a provider (Codex or Claude) did not auto-select a sensible default model; users had to pick manually every time.
- The sync icon (upload to Drive) remained a generic two-arc sync shape after BUG-081; the user requested a directional upload-arrow icon.
- The active account card in the sidebar showed usage percentages but omitted the reset-time stamps visible in the expanded detail modal.

### Current Ask

- Icon-only restore button with loading spinner per item.
- "Restore All" bulk action in the Remote Chats header.
- Hide restore button on items that are unavailable (already have `unavailableReason`).
- Auto-select `o4-mini` (Codex) or `sonnet` (Claude) when the provider chip is clicked.
- Replace sync glyph with a directional upload-arrow (↑ with base line).
- Show `resets <date>` inline on the active account card usage bars.

### Key Decisions

- `V-1` Per-item restore loading is tracked in local `restoringIds: Set<string>` component state; no store changes needed.
- `V-2` Restore-all iterates sequentially, consistent with sync-all pattern.
- `V-3` Model auto-select in `selectProvider()` uses a substring match (`o4-mini` / `4-mini` for Codex, `sonnet` for Claude) against enabled models only; falls back to `undefined` (Default) when no match is found.
- `V-4` Reset time on the active card reuses the existing `formatDateTime` helper.

### Constraints

- Icon must render legibly at 11×11 px using `currentColor` stroke — no library import.
- Model auto-select must not override the user's choice when they switch back to the same provider mid-session (acceptable: first-click auto-select is sufficient).
- No backend changes required.

### Open Questions

- None.

### Source Refs

- User screenshots and description, 2026-06-19.
- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx`

## 1. Issue Summary

Five distinct defects were reported together:

1. **Restore button no icon** — each Remote Chat row showed a plain text "Restore" button with no visual glyph, inconsistent with the icon-driven sync affordance elsewhere in the Navigator.
2. **No "Restore All" action** — users had to click Restore on each remote item individually; the Sync All pattern in the history section had no equivalent here.
3. **No loading state on restore** — clicking Restore gave no feedback while the API call was in flight; the item stayed visible with an active button until the list reloaded.
4. **After restore, button should disappear** — the button was hidden only after the remote session list was reloaded. Items with `unavailableReason` (restore failed) still showed the restore button.
5. **Sync icon** — user requested the upload-to-Drive icon be changed from the two-arc sync shape to a directional upload arrow (↑ with base line).
6. **Provider model auto-select** — when the user clicked the Codex chip, the model dropdown stayed on "Default"; likewise for Claude. The user expects `o4-mini` to be pre-selected for Codex and `sonnet` for Claude.
7. **Active account card missing reset time** — the sidebar active-account card showed usage percentage bars but omitted the "resets Jun 19, 11:48 AM" stamp visible in the expanded detail modal.

## 2. Parent Links

- impacted coding plan: —
- impacted tech design: `SD-15-Claude-Cross-Acc-Pc-Sync-Design.md` (account switching context)
- impacted system spec: —

## 3. Environment and Reproduction

- environment: Desktop Electron app, any OS
- reproduction steps:
  1. Open Navigator sidebar with remote chats available → observe text "Restore" with no icon, no loading on click.
  2. Select Codex or Claude provider chip → model stays "Default".
  3. Click a provider account card → expand the usage bars in the detail modal vs. the sidebar card (reset time missing from sidebar).
- frequency: 100% reproducible

## 4. Expected vs Actual

- expected: Restore button shows a download-arrow icon; loading spinner appears while restoring; unavailable items hide the button; "Restore All" chip in header; Codex auto-selects o4-mini, Claude auto-selects sonnet; sidebar active card shows reset times.
- actual: Plain text "Restore" on all items; no loading; unavailable items still show the button; no bulk restore; no model auto-select; sidebar card missing reset times; sync icon was generic two-arc.

## 5. Impact

- users affected: All desktop users
- workflows affected: Remote chat restore flow, provider/model selection, account usage display
- severity: Medium — no data loss, but poor UX and missing affordances slow restore and model selection workflows

## 6. Root Cause

- hypothesis: Restore section was implemented as a minimal MVP; loading state, icons, and bulk action were deferred.
- confirmed cause:
  - `Navigator.tsx` restore buttons used a plain text label with no state tracking or icon component.
  - `store.ts` `selectProvider()` only set `selectedProvider`; no model inference was wired.
  - `ProviderAccountsPanel.tsx` pinned card rendered `line.remainingPercent` only, omitting the `line.resetAt` field used in the expanded detail view.
  - `SyncGlyph` was updated in BUG-081 to a two-arc icon; user requested a directional upload-arrow instead.
- evidence: Direct code inspection of the four files listed above.

## 7. Fix Strategy

- `F-1` **Navigator.tsx — SyncGlyph**: replaced two-arc SVG with an upload-arrow glyph (↑ shaft + arrowhead + base line, same 11×11 px / `currentColor`).
- `F-2` **Navigator.tsx — RestoreGlyph**: added a new download-arrow glyph (↓ shaft + arrowhead + base line) for restore buttons.
- `F-3` **Navigator.tsx — restoring state**: added `restoringIds: Set<string>` and `restoringAll: boolean` local state; `restoreItem()` and `restoreAll()` callbacks wrap the store action with set/clear bookkeeping.
- `F-4` **Navigator.tsx — Restore All button**: added to the Remote Chats section header alongside the Loading indicator, shown only when there are restorable (non-unavailable) items.
- `F-5` **Navigator.tsx — per-item restore button**: replaced text "Restore" with `RestoreGlyph` (or spinner when restoring); button is hidden for items that have `unavailableReason`.
- `F-6` **store.ts — selectProvider()**: after setting `selectedProvider`, queries `supportedModels` for the provider; picks first model whose `modelId` contains `o4-mini`/`4-mini` (Codex) or `sonnet` (Claude); sets `selectedModel` to that modelId or `undefined` if not found.
- `F-7` **ProviderAccountsPanel.tsx — active card**: inline `· resets <date>` appended to the percentage span using the existing `formatDateTime` helper when `line.resetAt` is set.

## 8. Validation

- `V-1` `npx tsc --noEmit -p apps/desktop-flowpilot/tsconfig.json` — zero errors after all changes.
- `V-2` Visual review: restore buttons show download-arrow icon; spinner appears during restore; unavailable items show no restore button; "Restore All" appears when restorable items exist.
- `V-3` Provider chip click: model dropdown auto-advances to the matched model ID for Codex and Claude.
- `V-4` Active account card in sidebar: usage bars include `· resets <date>` when `resetAt` is non-null.

## 9. Regression Guard

- tests: No automated UI tests for Navigator icons or model selection; visual regression is low-risk (presentational only, no logic change in data layer).
- alerts: —
- audit checks: `SyncGlyph` is used only in `Navigator.tsx`; `selectProvider` callers should confirm model resets correctly on provider toggle.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — all changes are UI-layer improvements; no business rules or data contracts were altered.
- notes left unchanged on purpose: The `restoreRemoteChatSession` store action itself is unchanged; loading state is local to the component.
