---
id: CA-490
feature_key: cli-tui
title: Skill picker Enter preserves draft prompt
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-489-tui-slash-mid-input
will_not_undo: CA-489 mid-draft /; CA-488 /image; skill chip F3; SelectedSkills wire
```

## Change

**Bug:** Enter on the `/skill` suggestion list called `clearInputValue()`, wiping any
draft the operator typed before opening the picker (e.g. "hãy dùng skill a và b /skill …").

**Expect:** Same as attaching skills for the status chip (F3) — ticks stay on
`selectedSkills` — but the **prompt draft stays** so skill names can remain in the
message text. Internal injection is unchanged.

### Internal inject (no code change)

1. TUI `buildTurnInput` copies `m.selectedSkills` → `TurnInput.SelectedSkills`.
2. Client POST includes `selectedSkills`.
3. Runner `injectSelectedSkills` prepends a `## Selected Skills` block with path
   pointers (+ frontmatter description), then `---`, then the user prompt.
4. Wired for Claude / Codex / Grok via `ProviderRegistryFor` prompt prep (provider-agnostic
   selection list; adapters still resolve paths per provider).
5. After the turn is accepted, `clearPendingTurnPayload` clears selected skills.

User-visible prompt text is **not** rewritten by inject — names in the draft are
optional operator wording; the model always gets the Selected Skills block when
the chip has skills.

### TUI fix

- Enter on skill rows: `stripActiveSlashCommand` removes only the active `/skill` /
  `/s` fragment (word-boundary from CA-489), keeps prefix draft, sticky caret end.
- Tab tick behavior unchanged (multi-select without closing).
- Bare `/skill ` + Enter still leaves empty input (no draft).
- Help copy notes Enter keeps the prompt.

## Tests

`skill_prompt_preserve_test.go` — strip helper matrix, mid-draft Enter preserve +
chip + `buildTurnInput`, bare slash clear, `/s` alias.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI /skill Enter keeps draft prompt while skills stay on status chip
# --->8---
