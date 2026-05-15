---
name: token-optimization-skill
description: MANDATORY skill for all agents to enforce 2-3M token budget per feature.
---

# A. Purpose
To maintain a strict token budget (2-3M per feature) by optimizing context loading and search patterns. This is a HARD constraint for all 5 phases of development.

# B. Core Rules (STRICT)

## 1. Search & Discovery
- **NO BROAD GLOBS**: Never use `**/*.kt` or `**/feature/**`. Use targeted patterns like `**/feature/domain/usecase/*.kt`.
- **GREP LIMITS**: Every `grep_search` MUST include `limit: 5-10`.
- **MAX 25 FILES**: Never load more than 25 files in a single search. If matches > 25, **STOP AND ASK** the user to narrow the path.
- **STOP AND ASK**: If a domain search finds > 10 files, present the list and ask: "Which specific area should I focus on?"

## 2. Reading Efficiency
- **OFFSET/LIMIT**: For files > 100 lines, use `offset` and `limit` to read only relevant logic or structure.
- **TEST FILES**: When reading test files for structure or patterns, read ONLY the first 50-60 lines. Do NOT read the entire file.
- **FILE COUNT TRACKING**: Track your file read count. Target **< 10 files per phase**.

## 3. Strict Batch Error Fixing (CRITICAL)
- **THE LOOP**: Build ONCE → Collect ALL errors → Read error lines with offset/limit → Fix ALL errors in ONE batch edit → Rebuild.
- **NO INCREMENTAL FIXES**: Never fix errors one by one. This wastes massive amounts of tokens re-reading the same contexts and running multiple builds.

## 4. Subagent Handover
- **EXACT PATHS**: Pass EXACT file paths to subagents. Do NOT let them search.
- **READ ONCE**: Use the constraint: "Read specs exactly ONCE at start, then avoid redundant re-reading."

# C. Token Budget targets
| Phase | Target | Max |
|-------|--------|-----|
| 1. Planning | 300K | 500K |
| 2. Architecture | 400K | 600K |
| 3. TDD | 500K | 750K |
| 4. Coding | 600K | 1M |
| 5. Review | 400K | 600K |

# D. Emergency Brake
If usage exceeds 2x target in any phase:
1. **PAUSE** execution.
2. **NOTIFY USER** with current usage vs budget.
3. **PROPOSE** scope reduction or skipping non-essential reads.



