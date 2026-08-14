---
id: CA-492
feature_key: cli-tui
title: Skill Tab inserts [name] into prompt draft
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-491-tui-tab-slash-keeps-draft
will_not_undo: CA-491 Tab keeps draft; CA-490 Enter strip; SelectedSkills inject
```

## Change

Operator wants skill **names in the prompt** as well as the status chip:

```text
abc /skill  → Tab coding → abc [coding] /skill
            → Tab review → abc [coding] [review] /skill
            → Enter      → abc [coding] [review]␠
            → type def   → abc [coding] [review] def
```

- **Chip / wire:** unchanged — `selectedSkills` → `injectSelectedSkills` (path + desc).
- **Prompt:** Tab tick inserts `[skill-name]` before the active `/skill` when a draft
  prefix exists; untick removes the token. Bare `/skill` (no draft) stays chip-only
  so legacy multi-tick tests stay green.
- **Enter:** strips `/skill…` and leaves a trailing space so continued typing
  becomes `… def` without gluing.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: TUI skill Tab inserts [skill-name] into draft prompt alongside chip attach
# --->8---
