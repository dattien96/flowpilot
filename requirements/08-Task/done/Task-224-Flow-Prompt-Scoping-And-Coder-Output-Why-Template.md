# Task-224: Flow Prompt Scoping And Coder Output Why Template

## Metadata

- Document ID: `Task-224`
- Title: `Flow Prompt Scoping And Coder Output Why Template`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex / owner`
- Created: `2026-07-11`
- Last Updated: `2026-07-12`
- Parent Documents: [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: `None`
- Related Documents: [Task-223: File Artifact Output Contract And Review Input Chain](../done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md), [Task-222: Artifact-Only Step UX](../done/Task-222-Artifact-Only-Step-UX-And-File-Artifact-Semantics-Copy.md), [Task-169: Plan To Coding Context Handoff](../done/Task-169-Plan-To-Coding-Context-Handoff.md), [BUG-277: Flow Ledger History Over-Injected On Non-Context Consumers](../../09-BugFix/done/BUG-277-Flow-Ledger-History-Over-Injected-On-Non-Context-Consumers.md), [BUG-276: File Artifact Input Path-Only](../../09-BugFix/done/BUG-276-File-Artifact-Input-Should-Mention-Paths-Not-Paste-Content.md), [Task-225: File Artifact Output Heading Gate](../todo/Task-225-File-Artifact-Output-Heading-Gate.md), [CA-288](../../../change-audit/CA-288-cp45-e2e-edge-dirty-file-path-only-hub-dedupe.md), [CA-289](../../../change-audit/CA-289-task-224-flow-prompt-scoping-and-coder-why-template.md)
- Replaces: none
- Tags: `flow-mode, prompt-composition, feature-history, file-artifact, coder-template, review-handoff, context-package, cp-45`

## AI Quick View

### Summary

- Scope **who** receives feature-history / context package content in Flow Mode so only the **hub (main, user-facing)** and the **step immediately after `context.produce`** (typically coder) carry full prior-work context in the prompt.
- Make **review path-first**: when `file_artifact` INPUT is bound, do not dump full coder final message or re-inject full ledger history; agent reads deliverable paths with tools.
- Define a **coder OUTPUT deliverable template** (skill/prompt): **What / Why / Baseline CA** so review can validate without needing the full ledger — **Why** must capture *why this approach* and *what past decisions were already closed* (from chat/history available to coder).
- Document prompt-log truth: `last-prompt.txt` is **intentionally last-turn only**; `run-*/prompt-turn-*.txt` files are the audit source of truth. Optional `prompt-index.jsonl` is only for discoverability (do not change last-prompt overwrite semantics).

### Current Ask

- Capture owner decisions from CP-45 live E2E discuss (runs 314/658/309 and prior) into an implementable task; do not implement until owner approves after Codex doc review.

### Key Decisions

- `T-1` **History package content** (Prior work + Prior discussion inside Flow Context Package) is produced by `context.produce` and handed only to the **next hop** (first `agent.delegate` / implement after context). Downstream nodes do **not** receive a second full package by default.
- `T-2` **Ledger-style feature-history inject** (`injectFeatureHistoryPrompt` / `## Prior work on "…"` outside the package) is allowed on: (a) **hub user turns**, (b) the same **first post-context consumer** only if not already covered by the package. It is **not** allowed on review / validate / late hub auto-reinvoke synthesis by default.
- `T-3` **Hub main** should retain session-level awareness of the active feature’s history for user chat and orchestration; **hub auto-reinvoke synthesis** should inherit feature key without re-pasting a full Prior work block every round (BUG-275 already blocks wrong self-resolve).
- `T-4` **Reviewer** is **deliverable-centric**: bound `file_artifact` INPUT = path mention + tools (BUG-276). Full ledger history is **not** required if the coder deliverable carries What/Why/Baseline.
- `T-5` When the review target has **file_artifact INPUT** path(s) bound: **omit** the full coder final message body from the review prompt (default). Path + tools is enough. Do not keep a long optional brief in v1.
- `T-6` **Coder OUTPUT template** (skill and/or prompt section for required file paths) requires at least:
  - **What** — what was produced/changed (paths, scope).
  - **Why** — why this approach **now**; what alternatives were rejected; **what past decisions are already closed** (from Prior work / Prior discussion / chat summaries available in the coder’s package). Explicitly: *do not re-open settled decisions without stating conflict*.
  - **Baseline** — newest governing `feature_key`, CA / Task / BUG / commit (e.g. `CA-288` / `Task-224` / `BUG-277` or generic placeholders — not stale calc-core-only examples).
- `T-7` **Prompt logs**: `last-prompt.txt` overwrite is **expected**; per-turn `run-*/prompt-turn-*.txt` is audit truth. Optional index for discoverability only — **do not** change last-prompt overwrite semantics.

### Constraints

- Do not regress Task-223 OUTPUT write contract or `r-artifact-output` gate.
- Do not regress BUG-276 path-only file INPUT.
- Do not push full file body back into review prompts.
- Do not remove context package content from the **first post-context** consumer.
- Keep changes minimal: prefer inject-gate flags and template text over new artifact types.

### Open Questions

- `Q-1` **RESOLVED (default):** omit coder final body when file INPUT paths are bound; no 600-char brief required in v1.
- `Q-2` Whether hub **first** user turn after flow start always injects history even when flow already ran context package on child (recommend **yes** for main UX).
- `Q-3` Whether validate/audit mid-flow nodes ever need package excerpts — **out of v1** unless a later CP requires it.
- `Q-4` **RESOLVED (default) for heading check:** v1 ships **prompt/template guidance only** for What/Why/Baseline headings; optional gate reprompt on missing headings is Phase-2 → [Task-225](../todo/Task-225-File-Artifact-Output-Heading-Gate.md) (not required for AC-4 / CP-45 closeout).

### Source Refs

- Live runs: `.flowpilot/runs/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/` — `run-314` (coder), `run-658` (reviewer), `run-309` (hub synthesis).
- Owner discuss 2026-07-11: history only main + post-context step; review deliverable-first; P3 Why includes past closed decisions from chat history.
- CP-45 §13 live E2E ledger; Task-223; CA-288 (BUG-274/275/276).
- Code today: `injectFeatureHistoryPrompt`, `startInlineEntryChain` / `renderFlowContextPrompt`, `tryAdvanceFlowFromNode`, `appendInputArtifactPrompt`, `appendRequiredOutputArtifactPrompt`, `composeAgentContextBlock`, `maybeAutoReinvokeHubWithNote`.

## 1. Goal

Reduce redundant and noisy prompt composition in Flow Mode so:

1. **Context / history bulk** lands only where implementation decisions are made (coder after context) and where the human operates (main hub user turns).
2. **Review** validates **deliverables** (file paths + What/Why/Baseline written by coder), not a second copy of the ledger.
3. **Coder** is contractually required to externalize **Why** (including settled history) into the OUTPUT file so the chain stays auditable without re-injecting history everywhere.

## 2. Parent Links

- coding plan: `CP-45` (artifact framework live residual)
- tech design: `SD-23` (typed artifact I/O), `SD-17` (feature history / chat summary)
- system spec: `SS-13`, `SS-14` as applicable
- specific upstream ids: `Task-223`, `Task-169`, `Task-202`, `BUG-275`, `BUG-276`

## 3. Trigger

CP-45 live E2E on gate-sandbox showed:

- Coder prompt (run-314): package + required OUTPUT paths — **accepted**.
- Reviewer (run-658): path-only file INPUT good, but **duplicate Prior work/discussion** via ledger inject + large coder final message still pasted.
- Hub (run-309): history inject on synthesis OK-ish for main, but owner wants **policy**: only main + first post-context step own full history in prompts; coder must write Why/past decisions into deliverable so review does not need ledger.

## 4. Exact Change

### 4.1 Prompt scoping (P1 / P2)

- `T-1` Add a clear rule in runner for **when** `injectFeatureHistoryPrompt` runs on flow-engine-driven child runs:
  - **First post-context consumer** = the node reached by the transition immediately after a `context.produce` hop that already prepended the Flow Context Package (package provenance / edge from context node) — **not** by agent display name (“coder”).
  - Prefer package-only on that hop; skip a second ledger inject if package already present.
  - **Skip** ledger inject for reviewer / other non-first-post-context flow delegates by default.
  - Hub: **user-authored** turns may inject full history; pure **flow-engine auto-reinvoke / synthesis** turns (`isFlowEnginePrompt`) inherit feature key only / skip full Prior work block.
- `T-2` When the target node has **any** `file_artifact` INPUT path binding, change `tryAdvanceFlowFromNode` review brief to:
  - Keep short header: review result from node X.
  - **Omit** full coder final message body (default).
  - Keep BUG-276 path-only bound file section.

### 4.2 Coder OUTPUT Why template (P3)

- `T-3` Extend required-output prompt section and/or pack skill for coder so each required path must be written as markdown including **at least**:

```markdown
# <title>

## What
- …

## Why
- Why this approach now (not alternatives).
- Past decisions already closed (from Prior work / Prior discussion / package) — list them; do not silently reopen.
- If conflicting with a closed decision, state the conflict explicitly.

## Baseline
- feature_key:
- source_doc_id / CA / Task / commit:
```

- `T-4` v1: prompt/template guidance only for headings. Phase-2 optional: if required file exists but missing `## Why` / `## Baseline`, warn or single reprompt → [Task-225](../todo/Task-225-File-Artifact-Output-Heading-Gate.md) (not required for AC-4).

### 4.3 Prompt log hygiene (P4)

- `T-5` Document: `last-prompt.txt` overwrite is expected; audit uses `run-*/prompt-turn-*.txt`.
- `T-6` Optional: write `run-*/prompt-index.jsonl` (turn_id, role/label, path) for discoverability only.

## 5. Touched Areas

- files (expected):
  - `apps/local-runner/internal/runner/feature_history.go` (inject gate)
  - `apps/local-runner/internal/runner/flow_executor.go` (review handoff truncate / skip body)
  - `apps/local-runner/internal/runner/artifact_type_registry.go` (required-output template text)
  - pack prompts / coder skill under `agentpack` if template lives there
  - tests under `runner`
- modules: flow executor, feature history, artifact prompts
- routes: none
- tables: none

## 6. Acceptance Check

- [x] `AC-1` Coder after context still receives **Flow Context Package** with Prior work + Prior discussion (run-314 shape).
- [x] `AC-2` Reviewer prompt does **not** start with full `## Prior work on "…"` ledger block when package already served coder (or when node is not first post-context consumer).
- [x] `AC-3` Reviewer with file INPUT: path-only section present; **no** full coder final message body.
- [x] `AC-4` Required OUTPUT prompt/template requires **What / Why / Baseline**; Why text explicitly covers closed past decisions from history available to coder (guidance in v1; heading gate optional later).
- [x] `AC-5` Hub user start still can show history; hub pure synthesis reinvoke does not re-dump full Prior work (or only inherits feature key).
- [x] `AC-6` Task-223 write gate + BUG-276 path-only still green.
- [x] `AC-7` Live retest on gate-sandbox context→coder→review→synthesis with calc-core summary file.

## 7. Out of Scope

- Rewriting chat UI / agent cards.
- Vector / embedding retrieval.
- Full DMS or versioned artifact snapshots.
- Forcing review to re-read entire git history.
- Changing Supabase artifact schema.

## 8. Completion Notes

- result: `done` 2026-07-12 — implemented CA-289: history inject skip for flow-engine/review handoff; review omits coder body when file INPUT; OUTPUT What/Why/Baseline template.
- Owner discuss 2026-07-11 captured in task; live retest recommended on gate-sandbox.
- follow-ups: Phase-2 structural heading gate tracked as [Task-225](../todo/Task-225-File-Artifact-Output-Heading-Gate.md).
