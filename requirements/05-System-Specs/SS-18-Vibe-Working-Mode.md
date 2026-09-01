# SS-18: Vibe Working Mode

## Metadata

- Document ID: `SS-18`
- Title: `Vibe Working Mode (Non-Tech Auto Sprint)`
- Feature Keys: `vibe-mode`
- Phase: `system_spec`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-01`
- Last Updated: `2026-09-01`
- Parent Documents: `Product Vision`, `SP-01 Human In Loop`, `SS-04 Workflow`, `SS-08 Approve Gate`, `SS-11 Workflow With Session`, `SS-13 AI-Followable Document Contract`, `SS-14 Code Context And Regression Safety`, `SS-16 Agent Flow Engine`
- Child Documents: [SD-24: Vibe Working Mode](../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md)
- Related Documents: [SS-15: Agent Review Loop](./SS-15-Agent-Review-Loop-Until-Clean.md), [SD-19: Agent Flow Engine](../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SS-06: Workflow Skill Agent](./SS-06-Workflow-Skill-Agent.md), [SD-20: Flow Gate Rule Semantics](../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Replaces: `None`
- Tags: `vibe-mode, working-mode, agent-flow, tdd, owner-debate, sprint`

## AI Quick View

### Summary

- Introduce a second `working_mode` beside the existing Dev-controlled mode: **Vibe** for non-tech users who supply a single requirement file (detailed game spec or raw idea) and want FlowPilot to run **auto sprint-by-sprint** to completion. Vibe lives only in **Desktop app + TUI** (`cli-tui`); **not in Admin Web**.
- `SS` must be **locked first** before any sprint runs. The ingested SS list is the standard; `Task` slicing is fully automatic afterwards — AI self-organizes sprints/tasks, no user lock required.
- In Vibe, every gate except one stays evaluated but is **auto-resolved by two isolated Owner agents** (`Owner_1` ∥ `Owner_2` → Main synthesis) debating through the Main hub, not by showing Dev cards `1/2/3`. Only two cases surface to the non-tech user.
- TDD is mandatory and ordered: SS is produced first, test signatures are written from that SS, code is written to make those tests green, and after green a 1:1 signature↔requirement check runs under AI.
- Add a single new user-only gate `r-requirement` (`g-requirement`): tests are green but the test signatures no longer adapt to the SS acceptance criteria, or a fix would need to change the SS. That gate — and a 5-round Owner debate with no consensus — are the only user asks in Vibe.
- Realize Vibe as **data, not engine code**: three FlowDefinitions (`vibe-ingest`, `vibe-sprint`, `vibe-owner-debate`) plus one `owner` agent persona, on the existing SS-16/SD-19 generic engine. Dev mode is unchanged.

### Current Ask

- Freeze the business contract for Vibe as a working-mode policy over the current Workflow + FlowGate + Agent-Flow substrate, so SD-24 and the three flow YAML skeletons can be written without engine changes.

### Key Decisions

- `AC-0` Vibe is a **policy** (`working_mode=vibe`), not a separate orchestrator; the runner still owns Workflow/Session/Gate and the harness `Dev — FlowPilot — AI` / `User — FlowPilot — AI` stays intact.
- `AC-0b` Two isolated Owner agents (configurable same or different provider/model) replace every Dev `1/2/3` decision. One Owner is not trusted alone; consensus is required.
- `AC-0c` Only `r-requirement` and a 5-round no-consensus debate escalate to the user; every other `r-*`/`permission_required`/`user_question` is Owner-handled.

### Constraints

- Must reuse the SS-16/SD-19 domain-free flow engine, the existing `flowgate` rule set, `SS-13` document contract, and `SS-14` oracle rule (`SS-14 AC-6`: never weaken a test to go green).
- Must not break Dev mode: no change to the Dev `1/2/3` gate cards, no new YOLO semantics, no silent `done` with open issues.
- `working_mode` is the SSOT for resolver choice; `provider_session_id` remains a runtime optimization (`SS-11`).

### Open Questions

- `Q-1` Resolved — `vibe-ingest` produces draft SS artifacts and the Desktop/TUI shows an `SS Preview & Lock` step. The user must lock the SS list before any `vibe-sprint` runs (no auto-commit).
- `Q-2` Should a single requirement file that already lists 50+ features be split into 50 sprints or chunked into fewer slices (cost vs. granularity)? `Task` slicing is AI-auto and not user-gated; chunking is an internal ingest decision.

### Source Refs

- `Product Vision` §5 (context accumulation, step adherence); `SP-01` §1 (human loop); `SS-04` §3.5 (MVP steps, TDD), §3.7 (step contract), §4.1 (run/step states); `SS-08` §2-§3 (Safe/YOLO SSOT); `SS-11` §4 (subagent session), §9 (persistence); `SS-13` §5-§7 (metadata, AI Quick View, chain); `SS-14` §6-§7 (`AC-6` oracle); `SS-16` `AC-3`/`BR-1` (engine domain-free, flow is data); `SD-19` `D-1`..`D-7`, `D-5` bridge; `SD-20` `D-1`..`D-7` (gate hook, `r-*` semantics); `review-loop.yaml` / `context-coding-review-synthesis.yaml` (node/edge reference shape).

## 1. Goal

Let a non-tech user hand FlowPilot one requirement file describing a game (either a well-sliced task list or a vague idea) and have the system **automatically run end-to-end, sprint by sprint**, under a harness the user can trust: every decision is either proven from the SS or debated by two Owners, and the human is only asked when the requirement itself would need to change.

The existing Dev mode (`Dev — FlowPilot — AI`, gate cards `1/2/3`) is untouched. Vibe (`User — FlowPilot — AI`, auto gate) is the second working mode of the same runner.

## 2. Problem

Today FlowPilot is optimized for Devs: every `g-*` gate (e.g. `g-ca`/`r-ca`, `g-bug`/`r-bug`, `r-tests`/`r-reg`) surfaces a card for the Dev to pick `1/2/3`. A non-tech user does not want — and cannot — make those calls per gate. They also do not want to drive a chat turn by turn; they have a file or an idea and want sprints to happen.

At the same time, fully autonomous `YOLO` is not the answer: YOLO skips gates, loses the TDD-before-code ordering (`SS-04 §3.5.8`: TDD must cover all use/edge/error cases from business requirement and guard coding), and cannot detect when green tests have drifted from the SS that is the standard.

What is missing is a deterministic Vibe policy: same Workflow/Session/Gate substrate, same `Dev — FlowPilot — AI` harness shape, but the resolver for every non-requirement gate is **two Owners debating through Main**, with a single explicit human gate when the SS itself is at stake.

## 3. Scope

- **In scope:**
  - A `working_mode` discriminant (`dev` | `vibe`) that selects which resolver handles a gate violation. `dev` = current Dev cards; `vibe` = Owner debate. `YOLO` is not altered. **Vibe entry exists only in Desktop app + TUI** (`cli-tui`); **not in Admin Web**.
  - Two intake shapes for Vibe:
    1. **Pre-sliced input:** the file already lists features/tasks/sprints — follow that list exactly, no AI re-slicing.
    2. **Vague idea:** the user has only a high-level idea — let the AI organize/slice into sprints and persist that slice list as the plan artifact before coding. **Task/sprint slicing is AI-auto and never user-gated.**
  - A **Desktop app / TUI** entry (TUI command or file picker, or a plain text file path/pasted idea) that the runner hands to the provider with a prompt requesting conversion to proper `SS-13`/`FORMAT-REFERENCE-SS` style. That SS list is the standard for the whole run and **must be locked by the user before any sprint runs**.
  - A strict ordering **once**: `SS ingest → SS Lock (user)` → **per sprint**: `TDD from locked SS slice → code → green → r-requirement signature↔SS check`. `Task` breakdown inside each sprint is AI-auto and never blocks the run.
  - One new gate `r-requirement` (`g-requirement` in user-facing prose): tests are green but the test signatures no longer map 1:1 to the SS acceptance criteria, or owning the fix would require changing the SS. Resolved by **user action only**.
  - An Owner debate cohort: exactly 2 isolated Owner sub-agents (`Owner_1` ∥ `Owner_2`), any provider/model (user-configurable, same model / different model / different provider all valid), debating through Main. At most 5 rounds; no consensus within 5 → escalate to user.
  - The closed user-ask set in Vibe: only `r-requirement` and 5-round no-consensus. Goal artifact: one detailed game requirement file drives **auto, sprint-by-sprint** execution.
- **Out of scope:**
  - Game-specific content or balancing.
  - Changing YOLO SSOT (`SS-08`) or Dev `1/2/3` cards.
  - A second orchestrator or a second run store; Vibe reuses the single Go runner and the single `sessions.ndjson` run sink (`CP-36` `P-5`).

## 4. Non-Goals

- Not a general auto-agent framework beyond the sprint harness; every sprint is still a `preflight_contract_plan → preflight_contract_freeze → … → synthesis` FlowDefinition inside one `agent_flow` Workflow step (`SS-16 §7`, `SD-19 D-5`).
- Not auto-commit/merge of a sprint; the flow ends `done`/`escalate`/`failed` and surfaces artifacts, it does not push.
- Not weakening tests to go green; `SS-14 AC-6` and `SD-20 D-1` stay authoritative.
- Not flattening the agent sub-graph into `workflow_steps`.

## 5. User Stories or Primary Use Cases

- `US-1` As a non-tech game owner, I want to point FlowPilot at one requirement file (or paste my idea) and have it convert the content into proper `SS` style, so that the SS list becomes the standard without me writing `05-System-Specs` by hand.
- `US-2` As a non-tech game owner, when my file already slices features/tasks/sprints, I want FlowPilot to follow that exact slicing; when I only have a vague idea, I want the AI to slice into sprints for me, so both starting points work.
- `US-3` As a non-tech game owner, I want every sprint to do TDD from its SS slice before any production code, and after the tests go green an **AI** 1:1 check that the test signatures still adapt to the requirement.
- `US-4` As a non-tech game owner, I want any failing gate other than requirement drift to be handled by two Owners debating on my behalf (isolated, Main-synthesized), so I am not asked to pick `1/2/3` per gate.
- `US-5` As a non-tech game owner, the only time I am asked is when the requirement itself is in question (`r-requirement`) or the two Owners cannot agree within 5 rounds, so my attention is bounded to requirement input.
- `US-6` As a Dev, I want Dev mode behavior to be **identical** to today when `working_mode=dev`, so Vibe never regresses Dev.

## 6. Acceptance Criteria

- `AC-1` A single Vibe entry **in Desktop app and TUI only** (`cli-tui`; not Admin Web) that accepts either a pre-sliced requirement file or a vague idea prompt is available, and the runner's ingested SS conversion is produced in `SS-13`/`FORMAT-REFERENCE-SS` shape from the raw input.
- `AC-2` The ingested SS list is shown in a **Desktop/TUI `SS Preview & Lock` step and must be locked by the user before any `vibe-sprint` starts**. Task/sprint slicing afterwards is AI-auto and never user-gated: when the input is pre-sliced, the sprint list equals that list exactly; when vague, AI auto-slices and persists the plan as the first artifact without a user lock.
- `AC-3` Per sprint, a TDD node writes **signature-only** tests (`SS-04 §3.5.8`) derived from the sprint's SS acceptance criteria, and a coder `agent.code` node follows only after that TDD artifact exists.
- `AC-4` The global sequence is strictly **SS ingest → SS Lock (user, Desktop/TUI) → per sprint: TDD signatures from locked SS slice → code → tests green → `r-requirement` signature↔SS check**. `Task` auto-slicing happens between the lock and the first sprint. If `r-requirement` fires after green, the sprint does not quietly `done`; it escalates to `ask_user` for the non-tech user.
- `AC-5` Every `r-*`/`permission_required`/`user_question` other than `r-requirement` is **not** surfaced as a Dev `1/2/3` card in Vibe. Instead, the gate resolver spawns exactly 2 isolated Owner agents (`Owner_1`, `Owner_2`) with user-configurable provider/model, running in parallel (cohort `vibe_owner` / `owner_debate`) and debating only through Main (hub-only, `SS-15 BR-1` / `SS-16 BR-5`).
- `AC-6` An Owner debate is bounded to **at most 5 rounds** (`cap: 5`, `onCap: escalate`). If the two Owners reach consensus within the cap, that remediation is applied automatically without a user card (retry/reprompt/re-scope the sprint). If they do not agree within 5 rounds, the flow escalates to `ask_user` and no further automatic retry occurs until the user resolves.
- `AC-7` Only two cases ever produce a user ask in Vibe: `r-requirement` (requirement drift, `BR-4`) or 5-round no-consensus (`AC-6`). Every other gate path terminates `done` or auto-retries under the Owner cap, never as a user card.
- `AC-8` In `dev` mode, gate handling is byte-for-byte the current Dev behavior (`T-3`..`T-9` in `SD-20`, Dev `1/2/3` cards). A regression test proves `dev` mode does not receive Owner debate.
- `AC-9` The runner persists the whole Vibe run (including Owner debate rounds and the `r-requirement` check) to the single local run sink (`sessions.ndjson`, Drive-synced; `CP-36 P-5`) and the run remains resumable after restart, with the sprint index and `working_mode=vibe` intact.
- `AC-10` An out-of-the-box detailed game requirement file drives a full **auto sprint-by-sprint** demonstration (e.g. 3 sprints): each sprint passes `AC-3`/`AC-4`, non-requirement gates auto-resolve via `AC-5`/`AC-6`, and the only pauses, if any, are at `r-requirement`.

## 7. Business Rules

- `BR-1` **`working_mode` controls only the resolver.** `dev` → Dev card; `vibe` → Owner debate. It does not change `SS-08` YOLO SSOT, the `workflow_steps` state machine, or the set of rules evaluated (`SD-20` `D-2`/`D-3`). A live provider session is never migrated across providers (`SS-11` §5.1).
- `BR-2` **SS list is the standard and must be locked first.** The ingest FlowDefinition (`vibe-ingest`) produces SS-shaped artifacts (`FORMAT-REFERENCE-SS`, `SS-13`) shown in the Desktop/TUI `SS Preview & Lock` card. No `vibe-sprint` may start until the user locks that list. Every later TDD/code node derives from that frozen SS slice; the SS is the oracle for `r-requirement`. `Task` breakdown is AI-auto after the lock and never requires a user lock.
- `BR-3` **TDD guards coding.** A sprint's TDD signatures must cover all use/edge/error cases from the business requirement (`SS-04 §3.5.8`). If the coding plan or architecture does not match the TDD guard, the plan/architecture is changed, not the tests.
- `BR-4` **`r-requirement` is user-only and never Owner-auto-resolved.** It fires exactly when: tests are green but the passing test signatures no longer map 1:1 to the SS acceptance criteria, or a violation can only be fixed by changing the SS. Its action is `block → ask_user`. No Owner remediation may rewrite or weaken a test to satisfy the SS (`SS-14 AC-6`), and weakening a test to go green is **always** `r-requirement`, never an Owner-approved fix.
- `BR-5` **Two Owners, isolated, hub-only.** `Owner_1` and `Owner_2` are two isolated sub-agents running in parallel on the same job (cohort `vibe_owner`/`owner_debate`, `join: all`), each with its own provider/session (`SS-11` §4). They share only final results upward through Main; they never see each other's transcripts (`SS-15 BR-2`, `SS-16 BR-5`). Provider/model is configurable per node (same model, different model, or different provider all valid).
- `BR-6` **Bounded + explicit.** Every Vibe loop (sprint loop or Owner debate) has a `FlowPolicy{cap, onCap: escalate, extendBy, extendMax}` (`SD-19 D-7`). Owner debate's cap is fixed at **5**; the sprint loop's cap is set by the sprint FlowDefinition. At cap, the hub settles to `WAITING_USER_APPROVAL` with `BlockReason: cap` or `escalate` per `BUG-231`/`BUG-234`, never `RUNNING` or silently `done`.
- `BR-7` **Vibe is data.** Every Vibe behavior is declared in FlowDefinitions (`vibe-ingest`, `vibe-sprint`, `vibe-owner-debate`) and one `owner` AgentDefinition. The generic engine carries no role strings (`SS-16 AC-3`/`BR-1`). A new Vibe variation requires only YAML + skill, not engine code.
- `BR-8` **No silent success.** A green test run that still fails `r-requirement`, or a cap hit with `r-requirement` open, must surface as `ask_user`, never `done`.
- `BR-9` **Dev non-regression.** When `working_mode=dev`, the resolver emits the current Dev `1/2/3` / `block` modal (`SD-20` §2.4/`D-3`). Owner logic is unreachable in that branch.

## 8. Edge Cases

- `E-1` Input file mixes pre-sliced tasks and vague sections — follow the explicit slices and only let AI organize the vague remainder; persist what was followed vs. what was synthesized.
- `E-2` `TDD` or `coder` fails mid-turn — the gate resolver still fires `AC-5` (Owner debate) exactly once per violation batch; it never spawns two competing retries.
- `E-3` First sprint is already green and `r-requirement` is clean — the sprint ends `done` in one pass with no Owner spawn.
- `E-4` Cap is hit on the very first Owner debate — the flow escalates to `ask_user`, not silent stop.
- `E-5` User stops mid-sprint or mid-debate — in-flight work follows existing Stop semantics and no new round starts until resumed.
- `E-6` Runner restarts mid-Vibe — the sprint index, `working_mode=vibe`, Owner debate round, and SS list replay exactly; the run resumes at the same sprint.
- `E-7` Only one Owner could be spawned (second provider quota) — the debate degrades to a single-owner pass that still respects the 5-round cap and escalates on `r-requirement`, but a warning is surfaced noting no independent cross-check occurred.

## 9. Dependencies

- The Workflow/Step engine (`SS-04`, `cp07_workflow_engine`), Approve gates (`SS-08`), Workflow-with-session (`SS-11`), the generic Agent Flow Engine (`SS-16`/`SD-19`/`CP-36`), and Flow Gate rule semantics (`SD-20`) plus gate `flowgate` implementation.
- The agent substrate + board (`CP-19`/`SD-16`) and the `sessions.ndjson` run sink (`CP-36` `P-5`).
- Provider adapters (Codex/Claude/Grok): all three obey the same `working_mode=vibe` resolver contract.
- `SS-13` + `FORMAT-REFERENCE-SS`, `SS-14` + oracle (`SD-20` `D-4`), `SS-09` artifact-memory pipeline.

## 10. Open Questions

- `Q-1` Resolved — SS must be locked in Desktop/TUI before sprints; see `BR-2`.
- `Q-2` See §AI Quick View `Q-2` (sprint granularity); task slicing is AI-auto, SS is the only user-gated artifact.
- `Q-3` Should the per-sprint `r-requirement` check reuse an existing `synthesizer` inline node or warrant a dedicated `behavior: vibe.requirement_check` identifier?
- `Q-4` Should the Desktop/TUI Vibe entry be a first-class `working_mode` toggle in project settings, or only a per-run flag at `POST /client/workflow-runs` / `POST /client/flows/run`? (Admin Web has no Vibe entry.)

## 11. Definition of Done

- `AC-1`..`AC-10` are demonstrable with a real game requirement file using **all three** providers for the Owner debate (incl. the configurable same/different provider case).
- The `r-requirement` gate is proven: green tests with a signature↔SS mismatch are caught and surfaced as the sole user card; an Owner attempt to weaken a test does not go `done`.
- A dev-mode regression run confirms Dev `1/2/3` cards are unchanged and Owner logic is unreachable when `working_mode=dev`.
- The `vibe-ingest` → sequential `vibe-sprint` chain (with `vibe-owner-debate` for non-requirement gates) is specified in `SD-24` and seeded as skeletons in the flow pack; `SS-13` contract and `agent-flow-engine` Kill-Review boundary are preserved.

