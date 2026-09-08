# Task-321: Vibe CP-Driven Entry (CP-Lock + Task Slicer + Per-Task Sprint)

## Metadata

- Document ID: `Task-321`
- Title: `Vibe CP-driven entry — vibe-cp-ingest, CP Preview & Lock, per-Task vibe-sprint to done`
- Phase: `task`
- Status: `in_progress` (CA-765 pack+queue + CA-767 auto-detect/lock/Task plan)
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-05`
- Last Updated: `2026-09-08`
- Parent Documents: [CP-60: Vibe Working Mode](../../07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md), [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md), [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [CP-58: Bug / Task / CP Harness](../../07-Coding-Plan/done/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Replaces: `None`
- Tags: `vibe-mode, cp-driven, coding-plan, desktop, tui, flow-gate, TDD`
- Feature Keys: `vibe-mode`

## AI Quick View

### Summary

- Slice `CP-60 P-6` only: new skeleton `vibe-cp-ingest` + `CP Preview & Lock` card + `task_slicer` reusing `CP-58` splitter, then sequential `vibe-sprint` v2 per Task tới done (v2 topology itself is `Task-323`/`P-7`; this task consumes it, not re-spec it).
- Reuses Branch V resolver/sprint loop verbatim (`r-requirement` + 2-Owner debate, cap 5); explicitly not `task-harness`, no Dev cards, no engine change.
- Entry auto-detects CP vs raw requirement (fallback explicit `/vibe-cp`); Desktop/TUI only, Admin Web rejected.

### Current Ask

- **BLOCKED.** Do not land `vibe-cp-ingest` yet. `P-6` consumes `working_mode`, `r-requirement`, resolver, lock-card plumbing from `P-1`..`P-5` — none of those Tasks exist and none of that Go is in the runner. Unblock only after `P-1`..`P-4` (and Branch V `P-5` demo) are green.

### Key Decisions

- `T-1` Separate flow `vibe-cp-ingest`, not a branch inside `vibe-ingest` (YAML has no conditional edges; keeps `ValidateFlowDefinition` single-`continue` rule clean).
- `T-2` `task_slicer` reuses `CP-58` `prompts/task-splitter.md` + `cp_md INPUT → task_md[] OUTPUT` bindings verbatim; no new artifact type.
- `T-3` Per-Task coding is `vibe-sprint` v2, never `task-harness` (preserves vibe resolver semantics; v2 adds auto `context`/`validate`/`audit`, never a plan-review loop or reviewer cohort).

### Constraints

- Additive only; no `P-1`..`P-5` behavior change **and no re-implementation of those slices**.
- Sequencing: `P-3` → `P-1`+`P-2`; `P-6` → `P-1`..`P-4` + CP-58 splitter (already in `done/`). Pack inventory is **11 flows / 8 agents today**, not the stale `6 → 7` count in older AC text — rebase `LoadBuiltinPack` assert to `11 → 12` when this Task unblocks.
- No engine/adapter change; no Supabase migration; `selectableIn: []` for `vibe-cp-ingest`.
- No pre-existing test edited; `dev` byte-for-byte. Vibe lives only in Desktop + TUI.

### Open Questions

- `Q-1` Auto-detect rule final: path `requirements/07-Coding-Plan/**/CP-*.md` + frontmatter `Document ID: CP-*` — sufficient, or also sniff `P-*` sections?

### Source Refs

- `CP-60 P-6`, §5–§8; `CP-58 P-3`/`P-4` (`task_splitter`, `task_md`); `SD-24 D-3`/`D-6`/`D-7`; `SS-13` + `FORMAT-REFERENCE-CP`; `pack.go:ValidateFlowDefinition`/`ValidateFlowSafetyTopology`.

## 1. Goal

A user points Desktop/TUI at one existing `CP-*.md`, locks it once in an editable `CP Preview & Lock` card, and the run auto-slices Tasks then executes `vibe-sprint` per Task sequentially to done, with post-lock user asks limited to `r-requirement` and 5-round Owner no-consensus.

## 2. Parent Links

- coding plan: `CP-60 P-6` (this task implements exactly `P-6`; `P-1`..`P-5` are prerequisites, not in scope to re-implement).
- tech design: `SD-24 D-3` (resolver), `D-6`/`D-7` (ingest + slice), §5 (`vibe.locked_cp`, `vibe.task_plan`).
- system spec: `SS-18 BR-2` (slice AI-auto), `BR-4`/`BR-8` (`r-requirement` user-only, no silent success).
- specific upstream ids: `CP-60 P-6`, `CP-58 P-3`/`P-4`, `SD-24 D-3`.

## 3. Trigger

`CP-60` now promises two Vibe entries but only Branch V has skeletons (`89fe174a`); Branch C (`CP → Task → code tới done`) has spec without executable slice.

## 4. Exact Change

- `T-1` New skeleton `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-cp-ingest.yaml` (`selectableIn: []`, `policy: {cap:3, onCap: escalate}`): nodes `cp_reader (delegate, agents/vibe-intake.md, once) → cp_validator (hub.inline, agents/synthesizer.md, once) → cp_lock (user.confirm, once) → task_slicer (delegate, agents/doc-writer.md + prompts/task-splitter.md, once) → done`; edges `cp_lock --done--> task_slicer`, single `continue/back cp_lock → cp_reader`, `cp_lock/task_slicer --escalate--> ask_user`. `cp_reader` accepts only `requirements/07-Coding-Plan/**/CP-*.md` with `Document ID: CP-*`; `task_slicer` declares `cp_md` INPUT → `task_md[]` OUTPUT (`pathTemplate: requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md`), verbatim if CP pre-slices else synthesized, marked verbatim-vs-synthesized.
- `T-2` Register flow in `manifest.yaml` + mirror sync; update `pack_test.go` count **11 → 12 flows** (agents stay **8**); `ValidateFlowDefinition` + `ValidateFlowSafetyTopology` green for both ingests.
- `T-3` Runner: entry routing (auto-detect CP by path/frontmatter, explicit `/vibe-cp <path>` override; non-CP file to `/vibe-cp` rejected deterministically); persist `working_mode=vibe` + `vibe.locked_cp` + `vibe.task_plan` in `localFileSessionStore` (`sessions.ndjson`); guard `vibe-sprint` refuses start while `cp_lock` is `WAITING_USER_APPROVAL`; on `task_slicer done`, render `task_plan` read-only on the timeline and sequentially start `vibe-sprint` per Task slice with Task-index replay on restart (same sink as Branch V); enforce total-sprint budget cap per run (stop with `BlockReason: budget`, no silent continuation).
- `T-4` Desktop/TUI: `CP Preview & Lock` card for `cp_lock` (editable, write-back to CP draft + re-validate `SS-13` CP §§1–10 incl. `P-*`; `Lock → task_slicer`, `Continue → cp_reader`, `Escalate → ask_user`); `r-requirement` card shows plain-language mapping (which SS `AC-*` ↔ which test signature drifted + what SS edit fixes it); no Admin Web surface (reject `vibe` from admin client as `P-1`).
- `T-5` Validation: additive `agentpack` topology test (`cp_lock=user.confirm`, single back-edge, artifact bindings) + runner tests (auto-detect, `cp_lock` gates sprint, restart replays Task index) + manual `/vibe-cp CP-*.md` demo (lock → slice → N× `vibe-sprint` v2 to done incl. `validate` green + `audit` ledger; generic gate → Owner debate, `r-requirement` → requirement card only).

## 5. Touched Areas

- files: `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-cp-ingest.yaml` (new), `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml`, `apps/local-runner/internal/agentpack/pack_test.go`, `apps/local-runner/internal/runner/{interactive_service.go,gate_hook.go,local_file_session_store.go}`, `apps/desktop-flowpilot/**`, `apps/cli-tui/**`.
- modules: `agentpack` (load/validate), `runner` (entry routing + lock gate + per-Task loop), Desktop/TUI timeline cards.
- routes: `POST /client/workflow-runs`, `POST /client/flows/run` (reuse `P-1` `X-Client` guard); TUI `/vibe-cp`, `/vibe` auto-detect.
- tables: none (local `sessions.ndjson` only; no Supabase DDL).

## 6. Acceptance Check

- `LoadBuiltinPack` green with **12 flows** / **8 agents**; `ValidateFlowDefinition` + `ValidateFlowSafetyTopology` pass for `vibe-cp-ingest`.
- `/vibe-cp <CP-*.md>`: CP card editable, Lock persists + re-validates, slicer emits `Task-*.md` list (visible read-only), sequential `vibe-sprint` v2 per Task reaches done (per-Task `tdd` signature artifact with use/edge/error + `context` packaged, `validate` green, `audit` ledger present); non-CP input rejected with deterministic error; budget cap exceeded stops with `BlockReason: budget`.
- Post-lock asks are only `r-requirement` / Owner-cap; `r-requirement` card is non-tech readable; no Dev `1/2/3` cards in `vibe`; `dev` regression unchanged; `go test ./internal/agentpack ./internal/flowgate ./internal/runner` + `go vet` green.

## 7. Out of Scope

- `P-1`..`P-5` re-implementation (working_mode, r-requirement, resolver, SS lock); engine/adapter changes; Admin Web Vibe; `task-harness`/`cp-harness` Dev paths; second orchestrator or new artifact type.

## 8. Completion Notes

- result: `pending implementation`
- follow-ups: `TBD (demo evidence + Kill-Review delta for Branch C)`
- upstream docs updated: `CP-60 P-6` (this task is its slice; no upstream intent change)
