# CP-35: Context And Regression Engine Rollout

## Metadata

- Document ID: `CP-35`
- Title: `Context And Regression Engine Rollout`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-096: Commit-History Ledger](../../08-Task/todo/Task-096-Commit-History-Ledger.md) (P-1), [Task-097: Feature Catalog And Resolver](../../08-Task/todo/Task-097-Feature-Catalog-And-Resolver.md) (P-2), [Task-098: GitNexus Structure Provider](../../08-Task/todo/Task-098-GitNexus-Structure-Provider.md) (P-3), [Task-099: Post-Step Flow Gate](../../08-Task/todo/Task-099-Post-Step-Flow-Gate.md) (P-4), [Task-100: Regression Suite And Oracle Rule](../../08-Task/todo/Task-100-Regression-Suite-And-Oracle-Rule.md) (P-5), [Task-101: Flow Skill Pack Install](../../08-Task/todo/Task-101-Flow-Skill-Pack-Install.md) (P-6), [Task-102: Tooling Check And Capability Profile](../../08-Task/todo/Task-102-Tooling-Check-And-Capability-Profile.md) (P-7), [Task-103: Engine Local Store And Drive Sync](../../08-Task/todo/Task-103-Engine-Local-Store-And-Drive-Sync.md) (P-8)
- Related Documents: [CP-10: Integrations, Memory & Context Intelligence](./CP-10-Integrations-Hardening.md), [CP-34: Init Tool](../done/CP-34-Init-tool.md), [CP-31: Auto-Document Process](../done/CP-31-Auto-Document-Process.md), [CP-32: UnitTest Rule](../done/CP-32-UnitTest-Rule.md), [CP-23: Context Control & Wrong-Way Detection](../todo/CP-23-Auto-Learn-To-Skill.md), [SD-10: Context Resolver & RAG](../../06-System-Tech-Design/SD-10-Context-Resolver-RAG.md)
- Replaces: `None`
- Tags: `context, regression, commit-ledger, feature-resolver, flow-gate, skill-pack, gitnexus, tooling, local-runner`

## AI Quick View

### Summary

- Implements `SD-17` in eight slices (`P-1`..`P-8`), cheapest-first, each independently shippable and feature-flagged.
- Core = a **commit-history ledger** (Plane C) from `git log` + the commit-id convention + `change-audit/` notes; a **Feature Resolver** (lexical + LLM pick, no vector); **GitNexus** as an optional structure provider.
- A runner **Post-Step Flow Gate** (hooked after `finishTurn` in `interactive_service.go`) forces required outputs (change-audit note, Task/BugFix doc, green tests) and treats a previously-green test that breaks as a hard regression.
- This document is written to be **implementable without further questions**: it gives file paths, Go types, SQL DDL, git commands, regexes, slot wiring, the exact gate hook point, and per-slice acceptance.

### Current Ask

- Deliver context (ordered feature history + optional structure) and a forced, auditable flow for any bound project, with enough implementation detail that any engineer/agent can build it directly.

### Key Decisions

- `P-1` Build Plane C from `git log` + the commit-id convention; enrich from `change-audit/` `§13` blocks.
- `P-2` Resolve NL → `feature_key` with lexical match + LLM pick-from-list; no vector DB.
- `P-4` Flow Gate is runner-owned (post-`finishTurn`), provider-agnostic.
- `P-5` Regression suite (was-green-now-red) is the primary regression signal and delivers `CP-32`.

### Constraints

- Extend, do not break, CP-10 Part B `internal/contextresolver/`; if that module is not yet built, P-2 ships the minimal slot dispatch described in §4.2.
- Per-`project_id`, local-first under `<target>/.flowpilot/`, optional Supabase mirror under existing RLS.
- All build/index/check steps non-fatal and retryable; never block the raw artifact save.
- Reuse the runner's existing `os/exec` pattern (`sessions.go`, `claude_process.go`) for git/test/tooling subprocesses, and `chat_session_sync.go` helpers for Drive sync.

### Open Questions

- `Q-1` Feature Resolver confidence threshold + confirm UX (`SD-17 Q-1`).
- `Q-2` Flow Gate auto-reprompt budget and which rules block vs warn (`SD-17 Q-2`).
- `Q-3` GitNexus run trigger/staleness on the bound repo (`SD-17 Q-3`).

### Source Refs

- `SD-17` D-1..D-12, §5/§5.1 data model + storage, §6 interfaces, §7 flows.
- `SS-14` AC-1..AC-15. `CP-10 §3` (Plane A), `CP-34` (install/setup UI), `CP-32` (oracle), `SS-13 §13` (ledger block).
- Code refs: `interactive_service.go` (`runTurn`:1586 → `finishTurn`:1772 → gate hook), `chat_session_sync.go` (Drive helpers), `os/exec` pattern in `sessions.go`/`claude_process.go`, `.flowpilot/` via `r.workspace`.

## 1. Goal

Implement `SD-17` for any bound project — ordered feature-history context, optional GitNexus structure, a runner-forced auditable flow, the oracle/regression rule, the skill pack, tooling health, and local+Drive storage — staged cheapest-first, satisfying `SS-14` AC-1..AC-15, with no implementation ambiguity.

## 2. Input Documents

- `SD-17` (primary design + §5.1 storage), `SS-14` (acceptance criteria AC-1..AC-15).
- `CP-10` Part B (Plane A + context resolver this extends), `CP-34` (install/setup UI), `SS-13 §13` (change-ledger block), `CP-32` (oracle rule).

## 3. Implementation Strategy

- **Prerequisites:** Go runner builds (`go build ./...`); `git` on PATH; provider session mediation via `interactive_service.go`.
- **Order:** `P-1` → `P-2` → `P-3` → `P-4` (warn) → `P-5` → `P-6` → `P-7` → `P-8`. Flip `gate_mode` to enforce per project after warn-mode calibration.
- **Build order / parallelism (by Task):**
  - Start now (independent, can run in parallel): `Task-096` (P-1), `Task-098` (P-3), `Task-099` (P-4), `Task-101` (P-6), `Task-102` (P-7).
  - Then: `Task-097` (P-2, after 096) → `Task-100` (P-5, after 099) → `Task-103` (P-8, after 096 + 097).
  - Critical path: `096 → 097 → 103`, and `099 → 100`.
- **Dependency on CP-10:** P-2 registers slots in `internal/contextresolver/`. If absent, implement the minimal `Resolver` + slot dispatch in §4.2 and let CP-10 converge onto it later.
- **Module map (all new under `apps/local-runner/internal/`):**
  - `changeledger/` (P-1), `featurecatalog/` (P-2), `structure/` (P-3), `flowgate/` (P-4, P-5), `skillpack/` (P-6), `tooling/` (P-7), `contextsync/` (P-8).

## 4. Work Breakdown

### 4.1 `P-1` Commit-history ledger — `internal/changeledger/`

**Files:** `ledger.go` (types+store), `parse.go` (git), `enrich.go` (CA join), `query.go`.

**Type:**
```go
type Entry struct {
    CommitHash  string `json:"commit_hash"`
    FeatureKey  string `json:"feature_key"`
    SourceDocID string `json:"source_doc_id"` // Task-087 | BUG-130 | CP-35 | ""
    ChangeType  string `json:"change_type"`   // feature|bugfix|refactor|docs|other
    Summary     string `json:"summary"`
    CommittedAt string `json:"committed_at"`  // RFC3339
    OrderIndex  int    `json:"order_index"`   // ascending by commit time; newest = max
    Confidence  string `json:"confidence"`    // high|low (low = key inferred)
}
```

**parse.go algorithm:**
1. `git -C <repo> log --no-merges --pretty=format:%H%x1f%cI%x1f%s%x1f%b%x1e` (unit sep `0x1f`, record sep `0x1e`). Incremental: read cursor from `.flowpilot/ledger/.cursor`; if set, use `<cursor>..HEAD`.
2. Per record: `ChangeType` from subject regex `^\[(\w+)\]` (lowercased; default `other`). `SourceDocID` from first match of `(Task-\d+|BUG-\d+|CP-\d+[\w-]*)` in subject (then body). `Summary` = subject minus the tag/id prefix.
3. `CommittedAt` from `%cI`. Append to slice; after full pass, sort ascending by `CommittedAt` and assign `OrderIndex`.
4. Save newest hash to `.cursor`.

**enrich.go — feature_key resolution (priority):** (1) the `SS-13 §13` `flowpilot:change-ledger` block's `feature_key` in `change-audit/CA-*.md` matching `SourceDocID`; (2) the SS-13 parent-chain slug for the doc id; (3) the dominant top-level changed path from the commit (`git show --name-only`); (4) `SourceDocID` itself. Cases 3–4 set `Confidence=low`.

**store.go:** `.flowpilot/ledger/feature_history.ndjson`, one `Entry`/line, last-wins by `CommitHash`, mutex-guarded (mirror `local_file_session_store.go`).

**query.go:** `GetFeatureHistory(featureKey string) ([]Entry, error)` → filter, sort by `OrderIndex` (newest last). `ListFeatures() []string`.

**Acceptance:** on this repo, `GetFeatureHistory(<chat key>)` returns entries ending with the `Task-087` commit; runs on a repo with zero `change-audit/` notes using subjects alone.

### 4.2 `P-2` Feature catalog + resolver — `internal/featurecatalog/`

**Files:** `catalog.go` (build+types), `resolve.go`, `slots.go` (resolver wiring).

**Types:**
```go
type Feature struct {
    Key string `json:"feature_key"`; Title string `json:"title"`; Summary string `json:"summary"`
    Keywords []string `json:"keywords"`; FileGlobs []string `json:"file_globs"`; DocRefs []string `json:"doc_refs"`
}
type Candidate struct { Key string `json:"key"`; Score float64 `json:"score"` }
```

**build.go:** sources — (a) `requirements/05-System-Specs/SS-*.md` + `07-Coding-Plan/**/CP-*.md`: `Title` = H1, `Summary` = `AI Quick View` → `Summary` bullets, `DocRefs` = doc id; (b) distinct `feature_key`s from `changeledger`; (c) `FileGlobs` aggregated from each key's commits (`git show --name-only`); (d) `change-audit/FEATURE-KEYS.md` as the **authoritative** key list (`SS-13 §13`). `Keywords` = lowercased tokens of title+summary minus stopwords. Write `.flowpilot/catalog/features.ndjson`.

**resolve.go `ResolveFeature(nl string) ([]Candidate, error)`:**
1. Lexical: tokenize `nl`; per feature score = keyword overlap + title substring hit + file-glob hit; rank desc.
2. If top `Score >= threshold` and clear leader → return single candidate.
3. Else (ambiguous): one cheap LLM call — pass the candidate list (`key — summary`, ≤30 rows) inline and ask "Which feature key best matches: '<nl>'? Reply with the key or NONE." Return its pick. **No vector DB.**
4. If still none → return all candidates for the caller to ask the user.

**slots.go wiring (into `internal/contextresolver/`):**
- `feature.resolve` (priority 1): resolves run-intake/task text → `feature_key`, stored on run context.
- `feature.history` (priority 1): `feature_key` → `[]Entry` → packed (§4.2.1).

**4.2.1 Prompt packing (satisfies AC-3):** render history as:
```
## Prior work on "<feature>" (oldest → newest — build on the NEWEST, do not undo it)
- [Task-040 2026-03] chat feed + continue flow
- [BUG-060 2026-05] history replay fix
- [Task-087 2026-06] /s and /a slash commands   ← current truth
```

**Acceptance:** "update the chat UI" resolves to the chat feature; ambiguous asks return >1 candidate; resolved history is injected into the step prompt newest-last.

### 4.3 `P-3` GitNexus structure provider — `internal/structure/`

**Files:** `provider.go`, `gitnexus.go`, `fallback.go`.
```go
type Provider interface { Available() bool; Dependents(ctx context.Context, target string) (DependentsSummary, error) }
type DependentsSummary struct { Count int; Nearest []string; Flows []string; Complete bool }
```
- `gitnexus.go`: `Available()` = `tooling.Status("gitnexus").OK`; on bind run `npx gitnexus analyze` (background, gated); `Dependents` queries gitnexus (prefer `gitnexus_impact` MCP / `npx gitnexus impact <target> --json`; if no JSON interface, parse text). Set `Complete=false` when dynamic dispatch is detected (string-keyed/interface). Summarize to ≤N `Nearest`.
- `fallback.go`: file-level — import scan + co-change neighbors from `changeledger`.
- Slot `code.dependents` (priority 2): returns the summary, never a raw dump.

**Acceptance:** GitNexus present → real dependents with `Complete` flag; absent → file-level fallback, flagged low-confidence.

### 4.4 `P-4` Post-Step Flow Gate — `internal/flowgate/`

**Files:** `rules.go`, `observe.go`, `evaluate.go`, `enforce.go`.
```go
type Rule struct { ID, Scope, Trigger, RequiredOutput, Action string } // Action: reprompt|block|approve|warn
type ChangedFile struct { Path, Status string }                        // A|M|D
type TestOutcome struct { Ran bool; Passed, Failed []string }
type TurnResult struct {
    RunID, StepID, FinalMessage string
    ToolCalls []string
    GitDiff   []ChangedFile
    Tests     TestOutcome
}
type Violation struct { Rule Rule; Detail string }
```
**Default rules (`rules.go`, seeded to `settings/flow-rules.json`):**
```json
[ {"id":"r-ca","trigger":"code_changed","required_output":"change_audit_note","action":"reprompt"},
  {"id":"r-bug","trigger":"bug_fixed","required_output":"bugfix_doc","action":"block"},
  {"id":"r-tests","trigger":"tests_failed","required_output":"tests_green_or_explained","action":"block"},
  {"id":"r-reg","trigger":"regression_test_broke","required_output":"restore_green_without_weakening","action":"block"},
  {"id":"r-dep","trigger":"removed_referenced_code","required_output":"confirm_or_update_callers","action":"block"} ]
```
(`r-dep` active only when `structure.Available()`; satisfies AC-4.)

**observe.go (hook point — `interactive_service.go` `runTurn` after `finishTurn` returns done):** build `TurnResult` — `FinalMessage`/`ToolCalls` from the turn; `GitDiff` from `git -C <cwd> status --porcelain`; `Tests` from P-5.

**evaluate.go `Evaluate(tr, rules) []Violation`:**
- `code_changed` = any non-doc file in `GitDiff` (exclude `requirements/`, `change-audit/`, `*.md`) AND no `change-audit/CA-*.md` Added/Modified → violation.
- `bug_fixed` = `ChangeType==bugfix` (from commit/run) AND no `requirements/09-BugFix/**/BUG-*.md` in diff → violation.
- `tests_failed` = `Tests.Failed` non-empty.
- `regression_test_broke` = from P-5.
- `removed_referenced_code` = a `D` file/symbol whose `structure.Dependents` count > 0.

**enforce.go:**
- `reprompt`: inject a follow-up turn with the exact missing requirement; bounded by `max_reprompt_attempts` (default 2); then escalate to user.
- `block`: do not transition the step to `RunStatusCompleted`; emit the violation over SSE; require user action.
- `approve`/`warn`: raise approval gate (reuse SD-16 gate infra) / log only.

**Acceptance:** a code change with no CA note → reprompt then block; gate runs for every provider (Claude/Codex) because it is post-turn in the runner.

### 4.5 `P-5` Regression suite + oracle rule — delivers `CP-32`

**Files:** `flowgate/baseline.go`, `flowgate/oracle.go`.
- `test_command` resolution (config or auto): `go.mod`→`go test ./...`; `package.json`→`npm test`; `pytest.ini`/`pyproject`→`pytest -q`. Stored in `settings/flow-rules.json`.
- `CaptureBaseline()` at task start: run tests; write `.flowpilot/guard/test_baseline.json` `{captured_at, green_tests []string}`.
- Test-file detection in `GitDiff`: `_test\.go$|\.test\.[jt]sx?$|\.spec\.[jt]sx?$|(^|/)test_.*\.py$`.
- `evaluate`: re-run tests; `regressed = baseline.green ∩ now_red ∩ {test file NOT in GitDiff}` → emit `regression_test_broke` (block); message: "Previously-passing tests now fail: <list>. Fix the code; do not change these tests."
- If a pre-existing test file IS in `GitDiff` → flag possible oracle tampering → surface for human review (do not auto-accept).
- Routing when a test legitimately must change: specs present (`requirements/05,06` + linked AC) → re-check governing AC (cite ids, `SS-13 §8.3`); no specs → ask the user. Never change code/tests blindly to pass.

**Acceptance:** breaking a pre-existing green test → blocked with the regressed list; editing a pre-existing test in a code turn → flagged; no-spec repo → asks the user.

### 4.6 `P-6` Skill pack — runner install logic here; setup page in `CP-34`

**Files:** `internal/skillpack/install.go`, embedded `internal/skillpack/flow-pack/**` via `//go:embed`.
- Pack contents (each a `SKILL.md` with a `version:` header): `git-commit-format` (reuse `.claude/skills/git-commit`), `oracle-rule`, `audit-logging` (reuse existing), `phase-doc` (reuse `phase-document-authoring`), `context-discipline`.
- `git-commit-format` enforces the **contract** `P-1` reads: `[Type]: <id> <desc>`, Type ∈ `Feature|BugFix|Refactor|Docs|Hotfix`, id ∈ `Task-NNN|BUG-NNN|CP-NN`.
- `Install(target)`: copy pack into `<target>/.claude/skills/flowpilot/`, `<target>/.codex/...`, `<target>/.gemini/...`; version-stamp; overwrite when bundled version is newer; re-run on bind.

**Acceptance:** after bind, the five skills exist in each provider dir with version stamps; re-bind re-syncs.

### 4.7 `P-7` Tooling check — runner logic here; setup page in `CP-34`

**Files:** `internal/tooling/check.go`.
- `CheckTool`: `gitnexus` (`exec.LookPath` else `npx gitnexus --version`), `rtk` (`rtk --version`), `node` (`node --version`), `skill_pack` (presence+version).
- Persist `.flowpilot/tooling.json` `{tool, version, status: ok|missing|stale, checked_at}` (machine-local, NOT synced).
- `ComputeCapabilityProfile()` → `{has_gitnexus, has_specs, has_tests, structure_tier, decision_tier, languages}`; engine reads it to pick behavior; never hard-fails on a missing tool.

**Acceptance:** with gitnexus removed, `tooling.json` shows `missing` and the engine selects file-level structure.

### 4.8 `P-8` Local store + Drive sync — `internal/contextsync/`, reuse `chat_session_sync.go`

- Persist per `SD-17 §5.1`. Sync only shared data (`feature_history.ndjson`, `features.ndjson`, `flow-rules.json`) to the project's chat Drive folder under `context-engine/`, reusing `ensureGoogleDriveFolderPath` / `upsertGoogleDriveFile` and the NDJSON `_index` + `manifest.json` + SHA256 pattern.
- Trigger: after ledger/catalog rebuild and on step complete (debounced). Keep machine-specific/ephemeral data (`test_baseline`, gate reports, `tooling.json`, GitNexus index) local-only.
- The feature-key registry (`change-audit/FEATURE-KEYS.md`) is a committed repo file — synced via **git**, not Drive; only the derived `.flowpilot/catalog/features.ndjson` cache is Drive-synced (`SS-13 §13`).
- Reuses `CP-33` Drive folder selection.

**Acceptance:** shared files appear under `context-engine/` in the project Drive folder with a manifest; tooling/baseline never sync.

## 5. Touched Areas

- **New modules:** `internal/{changeledger,featurecatalog,structure,flowgate,skillpack,tooling,contextsync}/`.
- **Extended:** `internal/contextresolver/` (new slots; create minimal dispatch if absent), `interactive_service.go` (`runTurn` post-`finishTurn` gate hook + `startTurn` baseline capture), `step_context_slots.resolver` enum.
- **Reused:** `chat_session_sync.go` Drive helpers; `os/exec` subprocess pattern; `local_file_session_store.go` NDJSON pattern; SD-16 approval-gate infra.
- **Owned by CP-34 (UI only):** setup/health page; this CP provides the runner-side `tooling`/`skillpack` logic it calls.
- **Database:** mirror tables (§6) + new resolver enum values.

## 6. Data or Migration Steps

**Supabase mirror (one migration; local NDJSON is primary, mirror optional, RLS mirrors CP-10 §3.1):**
```sql
create table feature_history (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  commit_hash text not null, feature_key text not null, source_doc_id text,
  change_type text not null, summary text not null,
  committed_at timestamptz not null, order_index int not null,
  unique(project_id, commit_hash)
);
create table feature_catalog (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  feature_key text not null, title text, summary text,
  keywords jsonb not null default '[]', file_globs jsonb not null default '[]', doc_refs jsonb not null default '[]',
  unique(project_id, feature_key)
);
create table flow_rules (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  scope text not null default 'project', trigger text not null,
  required_output text not null, action text not null default 'warn',
  enabled boolean not null default true
);
alter table feature_history enable row level security;
alter table feature_catalog enable row level security;
alter table flow_rules enable row level security;
-- SELECT/ALL policies: project_id in (project_teams ∩ team_members for auth.uid()) — copy CP-10 §3.1.
-- tooling_status / capability_profile are machine-local; NOT mirrored.
```
**New `step_context_slots.resolver` enum values:** `feature.resolve`, `feature.history`, `code.dependents`.

**Config (`settings/flow-rules.json`, per project):** `gate_mode: warn|enforce` (default warn), `test_command: string` (auto-detected), `structure_provider: gitnexus|fallback`, `resolver_embeddings: off`, `max_reprompt_attempts: 2`. `gate_mode` governs only the documentation rules (`r-ca`, `r-bug`); the regression rule (`r-reg`) and failing task-tests (`r-tests`) are **always enforced** regardless of `gate_mode` (`SS-14 Q-1`).

**Backfill:** build `feature_history` + `feature_catalog` from existing git history and the 97+ `change-audit/` notes on first bind.

## 7. Validation Plan

- **Unit:** P-1 git-log parse (tag/id regex, ordering, incremental cursor) + CA `§13` enrich; P-2 lexical ranking + ambiguity → LLM pick; P-3 gitnexus-absent fallback + `Complete` flag; P-4 each trigger in `Evaluate`; P-5 `regressed` set formula + test-file detection + no-spec routing; P-6 install/version-stamp; P-7 tier selection; P-8 manifest + SHA256 + sync/local split.
- **Integration:** bind no-spec third-party repo → catalog/history from git; "update the chat UI" → chat feature + ordered history packed; code change without CA note → reprompt then block; break a pre-existing test → `regression_test_broke` block; remove referenced symbol (gitnexus present) → `removed_referenced_code` block; shared files sync to `context-engine/`, tooling stays local.
- **Manual:** confirm gate runs for both Claude and Codex turns; confirm no FlowPilot-repo content leaks into context; confirm newest history entry is treated as current truth.

## 8. Rollout and Fallback

- Order `P-1`→`P-8`; `gate_mode` defaults `warn`, flip to `enforce` per project after calibration. Each slice feature-flagged; disabling reverts to prior capability with no data loss (local `.flowpilot/` retained; git remains source of truth). Monitoring: gate decisions, forced remediations, oracle/regression classifications, resolver mismatches → audit log (extends CP-10 §5.2 / CP-23 telemetry).

## 9. Risks

- `R-1` Wrong feature resolution → confirm-on-ambiguity, show matched history (`SD-17 R-1`).
- `R-2` Over-forcing loops → bounded retries, warn-before-block (`SD-17 R-2`).
- `R-3` GitNexus cost/staleness → on-demand/scheduled, suppress stale (`SD-17 R-3`).
- `R-4` Overlap with `CP-23` runtime loop → share hooks/telemetry; split = structural-history (here) vs behavioral-drift (CP-23).
- `R-5` Test command misdetected on polyglot repos → `test_command` is explicit config; auto-detect is only a default.

## 10. Definition of Done

### Per-slice

- [ ] `P-1` `GetFeatureHistory` returns ordered commits (newest last) on a no-spec repo from git alone; incremental cursor works; CA `§13` enrich applied when present.
- [ ] `P-2` NL ask resolves to the correct `feature_key` (lexical + LLM pick, no vector); ambiguous asks return candidates; resolved history packed into the prompt newest-last with the "build on newest" framing.
- [ ] `P-3` GitNexus present → summarized dependents with `Complete` flag; absent → file-level fallback, flagged.
- [ ] `P-4` Gate hooks after `finishTurn`; code change without a CA note → reprompt then block; runs for all providers.
- [ ] `P-5` A previously-green test that breaks (not changed by the task) blocks the step; pre-existing test edits flagged; no-spec path asks the user (delivers `CP-32`).
- [ ] `P-6` Five-skill flow pack auto-installed into `.claude/.codex/.gemini` on bind with version stamps; `git-commit-format` enforces the `P-1` contract.
- [ ] `P-7` `tooling.json` reflects gitnexus/rtk/node/skill_pack; capability tier selected; missing tool degrades, never fails.
- [ ] `P-8` Engine files persist under `.flowpilot/`; shared data syncs to `context-engine/` via the chat-sync mechanism; machine-specific data stays local.
- [ ] `go build ./...` and `go test ./internal/...` pass for all new modules.

### Shared

- [ ] All mirror tables RLS-scoped per `project_id`; FlowPilot's own repo never used as context (AC-1).
- [ ] All build/index/check steps non-fatal and retryable; raw artifact save always succeeds (AC-9).

### AC coverage matrix (`SS-14` → slice / DoD)

| AC | Covered by | AC | Covered by |
|----|-----------|----|-----------|
| AC-1 target-only/isolated | Shared DoD, §6 RLS | AC-9 non-fatal | Shared DoD |
| AC-2 works without docs | `P-1` | AC-10 NL resolution | `P-2` |
| AC-3 history awareness | `P-2` §4.2.1 packing | AC-11 force outputs | `P-4` |
| AC-4 removal surfaced (when avail.) | `P-3` + `r-dep` rule | AC-12 skills auto-installed | `P-6` |
| AC-5 dependents summarized | `P-3` | AC-13 tooling health | `P-7` |
| AC-6 oracle integrity | `P-5` | AC-14 regression test = block | `P-5` |
| AC-7 auditable (context + change) | `P-4` gate report + SD-10 `workflow_prompt_context_items` | AC-15 local + Drive sync | `P-8` |
| AC-8 staleness | v1: Plane C is git-derived = always current; Plane A staleness owned by SD-10; symbol-level staleness deferred (`SD-17 D-11`) — no separate v1 mechanism | | |

### US coverage (`SS-14` user stories → AC / slice)

| US | Covered by | US | Covered by |
|----|-----------|----|-----------|
| US-1 whole-project structure | AC-5 → Task-098 | US-5 legacy/no-docs repo | AC-2 → Task-096 |
| US-2 know prior work | AC-3 → Task-097 | US-6 per-step audit record | AC-7 → Task-099 |
| US-3 declare scope + flag drift | **deferred** (`SD-17 D-11`); intent partly via AC-3/AC-6/AC-14 | US-7 NL feature, no id | AC-10 → Task-097 |
| US-4 failing test → spec recheck | AC-6/AC-14 → Task-100 | US-8 force record + honest tests | AC-11/AC-14 → Task-099/100 |

US-3's literal "declare-and-flag-drift" is the deferred scope-drift; v1 covers its *intent* (don't unexpectedly break or undo work) through history awareness (Task-097) + the regression suite (Task-100).
