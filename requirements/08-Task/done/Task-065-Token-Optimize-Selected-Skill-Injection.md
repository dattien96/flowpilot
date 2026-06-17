# Task-065: Token-Optimize Selected Skill Injection

## Metadata

- Document ID: `Task-065`
- Title: `Token-Optimize Selected Skill Injection`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](./Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- Child Documents: `none`
- Related Documents: [BUG-076: Selected Skills Ignored When User Prompts Main Task](../../09-BugFix/done/BUG-076-Selected-Skills-Ignored-When-User-Prompts-Main-Task.md), [CA-092: Token-Optimize Selected Skill Injection](../../../change-audit/CA-092-token-optimize-selected-skill-injection.md)
- Replaces: `none`
- Tags: `runner, skills, token-efficiency, claude, codex, prompt-engineering`

## AI Quick View

### Summary

- The previous skill injection strategy embedded the full markdown content of every selected skill into the turn prompt — 400–800+ tokens per skill regardless of file size.
- The new strategy emits a compact path-pointer block: skill name, absolute file path, and the one-line frontmatter description only.
- The AI provider reads the full skill file via its Read tool when it needs the content, keeping injected token count at ~60 tokens per skill regardless of skill size.
- Applies to both Claude and Codex providers, since both share `injectSelectedSkills()` via their `promptPrep` hooks.

### Current Ask

- Done. `injectSelectedSkills()` now emits path pointers. `readSelectedSkill()` replaced by `resolveSkillPath()` + `readSkillFrontmatterDescription()`. Tests updated to assert path presence and prohibit embedded content.

### Key Decisions

- `T-1` Skill content is NOT embedded in the prompt. The AI reads the file on demand using its Read tool.
- `T-2` The one-line `description:` field from the skill's YAML frontmatter IS included — it gives the model enough context to know what each skill does before reading it.
- `T-3` Path resolution priority is unchanged: explicit `SkillSelection.Path` first, then workspace discovery by name.
- `T-4` The `## Selected Skills` block and the prepend order (from BUG-076) are preserved — this task changes only the content of each skill entry, not the block structure or ordering.

### Constraints

- Do not change the block header structure or the prepend order established in BUG-076.
- Do not alter `SkillSelection` struct fields — no API contract change.
- Do not expand to unrelated prompt assembly changes.

### Open Questions

- None.

### Source Refs

- User request `2026-06-17`: "we don't need to recap all skill content here — just point the AI to it, the path is also fine"
- Related: BUG-076 investigation that surfaced the full-content injection as the prior implementation

## 1. Goal

Reduce the per-skill token cost from ~600 tokens (full embedded content) to ~60 tokens (name + path + one-line description) by switching `injectSelectedSkills()` from content embedding to file-path pointers. The AI reads the skill file itself when it needs to follow the process.

## 2. Parent Links

- coding plan: [Task-044: Desktop Chat Mode Split And Provider Controls](./Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: BUG-076 (immediate trigger)

## 3. Trigger

BUG-076 investigation revealed that the runner embedded the entire skill file content into every turn prompt. The user requested a token-efficient alternative: point the AI at the file path and let it read the skill when needed, rather than pre-loading all content unconditionally.

## 4. Exact Change

- `T-1` `runner.go` — `injectSelectedSkills()`: rewrote to emit `- /name → path\n  > description` per skill instead of fenced markdown content blocks. Instruction text updated to "Read each skill file with your Read tool and follow its process before responding."
- `T-2` `runner.go` — replaced `readSelectedSkill()` (reads full file content) with `resolveSkillPath()` (returns path + name only).
- `T-3` `runner.go` — added `readSkillFrontmatterDescription()`: reads first 512 bytes of a skill file, extracts the `description:` frontmatter field, returns the one-line summary.
- `T-4` `skill_injection_test.go` — rewrote assertions: skill paths and descriptions must appear; full skill body content must NOT appear; path-less fallback test now asserts the resolved path is present rather than embedded content.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/runner.go`
  - `apps/local-runner/internal/runner/skill_injection_test.go`
- modules: local-runner › runner
- routes: none
- tables: none

## 6. Acceptance Check

- `go test ./internal/runner/... -run TestInjectSelectedSkillsDeliversSelection` passes — path appears, description appears, full body does not appear, unselected skill not mentioned.
- `go test ./internal/runner/... -run TestCodexTurnStart` passes — no regression on Codex param builder.
- `go build ./...` clean in `apps/local-runner`.
- New injected block for one skill is approximately 60 tokens (name + path + description line) vs ~600 tokens before.

## 7. Out of Scope

- Changes to `SkillSelection` or `ProviderSkill` struct fields or the HTTP contract.
- Prompt engineering for ask_user reinforcement or the `claudeAskUserReinforcement` suffix.
- Skill resolution logic changes beyond extracting `resolveSkillPath()` from the old `readSelectedSkill()`.
- Codex native `turn/start` skill field changes (covered by BUG-076).

## 8. Completion Notes

- result: implemented and verified in this turn; all tests pass, clean build.
- follow-ups: none required — the description field is optional (returns `""` gracefully when frontmatter has no `description:` key).
- upstream docs updated: none required — this is an implementation optimization that does not change business rules or acceptance criteria stated in SS-11 or SD-06.
