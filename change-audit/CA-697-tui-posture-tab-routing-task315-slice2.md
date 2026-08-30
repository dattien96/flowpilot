# CA-697 — TUI posture Tab cross-provider routing + bare-model derive-once (Task-315 slice 2)

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: feature
summary: posture Tab / apply routes cross-provider pins through the switch endpoint (BUG-330 fix at the TUI entry point); bare-model pins derive their provider once via the catalog and persist it; the queued posture re-applies fully on the new leg after adoption
# --->8---

## What changed

- `chat_posture.go` apply case: `routePostureSwitch` runs before the in-place apply — a cross-provider pin on a live chat switches legs instead of swapping the model inside the old adapter (the exact BUG-330 mechanism, now impossible at this entry point).
- `chat_switch.go`: `pinnedProviderFor` (explicit pin → catalog; the catalog covers the runner-served ids so a prefix mirror would only drift), `routePostureSwitch` (derive-once: a bare-model pin stamps the derived provider into the profile, marks dirty, batches `cmdSaveChatPosture` with the switch — CP-59 P-6/Q-2), queued-posture adoption now re-applies the FULL profile on the new leg (`applyChatPostureProfile`, CA-685 semantics — reasoning/yolo/model pins land post-switch).

## R1 evidence

- 7 TUI switch tests green (2 new: Tab codex→grok routes + guards + queue; same-provider Tab in-place; bare-model derive + persist + dirty flag).
- Full TUI suite: 7 failures — identical to baseline.

## Honest gaps

- Detached-chat reattach predicate and `/open` restore-by-chat remain (Task-315 slice 3 / Task-317); the runner 409 `chat_no_active_leg` surfaces as a typed error meanwhile.
- Desktop (Task-316) and Drive sync/restore (Task-317) remain open.
