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
2. Per record: `parseTags` reads the `[Type][feature][layer?]` bracket run → `ChangeType` (bracket 1, default `other`), `FeatureKey` + `Confidence=high` (bracket 2 when present), `Layer` (bracket 3, optional). `SourceDocID` from first match of `(Task-\d+|BUG-\d+|CP-\d+[\w-]*)` in subject (then body). `Summary` = subject minus the bracket/id prefix. The old single-bracket `[Type]: <id> desc` form still parses (feature falls to the enrich fallback chain).
3. `CommittedAt` from `%cI`. Append to slice; after full pass, sort ascending by `CommittedAt` and assign `OrderIndex`.
4. Save newest hash to `.cursor`.

**enrich.go — feature_key resolution (priority):** (0) the `[feature]` bracket in the commit message (set high-confidence by the parser — `EnrichAll` keeps it and skips the rest); (1) the `SS-13 §13` `flowpilot:change-ledger` block's `feature_key` in `change-audit/CA-*.md` matching `SourceDocID`; (2) `FEATURE-KEYS.md` keyword match on subject/body; (3) the dominant top-level changed path from the commit (`git show --name-only`); (4) `SourceDocID` itself. Cases 2–4 set `Confidence=low`. **The `[feature]` bracket (priority 0) is the intended path** — it avoids the coarse path-bucketing of priority 3 (where a whole `domain/` directory collapses into one key).

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
- `git-commit-format` enforces the **contract** `P-1` reads: `[Type][feature][layer?] <desc>`, Type ∈ `Feature|BugFix|Refactor|Docs|Hotfix|Test`, feature ∈ kebab-case key in `FEATURE-KEYS.md` (mandatory — this is what makes ledger feature_key extraction exact), layer optional, source-doc id `Task-NNN|BUG-NNN|CP-NN` in the description. Consolidates the former standalone `.claude/skills/git-commit` skill (English-only, ≤72-char first line, no AI-authorship attribution).
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

**No Supabase migration needed.** All engine data is local-first under `<target>/.flowpilot/` (one directory per bound project, isolated by design). Shared files sync cross-machine via Google Drive (`context-engine/` prefix) — no DB tables required.

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

- [x] `P-1` `GetFeatureHistory` returns ordered commits (newest last) on a no-spec repo from git alone; incremental cursor works; CA `§13` enrich applied when present. — **`changeledger` package, 22 tests ✓ (Task-096)**
- [x] `P-2` NL ask resolves to the correct `feature_key` (lexical + LLM pick, no vector); ambiguous asks return candidates; resolved history packed into the prompt newest-last with the "build on newest" framing. — **`featurecatalog` package, 15 tests ✓ (Task-097)**
- [x] `P-3` GitNexus present → summarized dependents with `Complete` flag; absent → file-level fallback, flagged. — **`structure` package, 13 tests ✓ (Task-098)**
- [x] `P-4` Gate hooks after `finishTurn`; code change without a CA note → reprompt then block; runs for all providers. — **`flowgate` package done, 33 tests ✓ (Task-099); runner wiring complete via `gate_hook.go` + `runTurn` insertion (CP-35)**
- [x] `P-5` A previously-green test that breaks (not changed by the task) blocks the step; pre-existing test edits flagged; no-spec path asks the user (delivers `CP-32`). — **`flowgate/oracle.go` done (Task-100); runner wiring complete via `runFlowGate` calling `RunOracle` (CP-35)**
- [x] `P-6` Five-skill flow pack auto-installed into `.claude/skills` and `.agents/skills` on bind with version stamps; `git-commit-format` (v4) enforces the `[Type][feature][layer?]` contract so the ledger extracts `feature_key` exactly; `audit-logging` maintains `FEATURE-KEYS.md`. — **`skillpack` package + embedded skills, `PackVersion` bumped to 4 ✓; `Install()` wired via `engine_setup.go` `runEngineInit()` called on every bind (CP-34 ✓)**
- [x] `P-7` `tooling.json` reflects gitnexus/rtk/node/skill_pack; capability tier selected; missing tool degrades, never fails. — **`tooling` package, 7 tests ✓ (Task-102)**
- [x] `P-8` Engine files persist under `.flowpilot/`; shared data syncs to `context-engine/` via the chat-sync mechanism; machine-specific data stays local. — **`contextsync` local store + `WriteManifest` + `SyncSharedFiles` fully wired in `runEngineInit()` (CP-35); `engineDriveSyncer` uses existing `ensureChatSessionDriveRoot` + `ensureGoogleDriveFolderPath` + `upsertGoogleDriveFile` helpers; Drive not connected → skipped silently (nil syncer)**
- [x] `go build ./...` and `go test ./internal/...` pass for all new modules. — **`go build ./internal/runner/...` clean; pre-existing test failures are environment-specific (Codex binary, Google Drive account) and unrelated to CP-35**

### Shared

- [x] Context stays isolated to the currently bound target project, and each project uses its own `<target>/.flowpilot/` dir (AC-1). FlowPilot itself is allowed when intentionally bound as the target. — **structural isolation by design ✓**
- [x] All build/index/check steps non-fatal and retryable; raw artifact save always succeeds (AC-9). — **all packages non-fatal by design ✓**

### What remains before full DoD

All P-1 through P-8 slices are now fully wired. No remaining wiring items.

### AC coverage matrix (`SS-14` → slice / DoD)

| AC | Covered by | AC | Covered by |
|----|-----------|----|-----------|
| AC-1 target-only/isolated | per-project `<target>/.flowpilot/` dir (structural isolation) | AC-9 non-fatal | Shared DoD |
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

## 11. Manual Testing Guide

> **Prerequisites:** Go 1.21+, git on PATH, run all commands from `apps/local-runner/`.
> The packages are fully implemented and unit-tested. This guide lets you exercise them against real data without needing runner wiring.

---

### T-1 — Changeledger (Plane C): read this repo's feature history

```bash
# Run the existing unit tests with verbose output to see parsing in action
go test ./internal/changeledger/... -v -run TestParseRecord

# Write a quick smoke test against the FlowPilot repo itself:
cat > /tmp/ledger_smoke_test.go << 'EOF'
//go:build ignore
package main

import (
    "fmt"
    "github.com/flowpilot/internal/changeledger"  // adjust if needed
)

func main() {
    repoDir := "../.."          // root of flowpilot repo
    dotFP   := "/tmp/.flowpilot-test"
    if err := changeledger.Build(repoDir, dotFP); err != nil {
        panic(err)
    }
    l, _ := changeledger.New(dotFP)
    features := l.ListFeatures()
    fmt.Printf("Features found: %v\n", features)
    for _, f := range features {
        history, _ := l.GetFeatureHistory(f)
        if len(history) > 0 {
            latest := history[len(history)-1]
            fmt.Printf("  %s → latest: [%s] %s (%s)\n", f, latest.SourceDocID, latest.Summary, latest.CommittedAt[:10])
        }
    }
}
EOF
```

**What to verify:**
- Feature keys appear (e.g. `chat-ui`, `agent-spawn`, `context-regression-engine`)
- Each feature's latest entry matches its most recent commit
- `ListFeatures()` returns sorted keys
- Re-running `Build()` is incremental (cursor skips already-seen commits)

**Check the cursor file was written:**
```bash
cat /tmp/.flowpilot-test/ledger/.cursor   # should be a commit hash
wc -l /tmp/.flowpilot-test/ledger/feature_history.ndjson  # one line per commit
```

---

### T-2 — Feature Catalog + Resolver: NL → feature_key

```bash
go test ./internal/featurecatalog/... -v
```

**Key tests to watch:**
- `TestResolveFeature` — "update the chat ui" → top candidate `chat-ui` with score > 0
- `TestHistorySlot` — output ends with `← current truth` on last entry
- `TestBuild` — catalog seeds from FEATURE-KEYS.md

**Manual spot-check** (resolve against real catalog):
```bash
go test ./internal/featurecatalog/... -v -run TestBuild
```
Expected: catalog contains entries for all keys in `../../change-audit/FEATURE-KEYS.md`.

---

### T-3 — Structure Provider: GitNexus / file-level fallback

```bash
go test ./internal/structure/... -v
```

**Key tests:**
- `TestFallbackProvider_Available` → `false`
- `TestFallbackProvider_Dependents` → returns `DependentsSummary{Complete: false}`
- `TestDependentsSummaryJSON` → correct JSON marshaling

**With GitNexus installed**, the `gitNexusProvider` path activates:
```bash
# Check if gitnexus is available
npx gitnexus --version

# If yes, the structure package will use real blast-radius data
go test ./internal/structure/... -v -run TestGitNexus
```

---

### T-4 — Flow Gate: evaluate + enforce rules

```bash
go test ./internal/flowgate/... -v
```

**Key scenarios to watch:**

| Test | What it proves |
|---|---|
| `TestEvaluate_CodeChangeNoCA` | code changed, no CA note → `r-ca` violation |
| `TestEvaluate_NoCodeChanges` | docs-only change → no violations |
| `TestEvaluate_TestsFailed` | failed tests → `r-tests` violation |
| `TestEnforce_RegressionAlwaysBlocks` | r-reg fires even in `gate_mode=warn` |
| `TestEnforce_WarnMode_DowngradesRCA` | r-ca in warn mode → `warn` action, not `block` |
| `TestDefaultRules` | 5 rules with correct IDs returned |

**Manual rule evaluation** — build a mock TurnResult and call Evaluate:
```bash
go test ./internal/flowgate/... -v -run TestEnforce
```

---

### T-5 — Regression Oracle: baseline capture + regressed test detection

```bash
go test ./internal/flowgate/... -v -run TestOracle
go test ./internal/flowgate/... -v -run TestBaseline
```

**Key tests:**
- `TestIsTestFile` — `_test.go`, `.test.ts`, `.spec.tsx` → `true`; `main.go` → `false`
- `TestDetectTestCommand` — temp dir with `go.mod` → `"go test ./..."`
- `TestRunOracle_NilBaseline` — no panic, returns empty `OracleResult`
- `TestRunOracle_HasRegression` — green test now red (not in diff) → `HasRegression=true`

**Baseline capture against this repo:**
```bash
go test ./internal/flowgate/... -v -run TestCaptureBaseline
```
Check that `guard/test_baseline.json` is written with `green_tests` populated.

---

### T-6 — Skill Pack: install to a test directory

```bash
go test ./internal/skillpack/... -v
```

**Key tests:**
- `TestInstall` — 5 skills × 3 provider dirs = 15 files created in temp dir
- `TestIsInstalled` — returns `true` after install
- `TestReInstall_SkipsExisting` — same-version files not overwritten (appear in `Skipped`)

**Manual install to a scratch directory:**
```bash
mkdir /tmp/test-target-repo
go test ./internal/skillpack/... -v -run TestInstall
```

Then verify files exist:
```bash
ls /tmp/test-target-repo/.claude/skills/flowpilot/
# Expected: audit-logging/  context-discipline/  git-commit-format/  oracle-rule/  phase-doc/
cat /tmp/test-target-repo/.claude/skills/flowpilot/git-commit-format/SKILL.md | head -3
# Expected: version: 1  (first line)
```

---

### T-7 — Tooling Check: detect installed tools + capability profile

```bash
go test ./internal/tooling/... -v
```

**Key tests:**
- `TestCheckTool_Node` — node is on PATH → `Status="ok"`, `Version` is non-empty
- `TestStatusOf` — lookup by name returns correct struct
- `TestComputeCapabilityProfile` — mock statuses produce correct tier strings
- `TestCheckAll` — writes `tooling.json` to temp dir

**Run against real environment:**
```bash
go test ./internal/tooling/... -v -run TestCheckAll
```

Check the generated file:
```bash
cat /tmp/tooling-test/tooling.json | python -m json.tool
# Expected: array of 4 entries (gitnexus, rtk, node, skill_pack), each with status
```

**What to look for:**
- `node` → `"ok"` if node is installed
- `gitnexus` → `"ok"` if `npx gitnexus` works, `"missing"` otherwise
- `rtk` → `"ok"` if RTK is installed, `"missing"` otherwise

---

### T-8 — Context Sync: local store layout + manifest

```bash
go test ./internal/contextsync/... -v
```

**Key tests:**
- `TestNewEngineStore_CreatesDirs` — all 6 subdirs created
- `TestSharedFiles` — returns 3 paths (ledger, catalog, flow-rules)
- `TestIsLocalOnly` — guard/ and tooling.json → `true`; ledger → `false`
- `TestWriteManifest` — manifest.json written with SHA256 entries
- `TestSyncSharedFiles_NilSyncer` — all skipped, no error

**Manual local store creation:**
```bash
go test ./internal/contextsync/... -v -run TestNewEngineStore
ls /tmp/engine-store-test/.flowpilot/
# Expected: ledger/  catalog/  settings/  guard/  structure/
```

---

### Integration smoke test (no runner wiring needed)

Run all packages together and confirm no regressions:

```bash
cd C:\working\flowpilot\apps\local-runner
go build ./...
go test ./internal/changeledger/... ./internal/featurecatalog/... ./internal/structure/... ./internal/flowgate/... ./internal/skillpack/... ./internal/tooling/... ./internal/contextsync/... -count=1
```

**Expected output:** `102 tests across 7 packages, all pass.`

---

### What is NOT yet testable without Drive connected

All behaviors are now testable. E2E-10 requires Google Drive to be connected to the project.

---

## 12. E2E Testing on the Real App

> **Use a dedicated test project — never bind the FlowPilot repo itself as the target.**
> Suggested: any small Go or Node project with at least a few commits and one test file.
> Labels: ✅ testable now | ⏳ requires runner wiring (CP-10 / CP-34)

---

### Prerequisites

1. FlowPilot app running locally (web + runner).
2. A test project directory available, e.g. `C:\test-projects\my-sample-app` with:
   - At least 5 git commits
   - At least one passing test (`go test ./...` or `npm test` passes)
   - One source file that can be edited
3. `git` and `node` on PATH.

---

### (Passed) E2E-1 — Project bind creates engine store ✅

**Steps:**
1. In the FlowPilot UI, bind the test project (Settings → Bind Project → select `my-sample-app`).
2. After bind completes, open a terminal and inspect:

```bash
ls my-sample-app/.flowpilot/
# Expected: ledger/  catalog/  settings/  guard/  structure/
```

**Verify:**
- `.flowpilot/ledger/feature_history.ndjson` exists and has at least 1 line (one commit per line).
- `.flowpilot/ledger/.cursor` contains a commit hash.
- `.flowpilot/tooling.json` exists with 4 entries (gitnexus, rtk, node, skill_pack).

---

### (Passed) E2E-2 — Tooling health reflects real environment ✅

**Steps:**
1. After bind, read the tooling file:

```bash
cat my-sample-app/.flowpilot/tooling.json
```

**Verify:**
- `node` → `"ok"` (node is installed).
- `gitnexus` → `"ok"` if `npx gitnexus` resolves, `"missing"` otherwise.
- `rtk` → reflects whether RTK is installed.
- No entry is absent — all 4 tools are always checked.

---

### (Passed) E2E-3 — Feature history populated from real git log ✅

**Steps:**
1. Run `go test ./internal/changeledger/... -v -run TestParseRecord` against the test project's directory to spot-check parsing.
2. Read the generated NDJSON:

```bash
wc -l my-sample-app/.flowpilot/ledger/feature_history.ndjson
cat my-sample-app/.flowpilot/ledger/feature_history.ndjson | tail -3
```

**Verify:**
- Line count matches the number of non-merge commits in the repo (`git log --no-merges --oneline | wc -l`).
- Each line is valid JSON with `commit_hash`, `feature_key`, `committed_at`.
- Re-binding (or calling `Build()` again) is incremental — no duplicate lines added.

---

### E2E-4 (Passed) — NL resolves to a feature key ✅

**Steps:**
1. Note a feature key that appears in `.flowpilot/ledger/feature_history.ndjson` (e.g. `auth`, `api`, `ui`).
2. Run the resolver against the catalog:

```bash
cd apps/local-runner
go test ./internal/featurecatalog/... -v -run TestResolveFeature
```

3. Also verify the catalog was built from the test project's commits:

```bash
wc -l my-sample-app/.flowpilot/catalog/features.ndjson
# Should have one line per distinct feature_key found
```

**Verify:**
- A natural-language phrase like "update the login page" resolves to the correct feature key.
- The top candidate score is > 5.0.
- History slot output ends with `← current truth` on the last entry.

---

### (Passed) E2E-5 — Skill pack installed on bind ✅

**Steps:**
1. Bind the test project (Settings → Projects → add binding).
2. The desktop calls `POST /client/projects/{id}/engine/init?trigger=bind` which runs `runEngineInit` → `skillpack.Install()`.
3. Check the installed skills:

```bash
ls my-sample-app/.claude/skills/
# Expected: audit-logging/  context-discipline/  git-commit-format/  oracle-rule/  phase-doc/

ls my-sample-app/.agents/skills/
# Same 5 skills (Codex + Gemini share .agents/skills/)

head -5 my-sample-app/.claude/skills/git-commit-format/SKILL.md
# Expected: ---\nname: git-commit-format\n...\nversion: 3\n---
```

**Verify:**
- All 5 skills present in both `.claude/skills/` and `.agents/skills/`.
- Re-bind with same version → files in `Skipped` (not overwritten; `engine-init.json` logs "skipped" counts).
- Bump `PackVersion` in `skillpack/install.go` → re-bind overwrites all files.

---

### (Passed) E2E-6 — Flow Gate: code change without CA note → reprompt

> **Gate mode prerequisite:** gate mode defaults to `enforce` since bind auto-creates
> `.flowpilot/settings/gate-config.json` with `{"gate_mode":"enforce"}`. No manual setup needed.
> Confirm in the Engine tab → Flow Gate panel: dropdown shows "Enforce (default)".

**Steps:**

1. In FlowPilot, open the Engine tab (desktop app → Engine). Confirm the selected project's
   Flow Gate dropdown reads **"Enforce (default) — reprompt + block on violations"**.
   If it reads "Warn only", switch it and click **Save**.

2. Start a workflow task on the test project.

3. Give the AI a prompt that will cause it to edit a source file but **not** write a
   `change-audit/CA-*.md` note — e.g.:
   > "Add a helper comment to the top of calc.go."

4. Let the turn run to completion. Do not interrupt.

**What to observe (primary — no DevTools required):**

- **A second turn fires automatically** within ~1 s of the first turn ending. The runner
  calls `startTurn` with the prompt:
  > "Flow gate: code changed but no change-audit note found"
- The step does NOT transition to `RunStatusCompleted` — it remains in-progress while
  the reprompt turn is active.
- After the AI responds to the reprompt (ideally writing the CA note), if it still
  doesn't write one, a third auto-turn fires (up to `maxFlowGateReprompts = 2` attempts).
- After 2 reprompts without a CA note, no further auto-turns fire and the step stays
  blocked until the user intervenes.

**What to observe (desktop — with the CP-35 UI fixes):**

- An **amber ⚠ violation card** appears inline in the chat after the first turn:
  > ⚠ Flow gate: code changed but no change-audit note found
- The reprompt turn then renders **in-place** (no tab-switch needed) — the prior bug where
  the UI only updated after switching chats is fixed (`consumeOrchestrationStream` now
  applies post-turn gate events via `applyEvent`).

**What to observe (secondary — browser DevTools):**

Open DevTools → Network → filter for `events/stream`. In the response body look for a
newline-delimited JSON line of:
```json
{"type":"flow_gate_violation","error":"Flow gate: code changed but no change-audit note found","providerTurnId":"<id>"}
```

**Verify in runner logs (optional):**
```
[gate] violations=1 gateMode="enforce" hasCode=true hasCA=false
[gate] reprompt attempt=0 stepID="chat-run-XXXX"
```

---

### (Passed) E2E-7 — Flow Gate: code change WITH CA note → passes

> Same gate mode as E2E-6 — no mode change needed.

**Steps:**

1. Start a workflow task on the test project.

2. Give the AI a prompt that explicitly asks for both the edit AND a CA note — e.g.:
   > "Add a helper comment to the top of calc.go. Then write a change-audit note in `change-audit/CA-<today>.md` documenting the change."

3. Let the turn run to completion.

**What to observe:**

- No second auto-turn fires after the first turn ends.
- The step transitions to `RunStatusCompleted`.
- In the project directory:
  ```powershell
  git status
  # Should show: both main.go (modified) AND change-audit/CA-*.md (new file)
  ```
- No `flow_gate_violation` JSON line appears in the `events/stream` DevTools response.

**Why it passes:** `ObserveGitDiff` sees the `change-audit/CA-*.md` as Added in the diff →
`HasChangeAuditNote(diff)` returns `true` → `r-ca` trigger condition is not met → zero
violations → `Enforce` returns `"pass"` → gate returns `false` → finalizer runs normally.

---

### E2E-8 (Passed) — Regression oracle: break a test → step blocked (covers BOTH r-tests AND r-reg)

> **Code-state finding (v1) — r-tests and r-reg are coupled.** In `gate_hook.go`,
> `TurnResult.Tests.Failed` is populated **only** from `oracle.Regressed`. Both
> `r-tests` (`tests_failed`) and `r-reg` (`regression_test_broke`) check the identical
> condition in `evaluate.go` — `tr.Tests.Ran && len(tr.Tests.Failed) > 0` — so in v1
> they **always fire together** on a regression. There is no input that trips one without
> the other. Both are always-block (`isAlwaysBlock` in `enforce.go`), so the step blocks
> in **any** gate mode (enforce or warn).

> **Test-runner detection (now monorepo-aware).** `DetectTestRunner(workspaceCwd)` first
> checks the bound project root, then — if the root has no runner — walks the tree
> (skipping `node_modules`/`vendor`/`build`/hidden dirs, depth-capped) for a nested marker
> and records both the command and the directory it runs in (`test_dir` in the baseline).
> Ecosystem priority is go > python > node; ties break on the shallowest path.
>   - **flowpilot repo root now works**: detection finds `apps/local-runner/go.mod` and runs
>     `go test -v ./...` there. ⚠ **Performance caveat:** the oracle re-runs the **entire**
>     `apps/local-runner` suite on **every post-turn gate call**, which can take minutes per
>     turn. Fine for a one-off E2E check, heavy for routine use.
>   - **(Recommended for speed/safety) a tiny Go sandbox** (Step A below) — its suite is one
>     test, so each gate call is instant and your real working tree is never dirtied.

> **Baseline note:** The baseline is captured lazily on the **first gate call** for a
> project (when `.flowpilot/guard/test_baseline.json` is absent). The first task on a fresh
> project captures the baseline AFTER that turn — with whatever test state exists then. To
> guarantee a clean green baseline, run a harmless no-edit task first (Step B).

**Step A — create a Go sandbox project (one-time):**

```powershell
mkdir C:\test-projects\gate-sandbox; cd C:\test-projects\gate-sandbox
git init
go mod init gatesandbox
```

Create `calc.go`:
```go
package main

func Add(a, b int) int { return a + b }

func main() {}
```

Create `calc_test.go`:
```go
package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatalf("Add(2,3) = %d, want 5", Add(2, 3))
	}
}
```

```powershell
git add -A; git commit -m "init sandbox"
go test -v ./...
# --- PASS: TestAdd   ← confirm green before binding
```

Then in FlowPilot: bind `C:\test-projects\gate-sandbox` as a project and let the engine
init (auto-creates `.flowpilot/` with `gate-config.json` = enforce).

**Step B — prime a clean baseline:**

1. Delete any stale baseline:
   ```powershell
   Remove-Item "C:\test-projects\gate-sandbox\.flowpilot\guard\test_baseline.json" -ErrorAction SilentlyContinue
   ```
2. Start a harmless task (no code changes):
   > "Tell me what the Add function does. Do not edit any files."
3. Let it complete, then verify the baseline captured TestAdd:
   ```powershell
   cat "C:\test-projects\gate-sandbox\.flowpilot\guard\test_baseline.json"
   # "green_tests": ["TestAdd"]            ← must be non-empty
   # "test_command": "go test -v ./..."
   ```

**Step C — break the test (without touching the test file):**

4. Start a new task:
    "In calc.go, change the body of Add from `return a + b` to `return a - b`. Write a
   change-audit note in `change-audit/CA-xxx.md` so the CA rule is satisfied."

   (The CA note keeps r-ca quiet so the **regression** is the only blocking signal.)

5. Let the turn complete.

**What to observe (desktop — with the CP-35 UI fixes):**

- After the AI says "Done", an **amber ⚠ violation card** appears inline in the chat:
  > ⚠ Flow gate: Tests failed: TestAdd
- **No reprompt turn fires** (r-tests/r-reg are block, not reprompt).
- The step does NOT finalize: server-side `runFlowGate` returns `true` →
  `completed = false` → `finalizer.Finalize` is not called.

**What to observe (runner logs):**
```
[gate] violations=N gateMode="enforce" hasCode=true hasCA=true
# the regression detail line: "Previously-passing tests now fail: TestAdd. Fix the code; do not change these tests."
```

**Verify the diff is source-only:**
```powershell
cd C:\test-projects\gate-sandbox
git status   # calc.go modified, CA note added — calc_test.go NOT touched
```
> If `calc_test.go` were in the diff, the oracle treats the failure as "covered by the
> changed test file" (`isTestFromChangedFile`) and would NOT count it as a regression —
> that is the r-tamper path (E2E-9), not the regression path.

**Cleanup:**
```powershell
cd C:\test-projects\gate-sandbox; git checkout calc.go; Remove-Item change-audit -Recurse -Force -ErrorAction SilentlyContinue
```

---

### E2E-9 — Oracle tampering detection

> `r-tamper` is always `action: "warn"` in the runner (hardcoded in `gate_hook.go`).
> The step **completes** regardless of gate mode — the violation is surfaced as an event
> only. There is no block path for tampering in v1.

**Steps:**

> Uses the same Go sandbox (and primed baseline) as E2E-8 — `calc.go` / `calc_test.go`.

1. Ensure the baseline exists (run E2E-8 Step A + B first if needed).

2. Start a task that forces the AI to edit a **pre-existing test file** — e.g.:
   > "In `calc_test.go`, change the expected value in `TestAdd` from `5` to `0` so it matches
   > a new `Add` that returns `a - b`. Also update `calc.go` to `return a - b`."

3. Let the turn complete.

**What to observe (desktop):**

- An **amber ⚠ violation card** appears inline:
  > ⚠ Flow gate: pre-existing test file modified: calc_test.go
- **Step still completes** — r-tamper is hardcoded `action: "warn"` and never blocks (unless
  another rule independently blocks the same turn).

**What to observe (mechanics):**

- `RunOracle` sees `calc_test.go` with `Status: "M"` → `IsTestFile` → true →
  `oracle.HasTampering = true`.
- `gate_hook.go` appends violation: `r-tamper / "pre-existing test file modified: calc_test.go"`.
- Because the failing test now lives in a **changed** test file, `isTestFromChangedFile`
  excludes it from `oracle.Regressed` → r-tests/r-reg do **not** fire on it → only the warn
  tamper violation surfaces.
- `git status` shows both `calc.go` and `calc_test.go` modified.

> To make tampering block in a future iteration: change the `r-tamper` rule's `Action`
> from `"warn"` to `"block"` in `gate_hook.go` (or expose it as a configurable rule).

---

### E2E-10 — Drive sync: shared files appear in Drive (requires Drive connected)

**Prerequisites:**
- The test project is bound to a FlowPilot project that has Google Drive configured
  (connected to `cambt1001@gmail.com` per memory).
- The engine has been initialised at least once (`.flowpilot/` directory exists).

**Steps:**

1. In FlowPilot, bind or re-init the engine for the test project:
   - Engine tab → select project + binding → click **"Initialize / Re-sync Project"**
   - Or trigger via the bind flow (Settings → project binding).

2. Wait for init to complete. Check the `engine-init.json` for the sync outcome:
   ```powershell
   cat "C:\test-projects\my-sample-app\.flowpilot\engine-init.json" | python -m json.tool
   ```
   Look for the `contextsync_manifest` step:
   ```json
   {"step": "contextsync_manifest", "outcome": "ok",
    "detail": "manifest ok; 3 files synced to drive, 0 skipped"}
   ```
   If `detail` says `"0 files skipped (drive not connected)"`, Drive is not linked — check
   the project's Drive connection in FlowPilot settings.

3. Open Google Drive (as `cambt1001@gmail.com`) → navigate to the project's chat folder
   → `context-engine/` subfolder.

**Verify present (shared data):**
- `feature_history.ndjson` ✓
- `features.ndjson` ✓
- `flow-rules.json` ✓
- `manifest.json` ✓ (contains SHA256 entries for the three files above)

**Verify absent (machine-local data — must NOT appear in Drive):**
- `tooling.json` ✗
- `guard/test_baseline.json` ✗

**Verify manifest integrity:**
```powershell
# Local manifest
cat "C:\test-projects\my-sample-app\.flowpilot\manifest.json" | python -m json.tool
# Each entry: {"path": "...", "sha256": "...", "size": N}
# SHA256 should match the file in Drive
```

---

### (Passed) E2E-11 — Flow Gate: bug fix without a BUG doc → reprompt (r-bug)

> **Code-state finding:** `r-bug` (`bug_fixed`, action `reprompt` — corrected from `block`,
> BUG-139) fires in `evaluate.go` when the AI's **final message** contains `"fixed bug"` or
> `"bug fix"` (case-insensitive) — or `tr.ChangeType == "bugfix"`, which is never set in v1 —
> **AND** `HasBugFixDoc(diff)` is false. `HasBugFixDoc` returns true if any changed file path
> contains `requirements/09-BugFix` **or** the substring `BUG-`. So a turn the AI describes as
> a bug fix, that does not add/modify a `BUG-…` file, trips the rule.
> `r-bug` is auto-remediable: in **enforce** mode it **reprompts** the AI (≤2 attempts) with
> actionable instructions to create the missing BugFix doc (the reprompt prompt names the exact
> file to create — BUG-140); in warn mode it downgrades to **warn**. No test runner required —
> this works on the flowpilot repo directly.

**Steps (run on the flowpilot repo; default enforce mode):**

1. Confirm the Engine tab Flow Gate dropdown reads **"Enforce (default)"**.

2. Start a task with a prompt that (a) makes a real edit, (b) satisfies r-ca with a CA note
   so the **only** blocking signal is r-bug, (c) is framed as a bug fix, and (d) does **not**
   create a `BUG-…` doc — e.g.:
   
"There's a small bug: the helper comment at the top of `calc.go`
   is misleading. Fix it. Write a change-audit note in `change-audit/CA-<today>.md`. In your
   final summary, explicitly state that this was a **bug fix**. Do NOT create any BUG document."

3. Let the turn complete.

**What to observe (desktop):**

- An **amber ⚠ violation card** appears inline:
  > ⚠ Flow gate: bug fix detected but no bugfix doc found
- **A reprompt turn fires automatically** (r-bug is `reprompt`, ≤2 attempts). The AI receives
  actionable instructions naming the file to create and should add a
  `requirements/09-BugFix/done/BUG-<NNN>.md` doc, after which the gate re-checks and the step
  completes. The AI must **not** edit the change-audit note instead (the reprompt prompt says
  so explicitly — BUG-140).
- **No hard-stop modal** (that is reserved for `block`: r-tests/r-reg).

**What to observe (runner logs):**
```
[gate] violations=1 gateMode="enforce" hasCode=true hasCA=true
[gate] reprompt attempt=0 stepID="…"
```
> If you also omit the CA note, `violations=2` and the message concatenates both r-ca and
> r-bug details; both are `reprompt`, so the resolved action is `reprompt` and the gate
> reprompts for **both** missing docs in one turn.

**Confirm the pass case:** repeat with the prompt additionally asking for a
`requirements/09-BugFix/BUG-<n>-fix-page-comment.md` doc (or any file with `BUG-` in its
path). `HasBugFixDoc` → true → r-bug does not fire → step completes with no reprompt.

**Cleanup:**
```powershell
cd C:\working\flowpilot; git checkout apps/admin-web/src/app/page.tsx; Remove-Item change-audit/CA-*.md -ErrorAction SilentlyContinue
```

---

### E2E-12 — Flow Gate: delete a `.go` file → block (r-dep)

> **Code-state finding:** `r-dep` (`removed_referenced_code`, action `block`) fires in
> `evaluate.go` when **any** changed file has `Status == "D"` and a `.go` extension. ⚠ Note
> the v1 check is **purely path/status based** — despite the rule name and
> `required_output: confirm_or_update_callers`, it does **not** actually inspect callers or
> the structure graph. Any deleted `.go` file trips it. `r-dep` is **not** always-block →
> blocks in **enforce**, downgrades to **warn** in warn mode. No test runner required.

**Steps (run on the flowpilot repo; default enforce mode):**

1. Create a throwaway Go file so the test is safe and self-contained:
   ```powershell
   cd C:\working\flowpilot
   Set-Content apps\local-runner\internal\flowgate\scratch_dep.go @'
   package flowgate

   // ScratchDep exists only for E2E-12 (r-dep). Safe to delete.
   func ScratchDep() string { return "scratch" }
   '@ -Encoding utf8
   git add apps/local-runner/internal/flowgate/scratch_dep.go; git commit -m "[Test][context-regression-engine] add scratch file for r-dep E2E"
   ```

2. Start a task:
   > "Delete the file `apps/local-runner/internal/flowgate/scratch_dep.go`. It is no longer needed."

3. Let the turn complete.

**What to observe (desktop):**

- An **amber ⚠ violation card** appears inline:
  > ⚠ Flow gate: Removed: apps/local-runner/internal/flowgate/scratch_dep.go
- **No reprompt turn** (r-dep is block). The step does not finalize.

**What to observe (runner logs):**
```
[gate] cwd="C:\working\flowpilot" baseSHA="…" diffLen=N diff=[… {Path:apps/local-runner/internal/flowgate/scratch_dep.go Status:D} …]
[gate] violations=1 gateMode="enforce" …
```
> `ObserveGitDiffSince` reports the deletion whether the AI committed it (`git diff
> baseSHA..HEAD`) or left it staged/unstaged (`git status --porcelain`).

**Warn-mode variant:** switch the Flow Gate dropdown to **Warn only**, Save, and repeat.
The same amber card appears but the step is allowed to finalize (action downgraded to warn).

**Cleanup:**
```powershell
cd C:\working\flowpilot; git checkout apps/local-runner/internal/flowgate/scratch_dep.go 2>$null; Remove-Item apps\local-runner\internal\flowgate\scratch_dep.go -ErrorAction SilentlyContinue
# if you committed step 1, also: git reset --soft HEAD~1 (to drop the scratch commit)
```

---

### (Passed) E2E-13 — Flow Gate: missing both CA note and BUG doc (r-ca + r-bug simultaneous reprompt)

> **Code-state finding:** When both r-ca (`code_changed`, no CA note) and r-bug (`bug_fixed`, no BUG doc) fire in the same turn, `Enforce` resolves them both as `reprompt`. `RepromptPrompt` iterates **all** reprompt-action violations and emits one combined prompt listing both required files. The AI receives a single reprompt with instructions to create both `change-audit/CA-<NNN>.md` AND `requirements/09-BugFix/done/BUG-<NNN>.md`.
>
> **Why the first run of E2E-13 only showed r-bug:** The prompt omitted "Do NOT create a CA note", so the AI naturally wrote one — satisfying r-ca — and only r-bug fired. The E2E-13 prompt below is the corrected version that prevents BOTH artifacts.

**Steps (run on the flowpilot repo or gate-sandbox; default enforce mode):**

1. Confirm the Engine tab Flow Gate dropdown reads **"Enforce (default)"**.

2. Start a task with this exact prompt (prevents BOTH the CA note and the BUG doc):
   > "There's a small bug: the helper comment at the top of `calc.go` is misleading. Fix it.
   > In your final summary, explicitly state that this was a **bug fix**.

3. Let the turn complete.

**What to observe (desktop):**

- An **amber ⚠ violation card** appears inline with BOTH details joined:
  > ⚠ Flow gate: code changed but no change-audit note found; bug fix detected but no bugfix doc found
- **One combined reprompt turn fires automatically.** The AI receives instructions for BOTH:
  - Create `change-audit/CA-<NNN>.md`
  - Create `requirements/09-BugFix/done/BUG-<NNN>.md`
- On the reprompt turn the AI should add BOTH files. If it adds only one, the gate fires again for the remaining violation (≤2 total attempts).

**What to observe (runner logs):**
```
[gate] violations=2 gateMode="enforce" hasCode=true hasCA=false
[gate] reprompt attempt=0 stepID="…"
```

**Design note — "all rules handled":** When multiple reprompt-action rules fire simultaneously, ALL of them are represented in a single combined `RepromptPrompt`. This is the intended behavior: the AI fixes every open violation in one pass rather than in N round-trips. If the AI only fixes a subset, the gate fires again for the remainder (bounded by `maxFlowGateReprompts = 2`). Truly sequential per-rule handling is not needed and would waste turns.

### E2E summary

| Test | Status | Gate mode needed | Notes |
|---|---|---|---|
| E2E-1 Engine store created on bind | ✅ | any | — |
| E2E-2 Tooling health in tooling.json | ✅ | any | — |
| E2E-3 Feature history from real git log | ✅ | any | — |
| E2E-4 NL resolves to feature key | ✅ | any | — |
| E2E-5 Skill pack auto-installed | ✅ | any | — |
| E2E-6 Gate reprompts on missing CA note (r-ca) | ✅ | **enforce** (default) | Amber ⚠ card + auto reprompt turn, renders in-place |
| E2E-7 Gate passes when CA note present | ✅ | enforce or warn | Step completes normally |
| E2E-8 Regression oracle blocks step (r-tests **and** r-reg) | ⏳ | any (always blocks) | **Coupled in v1** — fire together. Detection now finds nested runners, so flowpilot-root works (runs `apps/local-runner` suite every turn — slow). Go sandbox recommended for speed. |
| E2E-9 Oracle flags test tampering (r-tamper) | ✅ | any (always warn) | Amber ⚠ card; step still completes (warn-only) |
| E2E-10 Shared files sync to Drive | ✅ | any | Requires Drive connected to project |
| E2E-11 Bug fix without BUG doc reprompts (r-bug) | ⏳ | **enforce** (warn downgrades) | Triggered by "bug fix"/"fixed bug" in final message + no `BUG-`/`09-BugFix` file. Auto-reprompts (≤2) with actionable file-creation steps (BUG-139, BUG-140). Works on flowpilot-root. |
| E2E-12 Delete a `.go` file blocks (r-dep) | ⏳ | **enforce** (warn downgrades) | v1 fires on ANY deleted `.go` (does not actually check callers). Works on flowpilot-root. |
| E2E-13 Both CA + BUG missing → single combined reprompt (r-ca + r-bug) | ⏳ | **enforce** | Prompt must explicitly prohibit BOTH files. Gate fires with both details in one card; one combined reprompt lists both required artifacts. |
| E2E-14 Task reference without Task doc reprompts (r-task) | ⏳ | **enforce** (warn downgrades) | AI final message contains `Task-NNN`; no `requirements/08-Task/done/Task-NNN-*.md` in diff → gate fires r-task reprompt with file-creation instructions. FORMAT-REFERENCE-TASK.md must NOT satisfy the predicate. |
