## Metadata

- Document ID: `Task-063`
- Title: `Desktop Chat Provider Chip Picker`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](./Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- Child Documents: `none`
- Related Documents: [CA-088: Desktop ChatInput Provider Card Picker](../../change-audit/CA-088-desktop-chatinput-provider-card-picker.md), [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](./Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md)
- Replaces: `none`
- Tags: `desktop, chat, provider, ui, brand-icons, session-state`

## AI Quick View

### Summary

- Replace the provider `<select>` dropdown in the desktop chat controller with 3 compact clickable chip buttons — one each for Codex, Claude, and Gemini — each showing an official brand icon and the provider name.
- Provider selection is locked after the first turn is sent in a session; can only change when starting fresh (empty timeline).
- Opening a history run restores the provider that was used in that run (`RunHistoryItem.providerKey`), scoping provider state to the session rather than leaving it as a global value.

### Current Ask

- Done. T-1 through T-9 implemented and TypeScript verified.

### Key Decisions

- `T-1` Compact single-row chip layout: label + 3 equal-width chips all on one flex row (~32px tall vs previous ~80px card columns).
- `T-2` Brand colours as CSS variables: `--codex-brand #e0e0e0`, `--claude-brand #e17554`, `--gemini-brand #7c6ff0`.
- `T-3` Auto (no provider) state is implicit — no chip highlighted — rather than an explicit "Auto" option.
- `T-4` `selectProvider(undefined)` called on re-click of selected chip to deselect.
- `T-5` `providerLocked = isChatMode && timeline.length > 0`; chips disabled with tooltip once any turn is sent.
- `T-6` When locked, unselected chips dim to 0.65 opacity; the selected chip stays at full opacity so the active provider remains readable.
- `T-7` `openHistoryRun` restores `selectedProvider` from `RunHistoryItem.providerKey` and reloads skills for that provider.
- `T-8` Claude icon: 8-arm Anthropic asterisk (4 symmetric rounded rects at 0°/45°/90°/135°).
- `T-9` Codex icon viewBox padded (`"-3 -3 30 30"`) so the OpenAI blossom path renders at the same visual weight as Claude and Gemini.

### Constraints

- `selectProvider` action signature and `ProviderKey` type are unchanged.
- `RunHandle` does not carry `providerKey`; restoration relies on the already-loaded `runHistory` list.
- `selectedModel` is not restored on history run open (not stored in `RunHistoryItem`).

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/ChatInput.tsx`
- `apps/desktop-flowpilot/src/styles.css`
- `apps/desktop-flowpilot/src/state/store.ts` — `openHistoryRun`
- `SS-05` section 1 (supported providers: Codex, Claude, Gemini)
- `SD-06` section 1 (UI/UX Design — provider listing)

## 1. Goal

Replace the plain `<select>` dropdown for provider selection with compact clickable chip buttons
showing official brand icons; lock provider to the session once a turn is sent; and restore the
correct provider when reopening a historical chat run.

## 2. Parent Links

- coding plan: n/a (UI polish slice, no separate CP)
- tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md) §1
- system spec: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md) §1

## 3. Trigger

The existing `<select>` dropdown was functional but visually inconsistent with provider brand
identity. Separate follow-up issues exposed that provider state was global, not session-scoped:
switching providers on a new run would corrupt the displayed provider on a concurrently open
historical chat.

## 4. Exact Change

- `T-1` Removed `PROVIDER_OPTIONS`; added `CodexIcon`, `ClaudeIcon`, `GeminiIcon` inline SVG components and `PROVIDER_CARDS` constant in `ChatInput.tsx`.
- `T-2` Replaced `<label><select>` for Provider with `<div class="provider-picker">` + `<div class="provider-chips">` containing 3 `<button class="provider-chip provider-chip-{key}">` compact elements in a single flex row.
- `T-3` Added brand color CSS variables (`--codex-brand`, `--claude-brand`, `--gemini-brand`) to `:root` in `styles.css`.
- `T-4` Added `.provider-picker` (flex row), `.provider-chips`, `.provider-chip`, brand icon colour, and selected-state rules in `styles.css`; changed `.chat-controller-grid` from `repeat(3, 1fr)` to `1fr 1fr`; updated responsive breakpoint.
- `T-5` Added `timeline` subscription in `ChatInput`; computed `providerLocked = isChatMode && timeline.length > 0`; chips receive `disabled={blocked || providerLocked}` and `title` tooltip when locked.
- `T-6` CSS: `.provider-picker-locked .provider-chip` dims to 0.65; `.provider-chip-selected` stays at opacity 1 when locked.
- `T-7` `openHistoryRun` in `store.ts`: looks up `runHistory.find(item => item.runId === runId)` before calling `resumeRun`; spreads `selectedProvider: historyItem.providerKey` into `set()`; calls `loadSkills(historyItem.providerKey)` after set.
- `T-8` `ClaudeIcon`: replaced "A" lettermark path with 4 symmetric `<rect>` elements (8-arm asterisk matching the Anthropic brand mark).
- `T-9` `CodexIcon`: changed `viewBox` from `"0 0 24 24"` to `"-3 -3 30 30"` to normalise visual fill to match Claude and Gemini icon sizes.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - `apps/desktop-flowpilot/src/styles.css`
  - `apps/desktop-flowpilot/src/state/store.ts`
- modules: desktop chat controller UI, run history open action
- routes: n/a
- tables: n/a

## 6. Acceptance Check

- ✓ Three provider chips render in a single compact row (Codex, Claude, Gemini) with brand icon + name.
- ✓ Clicking an unselected chip selects it; clicking a selected chip deselects it (Auto).
- ✓ Selected chip shows brand-coloured border, icon, name, and subtle background.
- ✓ Chips are disabled when `blocked` is true (run in progress).
- ✓ After the first turn is sent, all chips are disabled; unselected chips dim, selected chip stays fully visible; tooltip reads "Start a new chat to change provider".
- ✓ Chips unlock when `resetRun()` clears the timeline (new session).
- ✓ Opening a history run restores `selectedProvider` to the provider used in that run.
- ✓ Skills are reloaded for the restored provider on history run open.
- ✓ `aria-pressed` attribute reflects selection state.
- ✓ `npx tsc --noEmit` passes with no errors.
- ⏳ Visual smoke-test in running app not performed (no preview server available).

## 7. Out of Scope

- Pixel-perfect brand icon SVGs from official provider brand kits.
- Adding an explicit "Auto" chip (implicit via deselect).
- Provider availability/install-state indicators on the chips.
- Restoring `selectedModel` on history run open (not in `RunHistoryItem`).
- Runner or contract changes.

## 8. Completion Notes

- result: Implemented and TypeScript-verified. Visual layer + `openHistoryRun` store action changed; runner and contract untouched.
- follow-ups:
  - Visual smoke-test in the running desktop app to confirm chip layout and lock behaviour.
  - Consider sourcing SVG paths from official brand kits if design team requires exact marks.
  - Add `model` field to `RunHistoryItem` if per-session model restoration is needed.
- upstream docs updated: none required (provider-to-session scoping is an implementation detail; no SS/SD/CP business rule changed).
