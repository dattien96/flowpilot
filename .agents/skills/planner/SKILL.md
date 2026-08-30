---
version: 6
name: planner-skill
description: Use this skill PROACTIVELY during the initial planning phase, when refining task breakdowns, or performing context research using the 4C Checklist.
---

# A. What is this skill for?

Use this skill PROACTIVELY during the planning phase to analyze requirements, resolve ambiguities, and establish a clear implementation roadmap. It enforces the 4C Checklist and ensures no development starts without explicit design approval.

# B. What are the tasks?

## 1. 4C Analysis Execution
Verify Context, Command, Constraints, and Criteria factors with the user.

## 2. Handshake Protocol Management
Create and block on the approval of `4c_summary.md` and `flow_variant_Confirm.md`.

## 3. Design Exploration
Propose 2-3 technical approaches with trade-offs (YAGNI emphasized) before implementation planning.

## 4. Multi-Agent Workflow Setup
Decompose complex tasks into the 5 standard phases (Planner, Architecture, TDD, Coder, Reviewer) and initialize `task.md`.

# C. Instructions

## 1. 4C Checklist (Mandatory Handshake)
### 1.1 **Context**: Module, package, and layer identification.
- Which module/package in the project does this feature/task belong to?
- Which layers are involved (domain/usecase, data/repository, presentation/api)?
- Is this a new feature, a bug fix, or a refactor?

### 1.2 **Command**: Specific requirements and expected results.
- What is the expected final result? (New UI, new endpoint, new use case, new class...)
- How broad is the scope of the change?

### 1.3 **Constraints**: Off-limits files, breaking changes, deadlines.
- Which files/layers are off-limits?
- Are there any breaking changes for clients (mobile/web)?
- Are there any deadlines or technical constraints to be aware of?

### 1.4 **Criteria**: Definition of Done, review strategy, test requirements.
- What defines a "correctly done" task? (Tests passing, UI displaying correctly, API returning the correct response spec...)
- Who will review the results? (Self-review, lead review, QA?)
- Is writing unit/integration tests required?


## 2. "No Guesses" Protocol (STRICT)
- If factors are missing, STOP and ask.
- **BLOCKED**: Forbidden from creating `implementation_plan.md` until User approves `4c_summary.md`.

## 3. Design Approval (Gate)
- Present 2-3 approaches with a clear recommendation.
- **HARD-GATE**: User MUST explicitly approve the chosen approach before an implementation plan is created.

## 4. Multi-Agent Setup
- If task involves > 1 layer, setup `task.md` with standard phases and load required workflow rules.

# D. Examples

Check [examples.md](examples.md) for 4C summary templates and ambiguity resolution dialogues.

Use the Read tool to load:

**Mandatory Skills (always load):**
- [`common-mistakes-skill`](../common-mistakes/SKILL.md) - Avoid common pitfalls
- [`lazy-loading-context-search-skill`](../lazy-loading-context-search/SKILL.md) ⭐ **Domain-specific search, max 25 files**


# H. FORCE rules

- **FORCE**: Load [`common-mistakes`](../common-mistakes/SKILL.md) and [`lazy-loading-context-search`](../lazy-loading-context-search/SKILL.md) for every planning task.
- **CRITICAL**: No `write_to_file` for architectural plans until Phase 1 Handshake is complete.
- **MANDATORY**: Propose at least 2 technical options for every new feature.

# I. Notes

- For simple tasks, the design can be brief but still requires user confirmation.



