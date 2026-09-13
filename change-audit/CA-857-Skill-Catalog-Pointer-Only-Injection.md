# CA-857 — CP-62 P-5 catalog tier: one-shot skill injection becomes pointer-only

# ---8<--- flowpilot:change-ledger
feature_key: skill-catalog
source_doc_id: Task-347
change_type: task
summary: injectSkillContent (one-shot prompt-execution path, called from the claude/codex promptPrep seams and PromptExecution) no longer reads and inlines full SKILL.md bodies under "## Included Skills" — it delegates to injectSelectedSkills and emits the Task-260 pointer block (name + frontmatter description + file path per skill) so the provider harness triggers installed skills natively and agents Read the body on demand; injected token count is now independent of skill file size
# --->8---

## Why

The one-shot prompt path still inlined whole SKILL.md files (the exact anti-pattern CP-62 P-5's catalog tier targets, and the thing the ZCode lesson flagged: name+description only, body on demand). The chat path was already pointer-only since Task-260; the one-shot path kept the old "### Skill: …```markdown<body>```" injection.

## Change

- `runner/runner.go` `injectSkillContent`: resolves requested skill ids via `ListSkills()` and delegates to `injectSelectedSkills` (pointer block "## Selected Skills", one line per skill: `- /name → path` + `> description` from frontmatter). Unresolvable ids keep a bare `- /id` pointer (Task-260 precedent); empty/blank ids return the prompt byte-identical. No callers changed — both adapter promptPrep seams and PromptExecution funnel through this single function.

## Tests

`runner/skill_catalog_pointer_test.go` (3, additive): pointer block emitted with frontmatter description while the distinctive body marker NEVER appears; multiple skills → one pointer each + unknown id → bare entry, no bodies; nil/blank ids → byte-identical prompt. No existing test pinned the old body injection ("### Skill:"/"Included Skills" absent from the whole test tree).

## Providers

Case 1 provider-agnostic — prompt content assembled runner-side before the provider turn; no adapter involvement.

## Prior claims intact

Task-260 chat pointer machinery reused, not modified (`injectSelectedSkills`, `resolveSkillPath`, `readSkillFrontmatterDescription` untouched); skillpack installer and `ListSkills` discovery untouched; Budget Packer (Task-334) and the Task-341 pruned-sections catalog untouched — this task lands the CP-62 P-5 design's skill dimension, not the packer.
