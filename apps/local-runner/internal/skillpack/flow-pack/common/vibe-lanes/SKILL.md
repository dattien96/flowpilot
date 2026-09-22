---
name: vibe-lanes
description: Protocol for running multiple parallel AI work lanes on one repo via git worktrees — lane ownership registry, shared-package serialization, merge order, explicit merge-back, and resume checkpoints. Use when working inside a worktree lane alongside other parallel agents, or when the user asks to run parallel/parallel-vibe/multiple-worktree flows.
version: 6
---

# vibe-lanes

Coordination contract for parallel work lanes when no tool enforces isolation.
A **lane** = one git worktree + one declared scope + one owner (agent session).
Physical isolation fixes file conflicts; this protocol prevents **semantic**
conflicts — lanes stepping on each other's intent.

---

## 1. Lane registration (before any work)

1. Every lane must be registered in `docs/lanes/LANE-REGISTRY.md` **before**
   its first edit:

   ```markdown
   | Lane | Worktree path | Owns (paths) | Depends on | Status |
   |------|---------------|--------------|------------|--------|
   | proquote | ../wt-proquote | apps/proquote/, requirements/08-Task/proquote-* | lane-core | active |
   ```

2. `Owns` is the lane's declared scope — the mutex. A lane may modify **only**
   the paths it owns. Overlap between two lanes' `Owns` is a registry bug:
   resolve it before either lane codes.
3. Unregistered lane = no lane. If you find yourself in a worktree with no
   registry entry, stop and register first.

## 2. Shared packages are a lane, not a commons

1. Shared code (`packages/*`, `core-*`, `shared/`, root config) is owned by a
   dedicated shared lane (e.g. `lane-core`).
2. Feature lanes **consume** shared code through its public API only. Never
   edit shared code from a feature lane — that is the #1 source of silent
   cross-lane breakage.
3. Need a change in shared code? File it as a request/task for the shared lane
   (a line in the registry + a note), then work around or wait. Do not patch it
   locally.

## 3. Merge order & merge-back

1. Foundation/shared lanes merge **first**. Feature lanes rebase onto the
   updated base before their own merge-back.
2. Merge-back is always an **explicit user decision** — `apply_patch` |
   `keep_branch` | `discard`. Never auto-merge, never auto-push, never commit
   in the user's main checkout.
3. On merge conflict: stop. Surface the conflicting paths and the patch. The
   user resolves or instructs — do not force-apply.
4. Two lanes finished simultaneously merge **sequentially**, never in parallel.

## 4. Lane state & resume

1. Each lane keeps `docs/lanes/<lane>.md` as its checkpoint: declared scope,
   current phase/gate, done list, next step, open findings.
2. Update the checkpoint at every pause. A session starting in a lane reads:
   `LANE-REGISTRY.md` → its lane file → `change-audit` entries for its
   `feature_key` — **in that order, before any edit**.
3. If your worktree directory or branch is gone externally: report the lane as
   `lost` and ask the user. Never silently recreate, re-clone, or re-apply.

## 5. Cross-lane communication

1. Lanes never coordinate by editing each other's files. The only channels are
   the registry, per-lane checkpoint files, and the change ledger.
2. One lane discovering a problem in another lane's code writes a finding in
   its own checkpoint + registry note — it does not fix the other lane.

## Anti-patterns (forbidden)

1. Editing shared/core code from a feature lane "just to unblock".
2. Two lanes owning the same path because "it's only temporary".
3. Auto-merging a lane because "the tests pass" — merge-back is the user's call.
4. Resuming a lane from memory instead of reading its checkpoint file.
5. Recreating a deleted worktree without asking.
