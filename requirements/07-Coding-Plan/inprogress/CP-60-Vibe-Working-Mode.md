# CP-60: Vibe Working Mode (SS-lock + Owner Debate + r-requirement)

## Metadata

- Document ID: `CP-60`
- Title: `Vibe Working Mode — Desktop/TUI SS-Lock, TDD-First Sprint, Owner Debate, r-requirement`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-01`
- Last Updated: `2026-09-01`
- Parent Documents: [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: `None` (Tasks T-1..T-5 to be created)
- Related Documents: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [CP-36: Agent Review Loop And Main-Hub Orchestration](../done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md), [CP-45: Generic Artifact Types And Instances](../done/CP-45-Generic-Artifact-Types-And-Instances.md)
- Replaces: `None`
- Tags: `vibe-mode, coding-plan, desktop, tui, flow-gate, agent-flow, TDD`

## AI Quick View

### Summary

- Implement `SS-18`/`SD-24` as a thin `working_mode` policy over the shipped generic engine + gate hook. Pack skeletons (`vibe-ingest`, `vibe-sprint`, `vibe-owner-debate` + `owner` agent) are already in the repo and inert; this plan wires them with **zero engine reshape**.
- `working_mode ∈ {dev, vibe}` (default `dev`) splits only the **resolver** mapping of a single `flowgate.EnforceResult`: `dev` → Dev card/modal (today), `vibe` → Owner debate cohort, except `r-requirement` which is always user-only. Gate evaluation (`flowgate.Evaluate`) is unchanged.
- UX order is `SS ingest → SS Preview & Lock (Desktop/TUI, mandatory)` → AI auto slice tasks/sprints → per-sprint `tdd → coder → owners → synthesis` with `hub.inline` `r-requirement` 1:1 check. Tasks never ask the user.
- One new hard gate `r-requirement` captures green-but-drifted TDD signatures vs. frozen SS (`SS-14 AC-6`). It and the 5-round Owner no-consensus are the only Vibe user asks; everything else auto-retries via Owners.
- Rollout is additive and Kill-Review-safe: skeletons land first (done, `89fe174a`), then Go `working_mode` + `r-requirement` + resolver + Desktop/TUI `SS Lock` card, with a Dev non-regression probe.

### Current Ask

- Provide a task-sliced plan to make `vibe-ingest` → locked SS → auto-sliced `sprint_plan` → sequential `vibe-sprint` (with `vibe-owner-debate` for non-requirement gates) runnable end-to-end in Desktop + TUI, with zero Admin Web scope and Dev parity.

### Key Decisions

- `P-1` `working_mode` is the SSOT enum on the run (default `dev`); gated only at resolver, not at rule evaluation.
- `P-2` Single new rule `r-requirement` (`block`, `requirement_signature_drift`) fed by the hub's `RequirementDrift` advisory.
- `P-3` Owner cohort is exactly 2 isolated `agent.delegate` owners (`cohort: vibe_owner`/`owner_debate`, `join: all`) via Main, configurable provider/model, cap 5.
- `P-4` Desktop/TUI `SS Preview & Lock` is the only pre-sprint user gate; task/sprint slicing is AI-auto post-lock.
- `P-5` Skeletons-first, Go-second: no runner path auto-runs vibe flows until the Go work merges.

### Constraints

- Reuse `SD-19`/`CP-36` generic engine, `SD-20` gate hook, `SS-13` contract, `CP-36 P-5` local run sink (`sessions.ndjson`); no new Supabase run migration; Vibe lives only in Desktop + TUI.
- Do not change `ProviderRuntimeAdapter` except by declaring `vibe-requirement-outcome` tool face; do not alter YOLO SSOT (`SS-08`).
- No pre-existing test edited; additive `r-requirement` must preserve Dev behavior byte-for-byte.

### Open Questions

- `Q-1` (`SD-24 Q-1`) Dedicated `behavior: vibe.requirement_check` vs `hub.inline` prompt for the `r-requirement` node — settle in P-2.
- `Q-2` (`SD-24 Q-2`) Per-sprint `working_mode` override vs project-level enum only — default to run-level enum in P-1.

### Source Refs

- `SS-18 AC-1`..`AC-10`, `BR-1`..`BR-9`; `SD-24 D-1`..`D-7`, §3–§8; `SD-19 D-1`..`D-8`, `D-5` bridge (`agent_flow` step), `D-7` vocab; `SD-20 D-1`..`D-7`, `gate_hook.go`, `flowgate/rules.go`; `SS-13`/`FORMAT-REFERENCE-SS`, `SS-14 AC-6`, `SS-04 §3.5.8`; `pack.go:ValidateFlowDefinition`/`ValidateFlowSafetyTopology`; built-ins `review-loop.yaml`/`context-coding-review-synthesis.yaml`.

## 1. Goal

Make `SS-18`/`SD-24` executable: a non-tech user attaches one requirement file (or pastes an idea) in **Desktop app or TUI**, sees an `SS Preview & Lock` card, locks the SS, then the system **automatically** slices tasks/sprints and runs sprint-by-sprint (`vibe-sprint`) with `TDD → coder → Owners → synthesis` and a single new hard gate `r-requirement`. Only `r-requirement` and a 5-round Owner no-consensus ever ask the user; Dev mode is unchanged and proven non-regressive.

Pack skeletons for this goal are already landed (`89fe174a`, 6 flows total). This plan completes the Go wiring + Desktop/TUI UX.

## 2. Input Documents

- [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md) — `AC-1`..`AC-10`, `BR-1`..`BR-9`; scope clarifies Vibe is Desktop/TUI only, SS must lock before sprints, task slice is AI-auto.
- [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md) — `D-1`..`D-7`, data model `WorkingMode`, `TurnResult.RequirementDrift`, resolver split, topology `preflight_contract_plan → freeze → tdd → coder → owner_1∥owner_2 → synthesis`.
- [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) + [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md) + [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) — generic engine substrate, `D-5` bridge, gate hook contract.
- [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md) + `FORMAT-REFERENCE-SS` / `FORMAT-REFERENCE-CP` + `SS-14`/`SS-04`/`SS-08`/`SS-11`.

## 3. Implementation Strategy

- **Overall approach:** Additive policy layer mirroring `D-1`..`D-7` in `SD-24`: introduce a run-level `working_mode` enum that the gate resolver reads, add one pure gate rule `r-requirement` whose signal is injected by the `vibe-sprint` synthesis hub, spawn the existing `owner` cohort for every other violation in `vibe`, and render the new `SS Preview & Lock` + `r-requirement` cards only in Desktop/TUI. The engine stays domain-free (`SD-19 D-1`); Vibe is data.
- **Sequencing logic (bottom-up, each P independently testable; skeletons already inert):**
  1. `P-1` `working_mode` SSOT + persistence (no UI except a default).
  2. `P-2` `r-requirement` rule + advisory plumbing + declared `vibe-requirement-outcome` face.
  3. `P-3` Resolver split + Owner cohort (exactly 2, isolated, cap 5) + settlement `BUG-231`/`BUG-234`.
  4. `P-4` Desktop/TUI `SS Preview & Lock` UX + skeletons UX wiring (ingest → lock → auto slice).
  5. `P-5` Kill-Review boundary + demo scenario + Dev non-regression.
- **Dependencies:** `P-3` → `P-1`+`P-2`. `P-4` → `P-1` (lock gates sprints) and `P-2` (r-requirement card). `P-2` is otherwise self-contained; `P-5` depends on `P-1`..`P-4`. No new migration beyond wiring the enum.

## 4. Work Breakdown

- `P-1` **Run `working_mode` SSOT (`dev` | `vibe`, default `dev`).** Add `type WorkingMode string` and `WorkingModeVibe`/`Dev` constants in `internal/runner` + `internal/agentpack` model area, persist on the run record (interactive run struct + `localFileSessionStore` JSON, plus a `workflow_runs.working_mode text check` column for Supabase header — definitions remain on Supabase, run sink stays local per `CP-36 P-5`). Accept `working_mode` at `POST /client/workflow-runs` and `POST /client/flows/run` (per-run override; optional project-level fallback). Default to `dev`; no definition-table migration beyond wiring. Entry for Vibe is exposed only via `apps/desktop-flowpilot` file picker and `cli-tui` (`/vibe` / `/vibe-file`) — **Admin Web receives no Vibe entry**.

- `P-2` **`r-requirement` gate + `vibe-requirement-outcome` tool face.** Append to `internal/flowgate/rules.go:DefaultRules()`:
  ```go
  {ID: "r-requirement", Scope: "step", Trigger: "requirement_signature_drift", RequiredOutput: "reconcile_tests_with_ss", Action: "block", Enabled: true}
  ```
  Extend `TurnResult` with `RequirementDrift bool` + `RequirementDriftDetail string` (parallel to `ScopeHighSeverity` injection). The `vibe-sprint` `synthesis` hub (inline `hub.inline` or future `vibe.requirement_check` alias — `SD-24 Q-1` settled here) computes the 1:1 signature↔frozen-SS check after a green suite and populates the advisory before `flowgate.Evaluate`. Register `tools/vibe-requirement-outcome.yaml` as a `flow_control` declared face (`aligned→done`, `drift_fixable→continue`, `requirement_change→escalate`).

- `P-3` **Resolver split + 2-Owner debate cohort (cap 5, exactly 2, isolated).** In `gate_hook.go`/`interactive_service.go`, keep `flowgate.Evaluate` unchanged (single highest `EnforceResult`). Branch the resolver:
  ```go
  v := flowgate.Evaluate(tr)
  if v.Rule.ID == "r-requirement" { render AskUser("requirement", v.Detail) } // both modes, block
  else if run.WorkingMode == "vibe" { spawn owner_1∥owner_2 (cohort vibe_owner/owner_debate, join:all, dependsOn coder/trigger, each agents/owner.md, independent provider/model per node, isolated sessions per SS-11 §4) → hub synthesis }
  else { render DevCard(v) } // byte-for-byte today
  ```
  Configure `vibe-owner-debate` (cap 5) and the `vibe-sprint` owner leg (each `owner_*` spawn) per `SD-24 D-3`/`D-4`; enforce `BUG-231`/`BUG-234` settlement (hub → `WAITING_USER_APPROVAL` with `BlockReason: cap|requirement`, gate all auto-advance paths until `continue`).

- `P-4` **Desktop/TUI `SS Preview & Lock` UX + ingest → auto-slice wiring (tasks AI-auto).** Desktop: file picker (`/vibe` flow) that triggers `vibe-ingest` and renders the `ss_validator` node as an editable `SS Preview & Lock` card; `Lock` resumes the run and only then does the runner auto-slice `sprint_plan` (verbatim if pre-sliced, else synthesized, per `SD-24 D-7` — no user card) and sequentially start `vibe-sprint` per slice. TUI: `/vibe <path|prompt>` and `/vibe-file` plus the same lock card on the flow timeline (reuse `WAITING_USER_APPROVAL` plumbing). Task breakdown per sprint is AI-auto (`BR-2`) and never a user gate. No Admin Web work.

- `P-5` **Kill-Review boundary + demo + Dev non-regression.** Freeze claims: Vibe user-asks are exactly `r-requirement` and 5-round cap no-consensus; Dev mode never spawns Owners. Demonstrate with a fixture game requirement file (pre-sliced branch and vague branch) over all three providers (same-model + cross-provider Owner pairs) — one run where `r-requirement` is clean and one where a green-but-drifted TDD signature is caught as `block/escalate`. Prove `dev` regression: identical `r-*` fixture in `dev` renders the Dev `1/2/3` / `block` modal and never reaches Owner code.

## 5. Touched Areas

- **Files:** `apps/local-runner/internal/agentpack/flow-pack/{manifest.yaml, agents/owner.md, flows/vibe-*.yaml, tools/vibe-requirement-outcome.yaml}` (already landed+skeleton, `89fe174a`), `apps/local-runner/internal/flowgate/{rules.go,evaluate.go,enforce.go}`, `apps/local-runner/internal/runner/{interactive_service.go,gate_hook.go,local_file_session_store.go,supabase_*.go}`, `apps/desktop-flowpilot/**`, `apps/cli-tui/**`, `requirements/05-System-Specs/SS-18*`, `requirements/06-System-Tech-Design/SD-24*`.
- **Modules:** `agentpack` (pack load + validation), `flowgate` (rule evaluation), `runner` (working_mode SSOT + resolver + session store), Desktop/TUI clients.
- **Database:** Single column addition `workflow_runs.working_mode text check (working_mode in ('dev','vibe'))` (default `dev`); run history otherwise stays in `sessions.ndjson` (`CP-36 P-5`). No other Supabase DDL.
- **External systems:** None. Provider adapters unchanged except per-node `owner` provider/model selection (same plumbing as `SS-04 §3.1`).

## 6. Data or Migration Steps

- **Schema:** Add `workflow_runs.working_mode` (nullable text, default `dev`, check constraint). Backfill existing rows to `dev` (or treat null as `dev` in code — runner default, no blocking migration).
- **Data backfill:** None beyond the default; existing flow-store definitions need no change (Vibe is run-level, not definition-level).
- **Config updates:** No user-facing config migration; `working_mode` is per-run input (optional project default later). Manifest/builtin sync already validated by `pack_test.go:23` (`6 flows`).

## 7. Validation Plan

- **Tests to add (additive only):**
  - `internal/agentpack`: `LoadBuiltinPack` already asserts 6 flows (`pack_test.go:23`); extend with explicit `vibe-*` existence + `ValidateFlowSafetyTopology` passes for `vibe-sprint` (writer `coder` dominated by freeze, acceptance `synthesis`).
  - `internal/flowgate`: `r-requirement` fires only on `Tests.Ran && len(Failed)==0 && RequirementDrift==true` as `block`, and its detail names the drifted `AC-*` / `SS` section.
  - `internal/runner`: resolver unit — `RequirementDrift==true` routes to `requirement` card in both modes; `RequirementDrift==false` generic `r-*` routes to Owner cohort in `vibe` and to Dev card in `dev`; 5-round cap re-arms settlement contract.
- **Manual checks (Desktop/TUI, real sample game file):**
  - `/vibe sample-game.md` (pre-sliced) — SS Preview card appears, `Lock` unlocks auto-slice, 3 sprints run `tdd → coder → Owners → synthesis`; only an intentional signature↔SS drift produces the single requirement user card.
  - `/vibe "vague idea: 2D roguelike..."` — same chain but AI-sliced sprint_plan (inspect persisted artifact), no SS beyond the lock.
- **Failure cases:** Single-owner fallback degraded warning; mixed pre-sliced+vague input; runner restart mid-SS-lock and mid-Owner round 3; tighten that weakening a test to go green always becomes `r-requirement`, never `continue`.

## 8. Rollout and Fallback

- **Rollout order:** Skeletons landed first (`89fe174a`, inert). Then `P-1` (enum), then `P-2` (gate), then `P-3` (resolver+Owners), then `P-4` (Desktop/TUI lock card), then `P-5` (boundary demo + regression). Each P merges independently; `P-5` gates approval.
- **Fallback path:** Before `P-3` merges, `P-1`/`P-2` default to `dev` with no user-visible change. After `P-3`, removing the resolver's `vibe` branch restores Dev behavior instantly; deleting `vibe-*.yaml` + `owner.md` returns the pack to 3 flows (revert `pack_test.go:23` to 3). No data migration to unwind.
- **Monitoring:** Log newly introduced `EnforceResult.Rule.ID == r-requirement` via existing gate audit path plus `BlockReason: requirement`; surface on the run timeline alongside existing `cap`/`escalate` settlement logs (`SD-19 §8 F-3`/`F-4`).

## 9. Risks

- `R-1` **Gate mis-routing.** Sending a real `r-requirement` through Owners would hide drift (`SS-18 BR-8`). Mitigation: `r-requirement` is the first resolver branch before any Owner spawn; regression probe `Tests.Green && RequirementDrift==true ⇒ block/escalate, never done` in `P-2`.
- `R-2` **Owner echo chamber.** Two same-model Owners rubber-stamp each other. Mitigation: per-node provider/model independence (`SD-24 D-3`) + Main synthesis must check `r-requirement==false` before `continue`; cap 5 forces human re-engagement.
- `R-3` **Sprint granularity.** One vague file could induce 50 micro-sprints. Mitigation: ingest's auto-slice (`SD-24 D-7`) is the sole slice point; persist the chosen slice for reviewer inspection and tune the converter prompt without changing the graph.
- `R-4` **SS-lock regression.** A future change could auto-run sprints before the SS lock. Mitigation: guard in `interactive_service.go` that `vibe-sprint` refuses to start while `vibe-ingest`'s `ss_validator` is still `WAITING_USER_APPROVAL`.
- `R-5` **Builtin drift.** Manifest ↔ `builtin.*` mismatch or `ValidateFlowDefinition` rejection (duplicate `continue` back-edge) fails pack load. Mitigation: existing CI `go vet` + `go test ./internal/agentpack -run TestLoadBuiltinPack` (`pack.go:769`).

## 10. Definition of Done

- All `SS-18 AC-1`..`AC-10` and `BR-1`..`BR-9` hold in `Desktop` and `TUI` (same commit, same providers): one vague-file run demonstrates `Attach → SS Preview & Lock → AI auto slice → 3× vibe-sprint` with zero `Task` lock cards and only the documented two user-ask kinds if triggered.
- `r-requirement` is proven hard-gated: a green-but-drifted TDD signature set is caught by the synthesis hub's 1:1 check, mapped to `block/escalate` → `WAITING_USER_APPROVAL` (`BlockReason: requirement`), and never `done`; weakening a test to go green is always classified as `r-requirement`.
- Dev mode regression: identical `r-*` fixture in `working_mode=dev` renders the current Dev `1/2/3` / `block` modal and never spawns an Owner (evidence: resolver branch not taken, no `owner` session on that run).
- `SS-18`/`SD-24` trace is closed, `vibe-ingest` → `sprint_plan` → `vibe-sprint` chain runs on a sample game spec, and `agentpack`/`flowgate`/`runner` `go test` + `go vet` are green on the new builtins.
- A fresh Kill-Review pass on this CP + the just-landed `SS-18`/`SD-24` + 6-flow pack (mode `plan`) reaches `KILL_CLEAN` or `KILL_WITH_FINDINGS` with a frozen claim and no `KILL_BLOCKED` (matrix closure per `skill/kill-review`).

