# CA-1071 — desktop: posture strip is toggleable Scan/Plan/Code (non = deselected)

## What changed

The Posture rail section rendered four tabs — Scan | Plan | Code | Non — where
"Non" was a dedicated tab for "no posture". Replaced with a toggle UX on the
three real postures: click a tab to select it, click the active tab again to
deselect → posture becomes `non` with no tab highlighted. The setup modal's
profile strip drops the (profile-less) `non` tab too — its own comment already
documented a 3-tab layout — and coerces `modalTab` to `scan` when opened while
`non` is active.

- `src/types/contract.ts`: `CHAT_POSTURE_TABS` (CHAT_POSTURES minus `non`) and
  `postureAfterTabClick(current, clicked)` — returns `"non"` when clicking the
  active key, otherwise `clicked`. `non` remains a real ChatPosture value and
  keeps its hint text; only the dedicated tab is gone.
- `src/components/ChatPosturePanel.tsx`: both strips map `CHAT_POSTURE_TABS`
  and switch to `tab-list-three`; the rail strip calls
  `setChatPosture(postureAfterTabClick(...))` and tab titles note "Click again
  for no posture."; the dead `non` branch of `PostureModeIcon` is removed.
- `styles.css`: `.tab-list-four` deleted (no remaining callers).

## Invariant

Posture semantics are unchanged — `non` is still a first-class posture sent to
the runner and persisted via `setChatPosture`; only its UI affordance moved
from a tab to "nothing selected". No store or wire changes.

## Tests

- `state/ca1071_posture_toggle_tabs.test.ts` (new, 3/3): tab list exposes only
  scan/plan/code, active-click deselects to `non`, other-click and
  non→posture select.
- Existing posture tests (`store.chatPostureSwitch`, `chat-posture-restore`,
  `chat-posture-grok-yolo`, `postureModelPicker`) all pass unchanged.
