# CA-638 — TUI restart-restore no longer clobbers the user's /reasoning choice

## What

Choosing reasoning in chat mode (`/reasoning low`) persisted to session prefs, but
reopening the TUI reset it back to the active posture profile's default (the
`code` profile pins `reasoningEffort: medium`). Operator report: "chọn reasoning
ở CHAT mode sẽ không được lưu lại, lần mở TUI sau bị reset về default".

## Why

`restoreChatPostureProfile` (the restart-resume variant) unconditionally applied
every pinned profile field, including `ReasoningEffort`. On startup the TUI
fetches the runner's persisted active posture (SSOT) and "restores" it — which
re-pinned reasoning to the profile default and overwrote both the in-memory value
and the persisted prefs entry (`persistSessionPrefs`).

`/reasoning` is an explicit user preference that persists to session prefs; the
restart-restore path must not undo it.

## Fix

`apps/local-runner/internal/tui/app/chat_posture.go` — `restoreChatPostureProfile`
no longer re-applies `prof.ReasoningEffort` on the restart-resume path, keeping
the user's last persisted choice. An explicit posture switch (`/mode plan` →
`applyChatPostureProfile`) still applies the profile pin — only the resume path
changed.

Provider-agnostic (Case 1): no `ProviderKey` branch; shared TUI logic.

Will not undo: CP-56 posture restore (provider/model/yolo still restored), TUI
session-prefs persistence (CA-581 area), chat-posture SSOT.

## Tests

Additive only — legacy `chat_posture_restore_test.go` untouched:

- `apps/local-runner/internal/tui/app/ca638_reasoning_restore_test.go` (new):
  - `TestRestorePostureKeepsUserReasoning` — profile pins `medium`, user has
    `low` → restore keeps `low`, no dirty.
  - `TestRestorePostureKeepsUserReasoningOtherProfile` — plan pins `high`, user
    has `medium` → restore keeps `medium`, posture still switches to plan.
  - `TestApplyPostureStillPinsReasoning` — explicit apply still pins `high`.

## Verification

- `go test ./internal/tui/app -count=1` → full suite green (8.1s).
- `go vet ./internal/tui/app` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: restart-restore no longer clobbers the user's /reasoning choice with the posture profile default (CA-638)
# --->8---
