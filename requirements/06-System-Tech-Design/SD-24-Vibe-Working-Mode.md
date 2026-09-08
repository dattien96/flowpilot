# SD-24: Vibe Working Mode

## Metadata

- Document ID: `SD-24`
- Title: `Vibe Working Mode (Runner Policy + Owner Debate Flows)`
- Feature Keys: `vibe-mode`
- Phase: `tech_design`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-01`
- Last Updated: `2026-09-08`
- Parent Documents: [SS-18: Vibe Working Mode](../05-System-Specs/SS-18-Vibe-Working-Mode.md)
- Child Documents: [CP-60: Vibe Working Mode](../07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md)
- Related Documents: [SD-19: Agent Flow Engine](./SD-19-Agent-Flow-Engine.md), [SD-20: Flow Gate Rule Semantics](./SD-20-Flow-Gate-Rule-Semantics.md), [SD-16: Agent Spawn And Tool-Calling Design](./SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SS-13: AI-Followable Document Contract](../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [SS-08: Approve Gate](../05-System-Specs/SS-08-Approve-Gate.md)
- Replaces: `None`
- Tags: `vibe-mode, working-mode, flow-gate, agent-flow, TDD`

## AI Quick View

### Summary

- Implement `SS-18` without a second orchestrator: one enum `working_mode` (`dev` | `vibe`), one new hard-gate rule `r-requirement` (vibe-only, `block`, `always-block`), and three FlowDefinitions (`vibe-ingest`, `vibe-sprint`, `vibe-owner-debate`) plus `owner` + `vibe-intake` agents on the existing SS-16/SD-19 generic engine. **Vibe lives only in Desktop app + TUI** (`cli-tui`); **not in Admin Web**.
- In Vibe, the gate still evaluates every rule (`flowgate.Evaluate` pure over `TurnResult`), but the **resolver** mapping splits: `dev` → Dev card/modal, `vibe` → start `vibe-owner-debate` flow (exactly 2 isolated owners, cap 5, configurable provider/model), except `r-requirement` and `TamperedTestPaths` weakening which are always user-only. YOLO `skip` semantics are unchanged.
- UX order is **SS ingest → SS Preview & Lock via `user.confirm` (user, Desktop/TUI) → AI auto task/sprint slice (`sprint_slicer`) → per-sprint** `preflight_contract_plan → preflight_contract_freeze → tdd → coder → synthesis` with a hub `r-requirement` check that maps to `done` / `continue` / `escalate`. `Task` slicing is automatic and never a user gate. The `r-requirement` signal is a single SSOT injected after the 1:1 signature↔SS hub check, **before** child `block → parent escalate`.
- Ingest handles either a pre-sliced file or a vague idea by converting raw requirement text to `SS-13`/`FORMAT-REFERENCE-SS` SS drafts (`vibe-intake` agent) that the user edits and locks; post-lock the same flow auto-slices the sprint plan.
- Rollout is additive and Kill-Review-safe: Dev mode paths are unchanged and covered by a dedicated non-regression probe; new YAMLs sit beside the existing pack and do not execute until the Go `working_mode` + `r-requirement` work lands.

### Current Ask

- Settle the data/contract/behavior changes needed so the `vibe-*` skeletons already in the repo remain inert until the resolver + gate land, then go live without reshaping the engine.

### Key Decisions

- `D-1` `working_mode` is a flow/run discriminator (`dev` default, `vibe`opt-in); resolver selects UX, never skips gate evaluation. YOLO still skips gates exactly as today; Vibe still **evaluates** permission/MCP and Owners decide.
- `D-2` `r-requirement` is a **vibe-only**, `block`, `always-block` gate (like `r-tests`/`r-reg`). Weaken-a-test-to-go-green (`r-additive-tests`/`TamperedTestPaths`) is mapped to `r-requirement` and thus user-only.
- `D-3` Resolver maps a top `EnforceResult` to **UX / FlowDefinition id only** (`r-requirement` → requirement card; otherwise `vibe` → start `vibe-owner-debate`). No Go code names `owner.md` or any role.
- `D-4` Cap is encoded in `FlowPolicy` (`SD-19 D-7`, `pack.FlowPolicy`); Owner debate cap is fixed 5 with `onCap: escalate` and `BUG-231`/`BUG-234` settlement semantics.
- `D-5` Skeletons first, execution second: YAMLs declare the accepted shape but no runner path auto-runs them until the resolver + `r-requirement` Go work merges.

### Constraints

- Reuse the CP-19/SD-16 spawn/barrier/pendingAgentContext/orchestrator bus plus `sessions.ndjson` run sink; no new per-agent Supabase tables; `working_mode` persists only in the local run record (`localFileSessionStore`), definitions stay on Supabase.
- Do not alter `ProviderRuntimeAdapter` except by declaring a per-flow control tool face; provider adapters remain behavior-aliased.
- Single run sink: both `dev` and `vibe` persist to `localFileSessionStore` and sync via Drive; Flow/Step definitions stay on Supabase.

### Open Questions

- `Q-1` Resolved — requirement check reuses `hub.inline` + an inline prompt (no dedicated `behavior: vibe.requirement_check` needed).
- `Q-2` Resolved — `working_mode` is a run-level enum (project default optional later); Vibe entry is Desktop/TUI only, run via `POST /client/workflow-runs` with `working_mode=vibe`.

### Source Refs

- `SS-18 AC-1`..`AC-10`, `BR-1`..`BR-9`; `SS-16 AC-3`/`BR-1` + `SD-19 D-1`..`D-7`, §5/§6; `SS-04 §3.5.8`/`§3.7`; `SS-08`/`SS-11`/`SS-13`/`SS-14 AC-6`; `SD-20 D-1`..`D-7` + `flowgate/rules.go` + `gate_hook.go` (`applyFlowControl`, `isAlwaysBlock`, `EnforceResult.Violations`); `pack.go:ValidateFlowDefinition`/`ValidateFlowSafetyTopology`; built-ins `review-loop.yaml` / `context-coding-review-synthesis.yaml`.

## 1. Goal

Provide a tech design that makes `SS-18` implementable as a thin policy layer over the shipped generic flow engine and gate hook: the same runner, the same `sessions.ndjson` sink, the same `flow_control` primitive, with `working_mode` selecting which UX the resolver renders and a single new hard gate capturing requirement drift.

## 2. Input Documents

- [SS-18: Vibe Working Mode](../05-System-Specs/SS-18-Vibe-Working-Mode.md) — `AC-1`..`AC-10`, `BR-1`..`BR-9`.
- [SS-16: Agent Flow Engine](../05-System-Specs/SS-16-Agent-Flow-Engine.md) + [SD-19: Agent Flow Engine](./SD-19-Agent-Flow-Engine.md) — generic engine + `FlowCatalog` + `D-1`..`D-8`.
- [SD-20: Flow Gate Rule Semantics](./SD-20-Flow-Gate-Rule-Semantics.md) + [SS-08: Approve Gate](../05-System-Specs/SS-08-Approve-Gate.md) + [SS-13: AI-Followable Document Contract](../05-System-Specs/SS-13-AI-Followable-Document-Contract.md) + [SS-14: Code Context And Regression Safety](../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md).
- `SS-04` TDD (§3.5.8), `SS-11` Workflow-with-session, `SS-06` skill/agent.

## 3. Architecture Decision

- `D-1` **`working_mode` enum, resolver-only split.** Add `working_mode ∈ {dev, vibe}` (default `dev`) persisted only in the **local run record** (`sessions.ndjson` via `localFileSessionStore`, `SS-11` `§9`; definitions stay on Supabase — no `workflow_runs.working_mode` column). Accept `working_mode` only on the Desktop/TUI-runner HTTP path (`POST /client/workflow-runs` / `POST /client/flows/run` with `X-Client: desktop|tui`); Admin Web is rejected for `vibe` (see `D-7`). The gate hook `runFlowGate` still calls `flowgate.Evaluate` for every enabled rule and resolves a single `EnforceResult`; the split happens in the **resolver** that maps `EnforceResult` → UX. YOLO flag semantics are unchanged — `yolo==true` still skips gates; `vibe` does **not** imply YOLO. `gate_mode`/`flowgate.Block/Warn/Reprompt` evaluation stays `provider-agnostic` (`SD-20 Constraints`).
  - *Alt:* second `Gater` implementation per mode — rejected: duplicates rule evaluation.
  - *Alt:* merge Vibe into YOLO — rejected: YOLO = skip; Vibe = evaluate, change who decides.

- `D-2` **`r-requirement` as a vibe-only, always-block gate.** Do **not** add it to `flowgate.DefaultRules()` (`TestDefaultRules` stays **18**). Export `flowgate.RequirementRule` and inject only via `EnabledRulesFor(working_mode=vibe)`:
  ```go
  {ID: "r-requirement", Scope: "step", Trigger: "requirement_signature_drift", RequiredOutput: "reconcile_tests_with_ss", Action: "block", Enabled: true}
  ```
  It fires iff the `vibe-sprint` hub's 1:1 signature↔frozen-SS check reports drift (`RequirementDrift==true`) — which includes any **green-but-weaken** case (`TamperedTestPaths` non-empty or TDD signatures no longer cover `AC-*`). `isAlwaysBlock` is extended to include `r-requirement` alongside `r-tests`/`r-reg`; `checkRule("r-requirement")` enforces `block` regardless of `warn` mode. The rule's `Enabled` is conditioned on `run.WorkingMode==vibe` (runner passes the mode into `Evaluate`'s enabled-filter; gate itself stays pure). In `dev`, `EnabledRulesFor` omits `r-requirement` even if an advisory were injected. This gate is the **only** Vibe rule whose `EnforceResult` routes to `ask_user` / `WAITING_USER_APPROVAL`; every other violation under `working_mode=vibe` routes to the `vibe-owner-debate` flow (see `D-3`). Before child `block → parent escalate` (today at `gate_hook.go:1209-1228`), the hook computes `RequirementDrift` and, if `vibe && r-requirement ∈ violations`, renders the requirement card **instead of** the default escalate path.
  - *Why here:* the signal must not be derived from file-scope heuristics alone (`SD-20 D-5/Q-2`): the `vibe-sprint` `synthesis` hub already holds the frozen SS + the TDD signature list, so drift is computed there and fed into `TurnResult` for `Evaluate`, matching the existing `ScopeHighSeverity` injection pattern.

- `D-3` **Resolver maps to UX / FlowDefinition id — never to a role/persona in Go.** The resolver walks `EnforceResult.Violations` (not `.Rule.ID`; `EnforceResult` is `[]Violation` + highest `Action`, per `enforce.go:26-28`, `evaluate.go`). Pseudocode:
  ```go
  res := flowgate.Evaluate(tr, enabledRulesFor(run.WorkingMode)) // enabled-filter hides r-requirement when dev
  // res.Violations is the authoritative set; res.Action is the top action
  if contains(res.Violations, "r-requirement") { render AskUserCard("requirement", detailFor("r-requirement")) ; return }
  if res.Action != Block && res.Action != Reprompt { return } // warn is not user-visible in Vibe
  if run.WorkingMode == "vibe" { start FlowDefinition "vibe-owner-debate" } // resolver knows only the id, not owner.md
  else { render DevCard(res) } // byte-for-byte today
  ```
  The debate flow itself declares two `agent.delegate` owners (`lifecycle: spawn`, `cohort: owner_debate`, `join: all`, `dependsOn: [debate_trigger]`, `agent: agents/owner.md`), each independently configurable per node (same model / different model / different provider all valid) as distinct isolated provider sessions (`SS-11` §4). Results are delivered only as a consolidated summary to the hub `debate_synthesis` (`SS-15 BR-1`/`BR-2`, `SD-19 D-6`), never transcript-to-transcript.
  - *Alt:* one Owner — rejected (`SS-18 AC-5`: single Owner is not trusted alone).
  - *Alt:* Owners debating each other directly — rejected (`SS-16 BR-5`: hub-only).

- `D-4` **Cap is `FlowPolicy`, settlement follows `BUG-231`/`BUG-234`.** Owner debate declares `policy: {cap: 5, onCap: escalate, extendBy: 2, extendMax: 2}` and reuses the settled settlement contract (`SD-19 §8 F-3`): at cap or `escalate`, the hub/control node settles to `WAITING_USER_APPROVAL` with `BlockReason: cap|escalate`, the client run status becomes distinct from `running`, and **every** auto-advance path stops (forward-edge spawn, cohort-join `RUNNING`, dependent release) until a `continue` resume re-arms it. The per-sprint outer loop's policy is the sprint FlowDefinition's `policy` (cap typically 3 for code loops); its `done` path from any `agent.code` writer must still traverse `acceptance_nodes` (`ValidateFlowSafetyTopology`).

- `D-5` **`vibe-sprint` topology and the requirement-check hub.** The canonical Vibe sprint is now a pure code loop — Owners are **not graph nodes** here; non-requirement gates are handled by starting `vibe-owner-debate` from the resolver (see `D-3`). It compiles to `SD-19 D-7` vocab as:
  ```
  nodes:
    preflight_contract_plan    {run: delegate, lifecycle: once,    behavior: agent.delegate,  agent: agents/contract-planner.md}
    preflight_contract_freeze  {run: inline,   lifecycle: once,    behavior: contract.freeze}
    tdd                        {run: delegate, lifecycle: reinvoke, behavior: agent.delegate,  agent: agents/tester.md}
    coder                      {run: delegate, lifecycle: reinvoke, behavior: agent.code,      agent: agents/coder.md, join: all}
    synthesis                  {run: inline,   lifecycle: reinvoke, behavior: hub.inline,      agent: agents/synthesizer.md, join: all}
  edges:
    plan → freeze (when: done, kind: forward)
    freeze → tdd (when: done, kind: forward)
    tdd → coder (when: done, kind: forward)
    coder → synthesis (when: done, kind: forward)
    synthesis → coder (when: continue, kind: back)
    synthesis → done  (when: done,    kind: forward)
    synthesis → ask_user (when: escalate, kind: forward)
  policy: {cap: 3, onCap: escalate, extendBy: 2, extendMax: 2}
  acceptance_nodes: [synthesis]
  tools: [tools/vibe-requirement-outcome.yaml]
  ```
  The hub `synthesis` owns the 1:1 signature↔SS check after a green suite (using `hub.inline` + an inline prompt; `Q-1` resolved). If signatures still map, it emits `flow_control(status: done)`; if `r-requirement` holds (including weaken-a-test), it emits `escalate` → `ask_user`. `r-additive-tests` weaken in `vibe` is subsumed by `r-requirement` before the resolver split (`C-8`), never `continue`.

- `D-6` **`vibe-ingest` and `vibe-owner-debate` as companion flows.** `vibe-ingest` is writer-free except for SS-doc writes via `vibe-intake` (read + write scoped to `requirements/05-System-Specs/`): `ingest_reader → ss_converter → ss_validator → ss_lock(user.confirm) → sprint_slicer → done`, no `agent.code`, so `ValidateFlowSafetyTopology` passes unconditionally and no acceptance declaration is needed. In the Desktop/TUI UX the `ss_lock` node renders as the **`SS Preview & Lock` card** (`SS-18 BR-2`, `AC-2`): the run **must** pause at `WAITING_USER_APPROVAL` for the user to edit + lock the SS list; `continue` re-enters `ss_converter` (re-generate), `escalate` surfaces as `ask_user`, `done` proceeds to `sprint_slicer` which auto-slices the plan and then `done`. Sprint slicing after the lock is AI-auto and never a user gate. `vibe-owner-debate` is the standalone 2-Owner loop invoked only for non-`r-requirement` violations in Vibe:
  ```
  nodes: debate_trigger (hub.inline, reinvoke) → owner_1/owner_2 (spawn, join: all) → debate_synthesis (hub.inline, reinvoke)
  edges: trigger → owners (done/forward), owners → synthesis (done/forward),
         synthesis → trigger (continue/back), synthesis → done (done/forward), synthesis → ask_user (escalate/forward)
  policy: {cap: 5, onCap: escalate, extendBy: 2, extendMax: 2}
  tools: [tools/submit-review-outcome.yaml]
  ```
  It shares settlement semantics (`D-4`) but a distinct control face (`submit_review_outcome` → `approved/changes_requested/blocked`) from the sprint's `vibe-requirement-outcome`.

- `D-7` **Ingest branching: follow vs. auto-slice (task is AI-auto, SS is user-gated).** The ingest FlowDefinition produces a single `sprint_plan` typed artifact (persists the chosen branch) after the `ss_lock` phase. If the input file contains an explicit task/sprint segmentation, the resolver emits that list verbatim (`SS-18 AC-2`, `BR-2`) and `vibe-sprint` iterates it. If the input is vague, the ingest auto-slices a sprint plan after the SS lock (no user card) and persists it as the same artifact shape. Both branches return the same artifact type so downstream binding (`SD-23` typed artifacts) is uniform. Task breakdown inside a sprint is likewise AI-auto and never blocks the run.

Why chosen: every decision stays declarative (FlowDefinition + agent + pack), the engine stays domain-free (`SD-19 D-1`), the bridge `agent_flow as one Workflow step` is untouched, and Dev's mutation surface is exactly one vibe-only rule plus a resolver that knows only FlowDefinition ids.

## 4. Component Impact

- **Impacted modules:**
  - `apps/local-runner/internal/agentpack` (`pack.go`, `flow_safety_topology.go`, `pack_test.go`): parsed `working_mode`-agnostic; new `owner` + `vibe-intake` agents + three flows.
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml`: registers `agents/owner.md`, `agents/vibe-intake.md` + `flows/vibe-*.yaml`.
  - `apps/local-runner/internal/flowgate` (`rules.go`, `evaluate.go`, `enforce.go`, `observe.go`): add vibe-only `r-requirement` (plus `isAlwaysBlock` + enabled-filter), carry hub's `RequirementDrift` + `TamperedTestPaths`-derived requirement drift into `TurnResult`; `r-additive-tests` weaken in vibe subsumed by `r-requirement`.
  - `apps/local-runner/internal/runner` (`interactive_service.go`/`gate_hook.go`, `localFileSessionStore`): store `working_mode` on the local run record only, dispatch to resolver per `SS-18 BR-1`, gate `ss_lock` before any `vibe-sprint`, reject Admin-Web `vibe` starts.
  - `apps/desktop-flowpilot` + `cli-tui` (TUI `flowpilot chat`): new Vibe entry (Desktop file picker / TUI `/vibe <path|prompt>` or `/vibe-file`), `SS Preview & Lock` card (`user.confirm` at `ss_lock`) + `r-requirement` (`ask_user` with `BlockReason: requirement`) rendering on the run timeline. **Admin Web is not in scope for Vibe** (`SS-18` scope; `selectableIn: []` + API reject).
- **New modules:** `agents/owner.md`, `agents/vibe-intake.md`, `flows/vibe-ingest.yaml`, `flows/vibe-sprint.yaml`, `flows/vibe-owner-debate.yaml`, `tools/vibe-requirement-outcome.yaml` (control-tool face for `vibe-sprint`).
- **Unchanged modules:** `Admin Web` (no Vibe entry), `ProviderRuntimeAdapter` / SSE transport (additive only); no Supabase run-table migration.

## 5. Data Model

- **Configuration:**
  ```go
  // FlowDefinition fields are unchanged; Vibe is expressed as new YAMLs
  // plus two shared agents. No new struct fields on FlowDefinition/FlowPolicy.
  type WorkingMode string // "dev" | "vibe"  // default "dev"
  // Persisted only in the local run record (localFileSessionStore, SS-11 §9):
  // e.g. interactiveRun.WorkingMode. Supabase run tables receive no new column
  // (CP-36 P-5: runs are local; definitions are on Supabase).
  ```
- **FlowGate rule (new, vibe-only):**
  ```go
  // r-requirement injects an advisory signal alongside ScopeHighSeverity:
  type TurnResult struct {
      // ... existing ...
      RequirementDrift       bool     `json:"requirement_drift,omitempty"`        // hub's 1:1 signature↔SS result
      RequirementDriftDetail string   `json:"requirement_drift_detail,omitempty"`
      // TamperedTestPaths already present (r-additive-tests); in vibe, green+tampered ⇒ RequirementDrift
  }
  // DefaultRules append (Enabled gated by run.WorkingMode==vibe in the caller):
  // {ID: "r-requirement", Scope: "step", Trigger: "requirement_signature_drift",
  //  RequiredOutput: "reconcile_tests_with_ss", Action: "block", Enabled: true}
  // isAlwaysBlock("r-requirement") == true.
  ```
- **Agents:** `AgentSpec{Name: "vibe-intake", Role: "intake", Tools: [Read,Write,Edit,Grep,Glob], Scope: requirements/05-System-Specs/*}` and `AgentSpec{Name: "owner", Role: "owner", Tools: [Read,Grep,Glob]}`.
  The per-node `FlowNode.Agent = "agents/vibe-intake.md"` / `"agents/owner.md"`; `owner` provider/model are resolved per `Owner_1`/`Owner_2` node via the same override that today allows per-step model selection (`SS-04 §3.1`).
- **FlowDefinitions (skeleton, §3):** `vibe-ingest` (`ss_lock=user.confirm`, no writer, no acceptance; `selectableIn: []`), `vibe-sprint` (writer `coder`, acceptance `[synthesis]`; `selectableIn: []`), `vibe-owner-debate` (no writer, `selectableIn: []`). All three reuse `SD-19 D-7` vocabulary and pass `ValidateFlowDefinition`/`ValidateFlowSafetyTopology`.
- **Artifacts (typed, SD-23):**
  ```yaml
  - id: vibe.locked_ss
    type: file_artifact
    instances: [{key: "locked-ss-list", config: {paths: ["requirements/05-System-Specs/SS-*.md"]}}]
  - id: vibe.sprint_plan
    type: file_artifact
    instances: [{key: "sprint-plan", config: {paths: [".flowpilot/vibe/sprint_plan.json"], structure: {sections: ["sprints"]}}}]
  ```
  `vibe-ingest` writes the locked SS list (persisted after `ss_lock` edit-back) and then the `sprint_plan` (verbatim vs synthesized); `vibe-sprint` reads `locked_ss`.

- **State transitions:** unchanged (`PENDING/RUNNING/WAITING_USER_APPROVAL/DONE/FAILED/SKIPPED` for steps; `done/continue/escalate` for `flow_control`). Vibe `WAITING_USER_APPROVAL` for `r-requirement` and for `ss_lock` carry distinct `BlockReason`

## 6. Interfaces and Contracts

- **Tool contract (Vibe sprint only):** one declared control-tool face `vibe-requirement-outcome` (mirrors `submit_review_outcome`):
  ```yaml
  id: vibe-requirement-outcome
  kind: flow_control
  mapsTo: flow.control
  exposesTool: true
  input:
    type: object
    properties:
      verdict: {type: string, enum: [aligned, drift_fixable, requirement_change]}
      summary: {type: string}
  statusMap: {aligned: done, drift_fixable: continue, requirement_change: escalate}
  ```
  `drift_fixable → continue` re-invokes the coder (no Owners); `requirement_change → escalate` routes to `ask_user`. The Owner debate flow uses the existing `submit_review_outcome` face (`approved→done`, `changes_requested→continue`, `blocked→escalate`), not the requirement face.

- **FlowGate contract:** `r-requirement` is a vibe-only `block` + `always-block` (like `r-tests`/`r-reg`). The resolver walks `EnforceResult.Violations`:
  ```go
  res := flowgate.Evaluate(tr, enabledRulesFor(run.WorkingMode))
  // res.Violations is authoritative; res.Action is the top Action
  if containsID(res.Violations, "r-requirement") {
      render AskUserCard("requirement", detailFor(res.Violations, "r-requirement")); return
  }
  if res.Action != Block && res.Action != Reprompt { return }
  if run.WorkingMode == "vibe" { start FlowDefinition("vibe-owner-debate") }
  else { render DevCard(res) } // byte-for-byte today
  ```
  The gate-hook computes `RequirementDrift` (including green+`TamperedTestPaths` weaken) **before** the existing child `block → parent escalate` path, so a Vibe violation does not surface as a third Dev ask.

- **Pack contract:** `manifest.yaml` ↔ `BuiltinMeta` agreement is validated by `validateManifestFlowMatchesDefinition` (`pack.go:769`). Each new flow's `builtin.{editable,selectableIn,chatSubModes,cloneable,chatBaseline}` must match its manifest entry byte-for-byte, else `LoadBuiltinPack()` fails. `ValidateFlowDefinition` also rejects unknown `behavior` (`user.confirm` is aliased), bad `lifecycle`, missing `dependsOn` refs, and duplicate `back` edges per `when` — so `vibe-ingest`'s single `continue` (`ss_lock → ss_converter`) stays clean.

- **Provider contract:** an `owner` run and the `vibe-intake` runs are ordinary `agent.delegate` provider sessions bound to `FlowNode.Agent`/model/provider; no new adapter behavior. `hub.inline` vs `agent.delegate` vs `user.confirm` distinction (`SD-19 D-5`) is preserved.

- **Files to add (already in repo as skeletons):**
  - `apps/local-runner/internal/agentpack/flow-pack/agents/owner.md`
  - `apps/local-runner/internal/agentpack/flow-pack/agents/vibe-intake.md`
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-ingest.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-owner-debate.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/tools/vibe-requirement-outcome.yaml`
  - `requirements/05-System-Specs/SS-18-Vibe-Working-Mode.md` (spec), `requirements/06-System-Tech-Design/SD-24-Vibe-Working-Mode.md` (this file) — trace chain `SS-13 §5.1` metadata + `AI Quick View`.

## 7. Execution Flow

- **Vibe ingest (once per run, if raw requirement file is present) — Desktop/TUI only:**
  1. Desktop file picker or TUI command (`/vibe <path|prompt>` / `/vibe-file`) resolves the requirement file path or pasted idea prompt and starts a run with `working_mode=vibe` (persisted only in `localFileSessionStore`; Admin Web `vibe` is rejected). No Supabase run column.
  2. `vibe-ingest` `ingest_reader` (vibe-intake) reads the raw Markdown/text; `ss_converter` (vibe-intake) is prompted to emit proper `SS` drafts per `FORMAT-REFERENCE-SS` + `SS-13 §5.1/§5.2` (metadata block + AI Quick View + numbered sections), seeded with `feature_key: vibe-mode` where suitable; `ss_validator` (`hub.inline`) checks required `SS-13` sections and that `AC-*` are present and testable.
  3. `ss_lock` (`user.confirm`) **renders as an `SS Preview & Lock` card and pauses at `WAITING_USER_APPROVAL`** — the user may edit the drafts inline; `Lock` persists the edited SS list (write-back, re-validate) and proceeds to `sprint_slicer`, `Continue` (user wants re-generation) back-edges to `ss_converter`, `Escalate` surfaces as `ask_user`. The user must lock the SS list before any sprint runs (`SS-18 BR-2`, `AC-2`).
  4. `sprint_slicer` (`hub.inline`) auto-slices the sprint/task plan (no user card): if the input already lists `features/tasks/sprints`, the emitted `sprint_plan` artifact equals that list verbatim; otherwise it synthesizes the plan and persists it as the same typed artifact (uniform downstream). Task breakdown per sprint is likewise AI-auto. On `done`, the plan artifact is persisted beside the run and the runner iteratively starts `vibe-sprint` for each sprint slice (generated Workflow of `agent_flow` steps, or one outer Vibe flow — not a Go for-loop naming personas; see SD-19 `D-7`).

- **Per sprint (`vibe-sprint`, bounded retry, synthesis owns drift):**
  1. `preflight_contract_plan` builds the frozen preflight draft for the slice; `preflight_contract_freeze` freezes it (`contract.freeze`). Every subsequent writer path is dominated by this freeze (`D-5`).
  2. `tdd` (`agents/tester.md`) writes **signature-only** tests from the frozen SS slice (`SS-04 §3.5.8`, `prompts/test-signatures.md`). No production code.
  3. `coder` (`agents/coder.md`, `agent.code`) implements to make the TDD signatures green. Tool scope is `Read/Edit/Write/Bash/Grep/Glob` per that agent, gated by `change-contract` (`CP-43`); `git commit` remains denied on coding children (`SD-20 D-7`).
  4. On `coder.done`, the gate hook — **before** any child `block → parent escalate` — computes `RequirementDrift` from the hub's 1:1 signature↔SS advisory plus green+`TamperedTestPaths` (weaken), feeds it into `flowgate.Evaluate` (enabled-filter hides `r-requirement` when `dev`), and the resolver (§6) decides: `r-requirement ∈ violations → requirement card` (no Owners), else `vibe → start vibe-owner-debate`, else Dev card.
  5. `synthesis` (`hub.inline`) is the hub that, after the resolver's decision, emits `flow_control`: if `r-requirement` held it already escalated; if non-requirement, the resolver has started `vibe-owner-debate` whose `debate_synthesis` will loop (`continue` back to `debate_trigger`, cap 5) before the sprint hub resumes. `synthesis` ultimately emits `done` (sprint is green + requirement-aligned) → Workflow advances to the next sprint slice; `continue` routes back to `coder` via the single `back` edge; `escalate` routes to `ask_user`.

- **Owner debate extraction (`vibe-owner-debate`, internal):**
  The same `owner` cohort + hub `debate_synthesis` loop (cap 5) is available to any future gate that wants Owners outside a sprint. `vibe-sprint`'s owner cohort is the outer sprint's instantiation of the same policy. Both observe the same settlement invariant (`SD-19 §8 F-3` auto-advance gate).

## 8. Failure and Edge Handling

- `F-1` Work rejected by `ValidateFlowDefinition` (unknown behavior, missing `dependsOn`, duplicate `continue` back-edge) → pack load fails at `go test ./...` / runner start, before any run; CI catches it.
- `F-2` `ValidateFlowSafetyTopology` failure (writer without freeze, writer-path bypasses freeze, `done` path without `acceptance_nodes`) → `LoadBuiltinPack()` fails on that YAML; the skeleton must satisfy the built-in writer/freeze precedent (`context-coding-review-synthesis.yaml`) before the resolver is wired.
- `F-3` Manifest ↔ YAML builtin mismatch (`validateManifestFlowMatchesDefinition`) → pack load fails; keep `manifest.yaml:flows[].path/editable/selectableIn/…` byte-for-byte equal to each flow's `builtin.*` (now `selectableIn: []` for all vibe flows, not user-pickable; Admin Web rejected).
- `F-4` Single-owner fallback: if only one `owner` provider session could be started for a violation, run the single-owner synthesis pass but tag the remediation with `warn: single-owner`; still honor the 5-round cap.
- `F-5` Mixed input (some slices explicit, some vague): ingest marks which sprints were verbatim vs. synthesized so a later `r-requirement` detail can attribute drift correctly and avoid re-slicing verbatim sprints. Editable lock persists the user-edited SS before slicing.
- `F-6` Vibe run interrupted/restarted mid-sprint or mid-Owner-debate: the local run sink already replays via `sessions.ndjson`; the sprint index and Owner round counter replay, so no sprint is skipped or duplicated.
- `F-7` `r-requirement` mis-classified: a weaken-to-green that was wrongly `continue` instead of `requirement` → regression probe that asserts `AC-6`/`BR-4`: any `Tests.Green && (RequirementDrift || Tampered)` must be `block/escalate`, never `done`.

## 9. Security and Operational Concerns

- **auth:** `working_mode=vibe` is a Desktop/TUI-run attribute under the current project authority (`POST /client/workflow-runs` with `X-Client` guard); it is never derived from provider output. `r-requirement`'s advisory is computed from frozen SS + runner-side artifact diff, not from model self-grading. **Admin Web `vibe` is rejected.**
- **secrets:** pack YAML + `owner`/`vibe-intake` prompts carry no credentials; provider switching per Owner node is through the runner's existing env isolation (`SS-11`), not through prompt injection. `vibe-intake` writes are scoped to `requirements/05-System-Specs/` SS drafts only.
- **audit:** every sprint's `tdd` artifact, `coder` diff, suite/TTR, `r-requirement` detail, Owner debate rounds, and hub `flow_control` signal are logged to the bus/run sink (share `r-ca`/`r-bug`-style audit footprint). `change-audit/*.md` + commit ledger remain owned by the audit/draft node (`SD-20 D-7`), not the coding children.
- **rollback:** additive only. With `working_mode=dev` (default) and before any `r-requirement` matcher is enabled, `go test ./...` and `LoadBuiltinPack()` are green on the new pack; deleting `vibe-*.yaml` + `owner.md` + `vibe-intake.md` returns the runner to the prior built-in set. GTM: land skeletons inert, then land the Go resolver in a second PR.

## 10. Risks and Trade-Offs

- `R-1` **Gate mis-routing.** Sending a genuine `r-requirement` through Owners would hide drift. Mitigation: `r-requirement` is the first branch walking `Violations` before any Owner start; `isAlwaysBlock("r-requirement")` prevents warn-downgrade. Probe `F-7`.
- `R-2` **Owner echo chamber.** Two same-model Owners rubber-stamp each other. Mitigation: allow distinct provider/model per Owner node and require `Main` synthesis to treat consensus as remediation only when `r-requirement` is false; cap 5 forces human re-engagement. Debate flow uses `submit_review_outcome`, not the requirement face.
- `R-3` **Sprint granularity.** 50 tiny sprints = 50 freezes/TDD cycles; 2 huge sprints = drift hides. Mitigation: `Q-2`/`D-7` persist the ingest's chosen slice so a reviewer can see the budget before any code runs, and a future tuning pass can adjust the converter prompt without changing the graph.
- `R-4` **Residual file-read in `owner.md` Scope.** Mitigated by constraining Owner tools to read-only-ish set (`Read/Grep/Glob`) and delegating any required edit to the delegated `coder` via `continue` reprompt — Owners never write. `vibe-intake` is the only writer for SS.
- `R-5` **Builtin-mismatch escapes.** A stale `manifest.yaml` ↔ `builtin.*` drift passes local dev but breaks CI/Release. Mitigation: CI runs `go vet` + `go test ./internal/agentpack -run TestLoadBuiltinPack` (already present) that calls `LoadBuiltinPack()` and would fail on `D-3` mismatch.

## 11. Validation Strategy

- **unit:** `go test ./internal/agentpack` — `LoadBuiltinPack()` + `ValidateFlowDefinition`/`ValidateFlowSafetyTopology` + `validateManifestFlowMatchesDefinition` for all **12** builtin flows (8 non-vibe + 4 vibe: ingest/sprint/owner-debate/cp-ingest, `selectableIn: []` on vibe). `TestLoadBuiltinPack` also asserts **8** agents. `go test ./internal/flowgate` — `r-requirement` (vibe-only, `always-block`) triggers on `Tests.Green && (RequirementDrift||Tampered)==true` as `block`, and the resolver walks `Violations` to `ask_user` only in `vibe`; `dev` hides the rule even if drift advisory were present.
- **integration:** drive `vibe-inspect` + `vibe-sprint` against a fixture game requirement file in both intake shapes (pre-sliced vs. vague) with a mock provider suite that returns (a) green+tampered → `r-requirement` escalates without Owners, (b) generic `r-*` → `vibe-owner-debate` starts and bounded-retries; confirm Admin Web `POST ... working_mode=vibe` is rejected and `dev` fixture stays on the Dev card path and never starts `vibe-owner-debate`.
- **manual:** exercise the desktop/TUI file import or chat prompt for a `vibe-ingest` (edit + Lock) → auto slice → 3× `vibe-sprint` chain on a sample game spec; every Owner-remediable gate auto-resolves via `vibe-owner-debate`, and only `r-requirement` produces the single requirement user card.
- **observability:** `SD-19 §8 F-3`/`F-4` settlement contract already logs `BlockReason` and caps; `ss_lock` and `requirement` block reasons are surfaced on the run timeline alongside existing `cap`/`escalate` rendering.

## 12. Traceability to Spec

- `AC-1` entry + SS conversion → `D-1`, `D-6`, `D-7`, §7 ingest (Desktop/TUI only, `ss_lock`).
- `AC-2` SS lock before sprints + auto slice → `D-6` `ss_lock(user.confirm)` + `sprint_slicer`, §5 `vibe.locked_ss`/`vibe.sprint_plan`.
- `AC-3` TDD signature-only before coder → `D-5` `tdd → coder` edge + `agents/tester.md` reuse.
- `AC-4` green → `r-requirement` check → `D-2`, `D-5` hub + resolver §6, `SD-20 D-2` (`always-block`).
- `AC-5` non-requirement → Owner debate → `D-3` resolver → `vibe-owner-debate`, isolation, `selectableIn: []`.
- `AC-6` cap 5 → `D-4` `FlowPolicy{cap:5}` on `vibe-owner-debate` + settlement, not on `vibe-sprint`.
- `AC-7` only two user asks → `D-1`/`D-2`/`D-3` resolver split walking `Violations`.
- `AC-8` Dev non-regression → `D-1` default `dev` + vibe-only `r-requirement` + Dev `isAlwaysBlock` unchanged, §11 dev probe.
- `AC-9` persistence/resume → §5 local-only `WorkingMode` + `sessions.ndjson`, `ss_lock` + `sprint_plan` replay, `D-4` per `CP-36 P-5`.
- `AC-10` auto sprint-by-sprint demo → `D-5` `vibe-sprint` + `D-6` ingest lock → slice chain.
- `BR-1`..`BR-9` → `D-1`..`D-5`, §8 `F-7`, `SD-19 D-1`/`D-6`; `Q-1`/`Q-2` resolved as `hub.inline` / run-level enum.

