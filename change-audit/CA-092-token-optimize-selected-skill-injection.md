# CA-092: Token-Optimize Selected Skill Injection

## Scope

Replaced full-content skill embedding with compact file-path pointers in `injectSelectedSkills()`.
Previously the runner read every selected skill file and embedded the entire markdown body in the
turn prompt — 400–800+ tokens per skill. Now the runner emits only the skill name, absolute file
path, and one-line frontmatter description (~60 tokens per skill). The AI reads the full skill
file on demand via its Read tool when it needs to follow the process.

Applies to both Claude and Codex providers (both call `injectSelectedSkills()` via their
`promptPrep` hooks). The `## Selected Skills` block structure and prepend order from BUG-076
are preserved.

## Completed

### `apps/local-runner/internal/runner/runner.go`

- `injectSelectedSkills()`: rewrote to emit one line per skill in the form
  `- /name → /absolute/path\n  > one-line description` instead of fenced markdown content blocks.
  Instruction changed to "Read each skill file with your Read tool and follow its process before responding."
- `readSelectedSkill()` removed (was the only caller of `os.ReadFile` inside this function).
- Added `resolveSkillPath(workspace, sel)`: returns `(path, name)` using the same priority as
  the old helper — explicit `sel.Path` first (stat-checked), then workspace discovery by name/id.
- Added `readSkillFrontmatterDescription(path)`: opens the skill file, reads first 512 bytes,
  extracts the `description:` YAML frontmatter field. Returns `""` gracefully when absent.

### `apps/local-runner/internal/runner/skill_injection_test.go`

- Updated `TestInjectSelectedSkillsDeliversSelection`:
  - `writeSkill()` now includes a `description:` frontmatter field.
  - Assertions changed: path must appear, description must appear, full skill body must NOT appear.
  - Path-less fallback test now asserts the resolved path is present rather than embedded content.
  - Ordering assertion (prepend) retained from BUG-076.

## Verification

- `go test ./internal/runner/... -run "TestInjectSelectedSkills|TestCodexTurnStart"` — all PASS.
- `go build ./...` in `apps/local-runner` — clean.

## Residual Notes

- Skill files without a `description:` frontmatter key produce no `> …` line in the injected block;
  the path pointer alone is still actionable for the AI.
- Task document: `requirements/08-Task/done/Task-065-Token-Optimize-Selected-Skill-Injection.md`
