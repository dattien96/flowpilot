---
id: CA-493
feature_key: cli-tui
title: Skill Tab closes /skill after attach
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-492-tui-skill-name-in-prompt
will_not_undo: CA-492 [name] in prompt; chip + SelectedSkills inject
```

## Change

After Tab select, do **not** leave `/skill` on the line:

```text
abc /skill  + Tab a  →  abc [a]␠
```

More skills: operator types `/skill` again. Untick: reopen `/skill`, Tab the same row.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI skill Tab closes /skill after inserting [name] (retype to attach more)
# --->8---
