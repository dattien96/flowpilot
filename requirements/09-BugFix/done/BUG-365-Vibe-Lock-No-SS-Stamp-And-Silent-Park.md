# BUG-365 — SS lock does not stamp approved; requirement park never reaches the TUI

## Metadata

- Document ID: `BUG-365`
- Title: `SS lock keeps status draft and requirement park is silent — cp_writer stalls, TUI stays Thinking (run-646702)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-10`
- Last Updated: `2026-09-24`
- Parent Documents: [CP-60: Vibe Working Mode](../../07-Coding-Plan/done/CP-60-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [BUG-363](./BUG-363-Vibe-Ingest-Writer-Output-Unverified.md), [BUG-364](./BUG-364-Slicer-Park-Skips-Step-Done.md), [SS-18](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [SD-24](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md)
- Replaces: `None`
- Tags: `vibe-mode, ss-lock, park-surfacing, live-bed, regression`

## AI Quick View

### Summary

- Live run-646702 (`gate-sandbox`, snake prompt): operator locked at `ss_lock` (`[DONE]` on the timeline) and the flow reached `cp_writer` → `task_slicer` → `[DONE]`, but nothing was ever written: `requirements/07-Coding-Plan/todo/` empty, `requirements/08-Task/todo/` empty, `snake/` empty. Only the SS drafts remained, all `status: draft`.
- D1: `resumeVibeLock` sealed the lock in memory only (`vibeSSSealed`); it never wrote `status: approved` to the SS files on disk. `cp_writer` (`prompts/vibe-cp-from-ss.md`) read the draft and asked the operator to "fix SS status to approved first" instead of writing the CP. The operator chose "Request changes" → no CP was written.
- D2: the slicer then completed with zero Task files and the BUG-363/364 park fired, but `parkVibeRequirement` mutated the loop state without `emitAgentGraph`. The TUI never received the `blocked/requirement` snapshot and kept "Thinking 3m29s" with no `[Retry]` card until the terminal was killed ("Input stalled").
- Fix: stamp the SS list approved on lock acceptance; emit the graph snapshot (and persist) when a requirement park fires; state in the CP writer prompt that the operator lock is the authority.

### Current Ask

- Locking at `ss_lock` must leave `SS-*.md` + `SPRINT-PLAN*.md` with `status: approved` on disk; a requirement park must surface as a blocked card; `vibe-cp-from-ss` must never stop at leftover draft frontmatter.

### Key Decisions

- `V-1` Stamp is surgical: only `status: draft` frontmatter lines and `- Status: \`draft\`` metadata lines are rewritten in `SS-*.md` / `SPRINT-PLAN*.md`; `FORMAT-*` files and prose mentioning draft are untouched.
- `V-2` `parkVibeRequirement` mirrors `parkVibeLock` (emit `EventAgentGraphUpdated` + `go persistParentSession`); the CA-818 DONE-before-park ordering is untouched.
- `V-3` CA-791 "missing CP still joins the slicer" behavior stays pinned; the park decision itself (BUG-363) is unchanged. This bug is lock persistence + park surfacing only.

### Constraints

- `feature_key: vibe-mode`.
- R1: additive tests only; the new tests must FAIL without the fix and PASS with it.
- R2: provider-agnostic (disk stamp + engine event; zero `providerKey` references).

### Open Questions

- None blocking. Follow-up candidate: live re-run the snake ingest on a clean sandbox after the fix.

### Source Refs

- Live: run-646702 TUI screenshot 2026-09-10 (`ss_lock [DONE]`, `cp_writer` question, empty `07-Coding-Plan/todo/`, empty `08-Task/todo/`, `snake/` 0 files, `Thinking 3m29s agent:main`, `Turn failed: interrupted by flow park (blocked: vibe_lock)`, `Input stalled`).
- Docs: `FORMAT-REFERENCE-SS.md` (§Metadata status), `SS-18` (`BR-2` SS locked before sprints), `prompts/vibe-cp-from-ss.md`, `prompts/task-splitter.md`.

## 1. Issue Summary

The operator-visible lock has no disk effect and the fail-closed park has no client effect. The run looks alive in the TUI but is stopped: no CP, no Task, no code, no card.

## 2. Parent Links

- impacted coding plan: `CP-60-Vibe-Working-Mode.md`; `CP-60-Test-Steps.md` (V2/V4/V6 evidence).
- impacted tech design: `SD-24-Vibe-Working-Mode.md` (`ss_lock` write-back + re-validate).
- impacted system spec: `SS-18-Vibe-Working-Mode.md` (`AC-2`, `BR-2`).

## 3. Environment and Reproduction

- environment: TUI on `/Users/tiendat/Desktop/BE/gate-sandbox`, provider grok-4.5, branch `cp60-vibe`.
- reproduction steps: fresh `/vibe` snake prompt → SS drafts written with `status: draft` → operator locks (empty Continue) → `cp_writer` reads drafts → operator answers the status question "Request changes" → slicer writes nothing → park fires silently.
- frequency: deterministic at unit level; observed once live (run-646702).

## 4. Expected vs Actual

- expected: lock leaves SS approved; `cp_writer` writes `requirements/07-Coding-Plan/todo/CP-*.md`; slicer writes Tasks; a zero-Task park shows a blocked card with `[Retry]`.
- actual: SS stayed `draft`, no CP, no Task, no card, TUI "Thinking" until terminal killed.

## 5. Impact

- users affected: operator live bed (run-646702 unusable after lock).
- workflows affected: every `vibe-ingest` run where the operator locks drafts in place (`cp_writer` prompt path) and every requirement park (BUG-363/364 class).
- severity: high (full pipeline stall behind a healthy-looking lock; no data loss).

## 6. Root Cause

- D1 confirmed: `resumeVibeLock` (`vibe_lock.go`) called `sealVibeLockLocked` only; no code ever rewrote the SS `status`. `cp_writer`'s prompt forbade writing under drift, so the LLM correctly stalled and asked.
- D2 confirmed: `parkVibeRequirement` (`vibe_sprint.go`) called `mutateLoop` + `parkFlowForAwaitingUser`, but unlike `parkVibeLock` it never called `emitAgentGraph`/`persistParentSession`; the TUI derived "running" from its last snapshot and could not clear "Thinking".
- evidence: live screenshot + on-disk state (`status: draft` in SS-100/101/102 + SPRINT-PLAN; empty todo folders); code paths above.

## 7. Fix Strategy

- `F-1` New `stampVibeSSApproved(cwd)` (`vibe_ss_stamp.go`): glob `SS-*.md` + `SPRINT-PLAN*.md` under `requirements/05-System-Specs/`, skip `FORMAT-*`, rewrite `^status: "?draft"?$` → `status: approved` and `^- Status: \`?draft\`?$` → `- Status: \`approved\``. Called from `resumeVibeLock` when `nodeID == ss_lock` (both empty lock and edit write-back).
- `F-2` `parkVibeRequirement` now captures the `mutateLoop` snapshot, then after `parkFlowForAwaitingUser` emits `EventAgentGraphUpdated` and `go persistParentSession` — exact mirror of `parkVibeLock`.
- `F-3` `prompts/vibe-cp-from-ss.md`: the `SS Preview & Lock` decision is the approval; never stop on leftover `status: draft`, write the CP now.
- Explicitly NOT doing: changing the CA-791 join (pinned), changing the BUG-363 park reason, changing CA-790 once-lock edges, auto-writeback at park time.

## 8. Validation

- `V-1` `bug365_ss_lock_stamps_approved_test.go` red-before/green-after (verified by stashing the production edits).
- `V-2` `bug365_requirement_park_emits_graph_test.go` red-before/green-after.
- `V-3` Vibe contract cohort (`TestBUG36*`, `TestCA7 7-9x/8 0-2x`, `TestOnVibe*`, `TestVibeSession_*`, `TestAdvanceHubDone*`, Task-321/326) green; full-suite failures verified byte-identical on baseline (pre-existing, environment/provider flakes) — not caused by this fix.
- `V-4` Live re-run on a clean sandbox: lock → CP written → Tasks sliced → sprint starts (operator tick, not claimed here).

## 9. Regression Guard

- tests: `bug365_*` (stamp surgical + park emit); the BUG-363/364 suites must stay green (park decision + DONE stamp ordering).
- alerts: any `bug36*` / CA-78x/79x red ⇒ STOP (R1).
- audit checks: CA-820 note with will-not-undo (CA-791 join, CA-790 once-lock, CA-817/818 park).

## 10. Follow-Up Document Updates

- upstream docs that must change: `change-audit/CA-820` (new note); `CP-60-Test-Steps.md` live evidence after `V-4`.
- notes left unchanged on purpose: CA-817/818 (park decision/ordering stay), CA-791 (join stays), CA-790 (once-lock stays).
