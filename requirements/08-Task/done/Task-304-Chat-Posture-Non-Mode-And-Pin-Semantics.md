# Task-304: Chat posture "non" mode + explicit pin semantics (CA-685)

## Metadata

- Document ID: `Task-304`
- Title: `Chat posture "non" mode + explicit pin semantics`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-08-29`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md) (posture model semantics from CA-679)
- Child Documents: `none`
- Related Documents: [CA-685](../../../change-audit/CA-685-chat-posture-non-mode.md), CA-679, BUG-330 (foreign-provider pin — core fixed here)
- Replaces: `none`
- Tags: `cli-tui, chat-posture, non-mode, pin-semantics, desktop-parity`

## AI Quick View

### Summary

Operator decision (2026-08-29) replacing the CA-679 interim hybrid rule with
explicit, predictable semantics:

1. **New posture `non`** ("no mode"): no pins are applied, ever. Tab cycles
   **Plan → Code → Non → Plan**; scan stays opt-in via `/mode scan`. In `non`
   the session keeps the user's last provider/model choice across restarts.
2. **scan/plan/code: the pin is the truth.** Restart restore and `/new`
   re-apply always show the pinned model; mid-session `/model` still works for
   the current chat but does not survive a restart in a real posture. The pin
   itself only changes via `/mode-setup` or the Desktop settings modal.
   CA-638 is untouched: reasoning is still never re-pinned on restore.
3. **Non is the default.** Fresh installs, missing posture files and invalid
   `active` values all resolve to `non` (runner `defaultChatPostureConfig` /
   normalize fallback; TUI `activePosture("")`; Desktop store init). Existing
   posture files keep their persisted active until the user changes it.
4. **Chrome display:** the no-mode default renders just `Chat:` in the chat
   bar — no `Chat: Non` suffix (scan/plan/code still render `Chat: Scan` etc.).
5. **BUG-330 guard (both surfaces):** a pinned model without an explicit
   provider pin infers its provider from the model id (grok-4.5 pin ⇒ grok),
   so a foreign pin can no longer stamp an opencode session. The runner's
   model-authoritative routing (BUG-171) remains the final SSOT.

### DOD

- Runner accepts `active: "non"` (`ValidChatPosture`); `IsReadOnlyChatPosture("non")=false`.
- TUI: Tab + `/mode` no-args cycle include `non`; apply/restore of `non` touches nothing.
- TUI: restore/re-apply in scan/plan/code pins provider+model always (with inference).
- Desktop: `ChatPosture` union + `CHAT_POSTURES` tab + non icon + 4-tab grid +
  `setChatPosture` inference parity; `profiles` typed `Partial` (non has none).
- Tests: `ca685_posture_non_mode_test.go` (cycle, no-pins apply, restore keeps
  user model, stale pins ignored); `ca679_model_restore_keeps_choice_test.go`
  rewritten to the CA-685 spec (same-day file); 2 pre-existing cycle
  assertions updated code→plan ⇒ code→non (the operator-requested cycle change,
  flagged per safe-fix R1). Desktop `tsc` clean on touched files.
