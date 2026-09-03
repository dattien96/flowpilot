# CA-725 — Desktop posture Tab cross-provider routes to the switch endpoint (TUI routePostureSwitch parity)

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-349
change_type: bugfix
summary: setChatPosture now routes a cross-provider posture Tab on a live chat to confirmProviderSwitch (new leg + one divider, timeline kept, pinned model carried) and derives+persists+notifies a bare-model pin exactly once — mirrors TUI routePostureSwitch; covered by 6 new store tests with neighbors 16/16 green
# --->8---

## Problem

- E7 live: Tabbing between postures with different providers (Plan=grok,
  Code=opencode) showed no `⇄ switched to …` divider. Runner timeline for
  `cht_8efbaa593d69` confirmed a single opencode leg — no switch happened.
- `setChatPosture` only applied pins to session state; TUI routes this case
  to the switch endpoint (`routePostureSwitch`, chat_switch.go).

## Changes

- `apps/desktop-flowpilot/src/state/store.ts` (`setChatPosture`):
  - Capture `prevProvider` before applying pins.
  - Derive-once: model pin without provider → persist derived provider into
    the runner config + one info notice (next Tab sees it set).
  - Switch route: pinned provider differs (case-insensitive) + live chat
    (`normal_chat`, `chatId`, `runId`, not detached, no switch in flight, no
    question/approval pending) → build `pendingProviderSwitch` with the
    PINNED model as target and `await confirmProviderSwitch()` (reuses the
    E1 divider + kept-timeline path; error handling stays inside confirm).
  - In-place otherwise (same provider, no live chat, detached, in flight) —
    previous behavior preserved.
- `src/state/store.chatPostureSwitch.test.ts` (new, 6 tests).

## Verification

- New suite 6/6; neighbors (`chatSwitch`, `chat-detached-parity`,
  `chatOpenTimeline`) 16/16 via phase1 `node --test`.
- Live E7 retest (divider on Tab Code↔Plan): pending operator.
