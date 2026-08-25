# CA-641 — /new no longer clobbers the user's /reasoning choice

## What

`/reasoning high` (or low) in chat mode was silently reset back to the active
posture profile's default (`code` pins `reasoningEffort: medium`) after running
`/new` — and the clobbered value was re-persisted to session prefs on disk.
Operator report: "Reasoning không được lưu khi tôi chọn" (choice survives until
the next `/new`, then reverts).

## Why

`/new` arms `chatPosturePending = "apply:" + activePosture()` to re-apply the
active posture's pinned provider/model/reasoning/yolo. The "apply:" pending
handler called `applyChatPostureProfile` unconditionally, which re-pins
`ReasoningEffort` from the profile (e.g. `code` → `medium`) and then calls
`persistSessionPrefs()` — overwriting both the in-memory value and the saved
session file with the profile default.

CA-638 fixed the restart-restore path only; the `/new` re-apply path had the
same clobber, and it additionally persisted the wrong value to disk.

## Fix

`apps/local-runner/internal/tui/app/chat_posture.go`:

- `applyChatPostureProfile(cfg, name, keepReasoning bool)` — skips the
  `ReasoningEffort` pin when `keepReasoning` is true.
- The `apply:` pending handler passes `keepReasoning = (name == activePosture())`
  — re-applying the already-active posture (`/new`) preserves the user's
  `/reasoning` choice; a real posture switch (Tab cycle, `/mode <name>`) still
  applies the profile pin (unchanged behavior, CA-638 contract intact).

Provider-agnostic (Case 1): no `ProviderKey` branch; shared TUI logic.

Will not undo: CA-638 restart restore, explicit posture-switch pinning
(`TestApplyPostureStillPinsReasoning` still green), `/mode-setup` modal edits
(they apply the user's explicit profile edit), chat-posture SSOT.

## Tests

Additive only — legacy suites untouched (`ca638_reasoning_restore_test.go`,
`chat_posture_restore_test.go`, `mode_setup_modal*` all unmodified):

- `apps/local-runner/internal/tui/app/ca641_reasoning_new_preserves_choice_test.go`
  (new):
  - `TestNewReapplyKeepsUserReasoning` — `/new` on active `code` keeps user `high`
    vs profile pin `medium`.
  - `TestNewReapplyKeepsUserReasoningEvenWhenActiveDiffersFromRunner` — active
    `plan` with user override `low` kept on re-apply.
  - `TestRealPostureSwitchStillPinsReasoning` — real switch to `plan` still pins
    `high` (mirrors CA-638 contract).
  - `TestNewReapplyKeepsUserReasoningAndPersistsIt` — session prefs file keeps
    `low` after `/new` re-apply (no disk clobber).

## Verification

- `go test ./internal/tui/app/ -count=1 -run 'TestNewReapply|TestRealPostureSwitch'` → 4 PASS.
- `go test ./internal/tui/... -count=1` → full TUI suite green (app 8.3s).
- `go vet ./internal/tui/app/` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: /new re-apply of the active posture no longer clobbers the user's /reasoning choice (CA-641)
# --->8---