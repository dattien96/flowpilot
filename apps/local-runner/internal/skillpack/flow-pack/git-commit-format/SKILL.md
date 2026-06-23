version: 1

# git-commit-format

Enforce the FlowPilot commit message format on every commit.

## Required Format

```
[Type]: <id> <description>
```

- **Type** MUST be one of: `Feature` | `BugFix` | `Refactor` | `Docs` | `Hotfix`
- **id** MUST reference one of: `Task-NNN` | `BUG-NNN` | `CP-NN`
- **description** is a short imperative phrase (no period at the end)

## Examples

```
[Feature]: Task-087 Add slash commands
[BugFix]: BUG-042 Fix null pointer in runner bootstrap
[Refactor]: CP-35 Extract skillpack install logic
[Docs]: Task-101 Add SKILL.md embedded files
[Hotfix]: BUG-099 Correct feature key lookup crash
```

## Rules

1. NEVER commit with a message that omits the `[Type]:` prefix.
2. NEVER commit with a message that omits the id reference (`Task-NNN`, `BUG-NNN`, or `CP-NN`).
3. If the change spans multiple tasks, pick the primary id and mention others in the body.
4. Merge commits are exempt; all other commits must comply.
5. Before finalizing a commit message, verify the id exists in the requirements tree.
