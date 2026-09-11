# CA-772 — TUI `/vibe` Tab offers on/off, not `/vibe-cp`

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-326
change_type: bugfix
summary: Exact /vibe Tab picker shows on/off; /vibe-cp remains hyphen-prefix only
# --->8---

## Why

`/vibe` is a prefix of `/vibe-cp`. Tab filled the next slash command (`/vibe-cp`) instead of `/vibe on|off`.

## Change

- `filterVibeArgSuggestions`: exact `/vibe` or `/vibe <q>` → on/off (`kind=vibe`)
- `collectSuggestions` returns that list before generic slash prefix match
- Tab accept → `/vibe on` / `/vibe off`
- Type `/vibe-` still lists `/vibe-cp`

## Tests

`task326_vibe_tab_args_test.go` (new). Did not edit `chat_ux_test.go`.

## Provider impact

Case 1 agnostic (TUI only).

## Will not undo

Task-326 `/vibe` `/vibe off` `/vibe-cp` handlers.
