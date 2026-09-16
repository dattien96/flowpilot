# SS-14: Code Context And Regression Safety

## Metadata

- Document ID: `SS-14`
- Title: `Code Context And Regression Safety`
- Feature Keys: `change-contract, lsp-runtime, reproduce-first-gate, living-knowledge-base`
- Phase: `system_spec`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-09-15` (added AC-18 Two-Tier Intelligence CP-63, AC-19 Reproduce-First CP-64, AC-20 Living Knowledge Base CP-66)
- Parent Documents: `Product Vision`
- Child Documents: [SD-17: Context And Regression Engine](../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SD-22: Pluggable Context Source Registry](../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md) (US-9), [SD-23: Generic Artifact Framework](../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md) (US-10, AC-17), [CP-35: Context And Regression Engine Rollout](../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-63: IDE-Grade LSP Runtime](../07-Coding-Plan/todo/CP-63-IDE-Grade-LSP-Runtime.md), [CP-64: Reproduce-First TDD Gate](../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [CP-66: Living Knowledge Base](../07-Coding-Plan/todo/CP-66-Living-Knowledge-Base-Context-Source.md)
- Related Documents: [SS-02: Project Context](./SS-02-Project-Context.md), [SS-09: Artifact Memory Context Retrieval](./SS-09-Artifact-Memory-Context-Retrieval.md), [SS-13: AI-Followable Document Contract](./SS-13-AI-Followable-Document-Contract.md), [SP-06: Oracle Rule And Schema First Gate](../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md)
- Replaces: `None`
- Tags: `context, regression, code-graph, oracle-guard, two-tier-intelligence, lsp, reproduce-first, living-knowledge`

## AI Quick View

### Summary

- FlowPilot wraps AI providers; its core promise is correct output on a real, evolving codebase, not just plausible text.
- Two failures break that promise: the AI lacks whole-project context (it edits inside one file's scope), and it has no change history (it conflicts with or deletes prior work — a regression).
- **Two-Tier Code Intelligence (CP-63)**: Operates at both the **Macro Plane** (GitNexus knowledge graph, 300 execution flows, blast radius) and the **Micro Plane** (embedded LSP runtime providing compiler diagnostics in <200ms on RAM).
- **The Reproduce-First Oracle (CP-64)**: Pre-existing tests are ground truth; for bugfixes, an agent is physically gated from editing production files until it produces an executable test that compiles and fails via assertion (`r-reproduce`).
- **Living Knowledge Base (CP-66)**: Distills repo execution flows into `.flowpilot/knowledge/`, injected via the `knowledge.flow` context source to eliminate exploratory file-reading token waste.
- This spec defines the business acceptance criteria for **context sufficiency** and **regression safety** that any FlowPilot-driven code change must meet across any bound repository.

### Current Ask

- Define, at the spec level, what "enough context", "two-tier intelligence", and "regression safety" mean as testable outcomes.

### Key Decisions

- `AC-1` The engine operates only on the currently bound target project with strict isolation.
- `AC-3` Before editing a feature, the AI receives that feature's prior change history ranked by locus.
- `AC-6` A failing pre-existing test is never resolved by weakening the test; it forces an SS/SD re-check.
- `AC-18` Live compiler feedback is supplied in <200ms via embedded LSP stdio JSON-RPC (Micro Plane), while architectural blast radius is gated via GitNexus (Macro Plane).
- `AC-19` Bugfix flows enforce `r-reproduce`: coder write access requires an executable test failing with assertion error.
- `AC-20` Living knowledge flows are incrementally maintained at flow audit and injected into planning contexts.

### Constraints

- Must build on, not replace, the SS-09 / SD-10 artifact-memory pipeline.
- Must work offline-first on the developer's machine and degrade gracefully when project documentation is absent.
- Regression enforcement must be safe to roll out incrementally (warn-before-block).

### Open Questions

- `Q-1` Resolved (§10) — regression enforcement is always on; documentation rules may stage warn→block.
- `Q-2` Resolved (§10) — N/A in v1: no per-language parser; GitNexus + file-level fallback.
- `Q-3` Resolved (§10) — on test/spec mismatch, raise and require explicit user confirmation.
- `Q-4` Resolved (§10) — single project only; strict per-project isolation.

### Source Refs

- `SS-02` project context; `SS-09` artifact memory & retrieval; `SS-13` §7.3 upstream-update triggers, §8.3 authority order.
- Downstream: `SD-17`, `CP-35`. Related plan: `CP-23` (behavioral drift & auto-learn), `CP-32` (unit-test rule).

## 1. Goal

Make FlowPilot reliably correct on a real, changing codebase by guaranteeing two things for every AI-driven code change:

1. The AI receives **enough project-wide context** to be correct at project scope, not just inside the file it happens to edit.
2. The change is **regression-safe**: it does not silently break, conflict with, duplicate, or delete behavior that prior work established, and failing tests are honored as truth rather than edited away.

## 2. Problem

Working with a raw AI provider, a developer hits two repeating failures:

- **No overall project context.** The AI sees only the changed file's scope, so its fix is wrong at whole-project scope — it misses callers, conventions, and architecture.
- **Regression.** The AI does not know what an earlier task or earlier code already did, so it produces conflicting changes, re-implements existing behavior, or deletes load-bearing code.

A plain semantic RAG does not solve either: "who depends on this symbol" is a structural question, and "what did prior work establish" is a historical one. Neither is reliably answered by similarity search.

A third, subtler failure compounds regressions: when a test fails, the AI "fixes" it by changing the code's intended behavior or the test itself until it passes — converting a caught regression into a shipped one.

## 3. Scope

- In scope:
  - Context sufficiency for code work on a bound project (structural + historical context).
  - Regression detection and prevention for AI-driven code changes.
  - Test-as-oracle integrity (no weakening tests to pass).
  - Operating on arbitrary bound repos regardless of documentation format.
  - Per-project, multi-tenant isolation of all context/history data.
- Out of scope:
  - Runtime vector retrieval implementation details (owned by `SD-10`).
  - Provider-internal behavior and prompt internals.
  - Behavioral progress/wrong-way detection and mistake-to-skill auto-learning (owned by `CP-23`).

## 4. Non-Goals

- This spec does not replace `SS-09` / `SD-10`; it adds structural and historical context planes around them.
- It is not a general code-review or static-analysis product; its checks are scoped to the change an AI step proposes.
- It does not auto-repair regressions without a human-safe gate; it detects, blocks, and routes.

## 5. User Stories or Primary Use Cases

- `US-1` As a developer who binds a repo, I want the AI to understand the whole project's structure and dependencies, so its fix is correct beyond the single file it edits.
- `US-2` As a developer, I want the system to know what prior tasks and code already established, so the AI does not conflict with, duplicate, or delete load-bearing work.
- `US-3` As a developer, I want the AI to declare what it intends to change and be flagged when it changes anything else, so unexpected edits surface immediately.
- `US-4` As a developer, I want a failing test to trigger a re-check of the spec/design, not a quiet rewrite of the test, so caught regressions are never shipped.
- `US-5` As a developer binding a legacy repo with no FlowPilot documents, I still want context and regression safety from code and git history alone.
- `US-6` As a reviewer, I want a per-step record of what context was used and what the change actually touched, so I can audit the AI's work.
- `US-7` As a developer, I describe a feature in plain words ("update the chat UI") without knowing any id, and the system finds the right feature and its ordered history for me.
- `US-8` As a team lead, I want the system to force the AI to record what it did (change-audit, Task/BugFix doc) and keep tests honest, so the process cannot be silently skipped.
- `US-9` As a developer — or the built-in pack itself — I want to add new *kinds* of context the AI is grounded on (e.g. an MCP driver file, a Jira/issue ticket, other external systems), without changing how the context package is structured, so the AI's grounding can grow to new inputs without a code refactor each time.
- `US-10` As a developer — or the built-in pack itself — I want a flow step to declare its grounding inputs and outputs as **typed artifacts** (not just context — e.g. a set of files, and later a review bundle or other kinds), configured once as reusable instances and bound to steps by type, so the same grounding mechanism serves many kinds of step input/output without a new code path for each kind.

## 6. Acceptance Criteria

- `AC-1` All context and history are built from and stored against the currently bound target project. If FlowPilot itself is the bound target, its own repo may be used as context for that project; no other project's data is visible to another.
- `AC-2` The engine produces useful structural context, change history, and regression checks on a bound repo that has no SS-13 documents and no FlowPilot change history, using code and git alone.
- `AC-3` Before editing a feature, the AI is given that feature's prior change history in order (newest = current truth), so it builds on existing work instead of undoing it.
- `AC-4` Where structural tooling (e.g., GitNexus) is available, a change that removes or breaks code still referenced elsewhere is surfaced before the step is accepted.
- `AC-5` When structural tooling is available, the code that depends on what is being edited is summarized into the AI's context (count, nearest dependents, affected flows), not dumped raw.
- `AC-6` A failing pre-existing test never results in that test being modified or deleted, nor in the code's intended behavior being changed, to make it pass; instead the system re-checks the governing acceptance criteria. The oracle (test) may change only after the upstream `System Spec` / `Tech Design` is updated and a human confirms.
- `AC-7` Every code-mutating step records the context it used and the symbols/files it actually changed, retrievable for audit.
- `AC-8` Context derived from code that has since changed is flagged as stale and de-prioritized rather than presented as current truth.
- `AC-9` Indexing, memory extraction, and regression checks are non-fatal: a failure degrades the experience and is retryable, but never blocks the raw work from saving.
- `AC-10` A developer can describe work in natural language without knowing any feature/task id; the system resolves it to the correct feature and its history, asking for confirmation when ambiguous.
- `AC-11` After an AI step, the system forces the required outputs for that step — e.g., an updated change-audit note, a Task or BugFix document, and passing (or explicitly explained) tests — and will not mark the step complete until they exist.
- `AC-12` Flow-aware skills (including the oracle rule) are auto-installed into each bound project so the AI follows the flow by default, across providers.
- `AC-13` The external tooling the engine relies on (e.g., GitNexus, RTK, node, the skill pack) is installable and health-checked from a setup surface; when a tool is missing the engine degrades to a lower capability tier rather than failing.
- `AC-14` A test that was passing before a change and fails after it (and was not itself changed by the task) is treated as a regression: the step is blocked, and the fix must restore the test by correcting the new code, not by changing the test.
- `AC-15` Engine data is stored locally under the bound project, and the shared, machine-independent parts (feature-history summaries, catalog, flow rules) sync to the project's configured Drive folder using the same mechanism as chat; machine-specific data (tooling status, indexes) stays local.
- `AC-16` A new source of context can be added and enabled per-flow *declaratively* (config/pack, not a code change to the context package): the context package composes an open set of typed sections rather than a fixed set of blocks. Every context source is deterministic (explicit lookup by key/reference — no similarity search), and when a source's backing system is unavailable it degrades to a warning rather than failing the step (`AC-9`, `AC-13`). External sources are limited to already-supported/connected integrations (e.g. MCP), never arbitrary command or path input.
- `AC-17` Step grounding is modeled as **typed artifacts** with three layers: a system-owned artifact **type** (a versioned contract; users cannot define custom types in v1), a user-authored **instance** (a reusable, project-scoped configuration validated against its type), and a step **binding** that attaches an instance to a step's input or output slot. A slot accepts only an instance whose type matches the slot; an incompatible binding, or a missing *required* one, fails at authoring or flow-load time with an actionable error (never as an opaque provider-turn failure), while a missing *optional* one degrades to a warning (`AC-9`). Artifact producers stay deterministic and their external backings stay limited to already-supported integrations (`AC-16`). At least one artifact type beyond context (e.g. a set of files) exists, so the mechanism is not context-specific. Existing context-source configurations (`AC-16`) remain valid and resolve through a compatibility fallback rather than a hard cutover.
- `AC-18` (Two-Tier Intelligence, CP-63): Code intelligence operates in two tiers: GitNexus governs macro-level architecture, blast radius, and 300 execution flows, while an embedded LSP client provides sub-200ms in-memory compiler diagnostics after each file write. Syntax and type errors are corrected before full test or build suites execute, saving token budgets.
- `AC-19` (Reproduce-First Defect Verification, CP-64): For bugfix workflows, the coder agent is physically barred from modifying production code until an executable test is authored that compiles cleanly and fails via assertion error (`r-reproduce`). A green test on arrival fails the reproduce gate; compile/syntax errors do not satisfy reproduction.
- `AC-20` (Living Knowledge Base, CP-66): Macro execution flows and symbol schemas are distilled into `.flowpilot/knowledge/` and exposed as a pluggable context source (`knowledge.flow`). Planning and scout nodes receive exact flow traces based on retrieval locus without reading raw source files. The knowledge base is updated incrementally at the flow audit stage.

## 7. Business Rules

- `BR-1` Authority order when documents disagree follows `SS-13 §8.3`: approved `System Spec` > `Tech Design` > `Coding Plan` > latest `Task` > latest `BugFix`. A test inherits the authority of the acceptance criterion it encodes.
- `BR-2` Downstream work may not silently redefine upstream intent; a needed change to behavior must update the upstream document first (`SS-13 §7.3`).
- `BR-3` Context storage is local-first on the developer's machine, with an optional remote mirror that enforces project-member access.
- `BR-4` Structured FlowPilot documents are an enrichment, never a precondition; the universal substrate is code plus git.
- `BR-5` A previously-green test that breaks is **always** a hard block (a high-confidence regression, not subject to staged rollout). Softer documentation rules (change-audit note, doc presence) may roll out warn→block per project.
- `BR-6` Enforcement is two-layered: bundled skills guide the AI (soft) and a runner-owned gate forces required outputs (hard); the runner is the source of truth and works across all providers.

## 8. Edge Cases

- `E-1` A bound repo in a language the indexer cannot parse to symbol level — context and checks degrade to file granularity, still functioning.
- `E-2` A repo with no specs, no tests, and a shallow git history — decision context is inferred from commits/comments and tagged low-confidence.
- `E-3` A very large repo — indexing is incremental and background; work is not blocked while indexing completes.
- `E-4` Generated or formatter-touched files producing apparent out-of-scope changes — excluded from regression flags via a per-project ignore set.
- `E-5` A flaky test — quarantined and reported; its assertions are never loosened to force a pass.
- `E-6` The spec really is wrong and the test really should change — allowed only through the upstream-update-then-confirm path (`AC-6`, `BR-2`).

## 9. Dependencies

- `SS-09` artifact memory & retrieval; `SS-02` project context; `SS-13` document contract.
- `SD-10` context resolver & RAG (Decision plane); `SD-17` the technical design for this spec.
- `CP-10` Part B (Plane A implementation); `CP-23` (shares the runtime loop and telemetry); `CP-32` (unit-test rule this spec formalizes).
- `CP-34` init/setup (skill + tooling install); `CP-31` existing-doc normalization; external tooling GitNexus and RTK.

## 10. Open Questions

- `Q-1` Resolved — regression enforcement (a previously-green test breaking) is **always on** (not opt-in); it is a high-confidence signal. Softer documentation rules (change-audit note, doc presence) may still roll out warn→block per project.
- `Q-2` Resolved (CP-63) — Two-Tier Intelligence model: GitNexus provides language-agnostic knowledge graphs and blast radius (Macro Plane), while language-specific Language Servers (gopls, vtsls, pyright, clangd, kotlin-language-server) supply real-time in-memory compiler diagnostics via standard JSON-RPC over stdio (Micro Plane).
- `Q-3` Resolved — when a test and its governing spec mismatch, the system raises the conflict and requires explicit user confirmation before either is changed; it never resolves the mismatch silently.
- `Q-4` Resolved — single project only for now: strict per-project isolation, no cross-project/organization-wide retrieval.

## 11. Definition of Done

- The structural and historical context planes are defined and traceable to acceptance criteria.
- `AC-1` through `AC-9` are each mapped to a verifiable mechanism in `SD-17` and a delivery slice in `CP-35`.
- A bound repo with no FlowPilot documents demonstrably yields structural context, change history, and a working regression check.
- The test-as-oracle rule is enforced end to end: no FlowPilot-driven flow can weaken a pre-existing test to pass.
- Per-step context-used and change-touched records are auditable.
