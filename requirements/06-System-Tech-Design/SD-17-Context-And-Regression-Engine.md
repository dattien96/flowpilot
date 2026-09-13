# SD-17: Context And Regression Engine

## Metadata

- Document ID: `SD-17`
- Title: `Context And Regression Engine`
- Phase: `tech_design`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-24`
- Parent Documents: [SS-14: Code Context And Regression Safety](../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SS-02: Project Context](../05-System-Specs/SS-02-Project-Context.md), [SS-09: Artifact Memory Context Retrieval](../05-System-Specs/SS-09-Artifact-Memory-Context-Retrieval.md), [SS-13: AI-Followable Document Contract](../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: [CP-35: Context And Regression Engine Rollout](../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](./SD-20-Flow-Gate-Rule-Semantics.md), [SD-21: Change Contract And Canonical Intent Signature](./SD-21-Change-Contract-And-Canonical-Intent-Signature.md) (activates deferred `D-11`)
- Related Documents: [SD-10: Context Resolver & RAG](./SD-10-Context-Resolver-RAG.md), [SD-16: Agent Spawn And Tool-Calling Design](./SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [CP-10: Integrations, Memory & Context Intelligence](../07-Coding-Plan/inprogress/CP-10-Integrations-Hardening.md), [CP-34: Init Tool](../07-Coding-Plan/done/CP-34-Init-tool.md), [CP-31: Auto-Document Process](../07-Coding-Plan/done/CP-31-Auto-Document-Process.md), [CP-32: UnitTest Rule](../07-Coding-Plan/done/CP-32-UnitTest-Rule.md), [CP-23: Context Control & Wrong-Way Detection](../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md)
- Replaces: `None (extends SD-10)`
- Tags: `context, regression, commit-ledger, feature-resolver, flow-gate, skill-pack, gitnexus, tooling, local-runner`

## AI Quick View

### Summary

- FlowPilot's accuracy problem has two roots a single RAG cannot fix: the AI lacks whole-project structural context, and it lacks change history, so it conflicts with, duplicates, or deletes load-bearing prior work (regression).
- The engine serves context from three planes — Decision (SD-10 artifact memory), Structure (**GitNexus as a pluggable provider**), and Change (**a commit-indexed, feature-tagged, ordered history**) — and enforces the flow with a runner-owned gate.
- Plane C is deliberately cheap: it is built from `git log` + FlowPilot's own commit-message convention (`Task-`/`BUG-`/`CP-` ids) + `change-audit/` notes. No symbol graph or AST hashing is required for v1.
- A natural-language ask is mapped to the right feature by a **Feature Resolver** over a small **Feature Catalog** (lexical match + an LLM pick-from-list — no vector index needed), then the ordered history is a direct key lookup — newest entry is current truth.
- Enforcement is two-layered: a **bundled flow skill pack** makes the AI cooperate (soft), and a **runner Post-Step Flow Gate** forces required outputs — CA note, Task/BugFix doc, green tests (hard).
- The strongest regression signal is the **existing test suite itself**: a previously-green test that breaks (and was not changed by the task) is a hard regression — cheaper and more reliable than any code graph.
- Required external tooling (GitNexus, RTK, node, skill pack) is installed and health-checked through the init/setup tool (`CP-34`); the engine degrades gracefully to the available capability tier.
- Symbol-level graphs, AST-normalized hashing, and Change-Contract scope-drift are **deferred to an optional advanced phase**, used only where a provider (GitNexus) supplies them cheaply.

### Current Ask

- Settle a runtime architecture that gives the AI enough project context and forces a safe, auditable flow, built from cheap, mostly-existing signals (git, commit convention, change-audit, GitNexus), not a bespoke code-intelligence engine.

### Key Decisions

- `D-1` Context is served by three planes: A Decision (semantic), B Structure (provider), C Change (ordered commit history).
- `D-2` The engine operates only on the bound target project; never FlowPilot's own source. Every plane is per-project, multi-tenant, local-first.
- `D-3` Plane C is a commit-indexed, feature-tagged, **ordered** history (newest = current truth), derived from `git log` + commit-id convention + `change-audit/` notes — no symbol graph or hashing.
- `D-4` A natural-language ask resolves to a `feature_key` via a Feature Resolver over a small Feature Catalog using lexical match + an LLM pick-from-list (the catalog is small enough to show the model inline); no vector index is required. History retrieval is then a direct key lookup. Catalog embeddings are an optional later enhancement; pgvector stays reserved for Plane A memory (SD-10).
- `D-5` Plane B is **GitNexus as a pluggable structure provider** run against the bound repo; if absent, structure degrades to file/commit level. No bespoke indexer is built.
- `D-6` Flow enforcement is runner-owned: a **Post-Step Flow Gate** observes the AI's response, tool calls, git diff, and test result, and forces required artifacts. Provider-native hooks are an optional accelerator, not a dependency.
- `D-7` A bundled, provider-agnostic **flow skill pack** is auto-installed into each bound project (`.claude/.codex/.gemini`) as the soft enforcement layer; it includes a commit-message-format skill so commits stay machine-parseable for the change ledger (`D-3`).
- `D-8` Required tooling (GitNexus, RTK, node, skill pack) is installed and health-checked via the init/setup tool (`CP-34`); the engine reads a tooling registry and runs at the available capability tier.
- `D-9` The universal substrate is code + git; SS-13 structured docs are an enrichment, never a precondition.
- `D-10` The oracle rule (a failing test forces a spec/business re-check or a user question, and code/tests are never changed blindly to pass) is enforced by the Flow Gate + skill pack — deterministic and cheap (delivers `CP-32`).
- `D-11` Symbol-level graph, AST-normalized hashing, and Change-Contract scope-drift are **deferred (advanced/optional)** and used only where GitNexus supplies them cheaply, because validation showed they are noisy and costly relative to their v1 value.
- `D-12` The pre-existing test suite is the primary regression signal: a test that was green before the change and is red after it (and was not itself modified by the task) is a hard regression and **always** blocks the step (not subject to `gate_mode`) — distinct from a task-authored test that is not yet green. This needs no code graph. On a test/spec mismatch, the gate raises it for explicit user confirmation rather than resolving silently.

### Constraints

- Must layer on the SD-10 / CP-10 Part B context-resolver pipeline additively, not break it.
- Must reuse the `<target>/.flowpilot/` local convention (`CA-097`) and Supabase RLS-by-`project_id`.
- Must work on arbitrary repos with no specs and no FlowPilot history (code + git only).
- All enforcement must be runner-owned so it works across every provider (`SD-16`); skills and provider hooks are additive.
- Indexing, catalog build, and history build must be non-fatal and retryable; never block the raw work.

### Open Questions

- `Q-1` Feature Resolver disambiguation UX: auto-pick top match above a threshold, always confirm, or confirm only when scores are close?
- `Q-2` Flow Gate remediation: how many auto re-prompt attempts before escalating to the user, and which rules block vs. warn by default?
- `Q-3` GitNexus run trigger and cost on the bound repo: on bind, on demand, or scheduled — and how stale may its index be before the structure signal is suppressed?
- `Q-4` Resolved for v1 — start with lexical match + an LLM pick-from-list (no embeddings); add catalog embeddings only if the catalog grows large or matching proves weak.

### Source Refs

- `SS-14` AC-1..AC-13; `SS-02`, `SS-09`, `SS-13 §7.3/§8.3/§13`.
- `SD-10`/`CP-10 §3` Plane A; `SD-16` runner-owned orchestration; `CP-34` init/setup + skill install; `CP-31` doc normalization; `CP-32` unit-test rule; `CP-23` behavioral drift (distinct).
- External: GitNexus CLI/MCP (`npx gitnexus analyze`); RTK token proxy.
- Code refs: `internal/contextresolver/` (CP-10), `interactive_service.go`, `local_file_session_store.go` (`.flowpilot/` pattern), provider adapters (`claude_*`, `codex_adapter.go`).

## 1. Goal

Make FlowPilot-driven AI work correct and regression-safe on any bound project, using cheap and mostly-existing signals:

- give the AI the **ordered change history** of the feature it is touching (so it builds on prior work instead of undoing it),
- give it **structural context** from GitNexus when available,
- **force a safe flow** after the response (update the change-audit, create the Task/BugFix doc, keep tests green) through a runner gate plus a bundled skill pack,
- and make the required tooling installable and verifiable on the user's machine.

Symbol-level diffing and scope-drift are explicitly out of v1; they are an optional later enhancement.

## 2. Input Documents

- `SS-14` Code Context And Regression Safety — acceptance criteria (`AC-1`..`AC-13`).
- `SS-02`, `SS-09`, `SS-13` (§7.3 upstream triggers, §8.3 authority, §13 change-ledger block).
- `SD-10` / `CP-10` Part B — Plane A (artifact memory) this builds on.
- `SD-16` — runner-owned side effects and gates.
- `CP-34` init/setup + skill & tooling install; `CP-31` existing-doc normalization; `CP-32` unit-test rule.

## 3. Architecture Decision

### 3.1 `D-1`/`D-2` Three planes, target-scoped

| Plane | Answers | Source | Retrieval |
|---|---|---|---|
| A. Decision | "what was decided / why" | `artifact_memories` + SS-13 docs (SD-10) | semantic + ID |
| B. Structure | "who depends on this now" | **GitNexus provider** on the bound repo | graph query (best-effort) |
| C. Change | "what changed for this feature, in order" | **git + commit convention + change-audit** | feature_key → ordered list |

All planes are built from and stored against the bound target project; FlowPilot never uses its own source as context.

### 3.2 `D-3` Plane C: the commit-history ledger

Every commit carries its feature linkage **directly in the commit message** through FlowPilot's commit contract `[Type][feature][layer?] <description>`:

```text
[Feature][slash-commands][ui] add /s and /a chat commands Task-087
[BugFix][agents-panel]      stabilize agents panel BUG-130
[Refactor][skill-pack]      extract install logic CP-35
```

The parser reads the brackets in order: bracket 1 → `change_type`, **bracket 2 → `feature_key` (`confidence: high`)**, bracket 3 → optional `layer`. So `git log` yields, for every commit, the exact feature it belonged to — no inference. The `change-audit/CA-###` note (with the `SS-13 §13` `flowpilot:change-ledger` block) still supplies the human summary and can also carry `feature_key`. The ledger is the join of these, keyed by feature and **sorted by commit order, newest last**. No AST, no symbol identity, no hashing.

Putting the feature **in** the commit is what makes history precise. Without a feature bracket the engine must guess from file paths, which collapses an entire top-level directory (e.g. `domain/`) into one bucket spanning many unrelated features — too coarse to answer "what changed for *this* feature." The mandatory feature bracket prevents that.

Because the ledger depends on this format, the bundled `git-commit-format` skill (§6.4) enforces it at commit time — the commit message is a contract between that skill (producer) and the ledger parser (consumer), the same pattern as the `SS-13 §13` block for change-audit notes. Feature keys are kept stable by a single registry file (`change-audit/FEATURE-KEYS.md`, `SS-13 §13`); the `git-commit-format` skill requires the `[feature]` value to exist there (the `audit-logging` skill appends new keys when none fits), and the Feature Catalog (§3.3) seeds from it.

**Fallback chain when the feature bracket is absent** (legacy commits / non-FlowPilot history), in priority order: (0) feature bracket in the commit → high; (1) `feature_key` in a matching CA `§13` block → high; (2) `FEATURE-KEYS.md` keyword match → low; (3) dominant top-level changed path → low; (4) the source-doc id itself → low. Levels 2–4 are coarse by nature and flagged `confidence: low`.

**Sibling Change-plane entry — the per-feature chat-summary timeline** (`Task-161`, resolving `Q-1` toward Plane C). Beside `feature_history.ndjson`, a `ledger/chat_summary.ndjson` keeps a time-ordered, per-`feature_key` record of what was *discussed* in past chats (decisions, rejected approaches, preferences, blockers) that never became a commit. It stays deterministic and ordered like the commit ledger — only the summary *text* is AI-generated (by a cheap-tier model of the chat's own provider, heuristic fallback). [Task-163](../08-Task/done/Task-163-Chat-Summary-Generation-Triggers.md) defines *when* it is produced — an idle timer, a manual control, and a startup backfill scan — and stores it as one upsert row per `(run, feature)` keyed by a transcript-state hash. Both timelines inject side by side at the prompt-assembly seam (`§6.1`).

### 3.3 `D-4` Feature resolution from natural language

The user rarely supplies a `feature_key`. Resolution is a two-step:

1. **Catalog match** — the catalog is small (the project's own features), so match the NL ask lexically (keywords + file globs) against it (built from SS/CP titles + AI Quick View summaries + `change-audit` `feature_key`s + touched-file globs) → ranked candidate keys.
2. **Confirm if ambiguous** — above a confidence threshold, auto-select; otherwise pass the candidate list (key + one-line summary) to the model inline and let it pick, or present the candidates for the user to pick (`Q-1`).

No vector search is needed at this scale: the *entry* uses lexical match + LLM pick; the history itself is a direct, deterministic key lookup. Catalog embeddings are an optional flag for very large catalogs only.

### 3.4 `D-5` Plane B: GitNexus as a provider

GitNexus (`npx gitnexus analyze`) indexes any repo, so the runner uses it as a pluggable structure provider against the bound repo (impact, dependents, flows). A provider interface lets FlowPilot degrade cleanly: GitNexus present → real blast radius; absent → file-level neighbors + commit history only. No bespoke indexer is built.

### 3.5 `D-6`/`D-7`/`D-10` Two-layer enforcement

- **Soft (skills):** a bundled flow skill pack installed per project tells the AI to follow the flow (write the CA note, honor failing tests, cite sources). Voluntary but provider-agnostic.
- **Hard (runner gate):** the Post-Step Flow Gate observes the result after the turn and forces compliance regardless of whether the AI cooperated. The gate is the source of truth because the runner mediates every provider (`SD-16`); provider hooks are an optional accelerator.

### 3.6 `D-8` Tooling availability

The engine depends on external tools (GitNexus, RTK, node) and the skill pack. These are installed and health-checked by the init/setup tool (`CP-34`); the engine reads a tooling registry and selects a capability tier rather than failing when something is missing.

### 3.7 `D-11` Deferred: symbol-level scope-drift

Symbol graphs, AST-normalized hashing, and the Change-Contract scope-drift loop are recorded as an optional advanced phase. They are used only where GitNexus already provides the data cheaply, because validation showed bespoke versions are noisy (false drift on ripple/format/rename) and costly. v1 covers regression through history awareness + tests + GitNexus-when-present.

### 3.8 Architecture Diagram

#### Layers — three context planes

| Plane | Answers | Source | Slot | Status |
|---|---|---|---|---|
| **A — Decision** | "what was decided / why" | SD-10 `artifact_memories` + SS-13 docs | `decision.memory` (priority 0) | owned by SD-10/CP-10 |
| **B — Structure** | "who depends on this now" | GitNexus provider → `npx gitnexus impact` | `code.dependents` (priority 2) | Task-098 ✓ |
| **C — Change** | "what changed, in order — newest = truth" | `git log` + `[Type]:id` convention + CA `§13` | `feature.history` (priority 1) | Task-096 ✓ |

Feature key resolved from NL by **Feature Resolver** (lexical + LLM pick, Task-097 ✓) before history lookup. No vector DB.

#### Per-step flow

```mermaid
graph TD
    subgraph PLANES["Context Planes"]
        PA["A — Decision\n(SD-10 artifact_memories)"]
        PB["B — Structure\n(GitNexus · Task-098 ✓)"]
        PC["C — Change ✓\n(commit ledger · Task-096 ✓)"]
    end

    FR("Feature Resolver\nNL → feature_key\nTask-097 ✓")

    subgraph FLOW["Per-Step Flow  ·  interactive_service.go"]
        PRE["① PRE-STEP\nload A+B+C → pack to budget"]
        AI["② EXECUTE\nAI provider + skill pack"]
        GATE{"③ POST-STEP GATE\nobserve · evaluate rules\nTask-099 + Task-100 ✓"}
        REC["④ RECORD\nfeature_history + CA note §13"]
    end

    STOP(["⛔ step blocked\nuser must resolve"])

    PC --> FR
    FR --> PRE
    PA --> PRE
    PB --> PRE
    PRE --> AI
    AI --> GATE
    GATE -- "violation: reprompt ≤2" --> AI
    GATE -- "pass" --> REC
    GATE -- "violation: block" --> STOP

    subgraph STORE["Storage  ·  Task-103 ✓"]
        L[".flowpilot/ local-first"]
        D["Drive context-engine/\n(4 shared files)"]
    end

    REC --> L
    L -. "sync" .-> D
```

#### Flow Gate rules

| Rule | Trigger | Required output | Action | Always? |
|---|---|---|---|---|
| `r-ca` | `code_changed` | change-audit note | reprompt | gate_mode |
| `r-bug` | `bug_fixed` | BugFix doc | block | gate_mode |
| `r-tests` | `tests_failed` | tests green or explained | block | **yes** |
| `r-reg ★` | `regression_test_broke` | restore green (no weakening) | **block** | **always** |
| `r-dep` | `removed_referenced_code` | confirm / update callers | block | GitNexus only |

`r-reg` and `r-tests` are **always enforced** regardless of `gate_mode`. `gate_mode: warn` downgrades only `r-ca` and `r-bug`.

#### Storage split

```text
<target>/.flowpilot/
  ledger/feature_history.ndjson    ← synced to Drive
  ledger/chat_summary.ndjson       ← synced to Drive
  catalog/features.ndjson          ← synced to Drive
  settings/flow-rules.json         ← synced to Drive
  guard/test_baseline.json         ← local only (machine-specific)
  tooling.json                     ← local only (machine-specific)
  structure/                       ← local only (rebuildable index)

change-audit/FEATURE-KEYS.md       ← git-synced (repo file, not Drive)
```

#### Tooling + skill pack (Task-101 ✓, Task-102 ✓)

- **Skill pack** (5 skills) auto-installed to `.claude/.codex/.gemini` on bind: `git-commit-format`, `oracle-rule`, `audit-logging`, `phase-doc`, `context-discipline`.
- **Tooling registry** checks `gitnexus`, `rtk`, `node`, `skill_pack` → writes `tooling.json` → drives `CapabilityProfile` → engine degrades gracefully when a tool is absent.

## 4. Component Impact

**New runner modules:**

```text
internal/changeledger/     # Plane C: git-log parse + change-audit join, ordered history
internal/featurecatalog/   # catalog build + Feature Resolver (NL -> feature_key)
internal/structure/        # GitNexus provider adapter (+ file-level fallback)
internal/flowgate/         # Post-Step Flow Gate: observe -> evaluate rules -> force
internal/tooling/          # tooling availability registry + capability tier
```

**Extended:** `internal/contextresolver/` (new `feature.*` and `code.*` slots), `interactive_service.go` (pre: resolve; post: flow gate). **Owned by `CP-34`:** skill-pack installer + setup page. **Not built (deferred):** symbol code-graph, AST hashing, change-contract drift.

## 5. Data Model

Local-first under `<target>/.flowpilot/`; optional Supabase mirror under RLS-by-`project_id`. All small.

```text
feature_catalog        # feature_key, title, summary, keywords[], file_globs[], doc_refs[], embedding?
feature_history        # commit_hash, feature_key, source_doc_id, change_type, summary, committed_at, order_index
flow_rules             # id, scope(step|workflow|project), trigger, required_output, action(reprompt|block|approve)
tooling_status         # tool, version, status(ok|missing|stale), checked_at
capability_profile     # has_specs, has_gitnexus, has_tests, structure_tier, decision_tier, languages[]
```

- `feature_history` is derived from git and rebuilt incrementally; it is a cache, not a source of truth (git is).
- Plane A `artifact_memories` (SD-10) is unchanged.
- Notable: no `code_symbols` / `code_edges` / `symbol_memory_links` in v1 (deferred).

### 5.1 On-disk layout and Drive sync

Engine data follows the existing `<workspace>/.flowpilot/` convention (alongside `artifacts/`, `chats/`, `settings/`):

```text
<target>/.flowpilot/
  ledger/feature_history.ndjson    # Plane C
  catalog/features.ndjson          # Feature Catalog
  settings/flow-rules.json         # flow-gate config
  guard/test_baseline.json         # per-task baseline (ephemeral)
  guard/<runId>-report.json        # flow-gate report
  tooling.json                     # tooling status + capability profile (machine-specific)
  structure/                       # GitNexus index cache (rebuildable)
```

Drive sync reuses the chat-sync mechanism (`chat_session_sync.go`): the same per-project Drive folder selected for chat (`CP-33`), under a sibling `context-engine/` prefix, with the same NDJSON `_index` + `manifest.json` + SHA256 integrity and the same `ensureGoogleDriveFolderPath` / `upsertGoogleDriveFile` helpers.

```text
context-engine/
  ledger/feature_history.ndjson
  catalog/features.ndjson
  flow-rules.json
  _index/manifest.json
```

What syncs vs stays local:

- **Synced (project-shared):** `feature_history` enriched summaries, `feature_catalog`, `flow_rules` — team-shared and machine-independent.
- **Local-only:** `test_baseline`, flow-gate reports, `tooling_status` / `capability_profile`, GitNexus index — machine-specific or ephemeral; syncing them across machines would be wrong (e.g., GitNexus may be installed on one machine only).
- **Git is already the sync for raw history:** the commit ledger is rebuildable from `git log` on any machine, so Drive only carries the non-git-derivable enrichment (AI summaries) + config; git remains the source of truth.
- **The feature-key registry rides git, not Drive:** `change-audit/FEATURE-KEYS.md` is a committed repo file, so it travels with the repo (clone/pull) like the `CA-*` notes — it is **not** part of the `context-engine/` Drive sync. The `.flowpilot/catalog/features.ndjson` cache seeds from it and is Drive-synced.

## 6. Interfaces and Contracts

### 6.1 Resolver slots (extend CP-10 §3.4)

| Resolver | Source | Priority | Behavior |
|---|---|---|---|
| `feature.resolve` | Feature Catalog | 1 | NL ask → ranked `feature_key`(s); may request user confirm |
| `feature.history` | `feature_history` | 1 | Ordered commit summaries for a `feature_key`, newest last |
| `code.dependents` | Structure provider | 2 | Best-effort blast radius (GitNexus) or file-level fallback |

### 6.2 Feature history lookup

```text
getFeatureHistory("chat-ui")  →  (oldest → newest)
  { order:1, task:"Task-040", commit:"abc123", date:"2026-03", summary:"chat feed + continue flow" }
  { order:2, bug:"BUG-060",  commit:"def456", date:"2026-05", summary:"history replay fix" }
  { order:3, task:"Task-087", commit:"ghi789", date:"2026-06", summary:"/s and /a slash commands" }  ← current truth
```

### 6.3 Flow rule + gate

```json
{ "trigger": "code_changed", "required_output": "change_audit_note", "action": "reprompt" }
{ "trigger": "bug_fixed",    "required_output": "bugfix_doc",        "action": "block" }
{ "trigger": "tests_failed", "required_output": "tests_green_or_explained", "action": "block" }
{ "trigger": "regression_test_broke", "required_output": "restore_green_without_weakening", "action": "block" }  // always enforced (not gate_mode-gated)
{ "trigger": "removed_referenced_code", "required_output": "confirm_or_update_callers", "action": "block" }  // only when structure provider available (AC-4)
```

`FlowGate.evaluate(turnResult) → { pass | violations[] }`, where `turnResult` carries the final message, executed tool calls, `git status` diff, and test outcome. Violations route to `reprompt` (inject the missing requirement and continue), `block` (step cannot reach DONE), or `approve` (surface to user).

### 6.4 Skill pack manifest and tooling health

- Skill pack: a versioned set of provider-agnostic skill files (`git-commit-format`, `oracle-rule`, `audit-logging`, `phase-doc`, `context-discipline`) with install targets `.claude/skills/` and `.agents/skills/` (Codex + Gemini share `.agents/`). `git-commit-format` enforces the commit-message contract `[Type][feature][layer?] <desc>` (Type ∈ `Feature`/`BugFix`/`Refactor`/`Docs`/`Hotfix`/`Test`; feature ∈ `FEATURE-KEYS.md`; layer optional; source-doc id `Task-`/`BUG-`/`CP-` in the description) that the change ledger (§3.2) parses for an exact `feature_key`. It also enforces English, a ≤72-char first line, and no AI-authorship attribution (this consolidates the former standalone `git-commit` skill).
- Tooling health: `checkTool(name) → { status, version }` for `gitnexus`, `rtk`, `node`, `skill_pack`; consumed by `tooling_status` and the setup page (`CP-34`).

## 7. Execution Flow

### 7.1 Per-step loop

1. **Resolve (pre):** if the task references a feature by NL, run `feature.resolve` → `feature_key`; load `feature.history` (ordered) + Plane A memory + `code.dependents` (if GitNexus present); pack within budget (SD-10 §7).
2. **Execute:** run the provider step (skill pack already installed, biasing behavior).
3. **Flow Gate (post):** observe response + tool calls + git diff + tests; evaluate `flow_rules`; `reprompt`/`block`/`approve` on violation.
4. **Record:** append to `feature_history` (from the new commit) and ensure the `change-audit` note exists with the `SS-13 §13` block; write the SD-10 prompt-context audit row.

### 7.2 Regression suite + oracle rule (delivers `CP-32`)

Tests are split into two sets, captured cheaply (no code graph): **pre-existing** tests (present and green at task start / on the base commit) and **task-authored** tests (added or changed by the current task).

- A **pre-existing test that was green and is now red, and was not modified by the task** = a **regression**: the new code broke previously-working behavior. The gate hard-**blocks**, and the fix must restore green by correcting the new code — never by editing or deleting that test.
- A **task-authored test that is red** = "not done yet" (normal TDD); the AI may iterate.
- If a pre-existing test was modified in the same turn as code, surface it for human review (possible oracle tampering); do not auto-accept.
- When a test legitimately must change: if specs exist, re-check the governing acceptance criteria (`SS-13 §8.3` authority); if not, ask the user. If a test and its governing spec disagree, **raise the mismatch and require explicit user confirmation before changing either** — never resolve it silently. Code/tests are never changed blindly to go green.

### 7.3 Bind / setup flow

On project bind (via `CP-34`): install the skill pack into provider dirs; health-check tooling (`gitnexus`, `rtk`, `node`); run `CP-31` to normalize any existing docs toward SS-SD-CP; build the Feature Catalog and `feature_history` from git + change-audit; compute the capability profile. All non-fatal.

## 8. Failure and Edge Handling

- `F-1` GitNexus absent/stale → structure degrades to file-level + commit history; structure signal flagged low-confidence.
- `F-2` Feature Resolver ambiguous or wrong key → present candidates for user confirm; never silently pick a low-confidence key for a destructive action.
- `F-3` AI ignores a forced requirement → bounded re-prompt attempts, then escalate to the user (`Q-2`).
- `F-4` Required tool not installed → setup page flags it; engine runs at the lower capability tier.
- `F-5` No specs in the repo → oracle rule degrades to "refuse to weaken tests + ask the user".
- `F-6` Commit lacks an id (no convention) → fall back to file-path clustering for `feature_key`; mark low-confidence.

## 9. Security and Operational Concerns

- **Tenant isolation:** all rows keyed by `project_id`; RLS mirrors CP-10 §3.1; no cross-project history.
- **Target-project only context:** the engine only indexes the currently bound target project. If FlowPilot itself is intentionally bound as that target, it is allowed and remains isolated from other projects.
- **Test execution sandboxing:** the Flow Gate runs the bound repo's tests = running third-party code; this must be sandboxed/isolated and is a stated security concern.
- **Tooling install trust:** auto-installing GitNexus/RTK runs external installers; the setup tool must pin sources/versions and show what it runs.
- **Local vs remote:** `.flowpilot/` is the working source of truth and git-ignored; remote mirror is opt-in under RLS; provider tokens stay on the runner (`CP-10 §2.3`). Drive sync of shared engine data reuses the chat-sync path (§5.1); machine-specific data (tooling status, GitNexus index) is never synced.
- **Audit:** flow-gate decisions, forced remediations, and oracle classifications are persisted.

## 10. Risks and Trade-Offs

- `R-1` Feature Resolver picks the wrong feature → mitigated by confirm-on-ambiguity and showing the matched history for review.
- `R-2` Over-forcing (re-prompt loops, false blocks) annoys users → bounded retries, warn-before-block defaults, per-step rule config.
- `R-3` GitNexus run cost/staleness on large repos → on-demand/scheduled runs, suppress stale structure (`Q-3`).
- `R-4` Skill drift across providers → versioned skill pack, re-sync on bind, gate as backstop.
- `R-5` Deferring symbol-level means some regressions (subtle dynamic-call breakage) are caught only by tests → accepted for v1; tests are the safety net.

## 11. Validation Strategy

- **unit:** git-log id parsing → `feature_history` order; change-audit `§13` block parse + join; Feature Resolver ranking + ambiguity threshold; flow-rule evaluation per trigger; GitNexus-absent fallback; capability tier selection.
- **integration:** bind a repo with no specs → catalog + history build from git alone; NL ask "update the chat UI" → resolves to the chat feature and returns ordered history; code change without a CA note → gate reprompts then blocks; failing test → "make it pass" blocked, oracle path taken; GitNexus present → `code.dependents` returns real callers.
- **manual:** bind a third-party repo and bind FlowPilot itself as a project; confirm skill pack installed, tooling health shown, and context always comes only from the currently bound target; verify newest history entry is treated as current truth.

## 12. Traceability to Spec

- `SS-14 AC-1` (target-only, isolated) → `D-2`, §9. `AC-2` (works without our docs) → `D-3`, `D-9`, §7.3. `AC-5` (structural context when available) → `D-5`, §6.1. `AC-6` (oracle integrity) → `D-10`, §7.2. `AC-7` (auditable) → §7.1(4), §9. `AC-9` (non-fatal) → §7.3, §8.
- `SS-14 AC-10` (resolve feature from NL) → `D-4`, §3.3, §6.1–6.2.
- `SS-14 AC-11` (force required outputs) → `D-6`, §3.5, §6.3, §7.1(3).
- `SS-14 AC-12` (flow-aware skills auto-installed) → `D-7`, §3.5, §6.4, §7.3.
- `SS-14 AC-13` (tooling installed + health-checked, degrade) → `D-8`, §3.6, §6.4, §8.
- Pain #1 (whole-project context) → Plane B provider (`D-5`) + Plane C history (`D-3`).
- Pain #2 (regression) → ordered history awareness (`D-3`/`D-4`) + Flow Gate (`D-6`) + oracle rule (`D-10`).

## 13. Implementation Notes (Q&A Addendum)

Clarifications based on implementation review (CP-35, 2026-06-24). Use these to resolve the most common misunderstandings.

---

### 13.1 What local files are created when a project is bound?

Binding triggers `POST /client/projects/{id}/engine/init?trigger=bind` in the desktop. The runner calls `runEngineInit()` in `engine_setup.go`, which runs these sequential steps (skill install runs **before** the tooling check so the `skill_pack` sentinel exists when it is probed):

```
1. skillpack.Install()       → .claude/skills/*/SKILL.md
                             → .agents/skills/*/SKILL.md
2. tooling.CheckAll()        → .flowpilot/tooling.json
3. changeledger.Build()      → .flowpilot/ledger/feature_history.ndjson
                             → .flowpilot/ledger/.cursor
4. featurecatalog.Build()    → .flowpilot/catalog/features.ndjson
5. contextsync.NewEngineStore + WriteManifest + SyncSharedFiles
                             → .flowpilot/ledger/ (subdir created)
                             → .flowpilot/catalog/ (subdir created)
                             → .flowpilot/settings/ (subdir created)
                             → .flowpilot/guard/ (subdir created)
                             → .flowpilot/structure/ (subdir created)
                             → .flowpilot/manifest.json
                             → Drive context-engine/ (if connected)
Then:
   engine-init.json          → .flowpilot/engine-init.json
```

All steps are non-fatal. If the repo has no git history or no `change-audit/` notes, steps 3–4 still succeed with partial results.

---

### 13.2 What are the correct skill install directories?

Skills are installed into **two** roots, not three:

| Root | Used by |
|---|---|
| `.claude/skills/<skill>/SKILL.md` | Claude Code |
| `.agents/skills/<skill>/SKILL.md` | Codex AND Gemini |

There is **no `.gemini/` directory**. Both Codex and Gemini share `.agents/skills/`. The `providerStatuses` struct in `skillpack/install.go` maps codex → `.agents/skills` and gemini → `.agents/skills` for status checking.

---

### 13.3 What happens if the user deletes `.flowpilot/` contents?

`shouldSkipBindInit()` checks three conditions before skipping a bind-triggered re-init:
1. `.flowpilot/` directory exists
2. skill pack is current (version matches)
3. `engine-init.json` exists

If **any** child file or subdirectory is missing (e.g. user deletes `feature_history.ndjson`), `shouldSkipBindInit` still returns `true` (it does NOT verify individual child files exist). The workaround is to trigger a **manual re-init** from `Settings → Engine → Initialize / Re-sync Project`, which bypasses the skip gate (`trigger=manual`). The engine treats `.flowpilot/` as a cache — it is always safe to delete and rebuild.

---

### 13.4 How is `feature_history.ndjson` built?

`changeledger.Build()` calls `git log --no-merges` with an incremental cursor. For each commit:

1. `parseTags()` reads the `[Type][feature][layer?]` bracket run → `ChangeType` (bracket 1), `FeatureKey` + `confidence: high` (bracket 2, when present), `Layer` (bracket 3, optional).
2. Subject + body regex `(Task-\d+|BUG-\d+|CP-\d+[\w-]*)` → `SourceDocID`.
3. `EnrichAll()` runs the priority resolution (see §13.5) — but **skips entries whose feature was already set high-confidence by the commit bracket** (priority 0).
4. One NDJSON line is appended per commit, sorted ascending by `CommittedAt`. `OrderIndex` is assigned after sort (newest = highest index).
5. The newest commit hash is written to `.flowpilot/ledger/.cursor` for the next incremental run.

**This is pure code — no AI involved.** The AI only appears later in the feature resolver (§3.3, D-4) when resolving NL → `feature_key`.

Task-157 now consumes this resolver in the live runner prompt-assembly seam so the packed feature history block is actually prepended to the AI prompt.

---

### 13.5 How is `feature_key` extracted from commits?

Feature-key resolution applies five priority levels in order. The first that matches wins:

| Priority | Source | Confidence |
|---|---|---|
| 0 | **`[Type][feature][layer?]` commit bracket** — the feature declared directly in the commit message (set by `parseTags` in `parse.go`) | `high` |
| 1 | `change-audit/CA-*.md` scissor block — `feature_key:` field inside `# ---8<--- flowpilot:change-ledger` block, matched by `SourceDocID` | `high` |
| 2 | `change-audit/FEATURE-KEYS.md` — keyword match against the commit subject/body | `low` |
| 3 | `git show --name-only` — dominant top-level changed path → coarse directory bucket | `low` |
| 4 | `SourceDocID` itself used as the key | `low` |

**This is pure code — no AI.** Priority 0 is the intended path: the `git-commit-format` skill makes the AI put the feature in the commit, so extraction is exact. Priorities 2–4 are coarse legacy fallbacks (priority 3 buckets a whole directory like `domain/` into one key) and are flagged `confidence: low` — meaningful feature history depends on commits adopting the bracket contract (priority 0) or carrying CA notes (priority 1).

---

### 13.6 How is `features.ndjson` (the Feature Catalog) built?

`featurecatalog.Build()` assembles the catalog from three sources:

1. **`change-audit/FEATURE-KEYS.md`** (authoritative key list) — each line `- key — description` becomes a `Feature` entry with `title` and `keywords`.
2. **SS-*.md + CP-*.md docs** — H1 headings and AI Quick View summaries contribute `title`, `summary`, and `DocRefs`.
3. **Distinct `feature_key` values** from the ledger — commits that have a key but no matching SS/CP doc get a stub entry.

`FileGlobs` per key are aggregated from `git show --name-only` across all commits for that key.

The file is truncated and rewritten on every call to `Build()`.

---

### 13.7 Who writes and maintains `FEATURE-KEYS.md`?

`change-audit/FEATURE-KEYS.md` is the authoritative feature key registry. Ownership:

| Actor | Role |
|---|---|
| **Human (owner)** | Seeds the initial list with domain-representative keys |
| **AI (audit-logging skill)** | Appends new keys when no existing key fits a change; NEVER deletes or renames existing keys |
| **Code** | Never writes to this file; only reads it (enrich.go, catalog.go) |

The `audit-logging` skill (v3, CP-35) instructs the AI to:
1. Open `FEATURE-KEYS.md` and find the best matching key before creating a CA note.
2. If no key fits, append a new `- my-new-key — description` line first.
3. Create the CA note using that key in the scissor block.

The skill is auto-installed to `.claude/skills/audit-logging/SKILL.md` and `.agents/skills/audit-logging/SKILL.md` on every project bind. The `PackVersion` mechanism ensures re-installs happen when the skill content changes.

---

### 13.8 How does the Post-Step Flow Gate execute in the runner?

The gate runs in `interactive_service.go:runTurn()` via `gate_hook.go:runFlowGate()`.

**Exact call site** (after `finishTurn()` completes and `rs.turnInFlight = false`):

```go
// Post-turn flow gate (CP-35 P-4/P-5)
if completed {
    if s.runFlowGate(ctx, rs, turnID, fin) {
        completed = false
    }
}
// Finalizer hook runs only on clean completions.
if completed {
    _ = s.finalizer.Finalize(fin)
}
```

**`runFlowGate` steps:**
1. `ObserveGitDiff(cwd)` — `git status --porcelain` against workspace.
2. `LoadBaseline(dotFP)` — load `.flowpilot/guard/test_baseline.json`; if absent, `CaptureBaseline()` creates it.
3. `RunOracle(cwd, baseline, diff)` — detect regressions (pre-existing green test now red, not in diff) and oracle tampering (pre-existing test file modified).
4. Build `TurnResult` with `FinalMessage`, `GitDiff`, `Tests.Failed`.
5. `LoadRules(settings/)` — load `flow-rules.json` or fall back to `DefaultRules()`.
6. `Evaluate(tr, rules)` — check all rule triggers.
7. Append oracle tampering as a `warn` violation if detected.
8. `Enforce(violations, "warn")` — resolve final `Action` per rule and `gate_mode`.
9. Emit `EventFlowGateViolation` SSE event with the violation message.
10. Route: `"block"` → return `true` (step suppressed); `"reprompt"` → launch a follow-up turn via `startTurn()`, return `true`; `"warn"` → log only, return `false` (step completes).

The gate is provider-agnostic because it runs in the runner after every `finishTurn()`, regardless of which provider (Claude, Codex, Gemini) produced the turn.

---

### 13.9 What is the `repromptAttempts` counter for?

`interactiveRun.repromptAttempts` prevents infinite reprompt loops. When `Enforce()` returns `action: reprompt`, the gate increments this counter and only launches a new turn if `attempts < maxFlowGateReprompts` (= 2). On the third attempt, the gate still returns `block=true` (suppressing the completion) but does NOT launch another turn, leaving the step in a state where only the user can resolve it.

The counter resets with each new `interactiveRun` (i.e., each new run / step start).

---

### 13.10 `tooling.json` vs `engine-init.json` vs `manifest.json` — are they redundant?

No. These three files in `.flowpilot/` answer three different questions and never replace one another.

| File | Written by | Answers | Contents | Drive-synced? |
|---|---|---|---|---|
| `tooling.json` | `tooling.CheckAll()` | "What tools are installed on *this machine*?" | Array of `{tool, version, status, checked_at}` for `gitnexus`, `rtk`, `node`, `skill_pack` | ❌ machine-local |
| `engine-init.json` | `saveEngineInitState()` | "What happened the *last time* init ran?" | `{trigger, status, skipped, skipReason, attemptedAt, completedAt, workingDirectory, install summary, steps[]}` — an audit record of each init step's outcome | ❌ run log (local) |
| `manifest.json` | `contextsync.WriteManifest()` | "What is the *integrity/state* of the shared data files?" | Array of `{path, sha256, size_bytes}` for shared files (`feature_history.ndjson`, `chat_summary.ndjson`, `features.ndjson`, `flow-rules.json`) | used as the Drive `_index` reference |

**Distinct roles:**

- **`tooling.json` — capability snapshot.** The desktop Engine page reads it to show which tools are present/missing and to compute the `CapabilityProfile` tier. It is about the *environment*. The "Refresh Tooling" action rewrites it independently of a full init, which is why it is a separate file from `engine-init.json`.
- **`engine-init.json` — last-run log.** Records that init was triggered (`bind`/`manual`), whether it was skipped, and the per-step results (`skillpack_install`, `tooling_check`, `changeledger_build`, `featurecatalog_build`, `contextsync_manifest`). `shouldSkipBindInit()` uses its existence as one of the three skip-gate conditions (§13.3), and the UI shows it as "Last Init". It is about the *process*.
- **`manifest.json` — content fingerprint.** sha256 + size of the actual shared data files so a Drive/cross-machine sync can detect changes and verify integrity. It is about the *data*.

**The only overlap** is conceptual: `engine-init.json`'s `steps[]` records that the `contextsync_manifest` step ran, and `manifest.json` is the artifact that step produced. One says "the manifest step ran OK"; the other *is* the manifest with the hashes. Neither replaces the other. The single theoretical merge candidate is folding `tooling.json` into `engine-init.json` (both local) — but they are deliberately separate because tooling is refreshed on its own button without a full init.

### 13.11 Q&A for key-feature in Task-157
#### 1. Who created feature-key ?
(1) - The AI declares the key — by writing the [Type][feature][layer] bracket in the commit subject (the **git-commit-format** skill tells it to). The feature bracket is the AI's declaration of "this commit belongs to feature X." parse.go reads bracket‑2 as the candidate key.
(2) - A human (or the AI, explicitly) creates the canonical key — by adding a line to **change-audit/FEATURE-KEYS.md**. That committed git file is the registry of valid keys

#### 2. How will one key was selected?
(1) After done feature/task -> AI commit and provide the feature-key in the [Type][feature][layer] bracket
(2) Our Runner Golang got that - But we need to validate cause AI can gen wrong
(3) enrich.go checks it by see the file **change-audit/FEATURE-KEYS.md**. because this file is our trust file. keys in here are correct keys (User can append key here yourself)
  - (3.1) If the key AI gen is exist in this file -> Good job, set **Confidence=high** and ok with that key
  - (3.2) If it was not exist in this file (OR AI actually DID not created the key, missing bracket) -> Will not reject but will not trust rightaway -> set **Confidence=low**
  - (3.3) GO to next step if Confidence=low: **The r-fk gate (commit_feature_key_missing)**
  -> this one will be triggerred if 1 key created by AI treated as low confident

    (3.3.1) Our code used **SuggestKey(changedPaths, message)**
    This one is our func, based on the message and the files actually changed
    -> got a list of candidate keys (but SuggestKey only ranks existing registry keys by how well the files you touched match; it never mints a new one)
    (3.3.2) Reprompt to Ai: something like this: Your key is wrong, can you see my candidate keys, you can pick suitable keys from it if has
    (3.3.4) Ai will continue work -> pick suitable key if has and re-commit or ( or registers a new one in FEATURE-KEYS.md)

  - (3.4) If all above failed -> the latest fallback is
  enrich.go walks CA‑note §13 block → keyword match → dominant changed path → source‑doc‑id → unknown, all stamped Confidence=low
  Based on our doc to gen a new key (just fallback, we can not make suare this key is correct)

  * (Q2.1): How SuggestKey work, what is the message it received?
``Signature: SuggestKey(changedPaths []string, message string, catalog *Catalog) []Candidate``
changedPaths = changedPathsFromDiff(diff)
message = strings.Join(commitSubjects, "\n") -> that mean get commit mes from what Ai commit

Worked example (your sandbox)
A turn commits [Feature][calculator][logic] add abs touching calc.go:

- Text: calc-core's keywords are [calc, core, arithmetic, operations]; the token calculator doesn't equal calc, the key calc-core isn't in the subject → ~0 text points.
- Path: calc-core's glob calc.go prefix-matches the changed calc.go → +6.0. So calc-core surfaces purely from the file you touched, even though you typed the wrong bracket calculator. That calc-core suggestion is what the gate puts in the reprompt.

#### 3. Does it work with bug too, or only Taks?
Both — the organizing unit is feature_key, not the change type. The commit contract is [Type][feature][layer]; the second bracket is the feature, and Type can be Feature, Bugfix, Refactor, etc. So a bug‑fix commit [Bugfix][calc-core][logic] fix divide overflow files under feature_key = calc-core exactly like a feature commit.

#### 4. When these improved data injected to the prompt?
Basically now we have
- Q1: what feature, and what changed for it? -> Handled in Task-157 by feature/commit history
- Q2: Why changed it? Handled in Task-157 by the ledger + CA. Ca file always exist cause we have gate r-ca
- Q3: what was discussed about this feature in past chats ? Handled in Task-161 with per-feature chat-discussion history

For our prompt now we have injectSkillContent on every chat turn if i selected skills ?
So how about my new data above? When do we inject it?

In the **live chat turn** ([interactive_service.go:1638‑1664](apps/local-runner/internal/runner/interactive_service.go:1638)), the prompt is assembled in this order:

```
providerPrompt = in.Prompt                       // raw user text
providerPrompt = prependModePrefix(...)          // Task/Bug mode banner (first turn)
rebuildLedgerIfDirty(...)                         // refresh ledger from new commits
providerPrompt = injectFeatureHistoryPrompt(...)  // ← **YOUR new data (Q1+Q2+Q3)**
req := TurnRequest{ Prompt: providerPrompt, SelectedSkills: in.SelectedSkills }
```

-> So the answer is **every chat turn** -> BUT **But Q1/Q2/Q3 data only appear when a feature resolves**
`injectFeatureHistoryPrompt` ([feature_history.go](apps/local-runner/internal/runner/feature_history.go)) is *called* every turn, but it returns the prompt **unchanged** unless:
1. catalog + ledger load, **and**
2. `ResolveFeature(userPrompt)` returns a top candidate with **score ≥ 5.0** (a confident feature match), **and**
3. `HistorySlot` is non‑empty.

If the turn doesn't resolve to a feature (e.g. "write a haiku"), nothing is injected. When it does resolve, both blocks go in together:

#### 5. If always run, does it affected the app performance ?
Let's say i have many prompts in 1 chat session and each one need to be gone through these thing?

It's OK. No network, no LLM call at injection time
Each turn `injectFeatureHistoryPrompt` does only:
- read 3 small local NDJSON files (`features.ndjson`, `feature_history.ndjson`, `chat_summary.ndjson`),
- in‑memory lexical scoring (`ResolveFeature` = tokenize + `strings.Contains`, O(features × tokens)),
- filter+render the matching feature's entries.

**The one real cost is the write side, and it's currently synchronous.**
At turn *end*, `recordChatSummaryIfNeeded` calls the cheap **Haiku/Gpt5.4mini/...** summarizer ([interactive_service.go:1734](apps/local-runner/internal/runner/interactive_service.go:1734)), and that runs **on the finalize path, blocking** (up to a 60s timeout). The `state_key` cache skips it when the transcript hasn't changed — but a normal turn *does* change the transcript, so in practice each feature‑resolving turn pays one Haiku round‑trip (~1–3s) at completion. The user already has the assistant's answer by then, but it delays "turn fully settled."

   → If that latency matters, the clean fix is to run `recordChatSummaryIfNeeded` in a goroutine (it's already best‑effort/non‑fatal, so fire‑and‑forget is safe). The read/injection path needs no change. **Want me to make the summary recording non‑blocking?**
  
-> Yes, need this update

### 13.12 Q&A for Task-161 - summary chat logic
#### 1. When ?
 in the turn-completion path, after the flow gate passes and only when completed == true
So: once per successfully-completed chat turn ("rolling"). A failed/interrupted/gate-blocked turn does not summarize.

#### 2. What ?
recordChatSummaryIfNeeded (chat_summary.go) does two things:

- Synchronously snapshots the just-finished transcript into an owned chatSummaryJob (so the live run is never read again).
- go s.runChatSummaryJob(job) — everything expensive **runs in a bg goroutine - Dont block app**.

The actual model call lives in runChatSummaryJob → summarizeChatTurns → Runner.SummarizeChatTranscript (one-shot claude --print / codex exec / gemini).


- Inside runChatSummaryJob, in order:

 + LoadCatalog fails → return
 + ResolveFeature(latestPrompt) fails → return
 + TopCandidate(≥ 5.0) not confident → return (unknown feature is never recorded — avoids noise)
 + NewChatSummaryLedger fails → return
 + state_key cache check (dedup, below) → maybe return
 + summarizeChatTurns empty → return
 + Append fails → return
 + best-effort Drive sync — error swallowed

#### 3. Error handling? — three layers, all non-fatal
- The model call (SummarizeChatTranscript): returns an error on no-connected-account, missing CLI, non-zero exit, 60s timeout (context.WithTimeout), or empty output.
- Fallback: **summarizeChatTurns** catches that error and falls back to **heuristicSummarizeTurns** — a deterministic keyword summary. So even total model failure still records something useful.
- The whole job: every step returns silently on error. Because it runs after the turn already completed, in a goroutine, nothing here can ever block, fail, or corrupt the user's turn. Worst case: no summary line is added this turn.

**(Q3.1) What is **heuristicSummarizeTurns** — a deterministic keyword summary? explain how it worked?**

It's a pure string-matching summarizer — no model, no semantics, same input → same output, instant and offline. It's the fallback floor when the cheap model is unavailable. Algorithm ([chat_summary.go](apps/local-runner/internal/runner/chat_summary.go)):

**Step 1 — Goal bullet.** Take the first turn with non-empty user text, compact it to ≤180 chars → `"Goal: <first user message>"`.

**Step 2 — one classified bullet per turn.** For each turn, look at the assistant text (or the user text if no assistant), lowercase it, and classify by the **first** keyword group it contains:

| Trigger words in the text | Bullet label |
|---|---|
| `blocked`, `can't`, `cannot` | `Blocked: …` |
| `prefer`, `should`, `want`, `need` | `Preference/decision: …` |
| `decided`, `use`, `keep`, `switch` | `Decision: …` |
| contains `?` | `Open question: …` |
| (none of the above) | the text itself, compacted |

**Step 3 — cap & format.** Stop at 5 bullets total; each gets a trailing `.`, prefixed `- `, joined by newlines, whole thing truncated to 900 bytes (UTF-8-safe). `compactTranscriptText` also collapses whitespace and escapes `</previous_conversation>`.

So a chat like *"add divide" → "Added Divide; division by zero returns 0"* becomes:
```
- Goal: add divide to calc.
- Decision: Added Divide; division by zero returns 0. (matched "use"/"decided"? → here "returns" no; "Divide"… actually plain unless a keyword hits)
```

**Honest limitation:** it's crude — first-match keyword labeling, and triggers like `use` are very loose (lots of sentences contain "use" → mislabeled "Decision"). It's a *floor* that guarantees a non-empty, deterministic summary, not a quality one. That's exactly why we added the real cheap-model path on top.

#### 4. Edge cases handled

##### 4.1 Workflow run (not runKind=="chat") ?
Not supported now. That mean this one only in chat mode
Consider to continue when do flow mode -Todo

##### 4.2 Empty workspaceCwd / 0 reconstructed turns
Current code: Skip recording (no summary line written)

Empty workspaceCwd -> the run isn't bound to a workspace directory, so there's no .flowpilot/ to read the catalog/ledger from or write chat_summary.ndjson to

0 reconstructed turns -> transcriptTurnsFromRun found no usable User/Assistant content in rs.events (e.g. a turn that errored before producing anything, or only system/tool events). Nothing to summarize.

##### 4.3 Prompt doesn't resolve to a feature (score < 5)
Current code: skipped, not recorded

`ResolveFeature` scores catalog features against the latest user prompt; `TopCandidate(5.0)` requires the top score ≥ 5.0 (e.g. the feature key/title appears in the prompt = +5, or two keyword hits = +3 each). A generic chat ("write a haiku") clears nothing → dropped. 

**Why skip:** filing a vague chat under a guessed feature would pollute that feature's discussion timeline with **noise** — **better to record nothing**.

-> DO it if it correct. Dont do if it is in correct, it is better than do 1 incorrect -> wrong context

##### 4.4 No catalog/ledger on disk yet
Current code: skipped

LoadCatalog/NewChatSummaryLedger returns an error (engine not yet built / brand-new project). Without a catalog you can't resolve a feature anyway. Silent skip.

##### 4.5 Model fails / no account / CLI missing
Current code: heuristic fallback

`SummarizeChatTranscript` errors (no connected account for the provider and no `ANTHROPIC_API_KEY`; binary not on PATH; non-zero exit from rate-limit/auth; 60s timeout; empty output). This does **not** skip — `summarizeChatTurns` falls back to `heuristicSummarizeTurns`, which produces a deterministic summary (we already have ≥1 turn here). So a summary line **is** written, just from the heuristic instead of the model.

##### 4.6 Codex- or Gemini-only user
Current code: uses that provider's model

##### 4.7 Transcript > 16 KB
Current code: truncated before the call (summarizerMaxInputBytes)

##### 4.8 Model output > 4 KB
Current code: clipped, then normalizeSummaryBullets caps to ≤ 5 bullets / 900 B

#### 5. Duplicate-call handling
This is example row in **chat_summary.ndjson**
``
{
"**run_id**":"run-seed-002",
"**turn_id**":"turn-1",
"**feature_key**":"calc-core",
"**state_key**":"run-seed-002:seed-b",
"**summary**":"- Decision: Divide returns 0 on a zero divisor and does not propagate an error yet.\n- Open question: whether to add a separate error-returning DivideChecked helper later.\n- Blocked: no decision yet on overflow handling for very large products in Multiply",
"**created_at**":"2026-06-26T09:15:00Z"}
``

The state_key = runID + ":" + <concatenated transcript text> (chat_summary.go:178). 

Before generating, it checks the newest stored entry for (feature_key, run_id):
if existing[len(existing)-1].StateKey == stateKey { return }  // skip, no model call

**(Q5.1) How it worked**?
`state_key = runID + ":" + latestTranscriptState(turns)`, where `latestTranscriptState` joins every turn as `User\nAssistant`, separated by `\n---\n`.

**Example** — run `run-42`, two turns:

```
turn 1  User: add divide to calc
        Assistant: Added Divide with a zero guard.
turn 2  User: what about overflow?
        Assistant: Multiply can overflow; not handled yet.
```

→
```
state_key = "run-42:add divide to calc
Added Divide with a zero guard.
---
what about overflow?
Multiply can overflow; not handled yet."
```

It's a **content fingerprint of the whole run's transcript**, prefixed by the run id.

**How dedup uses it** ([chat_summary.go:91](apps/local-runner/internal/runner/chat_summary.go:91)): before generating, it reads the newest stored summary for `(feature_key, run_id)` and compares its `StateKey` to the freshly-computed one:

- **Cancel → reopen, no new turn:** transcript text is identical → same `state_key` → **match → skip** (no model call, no duplicate line).
- **You send another prompt:** turn 3 appends new text → the concatenated string differs → different `state_key` → **no match → generate + append** a refreshed summary.
- **Retry/edit of a turn:** changes the text → new `state_key` → refreshes (this is why it hashes the *content*, not just the turn count or last message).

The same key is also what the handoff (`loadHandoffSummary`) uses to pick only a summary that matches the *current* transcript state — so a stale summary from before the latest turn is ignored.

**(Q5.2) You concat the raw text to key - Must you hash** -> Fixed
The hash changes **iff the transcript content changes**, so:

| Action                       | New summary generated?                                    |
| ------------------------------| -----------------------------------------------------------|
| Continue (new prompt)        | **Yes** — new turn → new hash                             |
| Try again, different answer  | **Yes** — content differs → new hash                      |
| Try again, identical answer  | No — same hash → deduped                                  |
| Cancel → reopen, no new turn | No — same hash → deduped                                  |
| Just viewing an old chat     | No — doesn't even trigger (only fires on turn completion) |

**(Q5.3) What if i send some prompt like: continue, try again,**
NOTE: prompt to try again, not any retry button ui.

Because when AI got error i can prompt to trigger it again
But these ones are un-useful prompt in term of comparing logic?


**With current code, yes it can be duplicate about the logic part - But never in the real usecase**

ok we can say that, when we chat these ones -> the state key is different hash -> trigger summary flow again. but inorder to chat these ones, THE PREVIOUS AI STATE must be error/cancel right (except 1 case, it's done but you still want other res -> you prompt try again -> gen re-trigger summary is VALID NOW)

ok so if the previous turn is error/cancel -> violate our check: ONLY SUMMARY WHEN STATE = COMPLETE
as well as we have logic TopCandidate(5.0) -> no Ai response so this fun ABSOLUTELY < 0.5 related score -> SKIP too

so 2 layers report FAILED -> can not duplicat here

**But it is actually a bug in logic part, need to handle**
Because in normal chat, you can prompt un-benefit prompt (hi claude, are you handsome ???) -> The code based on that latest prompt and failed TopCandidate(5.0) too

```
Turn 1  "add divide to calc-core"   → resolves calc-core ✓  (summary recorded, history injected)
Turn 2  "try again"                 → resolves nothing ✗  (no summary, AND no Prior-work/Prior-discussion injected this turn)
Turn 3  "continue"                  → resolves nothing ✗  (same)
```
Can you see even in same chat, in the turn 3, we lost the context of the feature now

So the fix is: DO NOT BINDLY based on latest prompt if it is not benefit
In above case we need : "try again" / "continue" → **falls back to turn 1's "add divide to calc-core" → still resolves calc-core.**
###### The fix: resolve from the newest *substantive* prompt

Instead of "use the last user message," scan user turns **newest → oldest** and use the first one that clears the threshold. So:

- "try again" / "continue" → falls back to turn 1's `"add divide to calc-core"` → still resolves **calc-core**.
- A genuine pivot ("now let's do chat-ui") → that prompt resolves on its own, so pivots still work (better than hard run-stickiness, which would cling to the old feature).

I'd apply it in one shared helper used by both the recorder and the injector, so they stay consistent. Roughly:

```go
func resolveTurnsFeature(turns []transcriptTurn, catalog *Catalog) (Candidate, bool) {
    for i := len(turns)-1; i >= 0; i-- {        // newest substantive prompt first
        if u := strings.TrimSpace(turns[i].User); u != "" {
            if c, ok := TopCandidate(ResolveFeature(u, catalog), 5.0); ok {
                return c, true
            }
        }
    }
    return Candidate{}, false
}
```

(Injection-side it'd scan the live transcript rather than just the current prompt.)

Net effect: continuation/retry turns inherit the conversation's established feature, so they keep getting history injected and contribute to the summary — while a real topic change still re-resolves.

> **Implemented refinement (CP-37 Test E4 — "Continuations-only").** The fallback is gated to **explicit continuation phrases only** (`isContinuationPrompt`: `continue` / `try again` / `do it` / `go on` / `proceed` / …), *not* to every prompt that fails `TopCandidate(5.0)`. A greeting or bare acknowledgement (`hi`, `ok`, `yes`, `no`) does **not** inherit — it injects nothing and is dropped from bucketing — so social/unrelated chatter (the `"hi claude, are you handsome???"` case above) never drags in stale feature context. There is deliberately **no blind word-count fallback**. Gate reprompts and cross-provider handoff envelopes are handled the same way: system prompts whose process-describing text (it names feature keys, embeds prior conversation) must never drive resolution — they inherit the established feature instead, and the handoff's first turn is seeded with the source feature resolved from the clean source transcript.

#### 6. Hit-limit / rate-limit case
This is fully covered, and it's the important one:

- If the cheap model is **rate-limited or out of quota**, `cmd.Run()` returns non-zero → error → **heuristic fallback** records a deterministic summary instead. The switch/chat is never blocked.
- A **hung** call is bounded by the 60 s timeout, then falls back the same way.
- Caveat from the provider-following design: the summarizer now runs on **your chat's own provider**. If *that* provider is the one being rate-limited (often exactly why you're about to switch), the summary call fails too — but the heuristic floor still produces a summary, and since it's async and post-turn, you never feel it. The summary does count against your provider quota (it's a real cheap call), which is the cost of getting a real AI summary instead of the heuristic.

#### 7. Performance with many prompts

For a chat where N turns each resolve to a feature:

- **Per turn:** one cheap-tier model call, **async** — it never delays your next prompt. The read/injection side (each turn's "Prior discussion" block) is local file reads + bounded to the most-recent **3** entries.
- **Cost:** N cheap-model calls accumulate over a long session (each on your provider's *mini/haiku/flash* tier). The state_key cache skips only *unchanged* turns, and a real turn always changes the transcript — so in practice **every content-changing turn does make one call.** That's the real cost to be aware of.
- **File growth:** `chat_summary.ndjson` gains one line per content-changing turn for that run. Injection stays bounded (last 3), but the file grows linearly and `GetFeatureSummaries` re-reads the whole file each turn. Modest, but it does grow — and there's mild redundancy (each line is a fuller summary of the same growing run, only the last 3 shown).

Ok i see:
* every content-changing turn does make one call
* File growth: chat_summary.ndjson
* in 1 chat - Do 2 features then the summary will be mix

We've nailed the three real costs. Let me separate them, because two have clean fixes and one is a genuine product tradeoff.

##### Root cause (one design choice drives all three)

Today: **every completed turn re-summarizes the whole transcript and *appends* a new line.** That single decision causes all three symptoms:

| Issue | Cause | Fix |
|---|---|---|
| **2. File growth** | append-per-turn → N lines per chat | **Upsert by `(run_id, feature_key)`** — one rolling line per chat-session per feature, rewritten in place. The timeline still shows the last 3 *chats*, which is the actual intent ("prior discussions"), not the last 3 turns. |
| **3. Feature mixing** | the summary is fed the *entire* transcript, so a `chat-ui` turn's summary includes earlier `calc-core` discussion | **Bucket turns by feature** (resolve each turn; explicit *continuation* turns attach to the running feature, while greetings/acks/off-topic are dropped) and summarize **only the current feature's turns**. `chat-ui`'s summary then contains only `chat-ui`. |
| **1. One model call per turn** | "rolling" = refresh on every change | *Tradeoff — your call below.* |

Fixes #2 and #3 are unambiguous correctness wins — I'll implement those regardless (upsert + per-feature bucketing, with the `state_key` then keyed to the *feature's* turns so a `chat-ui` turn doesn't needlessly refresh `calc-core`).

##### Issue #1 is the real tradeoff: how often to call the model

It's "rolling" precisely so a summary already exists when you switch providers mid-chat (Task-162's zero-budget case). Calling less often saves tokens but the summary can lag. Since the handoff degrades gracefully (`hybrid` → `target_summary` → `raw`) and the discussion block only needs a *recent-enough* summary, all three options below are safe — they differ in freshness vs. cost:


##### New refactor: 
When to call gen summary ?
Now after each response returned -> NO
Updated
- (1) When chat idle after complete for a window (5 mins). If it ran through 2 mins and you continue to chat -> counter reset from 0. count again
- (2) When user ACTIVELY press the button Gen summary -> 
  + design this button in the chat controller. Some where near the YOLO area. If the chat is running -> disable. Only enable when it is in completed state
  + If press this button but we have summary hash stored before -> ignore
  + If press this one before 5 mins count -> ingore 5mins count logic
- (3) When start server, 1 bg job to scan all chat that dont have summary in local file OR the hash do not match with latest response

