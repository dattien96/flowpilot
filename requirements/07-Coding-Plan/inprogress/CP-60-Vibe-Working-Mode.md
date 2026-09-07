# CP-60: Vibe Working Mode (SS-lock + CP-driven + Owner Debate + r-requirement)

## Metadata

- Document ID: `CP-60`
- Title: `Vibe Working Mode — Desktop/TUI SS-Lock / CP-Lock, TDD-First Sprint, Owner Debate, r-requirement`
- Feature Keys: `vibe-mode`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-01`
- Last Updated: `2026-09-05`
- Parent Documents: [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: [Task-321: Vibe CP-Driven Entry](../../08-Task/todo/Task-321-Vibe-Cp-Driven-Entry.md) (implements `P-6`), [Task-323: Vibe-Sprint v2 Parity](../../08-Task/todo/Task-323-Vibe-Sprint-V2-Parity.md) (implements `P-7`; `T-1`..`T-5` for `P-1`..`P-5` to be created)
- Related Documents: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [CP-36: Agent Review Loop And Main-Hub Orchestration](../done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md), [CP-45: Generic Artifact Types And Instances](../done/CP-45-Generic-Artifact-Types-And-Instances.md), [CP-58: Bug / Task / CP Harness](./CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Replaces: `None`
- Tags: `vibe-mode, coding-plan, desktop, tui, flow-gate, agent-flow, TDD, cp-driven`

## AI Quick View

### Summary

- Implement `SS-18`/`SD-24` as a thin `working_mode` policy over the shipped generic engine + gate hook. Pack skeletons (`vibe-ingest`, `vibe-sprint`, `vibe-owner-debate` + `owner`/`vibe-intake` agents) are already in the repo and inert; this plan wires them with **zero engine reshape**. `P-6` adds a second Vibe entry (CP-driven) with one new skeleton `vibe-cp-ingest`, reusing `CP-58` splitter prompts.
- `working_mode ∈ {dev, vibe}` (default `dev`, local only) splits only the **resolver** mapping of `flowgate.EnforceResult.Violations`: `r-requirement` → requirement card (vibe-only, `always-block`), otherwise `vibe` → start `vibe-owner-debate` flow (exactly 2 isolated owners, cap 5), `dev` → Dev card. Gate evaluation stays pure. The resolver/sprint loop is identical for both Vibe entries.
- UX has two entries, one sprint loop. Branch V (idea-only): `SS ingest → SS Preview & Lock via user.confirm (Desktop/TUI, mandatory)` → AI auto slice tasks/sprints → per-sprint `preflight_contract_plan → freeze → context → tdd → coder → validate → synthesis → audit` (hub owns 1:1 check; single code loop, fully auto). Branch C (CP-driven, `P-6`): `CP ingest → CP Preview & Lock via user.confirm (mandatory)` → `task_slicer` reuses `CP-58 task-splitter` (verbatim if CP pre-slices Tasks, else AI-synthesized) → sequential `vibe-sprint` v2 per Task. Tasks never ask the user beyond the single entry lock.
- `P-7` brings `vibe-sprint` to accuracy parity with `task-harness` by copying only the auto nodes (`context.produce`, `command.validate`, `artifact.audit_draft`) — never the plan-review loop, reviewer cohort, or Dev cards; review stays resolver-driven (`vibe-owner-debate` + `r-requirement`).
- One new hard gate `r-requirement` (vibe-only) captures green-but-drifted or green+`TamperedTestPaths` weakening (`SS-14 AC-6`, `r-additive-tests` subsumed). It and the 5-round Owner no-consensus are the only Vibe user asks after the entry lock (`ss_lock` or `cp_lock`).
- Rollout is additive and Kill-Review-safe: skeletons land first (done, `89fe174a`), then Go `working_mode` + `r-requirement` + resolver + Desktop/TUI `SS Lock` card, then `P-6` `vibe-cp-ingest` + `CP Lock` card, with a Dev non-regression probe.

### Current Ask

- Provide a task-sliced plan to make Branch V (`vibe-ingest` → locked SS → auto-sliced `sprint_plan` → sequential `vibe-sprint`) and Branch C (`vibe-cp-ingest` → locked CP → auto-sliced `task_plan` → sequential `vibe-sprint` per Task, i.e. `CP → Task → code tới done`) runnable end-to-end in Desktop + TUI, with zero Admin Web scope and Dev parity.

### Key Decisions

- `P-1` `working_mode` is the SSOT enum on the local run record only (default `dev`); gated at resolver, not at rule evaluation; Desktop/TUI only. Shared by both Vibe branches.
- `P-2` Single new rule `r-requirement` (vibe-only, `block`, `always-block`, `requirement_signature_drift`) fed by the hub's `RequirementDrift` advisory (including green+`TamperedTestPaths`); `isAlwaysBlock` extended, enabled-filter hides it in `dev`.
- `P-3` Resolver maps `Violations` to UX/FlowDefinition id only (`vibe-owner-debate`), never names `owner.md` in Go; Owners are exactly 2 isolated cohort members via Main, `join: all`, cap 5.
- `P-4` Desktop/TUI `SS Preview & Lock` is the pre-sprint user gate for Branch V (`user.confirm` at `ss_lock` with edit write-back + re-validate); task/sprint slicing is AI-auto post-lock.
- `P-5` Skeletons-first, Go-second: no runner path auto-runs vibe flows until the Go work merges.
- `P-6` Branch C reuses Branch V's sprint loop verbatim: new skeleton `vibe-cp-ingest` (`cp_reader → cp_validator → cp_lock=user.confirm → task_slicer → done`, single `continue` back-edge `cp_lock → cp_reader`) + `CP Preview & Lock` card; `task_slicer` reuses `CP-58` `prompts/task-splitter.md` + `file_artifact` bindings. Per-Task coding is sequential `vibe-sprint`, not `task-harness` (keeps vibe's `r-requirement` + Owner-debate resolver; no Dev `1/2/3` cards). Entry auto-detects CP vs raw requirement by path/frontmatter (`CP-*.md` + `Document ID: CP-*`), fallback explicit `/vibe-cp`.
- `P-7` `vibe-sprint` v2 parity (auto-only copy of `task-harness`): add `context` (`context.produce`, once) after freeze, `validate` (`command.validate`, once) after coder, `audit` (`artifact.audit_draft`, once) as terminal; keep single `continue/back` `synthesis → coder`, `policy cap:3`, `acceptance_nodes: [validate, synthesis, audit]`. Plan accuracy stays at the entry lock + slicer (no per-sprint plan-review loop); code accuracy matches harness via `context + tdd + validate + synthesis + r-requirement + Owner debate`; ledger via `audit`. No engine change, no reviewer cohort nodes, no Dev cards.

### Constraints

- Reuse `SD-19`/`CP-36` generic engine, `SD-20` gate hook, `SS-13` contract, `CP-36 P-5` local run sink (`sessions.ndjson`); no new Supabase run migration; Vibe lives only in Desktop + TUI.
- Do not change `ProviderRuntimeAdapter` except by declaring `vibe-requirement-outcome` (for `vibe-sprint`) and reusing `submit_review_outcome` (for `vibe-owner-debate`); do not alter YOLO SSOT (`SS-08`).
- No pre-existing test edited; additive `r-requirement` is vibe-only and preserves Dev byte-for-byte.
- `P-6` adds no engine change and no second orchestrator; `task_slicer` output shape (`Task-*.md` list) is consumed as `vibe-sprint` slices so `r-requirement`/Owner semantics stay identical across branches.

### Open Questions

- `Q-1` Resolved — requirement check reuses `hub.inline` inline prompt; no dedicated `vibe.requirement_check` behavior.
- `Q-2` Resolved — `working_mode` is run-level enum; project default optional later; Vibe entry Desktop/TUI only.
- `Q-3` Entry routing: auto-detect CP by `requirements/07-Coding-Plan/**/CP-*.md` path + `Document ID` frontmatter vs explicit `/vibe-cp <path>`? Proposed: auto-detect with explicit override (no new top-level picker section).

### Source Refs

- `SS-18 AC-1`..`AC-10`, `BR-1`..`BR-9`; `SD-24 D-1`..`D-7`, §3–§8; `SD-19 D-1`..`D-8`, `D-5` bridge (`agent_flow` step), `D-7` vocab; `SD-20 D-1`..`D-7`, `gate_hook.go:1209-1228` (`applyFlowControl` child→parent escalate), `flowgate/{rules,evaluate,enforce,observe}.go`; `SS-13`/`FORMAT-REFERENCE-SS`/`FORMAT-REFERENCE-CP`, `SS-14 AC-6`, `SS-04 §3.5.8`; `pack.go:ValidateFlowDefinition`/`ValidateFlowSafetyTopology` (`user.confirm` alias), `enforce.go:isAlwaysBlock`; built-ins `review-loop.yaml`/`context-coding-review-synthesis.yaml`; `r-additive-tests` / `TamperedTestPaths`; `CP-58 P-3`/`P-4` (`task_splitter`, `task_md` `file_artifact`, `prompts/task-splitter.md`).

## 1. Goal

Make `SS-18`/`SD-24` executable via two entries sharing one sprint loop: (V) a non-tech user attaches one requirement file (or pastes an idea) in **Desktop app or TUI**, sees an editable `SS Preview & Lock` card, locks the SS, then the system **automatically** slices tasks/sprints and runs sprint-by-sprint (`vibe-sprint` v2: `context → TDD → coder → validate → synthesis → audit` with hub 1:1 check, single auto code loop); (C) a user points at one existing `CP-*.md`, sees an editable `CP Preview & Lock` card, locks the CP, then the system auto-slices Tasks (verbatim if the CP pre-slices, else AI-synthesized via the `CP-58` splitter) and runs `vibe-sprint` v2 per Task sequentially **tới done**. Any non-`r-requirement` violation in `vibe` auto-starts `vibe-owner-debate` (cap 5); after the single entry lock, only `r-requirement` and that 5-round no-consensus ever ask the user; Dev mode is unchanged.

Pack skeletons for Branch V are already landed (`89fe174a`, 6 flows total, `selectableIn: []` for all vibe flows) and `vibe-intake` read+write scoped to SS docs. This plan completes the Go wiring + Desktop/TUI UX for Branch V, then adds Branch C skeleton `vibe-cp-ingest` + `CP Lock` card (no engine change).

## 2. Input Documents

- [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md) — `AC-1`..`AC-10`, `BR-1`..`BR-9`; scope clarifies Vibe is Desktop/TUI only, SS must lock before sprints, task slice is AI-auto, Desktop/TUI entry.
- [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md) — `D-1`..`D-7`, data model `WorkingMode` (local only), `TurnResult.RequirementDrift`, resolver walking `Violations`, topology `preflight_contract_plan → freeze → context → tdd → coder → validate → synthesis → audit` (`P-7` v2; was `freeze → tdd → coder → synthesis`) and `vibe-ingest` `ss_lock=user.confirm → sprint_slicer`.
- [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) + [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md) + [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) — generic engine substrate, `D-5` bridge, gate hook contract (`isAlwaysBlock`, child→parent escalate).
- [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md) + `FORMAT-REFERENCE-SS` / `FORMAT-REFERENCE-CP` + `SS-14`/`SS-04`/`SS-08`/`SS-11`.
- [CP-58: Bug / Task / CP Harness](./CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md) — `P-3`/`P-4` `task_splitter` (`cp_md` INPUT → `task_md[]` OUTPUT) + `prompts/task-splitter.md` reused by `P-6` `task_slicer`; slice-only default vs per-Task coding contrast for Branch C.

## 3. Implementation Strategy

- **Overall approach:** Additive policy layer mirroring `D-1`..`D-7` in `SD-24`: introduce a run-level `working_mode` enum (local only) that the resolver reads via `enabledRulesFor(workingMode)`, add one vibe-only pure gate rule `r-requirement` whose signal is injected by the `vibe-sprint` synthesis hub **before** child `block → parent escalate`, map `EnforceResult.Violations` to UX/FlowDefinition id only, and render the new `SS Preview & Lock` / `CP Preview & Lock` (`user.confirm`) + `r-requirement` cards only in Desktop/TUI. The engine stays domain-free (`SD-19 D-1`); Vibe is data; Admin Web `vibe` is rejected. Branch C (`P-6`) reuses the Branch V sprint loop + resolver verbatim and reuses `CP-58` `task-splitter.md` + `file_artifact` bindings for slicing.
- **Sequencing logic (bottom-up, each P independently testable; skeletons already inert):**
  1. `P-1` `working_mode` SSOT + persistence (local only, no UI except default `dev`; Admin reject).
  2. `P-2` `r-requirement` rule (vibe-only, `always-block`, subsumes weaken) + advisory plumbing + declared `vibe-requirement-outcome` face on `vibe-sprint` only.
  3. `P-3` Resolver split (walks `Violations`) + `vibe-owner-debate` cohort (exactly 2, isolated, cap 5) + settlement `BUG-231`/`BUG-234`, triggered before child escalate.
  4. `P-4` Desktop/TUI `SS Preview & Lock` UX (editable, write-back, re-validate) + ingest→`sprint_slicer` auto wiring (tasks AI-auto).
  5. `P-5` Kill-Review boundary + demo scenario + Dev non-regression (covers Branch V; extended by `P-6` for Branch C).
  6. `P-6` CP-driven entry: `vibe-cp-ingest` skeleton + `CP Preview & Lock` card + `task_slicer` (CP-58 reuse) + sequential per-Task `vibe-sprint` to done.
  7. `P-7` `vibe-sprint` v2 parity: `+context +validate +audit`, single loop, auto-only (no plan-review loop, no reviewer cohort).
- **Dependencies:** `P-3` → `P-1`+`P-2`. `P-4` → `P-1` (lock gates sprints) and `P-2` (r-requirement card). `P-2` is otherwise self-contained; `P-5` depends on `P-1`..`P-4`; `P-6` depends on `P-1`..`P-4` (reuses resolver + sprint loop) and on `CP-58` splitter contract; `P-7` depends on `P-2` (sprint topology + requirement face) and reuses `CP-58` node semantics (`context.produce`, `command.validate`, `artifact.audit_draft`). No Supabase DDL.

## 4. Work Breakdown

- `P-1` **Run `working_mode` SSOT (`dev` | `vibe`, default `dev`, local only).** Add `type WorkingMode string` and constants in `internal/runner` (+ `internal/agentpack` if needed), persist **only** on the local run record (`localFileSessionStore` JSON — `sessions.ndjson`, `SS-11` `§9`; definitions stay on Supabase). No `workflow_runs.working_mode` column (resolves `C9`). Accept `working_mode` at `POST /client/workflow-runs` and `POST /client/flows/run` only when `X-Client: desktop|tui`; `Admin Web` (`X-Client: admin` or missing) with `vibe` returns `403`. Default to `dev`; no definition-table migration. Entry for Vibe is exposed only via `apps/desktop-flowpilot` file picker and `cli-tui` (`/vibe` / `/vibe-file`).

- `P-2` **`r-requirement` gate + `vibe-requirement-outcome` tool face (vibe-only, always-block).** Append to `internal/flowgate/rules.go:DefaultRules()`:
  ```go
  {ID: "r-requirement", Scope: "step", Trigger: "requirement_signature_drift", RequiredOutput: "reconcile_tests_with_ss", Action: "block", Enabled: true}
  ```
  Extend `isAlwaysBlock` to include `r-requirement` and filter `Enabled` by `workingMode` (vibe-only: `dev`'s enabled set omits `r-requirement`). Extend `TurnResult` with `RequirementDrift bool` + `RequirementDriftDetail string`; in `vibe` green+`TamperedTestPaths` (weaken) is coerced to `RequirementDrift=true` before `Evaluate` (so `r-additive-tests` in vibe is subsumed — `C8`). The `vibe-sprint` v2 `synthesis` hub (`hub.inline` + inline prompt) computes the 1:1 signature↔frozen-SS check after `validate` reports a green suite and populates the advisory before `flowgate.Evaluate`. Register `tools/vibe-requirement-outcome.yaml` as a `flow_control` declared face on `vibe-sprint` only (`aligned→done`, `drift_fixable→continue` back to `coder`, `requirement_change→escalate`). `vibe-ingest` and `vibe-owner-debate` use `user.confirm` and `submit_review_outcome` respectively (fix `I3`).

- `P-3` **Resolver split (Violations) + 2-Owner debate flow (cap 5, exactly 2, isolated).** In `gate_hook.go`/`interactive_service.go`, keep `flowgate.Evaluate` pure; walk `EnforceResult.Violations`:
  ```go
  res := flowgate.Evaluate(tr, enabledFor(workingMode)) // enabledFor hides r-requirement in dev
  // Compute RequirementDrift (including green+Tampered) BEFORE the existing child block→parent escalate path (gate_hook.go:1209-1228)
  if containsID(res.Violations, "r-requirement") {
      render AskUserCard("requirement", detailFor(res.Violations, "r-requirement")); return // vibe-only; still always-block in gate terms
  }
  if res.Action != Block && res.Action != Reprompt { return }
  if run.WorkingMode == "vibe" {
      start FlowDefinition("vibe-owner-debate") // resolver knows only the id, not owner.md
  } else {
      render DevCard(res) // byte-for-byte today
  }
  ```
  `vibe-owner-debate` declares two `agent.delegate` owners (`cohort: owner_debate`, `join: all`, `dependsOn: [debate_trigger]`, `agent: agents/owner.md`, isolated sessions per `SS-11` `§4`) synthesized by `debate_synthesis`; cap 5 (`policy: {cap:5}`) with `BUG-231`/`BUG-234` settlement (`WAITING_USER_APPROVAL` with `BlockReason: cap|requirement`, gating all auto-advance paths until `continue`).

- `P-4` **Desktop/TUI `SS Preview & Lock` UX + ingest → sprint_slicer wiring (tasks AI-auto).** Desktop: file picker (`/vibe` flow) that triggers `vibe-ingest` and renders the `ss_lock` (`user.confirm`) node as an editable `SS Preview & Lock` card; edits are **persisted** (write-back to `requirements/05-System-Specs/SS-*.md` drafts), **re-validated** against `SS-13`, then frozen; `Lock` resumes to `sprint_slicer` which auto-slices `sprint_plan` (verbatim if pre-sliced, else synthesized, per `SD-24 D-7` — no user card) and sequentially starts `vibe-sprint` per slice. Guard that `vibe-sprint` refuses to start while `ss_lock` is still `WAITING_USER_APPROVAL`. TUI: `/vibe <path|prompt>` and `/vibe-file` plus the same lock card on the flow timeline (reuse `WAITING_USER_APPROVAL` plumbing). Task breakdown per sprint is AI-auto (`SS-18 BR-2`) and never a user gate. No Admin Web work.

- `P-5` **Kill-Review boundary + demo + Dev non-regression.** Freeze claims: after the single entry lock, Vibe user-asks are exactly `r-requirement` and 5-round cap no-consensus; Dev mode never starts `vibe-owner-debate`. Demonstrate with a fixture game requirement file (pre-sliced branch and vague branch) over all three providers (same-model + cross-provider Owner pairs) — one run where `r-requirement` is clean and one where a green-but-drifted or green+tampered signature is caught as `block/escalate` (no Owners). Prove `dev` regression: identical `r-*` fixture in `dev` renders the Dev `1/2/3` / `block` modal, never reaches Owners, and `r-requirement` is inert even if an advisory were injected.

- `P-6` **CP-driven entry (`CP → Task → code tới done` in vibe).** New skeleton `flows/vibe-cp-ingest.yaml` (`selectableIn: []`, `policy: {cap:3, onCap: escalate}`), topology `cp_reader (delegate, vibe-intake, once) → cp_validator (hub.inline, once) → cp_lock (user.confirm, once) → task_slicer (delegate, doc-writer + prompts/task-splitter.md, once) → done`, edges `cp_lock --done--> task_slicer`, single `continue/back` `cp_lock → cp_reader`, `cp_lock/task_slicer --escalate--> ask_user`. `cp_reader` accepts only `requirements/07-Coding-Plan/**/CP-*.md` (validated `Document ID: CP-*` + `SS-13` CP §§1–10 incl. `P-*`); `cp_validator` checks CP contract + that referenced SS/Tasks resolve; `cp_lock` renders as editable `CP Preview & Lock` card (write-back to the CP draft + re-validate, `Lock` → `task_slicer`, `Continue` → re-read, `Escalate` → `ask_user`). `task_slicer` reuses `CP-58` bindings verbatim (`cp_md` INPUT → `task_md[]` OUTPUT, `pathTemplate: requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md`): if the CP already lists Tasks, emit verbatim; else AI-synthesize and persist as `vibe.task_plan` (same artifact type, uniform downstream). On `done`, the runner sequentially starts `vibe-sprint` v2 per Task slice (same per-sprint `context → tdd → coder → validate → synthesis → audit` + `r-requirement` + Owner-debate resolver as Branch V; explicitly not `task-harness`, so no Dev cards). Guards: `vibe-sprint` refuses to start while `cp_lock` is `WAITING_USER_APPROVAL`; restart replays `working_mode=vibe` + Task index + Owner round (same `sessions.ndjson` sink); mixed verbatim+synthesized Tasks are marked so later `r-requirement` attributes drift without re-slicing verbatim Tasks.

- `P-7` **`vibe-sprint` v2 (accuracy parity with `task-harness`, auto-only).** Rewrite `flows/vibe-sprint.yaml` to `preflight_contract_plan (delegate, once) → preflight_contract_freeze (inline, once) → context (inline, once, context.produce) → tdd (delegate, reinvoke) → coder (delegate, agent.code, reinvoke) → validate (inline, once, command.validate) → synthesis (hub.inline, reinvoke) → audit (inline, once, artifact.audit_draft) → done`, single `continue/back synthesis → coder`, `escalate → ask_user`, `policy: {cap:3, onCap: escalate}`, `acceptance_nodes: [validate, synthesis, audit]`. Parity mapping (nothing user-gated): harness `context.produce` → v2 `context`; `test_signatures` → `tdd`; `implement` → `coder`; `validate` → `validate`; `reviewer + synthesis` → `synthesis` + resolver `vibe-owner-debate`; `audit` → `audit`; harness plan-review loop has no v2 counterpart by design (scope already approved at `ss_lock`/`cp_lock` + slicer). Freeze still dominates every writer path; all `done` paths cross `acceptance_nodes` (`ValidateFlowSafetyTopology`). `context` keeps `DONE` on code-loop re-entry (scoped reset via `forwardReachableNodeIDs`, same rule as `CP-58` `R-2`).

## 5. Touched Areas

- **Files:** `apps/local-runner/internal/agentpack/flow-pack/{manifest.yaml, agents/{owner,vibe-intake}.md, flows/vibe-*.yaml, tools/vibe-requirement-outcome.yaml}` (already landed+skeleton, `89fe174a` + fixes, `selectableIn: []` for all vibe flows) + `P-6` new `flows/vibe-cp-ingest.yaml` (skeleton, `selectableIn: []`) reusing `prompts/task-splitter.md` + `agents/doc-writer.md` from `CP-58` + `P-7` `flows/vibe-sprint.yaml` v2 (`+context +validate +audit`), `apps/local-runner/internal/flowgate/{rules.go,evaluate.go,enforce.go,observe.go}`, `apps/local-runner/internal/runner/{interactive_service.go,gate_hook.go,local_file_session_store.go}`, `apps/desktop-flowpilot/**`, `apps/cli-tui/**`, `requirements/05-System-Specs/SS-18*`, `requirements/06-System-Tech-Design/SD-24*`.
- **Modules:** `agentpack` (pack load + validation — `user.confirm` + `ValidateFlowSafetyTopology`; `P-6` adds `cp_lock=user.confirm` + `task_slicer` delegate with `file_artifact` bindings; `P-7` extends `vibe-sprint` with inline `context`/`validate`/`audit`, single `continue/back`), `flowgate` (vibe-only `r-requirement`, `always-block`, `Tampered` subsumption — unchanged by `P-6`/`P-7`), `runner` (working_mode local SSOT + resolver before child escalate + `ss_lock`→`sprint_slicer` and `cp_lock`→`task_slicer`; entry auto-detect CP vs raw + `/vibe-cp` override), Desktop/TUI clients.
- **Database:** No new Supabase column; `working_mode` and `vibe.locked_ss` + `vibe.sprint_plan` + `P-6` `vibe.locked_cp` + `vibe.task_plan` live in `sessions.ndjson` (`CP-36 P-5`). No other DDL; flow definitions stay on Supabase.
- **External systems:** None. Provider adapters unchanged except per-node `owner` provider/model selection (same plumbing as `SS-04 §3.1`).

## 6. Data or Migration Steps

- **Schema:** No `workflow_runs` migration. `WorkingMode` is a new `localFileSessionStore` JSON field (default `dev`); `vibe.locked_ss` (file_artifact `requirements/05-System-Specs/SS-*.md`) and `vibe.sprint_plan` (file_artifact `.flowpilot/vibe/sprint_plan.json` with `structure: {sections: ["sprints"]}`) are typed artifacts (`SD-24 §5`); `P-6` adds `vibe.locked_cp` (file_artifact `requirements/07-Coding-Plan/**/CP-*.md`) and `vibe.task_plan` (file_artifact `requirements/08-Task/todo/Task-*.md` list, same `task_md` instance shape as `CP-58 P-4`). Verify `ValidateFlowDefinition` alias `user.confirm` and `ValidateFlowSafetyTopology` still pass (both `vibe-ingest` and `vibe-cp-ingest` are writer-free except scoped doc writes).
- **Data backfill:** None; existing runs treat missing `WorkingMode` as `dev`; existing flow definitions need no change.
- **Config updates:** No user-facing config migration; `working_mode` is per-run input (optional project default later). Manifest/builtin sync validated by `pack_test.go:23` (`6 flows`, `7 agents` for Branch V; `7 flows` after `P-6` lands `vibe-cp-ingest`).

## 7. Validation Plan

- **Tests to add (additive only):**
  - `internal/agentpack`: `LoadBuiltinPack` asserts 6 flows + 7 agents (Branch V), `7 flows` after `P-6`; explicit `vibe-ingest` (edits: `ss_lock=user.confirm` + `sprint_slicer`) + `vibe-sprint` v2 (`context` + `validate` + `audit` nodes, no owner nodes, no reviewer cohort, single `continue` back-edge `synthesis → coder`, `acceptance_nodes: [validate, synthesis, audit]`) + `vibe-owner-debate` existence; `P-6`: `vibe-cp-ingest` (`cp_lock=user.confirm` + `task_slicer` delegate with `cp_md` INPUT → `task_md[]` OUTPUT, single `continue` back-edge `cp_lock → cp_reader`) + `ValidateFlowSafetyTopology` passes for both ingests and v2 sprint (freeze dominates writers, `done` paths cross acceptance); `vibe-intake` tool scope `Read/Write/Edit/Grep/Glob` limited to SS docs (+ CP docs for `cp_reader`/`task_slicer` via `doc-writer`); `tdd` asserts `agents/tester.md` + signature-only contract (`prompts/test-signatures.md`), edge `tdd → coder` exists with no coder path bypassing `tdd`, and `synthesis` verifies signature↔`AC-*` coverage before any `done`.
  - `internal/flowgate`: `r-requirement` (vibe-only, `always-block`) fires only on `(Tests.Green && (RequirementDrift||Tampered))` as `block` and is absent in `dev` even if `RequirementDrift` were set; `r-additive-tests` weaken in vibe maps to `r-requirement`; `TamperedTestPaths` non-empty in vibe ⇒ `r-requirement`. Unchanged by `P-6` (same per-sprint gate for both branches).
  - `internal/runner`: resolver walks `Violations` (not `.Rule.ID`); `r-requirement ∈ violations ⇒ AskUser("requirement")` and no `vibe-owner-debate`; Admin `vibe` rejected; `ss_lock`/`cp_lock` `WAITING_USER_APPROVAL` gates `vibe-sprint` start; entry auto-detect (CP path/frontmatter → `vibe-cp-ingest`, else `vibe-ingest`) + explicit `/vibe-cp` override; single-owner fallback tagged `warn`.
- **Manual checks (Desktop/TUI, real sample game file + real CP):**
  - `/vibe sample-game.md` (pre-sliced) — SS Preview card editable, `Lock` persists edit + auto-slices verbatim `sprint_plan`, 3 sprints run `context → tdd → coder → validate → synthesis → audit`; generic gate → `vibe-owner-debate` auto, only `r-requirement` produces requirement card.
  - `/vibe "vague idea: 2D roguelike..."` — AI-sliced sprint_plan (inspect), no SS beyond the lock; edits in SS lock survive slicing.
  - `/vibe-cp requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md` (or any `CP-*.md`) — CP Preview card editable, `Lock` persists edit + re-validates CP contract, `task_slicer` emits `Task-*.md` list (verbatim if CP pre-slices, else synthesized), then sequential `vibe-sprint` v2 per Task runs to done with the same `r-requirement`/Owner-debate behavior as Branch V.
  - Demo fixtures & evidence (both branches): fixture `sample-game.md` (pre-sliced, 3 sprints) + vague prompt (`2D roguelike`) + fixture `CP-*.md` (≥2 Tasks); provider matrix same-model + cross-provider Owner pairs; per-sprint evidence = `tdd` signature artifact (use/edge/error per `SS-04 §3.5.8`) + suite/TTR (`validate` green) + `synthesis` verdict + `audit` ledger; timeline shows zero per-sprint/per-task user cards.
- **Failure cases:** Single-owner fallback degraded warning; mixed pre-sliced+vague input; runner restart mid-`ss_lock`/`cp_lock` and mid-Owner round 3; non-CP file passed to `/vibe-cp` rejected deterministically (frontmatter check); coder attempted without a `tdd` artifact is refused (no bypass path); `tdd` signatures missing edge/error cases are caught at `synthesis` as `continue`, never `done`; total-sprint budget exceeded stops with `BlockReason: budget` (no silent continuation); `r-requirement` card renders plain-language `AC-*`↔signature mapping for a non-tech reader; tighten that weakening a test to go green always becomes `r-requirement`, never `continue` via Owners.

## 8. Rollout and Fallback

- **Rollout order:** Skeletons landed first (`89fe174a`, now fixed to `selectableIn: []` + `user.confirm`), inert. Then `P-1` (enum local), then `P-2` (vibe-only `r-requirement` always-block), then `P-3` (resolver Violations → FlowDefinition id, before child escalate), then `P-4` (Desktop/TUI lock card + `sprint_slicer`), then `P-5` (boundary demo + regression for Branch V), then `P-6` (`vibe-cp-ingest` skeleton + `CP Lock` card + `task_slicer` + per-Task sprint loop), then `P-7` (`vibe-sprint` v2: `+context +validate +audit`). Each P merges independently; `P-5` gates Branch V approval, `P-6` gates Branch C approval, `P-7` gates parity approval (v2 demo re-runs one Branch V + one Branch C sprint).
- **Fallback path:** Before `P-3` merges, `P-1`/`P-2` default to `dev` with no user-visible change. After `P-3`, removing the resolver's `vibe` branch restores Dev behavior instantly; deleting `vibe-*.yaml` + `owner.md` + `vibe-intake.md` returns the pack to 3 flows + 4 agents (revert `pack_test.go:23` to 3); deleting only `vibe-cp-ingest.yaml` disables Branch C while Branch V keeps working (revert count `7 → 6`). No data migration to unwind.
- **Monitoring:** Log newly introduced `EnforceResult` containing `r-requirement` via existing gate audit path plus `BlockReason: requirement`; surface on the run timeline alongside existing `cap`/`escalate` settlement logs (`SD-19 §8 F-3`/`F-4`), including `ss_lock`/`cp_lock` requests.

## 9. Risks

- `R-1` **Gate mis-routing.** Sending a real `r-requirement` through Owners would hide drift (`SS-18 BR-8`). Mitigation: resolver walks `Violations` and `r-requirement` is the first branch before any `vibe-owner-debate` start; `isAlwaysBlock("r-requirement")` prevents warn-downgrade; regression probe `Tests.Green && (RequirementDrift||Tampered)==true ⇒ block/escalate, never done` in `P-2`.
- `R-2` **Owner echo chamber.** Two same-model Owners rubber-stamp each other. Mitigation: per-node provider/model independence (`SD-24 D-3`) + synthesis must check `r-requirement==false` before any `continue`; cap 5 forces human re-engagement; debate uses `submit_review_outcome`, not requirement face.
- `R-3` **Sprint granularity.** One vague file could induce 50 micro-sprints. Mitigation: `vibe-ingest`'s `sprint_slicer` after the lock is the sole slice point; persist the chosen slice for reviewer inspection and tune the converter prompt without changing the graph. Same for `P-6`: `task_slicer` after `cp_lock` is the sole slice point for Branch C.
- `R-4` **SS-lock regression.** A future change could auto-run sprints before the SS lock. Mitigation: guard in `interactive_service.go` that `vibe-sprint` refuses to start while `ss_lock` (or `P-6` `cp_lock`) is still `WAITING_USER_APPROVAL`; lock persits edited SS/CP with re-validation.
- `R-5` **Builtin drift.** Manifest ↔ `builtin.*` mismatch or `ValidateFlowDefinition` rejection (duplicate `continue` back-edge) fails pack load. Mitigation: existing CI `go vet` + `go test ./internal/agentpack -run TestLoadBuiltinPack` (`pack.go:769`).
- `R-6` **CP-task drift (Branch C).** A CP edit at `cp_lock` could invalidate already-sliced Tasks, or a per-Task `r-requirement` fix could tempt re-slicing verbatim Tasks. Mitigation: freeze the edited CP at lock time; `task_slicer` output records verbatim-vs-synthesized per Task; later drift attributes to the Task slice without re-slicing verbatim entries; any CP-scope change requires a new run (no silent re-slice mid-loop).
- `R-7` **Single-lock failure.** One `ss_lock`/`cp_lock` puts the whole run's intent on a single non-tech review; a bad lock plus an AI-self-graded `r-requirement` check could run N wrong sprints before surfacing. Mitigation (no extra user gate): (a) always render `sprint_plan`/`task_plan` read-only on the timeline right after slicing so the user can stop early; (b) enforce a total-sprint budget cap per run (default small, e.g. stop with `BlockReason: budget` when exceeded instead of silently continuing); (c) `r-requirement` card must show plain-language mapping (which SS `AC-*` ↔ which test signature drifted + what SS edit would fix it), not a raw `RequirementDriftDetail` dump. Trace to `SS-18 BR-6` (bounded + explicit); no `SS-18`/`SD-24` intent change.

## 10. Definition of Done

- All `SS-18 AC-1`..`AC-10` and `BR-1`..`BR-9` hold in `Desktop` and `TUI` (same commit, same providers): one vague-file run demonstrates `Attach → SS Preview & Lock (editable) → AI auto slice → 3× vibe-sprint` with zero `Task` lock cards and only the documented two user-ask kinds after lock if triggered.
- `P-6` Branch C demo: one `CP-*.md` run demonstrates `Attach CP → CP Preview & Lock (editable) → AI auto slice (verbatim-or-synthesized) → N× vibe-sprint per Task tới done`, with zero per-Task lock cards and identical `r-requirement`/Owner-debate behavior to Branch V.
- `r-requirement` is proven hard-gated and vibe-only: a green-but-drifted or green+tampered TDD signature set is caught as `block/escalate` → `WAITING_USER_APPROVAL` (`BlockReason: requirement`), and never `done`; weakening a test to go green is always classified as `r-requirement`; `dev` with same `RequirementDrift` does not fire `r-requirement`.
- TDD-first proven per sprint: the signature-only `tdd` artifact from the frozen slice exists before `coder` runs (no bypass path), covers use/edge/error cases per `SS-04 §3.5.8`, and `synthesis` verifies signature↔`AC-*` coverage; per-sprint evidence = tdd artifact + suite/TTR + verdict + audit ledger.
- Dev mode regression: identical `r-*` fixture in `working_mode=dev` renders the current Dev `1/2/3` / `block` modal, never starts `vibe-owner-debate` (evidence: resolver branch not taken, no `owner` session on that run, API rejects Admin `vibe`).
- `SS-18`/`SD-24` trace is closed, `vibe-ingest` (`ss_lock=user.confirm → sprint_slicer`) → `sprint_plan` → `vibe-sprint` v2 (no owner nodes, no reviewer cohort) → `vibe-owner-debate` on gate fail chain runs on a sample game spec, and `vibe-cp-ingest` (`cp_lock=user.confirm → task_slicer`) → `task_plan` → per-Task `vibe-sprint` v2 runs on a sample CP; `agentpack`/`flowgate`/`runner` `go test` + `go vet` are green on the new builtins (`selectableIn: []`, `7 flows`, `7 agents`).
- Single-lock guardrails proven: sliced plan is visible read-only on the timeline, total-sprint budget cap stops the run with `BlockReason: budget`, and the `r-requirement` card is non-tech readable (which `AC-*` drifted + what SS edit fixes it).
- A fresh Kill-Review pass on this CP + the just-landed `SS-18`/`SD-24` + 7-flow pack (mode `plan`) reaches `KILL_CLEAN` or `KILL_WITH_FINDINGS` with a frozen claim and no `KILL_BLOCKED` (matrix closure per `skill/kill-review`).

