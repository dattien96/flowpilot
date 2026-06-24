---
name: git-commit-format
description: Use PROACTIVELY when preparing commit messages or finalizing a task. Enforce the FlowPilot commit contract so the change ledger extracts the exact feature.
version: 5
---

# git-commit-format

Enforce the FlowPilot commit message format on every commit. The feature is part of the message so the Context & Regression Engine extracts it exactly — no path-guessing.

## Required Format

```
[Type][feature][layer] <description>
```

- **[Type]** — MANDATORY. One of: `Feature` | `BugFix` | `Refactor` | `Docs` | `Hotfix` | `Test`
- **[feature]** — MANDATORY. The kebab-case `feature_key` from `change-audit/FEATURE-KEYS.md` (e.g. `chat-ui`, `agent-spawn`, `project-nav`). Register a new key there first if none fits (see the `audit-logging` skill).
- **[layer]** — OPTIONAL. The architectural layer touched: `ui` | `api` | `domain` | `data` | `infra` | `test` | `build` | `docs`. Omit the third bracket entirely when it does not apply.
- **description** — short imperative phrase, English, no trailing period. Include the source-doc id (`Task-NNN` | `BUG-NNN` | `CP-NN`) when the change traces to a requirements document.

## Examples

```
[Feature][project-nav][ui] add creation navigation Task-087
[Docs][agent] update agent spawn doc
[BugFix][chat-ui][api] fix message replay BUG-130
[Refactor][skill-pack] extract install logic CP-35
[Test][metric-sync] add branch coverage for suspend path
[Hotfix][supabase][data] correct legacy_id not-null default BUG-135
```

## Rules

1. NEVER commit without the `[Type]` bracket.
2. NEVER commit without the `[feature]` bracket — the engine keys the entire change ledger off it. A missing feature falls back to coarse path-clustering (e.g. a whole `domain/` folder becomes one bucket), which is exactly what this contract prevents.
3. The `[feature]` value MUST be a key that exists in `change-audit/FEATURE-KEYS.md`. If none fits, add the new key to that file (in the same commit) before committing — see the `audit-logging` skill.
4. The `[layer]` bracket is optional; include it only when a clear layer applies. Do not invent layers outside the listed set without reason.
5. Include the source-doc id in the description when the change traces to a `Task-NNN`, `BUG-NNN`, or `CP-NN` document; verify the id exists in `requirements/` first.
6. Language is English; keep the first line concise (max 72 characters).
7. Do NOT use custom Type prefixes without explicit user approval.
8. Do NOT include AI authorship attribution. Remove any `Co-authored-by` / `Co-Authored-By` lines from the commit message.
9. Merge commits are exempt; all other commits must comply.

## Force

- **FORCE**: the final step of any task that changed files MUST be a commit-format check before committing.
- **CRITICAL**: no commit without an authorized `[Type]` and a `[feature]` from `FEATURE-KEYS.md`.

## Why

The ledger parser (`changeledger/parse.go`) reads the brackets in order: bracket 1 → `change_type`, bracket 2 → `feature_key` (`confidence: high`), bracket 3 → `layer`. Putting the feature in the commit makes feature history precise: asking "what changed for `project-nav`?" returns exactly that feature's commits, not every commit that happened to touch a shared directory. Without the feature bracket, the engine guesses from file paths and the result is too coarse to be useful.
