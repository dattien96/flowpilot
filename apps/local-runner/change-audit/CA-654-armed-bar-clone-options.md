# CA-654 — armed-bar picker lists vibe-rooted clones (CA-652/653 follow-up)

## Scope
Third and last picker surface. The Flow-tab dropdown (CA-652) and the
runner gates (CA-652/653) already admit vibe-rooted clones, but the
composer's "Flow armed / Start a flow" bar rendered only the static
`flowPickerOptions` id list — clones never appeared there.

## Fix
- `workingMode.ts flowPickerOptions(mode, cloneRefs = [])`: appends clone
  refs after the static entries. No-arg behavior byte-identical (pinned).
- `ChatInput.tsx`: subscribes `workflows`, passes
  `[...vibeCloneIds(workflows)]`, labels clone UUID options by workflow
  name (static id labels unchanged — no row id ever equals a bare id).
  Choice/arm/forward flow is untouched: a clone UUID choice arms and
  forwards exactly like a pre-armed Workflow-tab pick, and the runner's
  ancestry gate (CA-652/653) admits it; `needsSourceDoc` stays false for
  UUIDs, matching the server fence which keys off canonical ids only.

## Tests
- `task326_working_mode.test.ts` (+1 test): RED (missing refs) → GREEN;
  existing no-arg expectations unchanged and green.
- `tsc --noEmit` (desktop package): clean.
