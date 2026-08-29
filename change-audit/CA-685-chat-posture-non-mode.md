# CA-685 — Chat posture "non" mode and explicit pin semantics

## Problem

Operator feedback after the CP-57 manual test run: the CA-679 interim rule
("pin wins only when the pinned provider differs") was confusing to reason
about — model reverts after restart were neither clearly right nor clearly
wrong. Decision: make posture semantics explicit and add a no-mode escape
hatch. Chat-only; flow mode never touches postures.

## Change

- Runner `chat_posture.go`: `ChatPostureNon="non"`; `defaultChatPostureConfig`
  and the normalize fallback now default to `non` (operator decision) —
  existing posture files keep their persisted active;
  previously `ChatPostureNon="non"` only; `ValidChatPosture` accepts
  it (so `normalizeChatPostureConfig` keeps `active:"non"` and the per-turn
  `resolveTurnChatPosture` accepts it). `IsReadOnlyChatPosture("non")=false`.
- TUI `chat_posture.go`:
  - Chat chrome: the no-mode default renders bare `Chat:` (no `Chat: Non`);
    scan/plan/code keep `Chat: Scan` etc.
  - `postureOrder` + `tabPostureOrder` (plan → code → non) + label
    "non — no posture (keeps your last model choice)".
  - `applyChatPostureProfile` / `restoreChatPostureProfile`: `non` applies
    NOTHING (provider/model/reasoning stay as the user left them); the three
    real postures always apply their pin (provider + model) on both restore
    and /new re-apply. `postureModelPinWins` (CA-679 interim) removed.
  - `posturePinProvider`: explicit provider pin, else infer from the pinned
    model id (BUG-330 guard — grok-4.5 pin never stamps an opencode session;
    the TUI-side inference mirrors the existing /mode-setup rule).
  - CA-638 untouched: reasoning is still never re-pinned on restore.
- Desktop: `ChatPosture` union + `TurnInputDTO.chatPosture` + `CHAT_POSTURES`
  tab "Non"; `ChatPosturePanel` non icon + 4-tab grid (`tab-list-four`);
  `ChatPostureConfig.profiles` typed `Partial` (non has no profile);
  `setChatPosture` infers provider from a provider-less model pin (TUI
  parity). Mock client profile records carry `non: {}`.

## Tests

- `ca685_posture_non_mode_test.go` — default posture = non (TUI + missing-file
  + normalize fallback), bare `Chat:` chrome title, Tab/cycle order (incl. scan opt-in
  unchanged), `apply:non` applies no pins + PUTs active, `non` restore keeps
  the user's model in-session AND in persisted prefs, stale pins ignored.
- `ca679_model_restore_keeps_choice_test.go` rewritten same-day to the CA-685
  spec (pin always wins in scan/plan/code; provider inference; /new re-apply).
- Two pre-existing cycle assertions updated (`chat_posture_test.go`:
  code→plan ⇒ code→non in `TestSlashMode_NoArgsCyclesPosture` and
  `TestTabCycleOnEmptyInputAdvancesPosture`) — the operator-requested cycle
  change made the old expectation unreachable; flagged per safe-fix R1.
- Old quota/posture suites green; full tui/app suite shows only the 7
  documented pre-existing render failures (identical baseline).
- Desktop `tsc` clean on all touched files.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-304
change_type: feature
summary: Chat posture non mode with no pins plus explicit pin-always semantics for scan/plan/code and provider inference from pinned models
# --->8---
