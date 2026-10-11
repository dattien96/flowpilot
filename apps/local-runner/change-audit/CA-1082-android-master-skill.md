# CA-1082: `skillpack` — android master skill (central router + audit gate)

Date: 2026-10-11
Refs: `internal/skillpack/flow-pack/android/android/SKILL.md`, CA-1051 (ndk-modern-cpp-mastery), CA-1072 (vibe-tasks pack pattern).

## Problem

The android flow-pack had the 13 sub-skills (adaptive multi-screen,
edge-to-edge, superapp architecture, SOLID, coding, compose-ui,
navigation, memory-leak, ndk, etc.) but was missing the master `android`
skill — the central router that decides which sub-skills to load
pre-implementation and doubles as the Master DoD audit gate
post-implementation. Without it, flow legs could not resolve the
`android` skill name that task docs reference in `Required Skills`.

## Solution

Install `internal/skillpack/flow-pack/android/android/SKILL.md` — the
master skill document (version 1):

- **Input mode**: navigation map routing to the 13 sub-skills.
- **Audit/DoD mode**: Master DoD checklist applied after coding.

Content mirrors the installed master skill in target projects
(`.agents/skills/android/SKILL.md`); the pack is the distribution source.

## Changes

- `internal/skillpack/flow-pack/android/android/SKILL.md` — new (added;
  previously untracked).

## Notes

- No Go code changed — pack data only (AGENTS.md §1: roles/prompts/
  skills live in pack data).
- The file's navigation links reference the target-project install path
  (`.agents/skills/<name>/SKILL.md`) — resolved at install time, not
  relative to the pack.
