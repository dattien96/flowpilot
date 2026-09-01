# SD-24: Vibe Working Mode

## Metadata

- Document ID: `SD-24`
- Title: `Vibe Working Mode (Runner Policy + Owner Debate Flows)`
- Phase: `tech_design`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-01`
- Last Updated: `2026-09-01`
- Parent Documents: [SS-18: Vibe Working Mode](../05-System-Specs/SS-18-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [SD-19: Agent Flow Engine](./SD-19-Agent-Flow-Engine.md), [SD-20: Flow Gate Rule Semantics](./SD-20-Flow-Gate-Rule-Semantics.md), [SD-16: Agent Spawn And Tool-Calling Design](./SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SS-13: AI-Followable Document Contract](../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [SS-08: Approve Gate](../05-System-Specs/SS-08-Approve-Gate.md)
- Replaces: `None`
- Tags: `vibe-mode, working-mode, flow-gate, agent-flow, TDD`

## AI Quick View

### Summary

- Implement `SS-18` without a second orchestrator: one enum `working_mode` (`dev` | `vibe`), one new hard-gate rule `r-requirement`, and three FlowDefinitions (`vibe-ingest`, `vibe-sprint`, `vibe-owner-debate`) plus one `owner` agent on the existing SS-16/SD-19 generic engine. **Vibe lives only in Desktop app + TUI** (`cli-tui`); **not in Admin Web**.
- In Vibe, the gate resolver still evaluates every rule (`flowgate.Evaluate` produces one `EnforceResult`), but the **resolver** mapping splits: `dev` → Dev card/modal, `vibe` → Owner debate cohort (exactly 2 isolated owners, cap 5, configurable provider/model) except `r-requirement` which is always user-only and never Owner-auto-resolved.
- UX order is **SS ingest → SS Preview & Lock (user, Desktop/TUI) → AI auto task/sprint slice → per-sprint** `preflight_contract_plan → preflight_contract_freeze → tdd → coder → owners → synthesis` with a hub `r-requirement` check that maps to `done` / `continue` (Owner retry) / `escalate` (user). `Task` slicing is automatic and never a user gate.
- Ingest handles either a pre-sliced file or a vague idea by converting raw requirement text to `SS-13`/`FORMAT-REFERENCE-SS` SS artifacts that freeze into the sprint's canonical head after the user locks them.
- Rollout is additive and Kill-Review-safe: Dev mode paths are unchanged and covered by a dedicated non-regression probe; new YAMLs sit beside the existing pack and do not execute until the Go `working_mode` + `r-requirement` work lands.

### Current Ask

- Settle the data/contract/behavior changes needed so the `vibe-*` skeletons already in the repo remain inert until the resolver + gate land, then go live without reshaping the engine.

### Key Decisions

- `D-1` `working_mode` is a flow/run discriminator, not a YOLO rewrite; resolver parity is `command ≈ YOLO`, gate evaluation is not.
- `D-2` `r-requirement` is a hard `block` gate owned by the requirement-check hub node, typed as `ask_user` and surfaced as the sole Vibe user card.
- `D-3` Owner cohort is exactly 2 isolated `agent.delegate` owners (`cohort: vibe_owner`/`owner_debate`, `join: all`), mirrored through Main, configurable provider/model per node.
- `D-4` Cap is encoded in `FlowPolicy` (`SD-19 D-7`, `pack.FlowPolicy`); Owner debate cap is fixed 5 with `onCap: escalate` and `BUG-231`/`BUG-234` settlement semantics.
- `D-5` Skeletons first, execution second: YAMLs declare the accepted shape but no runner path auto-runs them until the resolver + `r-requirement` Go work merges.

### Constraints

- Reuse the CP-19/SD-16 spawn/barrier/pendingAgentContext/orchestrator bus plus `sessions.ndjson` run sink; no new per-agent Supabase tables.
- Do not alter `ProviderRuntimeAdapter` except by declaring a per-flow control tool face; provider adapters remain behavior-aliased.
- Single run sink: both `dev` and `vibe` persist to `localFileSessionStore` and sync via Drive; Flow/Step definitions stay on Supabase.

### Open Questions

- `Q-1` Dedicated `behavior: vibe.requirement_check` identifier vs. reusing `hub.inline` + an inline prompt for the `r-requirement` node.
- `Q-2` Per-sprint `working_mode` override (sprint 2 in Dev, sprint 3 in Vibe) or project-level enum only. Resolved in scope: Vibe is Desktop/TUI only, not Admin Web.

### Source Refs

- `SS-18 AC-1`..`AC-10`, `BR-1`..`BR-9`; `SS-16 AC-3`/`BR-1` + `SD-19 D-1`..`D-7`, §5/§6; `SS-04 §3.5.8`/`§3.7`; `SS-08`/`SS-11`/`SS-13`/`SS-14 AC-6`; `SD-20 D-1`..`D-7` + `flowgate/rules.go` + `gate_hook.go`; `pack.go:ValidateFlowDefinition`/`ValidateFlowSafetyTopology`; built-ins `review-loop.yaml` / `context-coding-review-synthesis.yaml`.

## 1. Goal

Provide a tech design that makes `SS-18` implementable as a thin policy layer over the shipped generic flow engine and gate hook: the same runner, the same `sessions.ndjson` sink, the same `flow_control` primitive, with `working_mode` selecting which UX the resolver renders and a single new hard gate capturing requirement drift.

## 2. Input Documents

- [SS-18: Vibe Working Mode](../05-System-Specs/SS-18-Vibe-Working-Mode.md) — `AC-1`..`AC-10`, `BR-1`..`BR-9`.
- [SS-16: Agent Flow Engine](../05-System-Specs/SS-16-Agent-Flow-Engine.md) + [SD-19: Agent Flow Engine](./SD-19-Agent-Flow-Engine.md) — generic engine + `FlowCatalog` + `D-1`..`D-8`.
- [SD-20: Flow Gate Rule Semantics](./SD-20-Flow-Gate-Rule-Semantics.md) + [SS-08: Approve Gate](../05-System-Specs/SS-08-Approve-Gate.md) + [SS-13: AI-Followable Document Contract](../05-System-Specs/SS-13-AI-Followable-Document-Contract.md) + [SS-14: Code Context And Regression Safety](../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md).
- `SS-04` TDD (§3.5.8), `SS-11` Workflow-with-session, `SS-06` skill/agent.

## 3. Architecture Decision

- `D-1` **`working_mode` enum, resolver-only split.** Add `working_mode ∈ {dev, vibe}` (default `dev`) at the owning scope that already carries YOLO/provider intent: project-level config with per-run override at `POST /client/workflow-runs` / `POST /client/flows/run` (mirrors the scoping of `workflows.yolo_mode` / `step_definitions.yolo_mode` in `SS-08`, without changing that SSOT). The gate hook `runFlowGate` still calls `flowgate.Evaluate` for every enabled rule and resolves a single `EnforceResult`; the split happens in the **resolver** that maps `EnforceResult` → UX. `command posture ≈ YOLO` in Vibe means `permission_required`/`mcp` approvals auto-resolve under the runner's tool policy, not that the gate is skipped. `gate_mode`/`flowgate.Block/Warn/Reprompt` evaluation stays `provider-agnostic` (`SD-20 Constraints`).
  - *Alt:* second `Gater` implementation per mode — rejected: duplicates rule evaluation.
  - *Alt:* merge Vibe into YOLO — rejected: YOLO's semantics are "skip gates"; Vibe's is "evaluate every gate, change who decides".

- `D-2` **`r-requirement` as a user-only hard gate.** Add one new rule to `flowgate.DefaultRules()`:
  ```go
  {ID: "r-requirement", Scope: "step", Trigger: "requirement_signature_drift", RequiredOutput: "reconcile_tests_with_ss", Action: "block", Enabled: true}
  ```
  Fires iff `Tests.Ran && len(Tests.Failed)==0` is true **and** a hub `vibe.requirement_check` (see `D-5`) reports that the green test signatures no longer map 1:1 to the frozen SS acceptance criteria for this sprint, or no signature can be reconciled without changing the SS. It is the **only** Vibe rule whose `EnforceResult` routes to `ask_user` / `WAITING_USER_APPROVAL`; every other violation under `working_mode=vibe` routes to the Owner debate cohort (see `D-3`). This gate is also valid in `dev` mode (same `block` semantics) but in Dev its decision card may be reached directly, without Owner debate.
  - *Why here:* the signal must not be derived from file-scope heuristics alone (`SD-20 D-5/Q-2`): the `r-requirement` node already holds the frozen SS + the TDD signature list, so drift is computed there and fed into `TurnResult` for `Evaluate`, matching the existing `SkillImpactTargets` injection pattern.

- `D-3` **Owner debate = exactly 2 isolated cohort members through Main.** Declare two `agent.delegate` nodes `owner_1` and `owner_2` (`lifecycle: spawn`, `cohort: vibe_owner` on `vibe-sprint` / `owner_debate` on `vibe-owner-debate`, `join: all`, `dependsOn: [coder]` or `[debate_trigger]`), both using `agent: agents/owner.md`. Each node's `Agent`/`Provider`/`Model` is independently configurable per node (same model / different model / different provider all valid), validated as distinct isolated provider sessions (`SS-11` §4). Results are delivered only as a consolidated summary to the hub `synthesis` node (`SS-15 BR-1`/`BR-2`, `SD-19 D-6`), never transcript-to-transcript. Consensus = a single remediation (`retry`/`reprompt`/`continue`) the hub emits as `flow_control(status: continue)`; divergence within the cap is a further `continue` with the next Owners' guidance; `cap` or `r-requirement` divergence is `escalate`.
  - *Alt:* one Owner — rejected (`SS-18 AC-5`: single Owner is not trusted alone).
  - *Alt:* Owners debating each other directly — rejected (`SS-16 BR-5`: hub-only).

- `D-4` **Cap is `FlowPolicy`, settlement follows `BUG-231`/`BUG-234`.** Owner debate declares `policy: {cap: 5, onCap: escalate, extendBy: 2, extendMax: 2}` and reuses the settled settlement contract (`SD-19 §8 F-3`): at cap or `escalate`, the hub/control node settles to `WAITING_USER_APPROVAL` with `BlockReason: cap|escalate`, the client run status becomes distinct from `running`, and **every** auto-advance path stops (forward-edge spawn, cohort-join `RUNNING`, dependent release) until a `continue` resume re-arms it. The per-sprint outer loop's policy is the sprint FlowDefinition's `policy` (cap typically 3 for code loops); its `done` path from any `agent.code` writer must still traverse `acceptance_nodes` (`ValidateFlowSafetyTopology`).

- `D-5` **`vibe-sprint` topology and the requirement-check hub.** The canonical Vibe sprint compiles to `SD-19 D-7` vocab as:
  ```
  nodes:
    preflight_contract_plan    {run: delegate, lifecycle: once,    behavior: agent.delegate,  agent: agents/contract-planner.md}
    preflight_contract_freeze  {run: inline,   lifecycle: once,    behavior: contract.freeze}
    tdd                        {run: delegate, lifecycle: reinvoke, behavior: agent.delegate,  agent: agents/tester.md}
    coder                      {run: delegate, lifecycle: reinvoke, behavior: agent.code,      agent: agents/coder.md, join: all}
    owner_1                    {run: delegate, lifecycle: spawn,   behavior: agent.delegate,  agent: agents/owner.md, dependsOn: [coder], cohort: vibe_owner, join: all}
    owner_2                    {run: delegate, lifecycle: spawn,   behavior: agent.delegate,  agent: agents/owner.md, dependsOn: [coder], cohort: vibe_owner, join: all}
    synthesis                  {run: inline,   lifecycle: reinvoke, behavior: hub.inline,      agent: agents/synthesizer.md, join: all}
  edges:
    plan → freeze (when: done, kind: forward)
    freeze → tdd (when: done, kind: forward)
    tdd → coder (when: done, kind: forward)
    coder → owner_1 (when: done, kind: forward)
    coder → owner_2 (when: done, kind: forward)
    owner_1 → synthesis (when: done, kind: forward)
    owner_2 → synthesis (when: done, kind: forward)
    synthesis → coder (when: continue, kind: back)
    synthesis → done  (when: done,    kind: forward)
    synthesis → ask_user (when: escalate, kind: forward)
  policy: {cap: 3, onCap: escalate, extendBy: 2, extendMax: 2}
  acceptance_nodes: [synthesis]
  tools: [tools/vibe-requirement-outcome.yaml]   # declares vibe-requirement-outcome → flow_control
  ```
  The hub `synthesis` owns the 1:1 signature↔SS check after a green suite: if signatures still map, it emits `flow_control(status: done)`; if they don't but can be re-synthesized by Owners without changing the SS, it emits `continue`; if `r-requirement` holds (green but drifted, or fix would need to change SS), it emits `escalate` → `ask_user`. Weaken-a-test-to-go-green is routed to `r-requirement` by construction, never to `continue` (`SS-14 AC-6`, `SS-18 BR-4`).
  - *Identifier open question* (`Q-1`): whether the requirement-check is a separate `vibe.requirement_check` behavior alias or a prompt inside `hub.inline` does not change the graph; `Q-1` is settled in the CP.

- `D-6` **`vibe-ingest` and `vibe-owner-debate` as companion flows.** `vibe-ingest` is a writer-free ingest: `ingest_reader → ss_converter → ss_validator → done`, no `agent.code`, so `ValidateFlowSafetyTopology` passes unconditionally and no acceptance declaration is needed. It runs first when the raw input is present, producing `SS-13`/`FORMAT-REFERENCE-SS` SS artifacts whose freeze seeds `vibe-sprint`. In the Desktop/TUI UX the `ss_validator` renders as the **`SS Preview & Lock` card** (`SS-18 BR-2`, `AC-2`): the run **must** pause at `WAITING_USER_APPROVAL` for the user to lock the SS list before any `vibe-sprint` starts. `vibe-owner-debate` is the extracted Owner loop used when a non-sprint gate (e.g. a generic chat gate) needs Owners:
  ```
  nodes: debate_trigger (hub.inline, reinvoke) → owner_1/owner_2 (spawn, join: all) → debate_synthesis (hub.inline, reinvoke)
  edges: trigger → owners (done/forward), owners → synthesis (done/forward),
         synthesis → trigger (continue/back), synthesis → done (done/forward), synthesis → ask_user (escalate/forward)
  policy: {cap: 5, onCap: escalate, extendBy: 2, extendMax: 2}
  ```
  It shares the same `owner` agent and settlement semantics (`D-4`); `vibe-sprint`'s owner cohort is the outer sprint's instantiation of the same policy.

- `D-7` **Ingest branching: follow vs. auto-slice (task is AI-auto, SS is user-gated).** The ingest FlowDefinition produces a single `sprint_plan` typed artifact (persists the chosen branch). If the input file contains an explicit task/sprint segmentation, the resolver emits that list verbatim (`SS-18 AC-2`, `BR-2`) and `vibe-sprint` iterates it. If the input is vague, the ingest's `ss_converter` **auto-slices** a sprint plan after the SS lock (no user card) and persists it as the same artifact shape. Both branches return the same artifact type so downstream binding (`SD-23` typed artifacts) is uniform. Task breakdown inside a sprint is likewise AI-auto and never blocks the run.

Why chosen: every decision stays declarative (FlowDefinition + agent + pack), the engine stays domain-free (`SD-19 D-1`), the bridge `agent_flow as one Workflow step` is untouched, and Dev's mutation surface is exactly one new rule that also preserves Dev's existing modal.

## 4. Component Impact

- **Impacted modules:**
  - `apps/local-runner/internal/agentpack` (`pack.go`, `flow_safety_topology.go`, `pack_test.go`): parsed `working_mode`-agnostic; new `owner` agent + three flows.
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml`: registers `agents/owner.md` + `flows/vibe-*.yaml`.
  - `apps/local-runner/internal/flowgate` (`rules.go`, `evaluate.go`, `enforce.go`): add `r-requirement` rule, carry the requirement-check hub's signal into `TurnResult` (same injection pattern as `ScopeHighSeverity`).
  - `apps/local-runner/internal/runner` (`interactive_service.go`/`gate_hook.go`, `localFileSessionStore`/supabase run store): store `working_mode` on the run, dispatch to the correct resolver branch per `SS-18 BR-1`. Gate that the SS lock (`vibe-ingest` `ss_validator`) happens before any `vibe-sprint`.
  - `apps/desktop-flowpilot` + `cli-tui` (TUI `flowpilot chat`): new Vibe entry (Desktop file picker / TUI ` /vibe <path|prompt>` or `/vibe-file`), `SS Preview & Lock` card + `r-requirement` (`ask_user` with `BlockReason: requirement`) rendering on the run timeline. **Admin Web is not in scope for Vibe** (`SS-18` scope).
- **New modules:** `agents/owner.md`, `flows/vibe-ingest.yaml`, `flows/vibe-sprint.yaml`, `flows/vibe-owner-debate.yaml`, `tools/vibe-requirement-outcome.yaml` (control-tool face for `vibe.requirement_check`).
- **Unchanged modules:** `Admin Web` (no Vibe entry), `ProviderRuntimeAdapter` / SSE transport (additive only), linear `Workflow`/`Steps` engine table migration beyond a `working_mode` enum/column (see §5), published provider CLIs themselves.

## 5. Data Model

- **Configuration:**
  ```go
  // FlowDefinition fields are unchanged; Vibe is expressed as three new YAMLs
  // plus a single shared agent whose provider/model is per-node configurable.
  // No new struct fields on FlowDefinition/FlowPolicy.
  type WorkingMode string // "dev" | "vibe"  // default "dev"
  // Persisted on the run record that already carries yolo/provider intent,
  // e.g. interactiveRun / Supabase workflow_run header.
  // Column proposal: workflow_runs.working_mode text check (working_mode in ('dev','vibe')).
  ```
- **FlowGate rule (new):**
  ```go
  // r-requirement injects an advisory signal alongside ScopeHighSeverity:
  type TurnResult struct {
      // ... existing ...
      RequirementDrift bool     `json:"requirement_drift,omitempty"` // hub's 1:1 signature↔SS result
      RequirementDriftDetail string `json:"requirement_drift_detail,omitempty"`
  }
  // DefaultRules append:
  // {ID: "r-requirement", Scope: "step", Trigger: "requirement_signature_drift",
  //  RequiredOutput: "reconcile_tests_with_ss", Action: "block", Enabled: true}
  ```
- **Agent:** `AgentSpec{Name: "owner", Role: "owner", Provider: <configurable>, Model: <configurable>, Tools: [Read,Grep,Glob]}`
  The per-node `FlowNode.Agent = "agents/owner.md"`; provider/model are resolved per `Owner_1`/`Owner_2` node via the same override that today allows per-step model selection (`SS-04 §3.1`).
- **FlowDefinitions (skeleton, §3):** `vibe-ingest` (no writer, no acceptance), `vibe-sprint` (writer `coder`, acceptance `[synthesis]`), `vibe-owner-debate` (no writer). All three reuse `SD-19 D-7` vocabulary (`run`, `lifecycle`, `behavior`, `cohort`, `join`, `kind`) and `ValidateFlowDefinition`/`ValidateFlowSafetyTopology` checks.

- **State transitions:** unchanged (`PENDING/RUNNING/WAITING_USER_APPROVAL/DONE/FAILED/SKIPPED` for steps; `done/continue/escalate` for `flow_control`). Vibe `WAITING_USER_APPROVAL` for `r-requirement` carries `BlockReason: "requirement"` so the client renders a requirement drift card, not a generic retry card; Owner cap carries `BlockReason: "cap"`.

## 6. Interfaces and Contracts

- **Tool contract:** one new declared control-tool face `vibe-requirement-outcome` (mirrors `submit_review_outcome`):
  ```yaml
  id: vibe-requirement-outcome
  kind: flow_control
  mapsTo: flow.control
  exposesTool: true
  input: {verdict: {enum: [aligned, drift_fixable, requirement_change]}, summary: {type: string}}
  statusMap: {aligned: done, drift_fixable: continue, requirement_change: escalate}
  ```
  `drift_fixable → continue` re-invokes the coder/owner cohort; `requirement_change → escalate` routes to `ask_user`.

- **FlowGate contract:** `r-requirement` resolves to a top-tier `block` (`SD-20 D-2` family: `r-tests`/`r-reg` always-block). The resolver branch:
  ```go
  result := flowgate.Evaluate(turnResult) // single highest EnforceResult
  if result.Rule.ID == "r-requirement" { render AskUserCard("requirement", result.Detail) }
  else if run.WorkingMode == "vibe" { spawn Owner debate cohort }
  else { render DevCard(result) } // identical to today
  ```

- **Pack contract:** `manifest.yaml` ↔ `BuiltinMeta` agreement is validated by `validateManifestFlowMatchesDefinition` (`pack.go:769`). Each new flow's `builtin.{editable,selectableIn,chatSubModes,cloneable,chatBaseline}` must match its manifest entry byte-for-byte, else `LoadBuiltinPack()` fails. `ValidateFlowDefinition` also rejects unknown `behavior`, bad `lifecycle`, missing `dependsOn` refs, and duplicate `back` edges per `when` — so `vibe-*.yaml` must stay single-`continue` back-edge clean.

- **Provider contract:** an `owner` run is an ordinary `agent.delegate` provider session bound to its `FlowNode.Agent`/model/provider; no new adapter behavior. `hub.inline` vs `agent.delegate` distinction (`SD-19 D-5`) is preserved.

- **Files to add:**
  - `apps/local-runner/internal/agentpack/flow-pack/agents/owner.md`
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-ingest.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-owner-debate.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/tools/vibe-requirement-outcome.yaml`
  - `requirements/05-System-Specs/SS-18-Vibe-Working-Mode.md` (spec), `requirements/06-System-Tech-Design/SD-24-Vibe-Working-Mode.md` (this file) — trace chain `SS-13 §5.1` metadata + `AI Quick View`.

## 7. Execution Flow

- **Vibe ingest (once per run, if raw requirement file is present) — Desktop/TUI only:**
  1. Desktop file picker or TUI command (`/vibe <path|prompt>` / `/vibe-file`) resolves the requirement file path or pasted idea prompt and starts a run with `working_mode=vibe`. Admin Web has no Vibe entry.
  2. `vibe-ingest` `ingest_reader` reads the raw Markdown/text; `ss_converter` is prompted to emit proper `SS` shape per `FORMAT-REFERENCE-SS` + `SS-13 §5.1/§5.2` (metadata block + AI Quick View + numbered sections), seeded with `feature_key: vibe-mode` where suitable (`SS-13` optional field); `ss_validator` checks required `SS-13` sections and that `AC-*` are present and testable, then **renders as an `SS Preview & Lock` card and pauses at `WAITING_USER_APPROVAL`** — the user must lock the SS list before any sprint runs (`SS-18 BR-2`, `AC-2`).
  3. After the SS lock, the runner auto-slices the sprint/task plan (no user card): if the input already lists `features/tasks/sprints`, the emitted `sprint_plan` artifact equals that list verbatim; otherwise the converter synthesizes the plan and persists it as the same typed artifact (uniform downstream). Task breakdown per sprint is likewise AI-auto.
  4. On `done`, the plan artifact is persisted beside the run (`FlowContextBinding` typed artifact) and the runner iteratively starts `vibe-sprint` for each sprint slice.

- **Per sprint (`vibe-sprint`, bounded retry):**
  1. `preflight_contract_plan` builds the frozen preflight draft for the slice; `preflight_contract_freeze` freezes it (`contract.freeze`). Every subsequent writer path is dominated by this freeze (`D-5`).
  2. `tdd` (`agents/tester.md`) writes **signature-only** tests from the frozen SS slice (`SS-04 §3.5.8`, `prompts/test-signatures.md`). No production code.
  3. `coder` (`agents/coder.md`, `agent.code`) implements to make the TDD signatures green. Tool scope is `Read/Edit/Write/Bash/Grep/Glob` per that agent, gated by `change-contract` (`CP-43`); `git commit` remains denied on coding children (`SD-20 D-7`).
  4. On `coder.done`, the gate hook runs the full suite (`SD-20 D-6`, `ensureBaseline`/`gate_hook`), producing `TurnResult{Tests, Failed, Regressed, WrittenPaths}` plus the hub-supplied `RequirementDrift` advisory. `flowgate.Evaluate` returns one `EnforceResult`.
  5. If the violation is `r-requirement` → hub `synthesis` emits `flow_control(status: escalate)` → runner settles hub node to `WAITING_USER_APPROVAL` (`BlockReason: requirement`) and the client renders the single Vibe user card (the drift detail + the two choices: edit SS or edit tests). No Owner spawn.
  6. If any other violation → resolver spawns `owner_1 ∥ owner_2` (cohort `vibe_owner`, `join: all`, isolated sessions). `synthesis` merges their remediation picks. Bounded-retry-then-escalate per `SD-20 D-7` and `BUG-231`/`BUG-234` (cap → `escalate`, not auto-`done`).
  7. `synthesis` ultimately emits `done` (sprint is green + requirement-aligned) → Workflow advances to the next sprint slice; `continue` routes back to `coder` via the single `back` edge; `escalate` routes to `ask_user`.

- **Owner debate extraction (`vibe-owner-debate`, internal):**
  The same `owner` cohort + hub `debate_synthesis` loop (cap 5) is available to any future gate that wants Owners outside a sprint. `vibe-sprint`'s owner nodes are the in-sprint instance; `vibe-owner-debate` is the standalone reusable instance. Both observe the same settlement invariant (`SD-19 §8 F-3` auto-advance gate).

## 8. Failure and Edge Handling

- `F-1` Work rejected by `ValidateFlowDefinition` (unknown behavior, missing `dependsOn`, duplicate `continue` back-edge) → pack load fails at `go test ./...` / runner start, before any run; CI catches it.
- `F-2` `ValidateFlowSafetyTopology` failure (writer without freeze, writer-path bypasses freeze, `done` path without `acceptance_nodes`) → `LoadBuiltinPack()` fails on that YAML; the skeleton must satisfy the built-in writer/freeze precedent (`context-coding-review-synthesis.yaml`) before the resolver is wired.
- `F-3` Manifest ↔ YAML builtin mismatch (`validateManifestFlowMatchesDefinition`) → pack load fails; keep `manifest.yaml:flows[].path/editable/selectableIn/…` byte-for-byte equal to each flow's `builtin.*`.
- `F-4` Single-owner fallback: if only one `owner` provider session could be started at a violation, run the single-owner synthesis pass but tag the remediation with `warn: single-owner`; still honor the 5-round cap.
- `F-5` Mixed input (some slices explicit, some vague): ingest marks which sprints were verbatim vs. synthesized so a later `r-requirement` detail can attribute drift correctly and avoid re-slicing verbatim sprints.
- `F-6` Vibe run interrupted/restarted mid-sprint or mid-Owner-debate: the local run sink already replays via `sessions.ndjson`; the sprint index and Owner round counter replay, so no sprint is skipped or duplicated.
- `F-7` `r-requirement` mis-classified: a weaken-to-green that was wrongly `continue` instead of `requirement` → regression probe that asserts `AC-6`/`BR-4`: any `Tests.Green && RequirementDrift` must be `block/escalate`, never `done`.

## 9. Security and Operational Concerns

- **auth:** `working_mode=vibe` is a local-runner/run attribute under the current project authority; it is never derived from provider output. `r-requirement`'s advisory is computed from frozen SS + runner-side artifact diff, not from model self-grading.
- **secrets:** pack YAML + `owner` prompt carry no credentials; provider switching per Owner node is through the runner's existing env isolation (`SS-11`), not through prompt injection.
- **audit:** every sprint's `tdd` artifact, `coder` diff, suite/TTR, `r-requirement` detail, Owner debate rounds, and hub `flow_control` signal are logged to the bus/run sink (share `r-ca`/`r-bug`-style audit footprint). `change-audit/*.md` + commit ledger remain owned by the audit/draft node (`SD-20 D-7`), not the coding children.
- **rollback:** additive only. With `working_mode=dev` (default) and before any `r-requirement` matcher is enabled, `go test ./...` and `LoadBuiltinPack()` are green on the new pack; deleting `vibe-*.yaml` + `owner.md` returns the runner to the prior built-in set. GTM: land skeletons inert, then land the Go resolver in a second PR.

## 10. Risks and Trade-Offs

- `R-1` **Gate mis-routing.** Sending a genuine `r-requirement` through Owners would hide drift. Mitigation: `D-2` is the first branch in the resolver; Owner spawn is `else`, never before. Probe `F-7`.
- `R-2` **Owner echo chamber.** Two same-model Owners rubber-stamp each other. Mitigation: allow distinct provider/model per node and require `Main` synthesis to treat consensus as remediation only when `r-requirement` is false; cap 5 forces human re-engagement.
- `R-3` **Sprint granularity.** 50 tiny sprints = 50 freezes/TDD cycles; 2 huge sprints = drift hides. Mitigation: `Q-2`/`D-7` persist the ingest's chosen slice so a reviewer can see the budget before any code runs, and a future tuning pass can adjust the converter prompt without changing the graph.
- `R-4` **Residual file-read in `owner.md` Scope.** Mitigated by constraining Owner tools to read-only-ish set (`Read/Grep/Glob/Read`) and delegating any required edit to the delegated `coder` via `continue` reprompt — Owners never write.
- `R-5` **Builtin-mismatch escapes.** A stale `manifest.yaml` ↔ `builtin.*` drift passes local dev but breaks CI/Release. Mitigation: CI runs `go vet` + `go test ./internal/agentpack -run TestLoadBuiltinPack` (already present) that calls `LoadBuiltinPack()` and would fail on `D-3` mismatch.

## 11. Validation Strategy

- **unit:** `go test ./internal/agentpack` — `LoadBuiltinPack()` + `ValidateFlowDefinition`/`ValidateFlowSafetyTopology` + `validateManifestFlowMatchesDefinition` for all four flows (existing 3 + 3 new). `go test ./internal/flowgate` — `r-requirement` triggers on `Tests.Green && RequirementDrift==true` as `block`, and that `r-requirement` resolves to `ask_user` only in `vibe` (dev path still `block` but without Owner spawn).
- **integration:** drive `vibe-sprint` against a fixture game requirement file in both intake shapes (pre-sliced vs. vague) with a mock provider suite that returns (a) green-but-drifted signatures → `r-requirement` escalates, and (b) generic `r-*` → Owner cohort spawns and bounded-retries; confirm `dev` fixture stays on the Dev card path and never spawns Owners.
- **manual:** exercise the desktop/TUI file import or chat prompt for a `vibe-ingest` → 3× `vibe-sprint` chain on a sample game spec; every Owner-remediable gate auto-resolves, and only an intentional requirement mismatch produces the single Vibe user card.
- **observability:** `SD-19 §8 F-3`/`F-4` settlement contract already logs `BlockReason` and caps; add `requirement` block reason and surface it on the run timeline alongside existing `cap`/`escalate` rendering.

## 12. Traceability to Spec

- `AC-1` entry + SS conversion → `D-1`, `D-6`, `D-7`, §7 ingest.
- `AC-2` follow vs. slice → `D-7`, §5 `WorkingMode` plan artifact.
- `AC-3` TDD signature-only before coder → `D-5` `tdd → coder` edge + `agents/tester.md` reuse.
- `AC-4` green → `r-requirement` check → `D-2`, `D-5` hub logic, `RequirementDrift` advisory, `SD-20 D-2`.
- `AC-5` non-requirement → Owner debate → `D-3`, cohort `vibe_owner`, isolation.
- `AC-6` cap 5 → `D-4` `FlowPolicy{cap:5}` + settlement.
- `AC-7` only two user asks → `D-1`/`D-2`/`D-3` resolver split.
- `AC-8` Dev non-regression → `D-1` default `dev`, §11 dev probe.
- `AC-9` persistence/resume → §5 run store, `D-4` per `CP-36 P-5`.
- `AC-10` auto sprint-by-sprint demo → `D-5` `vibe-sprint` + `D-6` ingest + §7 chain.
- `BR-1`..`BR-9` → `D-1`..`D-5`, §8 `F-7`, `SD-19 D-1`/`D-6`.

