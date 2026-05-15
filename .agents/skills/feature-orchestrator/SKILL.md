---
name: feature-orchestrator
description: Orchestrates the AI development workflow (Plan -> Arch -> TDD -> Code/Test -> Review). Handles flow gates, handshakes, and communication rules.
---

# Feature Orchestrator

Use this skill for big tasks, new features, major refactors, and multi-component work that requires planning, TDD, implementation, and review.

## Phase Ownership

- Phase 1 Planning: main agent.
- Phase 2 Architecture: main agent.
- Phase 3 TDD signatures: main agent.
- Phase 4 Logic validation: main agent.
- Phase 5 Coding: delegate to `coder-agent`.
- Phase 6 Review: delegate to `reviewer-agent`.

## Operating Rules

- Start each phase by stating the phase, role, mode, and current status.
- Read `.codex/AGENTS.md` and `token-optimization-skill` before broad context gathering.
- Use domain-specific search. Keep search result sets under 25 files.
- Ask the user for a specific path if a domain search is likely to be broad.
- Do not make assumptions about storage, data contracts, module boundaries, or integration points.
- List ambiguities in `4c_summary.md` and wait for explicit user confirmation before implementation.

## Phase 1: Planning

- Apply the 4C checklist: Context, Command, Constraints, Criteria.
- Use `planner-skill`, `common-mistakes-skill`, `lazy-loading-context-search-skill`, and `token-optimization-skill`.
- Produce `4c_summary.md` and initialize `task.md`.
- Gate: wait for explicit user approval before architecture.

## Phase 2: Architecture

- Use `architecture-skill`, `lazy-loading-context-search-skill`, and `token-optimization-skill`.
- Propose 2-3 approaches with tradeoffs and a clear recommendation.
- Produce `implementation_plan.md` with exact source and test file paths.
- Gate: wait for explicit user approval before TDD.

## Phase 3: TDD Signatures

- Use `tdd-skill`, `lazy-loading-context-search-skill`, and `token-optimization-skill`.
- Create test files with signatures, inputs, and expected outputs only.
- Do not write assertions, mocks, stubs, implementation code, or real test bodies.
- Produce `tdd_signatures.md`.
- Gate: wait for explicit user approval before logic validation.

## Phase 4: Logic Validation

- Use `ut-logic-sync-skill` and `lazy-loading-context-search-skill`.
- Compare `tdd_signatures.md` with the business logic source.
- Ask for the source link or file path if it was not provided earlier.
- Produce `logic_sync_report.md` with `[SYNCED]`, `[OUTDATED]`, and `[MISSING]` markers.
- If discrepancies exist, loop back to Phase 3.
- Gate: proceed to coding only after logic is synced and approved.

## Phase 5: Coding

- Delegate implementation to `coder-agent`.
- Provide `coder-agent` with `implementation_plan.md`, `tdd_signatures.md`, exact file paths, and constraints.
- The main agent should not implement complex code directly in this phase.
- Gate: all listed tests pass and build verification succeeds.

## Phase 6: Review

- Delegate final audit to `reviewer-agent`.
- Provide `reviewer-agent` with modified files, `implementation_plan.md`, `tdd_signatures.md`, and `logic_sync_report.md`.
- Require review for plan compliance, test coverage, architecture boundaries, skill compliance, and logic drift.
- Produce `walkthrough.md` with a skill audit matrix and pass/fail recommendation.

## Network Policy

- Phase 1 Planning: network on for Drive, Jira, or live documentation.
- Phase 2 Architecture: network on when external references are needed.
- Phase 3 TDD: network off by default.
- Phase 4 Logic validation: network on when system requirements live online.
- Phase 5 Coding: network off by default.
- Phase 6 Review: network off by default.


