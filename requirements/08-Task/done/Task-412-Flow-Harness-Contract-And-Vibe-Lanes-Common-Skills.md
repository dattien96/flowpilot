# Task-412: Flow Harness Contract And Vibe Lanes Common Skills

## Metadata

- Document ID: `Task-412`
- Title: `Flow Harness Contract And Vibe Lanes Common Skills`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-34: Init Tool](../../07-Coding-Plan/inprogress/CP-34-Init-tool.md)
- Child Documents: `None`
- Related Documents: [Task-101: Flow Skill Pack Install](./Task-101-Flow-Skill-Pack-Install.md), [Task-107: Platform-Scoped Skill Pack Install](./Task-107-Platform-Scoped-Skill-Pack-Install.md)
- Replaces: `None`
- Tags: `skillpack, skills, common, flow-harness, vibe-lanes, agents-md, local-runner`

## AI Quick View

### Summary

- Adds two new skills to the always-installed `flow-pack/common/` group: `flow-harness-contract` (gated-flow discipline: frozen scope, reproduce-first, contract-first TDD, machine-checkable verdicts, doc intent ceiling, drift ladder, session checkpoints) and `vibe-lanes` (multi-worktree lane coordination: registry mutex, shared-package lane, merge order, explicit merge-back).
- Adds `apps/local-runner/AGENTS.md`: a compact working contract for contributors editing the runner itself — engine domain-free, hub-only routing, durability/recovery, fail-closed gates, worktree isolation, provider parity.
- No CP identifiers appear inside shipped skill bodies — rules are semantic only.

### Current Ask

- Mirror the runtime's flow-harness rules into advisory Markdown so every `flowpilot init`-installed project receives the soft layer automatically, and so local-runner contributors get a compact invariant card.

### Key Decisions

- `T-1` One dense `flow-harness-contract` skill instead of many single-rule skills — keeps the installed catalog small and avoids trigger-selection noise.
- `T-2` `vibe-lanes` is separate because multi-worktree coordination only applies when parallel lanes run; it must not pollute single-lane work.
- `T-3` `apps/local-runner/AGENTS.md` is contributor-facing and NOT shipped — it documents the invariants the Go code enforces.

### Constraints

- Skills must remain self-contained Markdown; no CP numbers in shipped content.
- `go:embed flow-pack` picks the new folders up automatically; no installer change needed.
- Defense-in-depth only: advisory layer, never a replacement for runtime gates.

### Open Questions

- None.

### Source Refs

- Skill pack install mechanism: `internal/skillpack/install.go` (common group always installed).
- Rule sources: harness/oracle/parity/durability/worktree contracts already enforced across the runner internals.

## 1. Goal

Ship the soft (advisory) mirror of FlowPilot's flow-harness rules so that:

1. Target projects initialized via `flowpilot init` receive the discipline layer automatically via the `common` skill group.
2. Contributors editing `apps/local-runner` see a compact invariant checklist before touching code.

## 2. Parent Links

- coding plan: `CP-35` (skill pack rollout), `CP-34` (init tool)
- specific upstream ids: platform-scoped install established by `Task-107`

## 3. Trigger

The user is preparing a new app monorepo and wants the FlowPilot flow emulated via Markdown while native multi-worktree support is still in progress. The correct delivery mechanism is the embedded `flow-pack/common/` group — not manual file copies into target projects.

## 4. Exact Change

- `T-1` New `flow-pack/common/flow-harness-contract/SKILL.md` — scope freeze/amend, reproduce-first red test, contract-first TDD + signature lock + batched renegotiation, machine-checkable done verdict with bounded loop, doc intent ceiling + DoD checklist, drift self-correction ladder, human-approved lesson promotion, STATE.md checkpoint/resume.
- `T-2` New `flow-pack/common/vibe-lanes/SKILL.md` — lane registry mutex, shared-package lane consumption-only rule, foundation-first merge order, explicit merge-back decision, lane checkpoint + resume order, lost-worktree handling.
- `T-3` New `apps/local-runner/AGENTS.md` — contributor contract: engine domain-free, hub-only routing, schema-first output, durability/recovery invariants, fail-closed gates, worktree isolation rules, provider parity, package→invariant map.

## 5. Definition of Done

- [x] `flow-harness-contract` skill written with no CP numbers in body
- [x] `vibe-lanes` skill written with no CP numbers in body
- [x] `apps/local-runner/AGENTS.md` written as compact invariant card
- [x] Both skills under `flow-pack/common/` so `Install` ships them to every project type
- [x] No changes to installer code (embed picks up new dirs automatically)
- [x] CA note written (`CA-903`)
