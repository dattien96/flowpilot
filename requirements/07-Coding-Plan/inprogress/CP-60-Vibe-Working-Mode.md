# CP-60: Vibe Working Mode (SS-lock + Owner Debate + r-requirement)

## Metadata

- Document ID: `CP-60`
- Title: `Vibe Working Mode — Desktop/TUI SS-Lock, TDD-First Sprint, Owner Debate, r-requirement`
- Feature Keys: `vibe-mode`
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

- Implement `SS-18`/`SD-24` as a thin `working_mode` policy over the shipped generic engine + gate hook. Pack skeletons (`vibe-ingest`, `vibe-sprint`, `vibe-owner-debate` + `owner`/`vibe-intake` agents) are already in the repo and inert; this plan wires them with **zero engine reshape**.
- `working_mode ∈ {dev, vibe}` (default `dev`, local only) splits only the **resolver** mapping of `flowgate.EnforceResult.Violations`: `r-requirement` → requirement card (vibe-only, `always-block`), otherwise `vibe` → start `vibe-owner-debate` flow (exactly 2 isolated owners, cap 5), `dev` → Dev card. Gate evaluation stays pure.
- UX order is `SS ingest → SS Preview & Lock via user.confirm (Desktop/TUI, mandatory)` → AI auto slice tasks/sprints → per-sprint `preflight_contract_plan → freeze → tdd → coder → synthesis` (hub owns 1:1 check). Tasks never ask the user.
- One new hard gate `r-requirement` (vibe-only) captures green-but-drifted or green+`TamperedTestPaths` weakening (`SS-14 AC-6`, `r-additive-tests` subsumed). It and the 5-round Owner no-consensus are the only Vibe user asks.
- Rollout is additive and Kill-Review-safe: skeletons land first (done, `89fe174a`), then Go `working_mode` + `r-requirement` + resolver + Desktop/TUI `SS Lock` card, with a Dev non-regression probe.

### Current Ask

- Provide a task-sliced plan to make `vibe-ingest` → locked SS → auto-sliced `sprint_plan` → sequential `vibe-sprint` (non-requirement gates via `vibe-owner-debate`) runnable end-to-end in Desktop + TUI, with zero Admin Web scope and Dev parity.

### Key Decisions

- `P-1` `working_mode` is the SSOT enum on the local run record only (default `dev`); gated at resolver, not at rule evaluation; Desktop/TUI only.
- `P-2` Single new rule `r-requirement` (vibe-only, `block`, `always-block`, `requirement_signature_drift`) fed by the hub's `RequirementDrift` advisory (including green+`TamperedTestPaths`); `isAlwaysBlock` extended, enabled-filter hides it in `dev`.
- `P-3` Resolver maps `Violations` to UX/FlowDefinition id only (`vibe-owner-debate`), never names `owner.md` in Go; Owners are exactly 2 isolated cohort members via Main, `join: all`, cap 5.
- `P-4` Desktop/TUI `SS Preview & Lock` is the only pre-sprint user gate (`user.confirm` at `ss_lock` with edit write-back + re-validate); task/sprint slicing is AI-auto post-lock.
- `P-5` Skeletons-first, Go-second: no runner path auto-runs vibe flows until the Go work merges.

### Constraints

- Reuse `SD-19`/`CP-36` generic engine, `SD-20` gate hook, `SS-13` contract, `CP-36 P-5` local run sink (`sessions.ndjson`); no new Supabase run migration; Vibe lives only in Desktop + TUI.
- Do not change `ProviderRuntimeAdapter` except by declaring `vibe-requirement-outcome` (for `vibe-sprint`) and reusing `submit_review_outcome` (for `vibe-owner-debate`); do not alter YOLO SSOT (`SS-08`).
- No pre-existing test edited; additive `r-requirement` is vibe-only and preserves Dev byte-for-byte.

### Open Questions

- `Q-1` Resolved — requirement check reuses `hub.inline` inline prompt; no dedicated `vibe.requirement_check` behavior.
- `Q-2` Resolved — `working_mode` is run-level enum; project default optional later; Vibe entry Desktop/TUI only.

### Source Refs

- `SS-18 AC-1`..`AC-10`, `BR-1`..`BR-9`; `SD-24 D-1`..`D-7`, §3–§8; `SD-19 D-1`..`D-8`, `D-5` bridge (`agent_flow` step), `D-7` vocab; `SD-20 D-1`..`D-7`, `gate_hook.go:1209-1228` (`applyFlowControl` child→parent escalate), `flowgate/{rules,evaluate,enforce,observe}.go`; `SS-13`/`FORMAT-REFERENCE-SS`, `SS-14 AC-6`, `SS-04 §3.5.8`; `pack.go:ValidateFlowDefinition`/`ValidateFlowSafetyTopology` (`user.confirm` alias), `enforce.go:isAlwaysBlock`; built-ins `review-loop.yaml`/`context-coding-review-synthesis.yaml`; `r-additive-tests` / `TamperedTestPaths`.

## 1. Goal

Make `SS-18`/`SD-24` executable: a non-tech user attaches one requirement file (or pastes an idea) in **Desktop app or TUI**, sees an editable `SS Preview & Lock` card, locks the SS, then the system **automatically** slices tasks/sprints and runs sprint-by-sprint (`vibe-sprint`: `TDD → coder → synthesis` with hub 1:1 check). Any non-`r-requirement` violation in `vibe` auto-starts `vibe-owner-debate` (cap 5); only `r-requirement` and that 5-round no-consensus ever ask the user; Dev mode is unchanged.

Pack skeletons for this goal are already landed (`89fe174a`, 6 flows total, `selectableIn: []` for all vibe flows) and `vibe-intake` read+write scoped to SS docs. This plan completes the Go wiring + Desktop/TUI UX.

## 2. Input Documents

- [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md) — `AC-1`..`AC-10`, `BR-1`..`BR-9`; scope clarifies Vibe is Desktop/TUI only, SS must lock before sprints, task slice is AI-auto, Desktop/TUI entry.
- [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md) — `D-1`..`D-7`, data model `WorkingMode` (local only), `TurnResult.RequirementDrift`, resolver walking `Violations`, topology `preflight_contract_plan → freeze → tdd → coder → synthesis` and `vibe-ingest` `ss_lock=user.confirm → sprint_slicer`.
- [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) + [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md) + [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) — generic engine substrate, `D-5` bridge, gate hook contract (`isAlwaysBlock`, child→parent escalate).
- [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md) + `FORMAT-REFERENCE-SS` / `FORMAT-REFERENCE-CP` + `SS-14`/`SS-04`/`SS-08`/`SS-11`.

## 3. Implementation Strategy

- **Overall approach:** Additive policy layer mirroring `D-1`..`D-7` in `SD-24`: introduce a run-level `working_mode` enum (local only) that the resolver reads via `enabledRulesFor(workingMode)`, add one vibe-only pure gate rule `r-requirement` whose signal is injected by the `vibe-sprint` synthesis hub **before** child `block → parent escalate`, map `EnforceResult.Violations` to UX/FlowDefinition id only, and render the new `SS Preview & Lock` (`user.confirm`) + `r-requirement` cards only in Desktop/TUI. The engine stays domain-free (`SD-19 D-1`); Vibe is data; Admin Web `vibe` is rejected.
- **Sequencing logic (bottom-up, each P independently testable; skeletons already inert):**
  1. `P-1` `working_mode` SSOT + persistence (local only, no UI except default `dev`; Admin reject).
  2. `P-2` `r-requirement` rule (vibe-only, `always-block`, subsumes weaken) + advisory plumbing + declared `vibe-requirement-outcome` face on `vibe-sprint` only.
  3. `P-3` Resolver split (walks `Violations`) + `vibe-owner-debate` cohort (exactly 2, isolated, cap 5) + settlement `BUG-231`/`BUG-234`, triggered before child escalate.
  4. `P-4` Desktop/TUI `SS Preview & Lock` UX (editable, write-back, re-validate) + ingest→`sprint_slicer` auto wiring (tasks AI-auto).
  5. `P-5` Kill-Review boundary + demo scenario + Dev non-regression.
- **Dependencies:** `P-3` → `P-1`+`P-2`. `P-4` → `P-1` (lock gates sprints) and `P-2` (r-requirement card). `P-2` is otherwise self-contained; `P-5` depends on `P-1`..`P-4`. No Supabase DDL.

## 4. Work Breakdown

- `P-1` **Run `working_mode` SSOT (`dev` | `vibe`, default `dev`, local only).** Add `type WorkingMode string` and constants in `internal/runner` (+ `internal/agentpack` if needed), persist **only** on the local run record (`localFileSessionStore` JSON — `sessions.ndjson`, `SS-11` `§9`; definitions stay on Supabase). No `workflow_runs.working_mode` column (resolves `C9`). Accept `working_mode` at `POST /client/workflow-runs` and `POST /client/flows/run` only when `X-Client: desktop|tui`; `Admin Web` (`X-Client: admin` or missing) with `vibe` returns `403`. Default to `dev`; no definition-table migration. Entry for Vibe is exposed only via `apps/desktop-flowpilot` file picker and `cli-tui` (`/vibe` / `/vibe-file`).

- `P-2` **`r-requirement` gate + `vibe-requirement-outcome` tool face (vibe-only, always-block).** Append to `internal/flowgate/rules.go:DefaultRules()`:
  ```go
  {ID: "r-requirement", Scope: "step", Trigger: "requirement_signature_drift", RequiredOutput: "reconcile_tests_with_ss", Action: "block", Enabled: true}
  ```
  Extend `isAlwaysBlock` to include `r-requirement` and filter `Enabled` by `workingMode` (vibe-only: `dev`'s enabled set omits `r-requirement`). Extend `TurnResult` with `RequirementDrift bool` + `RequirementDriftDetail string`; in `vibe` green+`TamperedTestPaths` (weaken) is coerced to `RequirementDrift=true` before `Evaluate` (so `r-additive-tests` in vibe is subsumed — `C8`). The `vibe-sprint` `synthesis` hub (`hub.inline` + inline prompt) computes the 1:1 signature↔frozen-SS check after a green suite and populates the advisory before `flowgate.Evaluate`. Register `tools/vibe-requirement-outcome.yaml` as a `flow_control` declared face on `vibe-sprint` only (`aligned→done`, `drift_fixable→continue` back to `coder`, `requirement_change→escalate`). `vibe-ingest` and `vibe-owner-debate` use `user.confirm` and `submit_review_outcome` respectively (fix `I3`).

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

- `P-5` **Kill-Review boundary + demo + Dev non-regression.** Freeze claims: Vibe user-asks are exactly `r-requirement` and 5-round cap no-consensus; Dev mode never starts `vibe-owner-debate`. Demonstrate with a fixture game requirement file (pre-sliced branch and vague branch) over all three providers (same-model + cross-provider Owner pairs) — one run where `r-requirement` is clean and one where a green-but-drifted or green+tampered signature is caught as `block/escalate` (no Owners). Prove `dev` regression: identical `r-*` fixture in `dev` renders the Dev `1/2/3` / `block` modal, never reaches Owners, and `r-requirement` is inert even if an advisory were injected.

## 5. Touched Areas

- **Files:** `apps/local-runner/internal/agentpack/flow-pack/{manifest.yaml, agents/{owner,vibe-intake}.md, flows/vibe-*.yaml, tools/vibe-requirement-outcome.yaml}` (already landed+skeleton, `89fe174a` + fixes, `selectableIn: []` for all vibe flows), `apps/local-runner/internal/flowgate/{rules.go,evaluate.go,enforce.go,observe.go}`, `apps/local-runner/internal/runner/{interactive_service.go,gate_hook.go,local_file_session_store.go}`, `apps/desktop-flowpilot/**`, `apps/cli-tui/**`, `requirements/05-System-Specs/SS-18*`, `requirements/06-System-Tech-Design/SD-24*`.
- **Modules:** `agentpack` (pack load + validation — `user.confirm` + `ValidateFlowSafetyTopology`), `flowgate` (vibe-only `r-requirement`, `always-block`, `Tampered` subsumption), `runner` (working_mode local SSOT + resolver before child escalate + `ss_lock`→`sprint_slicer`), Desktop/TUI clients.
- **Database:** No new Supabase column; `working_mode` and `vibe.locked_ss` + `vibe.sprint_plan` live in `sessions.ndjson` (`CP-36 P-5`). No other DDL; flow definitions stay on Supabase.
- **External systems:** None. Provider adapters unchanged except per-node `owner` provider/model selection (same plumbing as `SS-04 §3.1`).

## 6. Data or Migration Steps

- **Schema:** No `workflow_runs` migration. `WorkingMode` is a new `localFileSessionStore` JSON field (default `dev`); `vibe.locked_ss` (file_artifact `requirements/05-System-Specs/SS-*.md`) and `vibe.sprint_plan` (file_artifact `.flowpilot/vibe/sprint_plan.json` with `structure: {sections: ["sprints"]}`) are typed artifacts (`SD-24 §5`). Verify `ValidateFlowDefinition` alias `user.confirm` and `ValidateFlowSafetyTopology` still pass.
- **Data backfill:** None; existing runs treat missing `WorkingMode` as `dev`; existing flow definitions need no change.
- **Config updates:** No user-facing config migration; `working_mode` is per-run input (optional project default later). Manifest/builtin sync already validated by `pack_test.go:23` (`6 flows`, `7 agents`).

## 7. Validation Plan

- **Tests to add (additive only):**
  - `internal/agentpack`: `LoadBuiltinPack` asserts 6 flows + 7 agents; explicit `vibe-ingest` (edits: `ss_lock=user.confirm` + `sprint_slicer`) + `vibe-sprint` (no owner nodes, single `continue` back-edge) + `vibe-owner-debate` existence; `ValidateFlowSafetyTopology` passes; `vibe-intake` tool scope `Read/Write/Edit/Grep/Glob` limited to SS docs.
  - `internal/flowgate`: `r-requirement` (vibe-only, `always-block`) fires only on `(Tests.Green && (RequirementDrift||Tampered))` as `block` and is absent in `dev` even if `RequirementDrift` were set; `r-additive-tests` weaken in vibe maps to `r-requirement`; `TamperedTestPaths` non-empty in vibe ⇒ `r-requirement`.
  - `internal/runner`: resolver walks `Violations` (not `.Rule.ID`); `r-requirement ∈ violations ⇒ AskUser("requirement")` and no `vibe-owner-debate`; Admin `vibe` rejected; `ss_lock` `WAITING_USER_APPROVAL` gates `vibe-sprint` start; single-owner fallback tagged `warn`.
- **Manual checks (Desktop/TUI, real sample game file):**
  - `/vibe sample-game.md` (pre-sliced) — SS Preview card editable, `Lock` persists edit + auto-slices verbatim `sprint_plan`, 3 sprints run `tdd → coder → synthesis`; generic gate → `vibe-owner-debate` auto, only `r-requirement` produces requirement card.
  - `/vibe "vague idea: 2D roguelike..."` — AI-sliced sprint_plan (inspect), no SS beyond the lock; edits in SS lock survive slicing.
- **Failure cases:** Single-owner fallback degraded warning; mixed pre-sliced+vague input; runner restart mid-`ss_lock` and mid-Owner round 3; tighten that weakening a test to go green always becomes `r-requirement`, never `continue` via Owners.

## 8. Rollout and Fallback

- **Rollout order:** Skeletons landed first (`89fe174a`, now fixed to `selectableIn: []` + `user.confirm`), inert. Then `P-1` (enum local), then `P-2` (vibe-only `r-requirement` always-block), then `P-3` (resolver Violations → FlowDefinition id, before child escalate), then `P-4` (Desktop/TUI lock card + `sprint_slicer`), then `P-5` (boundary demo + regression). Each P merges independently; `P-5` gates approval.
- **Fallback path:** Before `P-3` merges, `P-1`/`P-2` default to `dev` with no user-visible change. After `P-3`, removing the resolver's `vibe` branch restores Dev behavior instantly; deleting `vibe-*.yaml` + `owner.md` + `vibe-intake.md` returns the pack to 3 flows + 4 agents (revert `pack_test.go:23` to 3). No data migration to unwind.
- **Monitoring:** Log newly introduced `EnforceResult` containing `r-requirement` via existing gate audit path plus `BlockReason: requirement`; surface on the run timeline alongside existing `cap`/`escalate` settlement logs (`SD-19 §8 F-3`/`F-4`), including `ss_lock` request.

## 9. Risks

- `R-1` **Gate mis-routing.** Sending a real `r-requirement` through Owners would hide drift (`SS-18 BR-8`). Mitigation: resolver walks `Violations` and `r-requirement` is the first branch before any `vibe-owner-debate` start; `isAlwaysBlock("r-requirement")` prevents warn-downgrade; regression probe `Tests.Green && (RequirementDrift||Tampered)==true ⇒ block/escalate, never done` in `P-2`.
- `R-2` **Owner echo chamber.** Two same-model Owners rubber-stamp each other. Mitigation: per-node provider/model independence (`SD-24 D-3`) + synthesis must check `r-requirement==false` before any `continue`; cap 5 forces human re-engagement; debate uses `submit_review_outcome`, not requirement face.
- `R-3` **Sprint granularity.** One vague file could induce 50 micro-sprints. Mitigation: `vibe-ingest`'s `sprint_slicer` after the lock is the sole slice point; persist the chosen slice for reviewer inspection and tune the converter prompt without changing the graph.
- `R-4` **SS-lock regression.** A future change could auto-run sprints before the SS lock. Mitigation: guard in `interactive_service.go` that `vibe-sprint` refuses to start while `ss_lock` is still `WAITING_USER_APPROVAL`; lock persits edited SS with re-validation.
- `R-5` **Builtin drift.** Manifest ↔ `builtin.*` mismatch or `ValidateFlowDefinition` rejection (duplicate `continue` back-edge) fails pack load. Mitigation: existing CI `go vet` + `go test ./internal/agentpack -run TestLoadBuiltinPack` (`pack.go:769`).

## 10. Definition of Done

- All `SS-18 AC-1`..`AC-10` and `BR-1`..`BR-9` hold in `Desktop` and `TUI` (same commit, same providers): one vague-file run demonstrates `Attach → SS Preview & Lock (editable) → AI auto slice → 3× vibe-sprint` with zero `Task` lock cards and only the documented two user-ask kinds if triggered.
- `r-requirement` is proven hard-gated and vibe-only: a green-but-drifted or green+tampered TDD signature set is caught as `block/escalate` → `WAITING_USER_APPROVAL` (`BlockReason: requirement`), and never `done`; weakening a test to go green is always classified as `r-requirement`; `dev` with same `RequirementDrift` does not fire `r-requirement`.
- Dev mode regression: identical `r-*` fixture in `working_mode=dev` renders the current Dev `1/2/3` / `block` modal, never starts `vibe-owner-debate` (evidence: resolver branch not taken, no `owner` session on that run, API rejects Admin `vibe`).
- `SS-18`/`SD-24` trace is closed, `vibe-ingest` (`ss_lock=user.confirm → sprint_slicer`) → `sprint_plan` → `vibe-sprint` (no owner nodes) → `vibe-owner-debate` on gate fail chain runs on a sample game spec, and `agentpack`/`flowgate`/`runner` `go test` + `go vet` are green on the new builtins (`selectableIn: []`, `6 flows`, `7 agents`).
- A fresh Kill-Review pass on this CP + the just-landed `SS-18`/`SD-24` + 6-flow pack (mode `plan`) reaches `KILL_CLEAN` or `KILL_WITH_FINDINGS` with a frozen claim and no `KILL_BLOCKED` (matrix closure per `skill/kill-review`).

