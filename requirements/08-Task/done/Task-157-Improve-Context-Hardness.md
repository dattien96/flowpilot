# Task-157: Feature-Key Accuracy For History Context

## Metadata

- Document ID: `Task-157`
- Title: `Feature-Key Accuracy For History Context`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-161: Per-Feature Chat-Summary Timeline](Task-161-Per-Feature-Chat-Summary-Timeline.md)
- Related Documents: [CP-37: Prompt Context Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md), [Task-096: Commit-History Ledger](../done/Task-096-Commit-History-Ledger.md), [Task-097: Feature Catalog And Resolver](../done/Task-097-Feature-Catalog-And-Resolver.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [Task-078: Cross-Provider Chat Handoff](Task-078-Cross-Provider-Chat-Handoff.md)
- Replaces: `None`
- Tags: `changeledger, featurecatalog, feature-key, history-context, history-injection, ca-note, two-tier-history, git-commit-format, gate, accuracy`

## AI Quick View

### Summary

- **The whole subsystem answers exactly two questions, and nothing more:** **Q1 — "what feature are we working on?"** (the Feature Resolver, [resolve.go](../../../apps/local-runner/internal/featurecatalog/resolve.go)) and **Q2 — "how was this feature changed before, and why?"** (the ordered history lookup, [slots.go](../../../apps/local-runner/internal/featurecatalog/slots.go), newest entry = current truth). Both answers pivot on **one value being correct: `feature_key`.** A noisy key makes *both* answers wrong, which can mislead the AI **more** than no context at all.
- **The pipeline that produces the key:** `commit` → `git-commit-format` skill (writes `[Type][feature][layer]`) → `changeledger/parse.go` (reads bracket-2 as the key) → `changeledger/enrich.go` (priority chain fills the key when the bracket is absent) → NDJSON ledger → `featurecatalog` (Resolver + `HistorySlot`) → **(should be)** injected into the AI prompt.
- **Finding A — the consumer is not wired (the silent gap).** `HistorySlot` and `ResolveFeature` have **zero live callers**: `engine_setup.go` / `ledger_live.go` only *build* the ledger and catalog on bind and on every commit. The packed "prior work" block is **never injected into any prompt.** Today the subsystem is pure cost (build/rebuild) with no payoff. Key accuracy only matters once something reads the history, so **wiring the consumer is a prerequisite, not an afterthought.**
- **Finding B — any bracket is trusted as truth (the correctness bug).** [parse.go:105](../../../apps/local-runner/internal/changeledger/parse.go) stamps **any** non-empty bracket-2 as `Confidence=high`, and [enrich.go:41](../../../apps/local-runner/internal/changeledger/enrich.go) returns it immediately **without checking it against `FEATURE-KEYS.md`.** So a typo (`chatui` vs `chat-ui`) or an invented synonym becomes "authoritative" and silently fragments a feature's history.
- **Finding C — the assist path is unbuilt.** `featurecatalog.Feature.FileGlobs` exists but is **never populated** ([catalog.go](../../../apps/local-runner/internal/featurecatalog/catalog.go) seeds only `Key`/`Summary`/`Keywords`), so resolve.go's `file_globs` scoring branch scores against an empty list. The path-anchored suggestion ("you touched these files → this is the feature") is a design that **must be built**, not something already running.
- **Finding D — the Q2 answer is too thin (content gap).** The ledger `Entry` stores only `summary = cleanSummary(subject)` — a **one-line commit subject** ([ledger.go:25](../../../apps/local-runner/internal/changeledger/ledger.go)). So `HistorySlot` answers "*what* changed, in order" but never "*why*, and what was deliberately left undone." The richer "why" already exists in `change-audit/CA-*.md` notes (`Scope` / `Completed` / **`Residual Notes`**), and [enrich.go `parseCABlock`](../../../apps/local-runner/internal/changeledger/enrich.go) already **opens every CA note** — but extracts only the `feature_key`/`source_doc_id` and **throws the prose body away.** The repo's best regression-avoidance context is in hand and dropped on the floor.
- **The fix is not RAG, and CA notes make Plane-A unnecessary here.** Plane C is a deterministic, ordered, git-cheap key lookup because **recency = truth** is an ordering property similarity search cannot provide (`SS-14` line 72, `SD-17 D-4`). The "why" that RAG/Plane-A (`SD-10`, semantic chat-summary recall) was imagined to supply is, for *this* job, **already captured durably in the CA note** — and the `r-ca` gate guarantees a CA note exists for every task/bugfix turn under `gate_mode: enforce`. So Plane-A RAG is **not needed for Task-157**: the job is to make the **key** reliable (consistent, validated, assisted, gated, measured) and to surface the CA "why" as a second history tier.

### Current Ask

- The `feature_key` continuity stack is now implemented end to end: prompt-assembly injects the ordered history block, commit keys are validated against the registry, changed paths feed key assistance, the missing-key gate reprompts, the confidence metric is recorded, and the CA excerpt carries the "why" alongside commit subjects.

### Key Decisions

- `T-0` **Wire the consumer first (prerequisite).** Add a per-turn feature-history injection at the prompt-assembly seam ([runner.go:903](../../../apps/local-runner/internal/runner/runner.go), sibling to `injectSkillContent`): resolve the feature from the turn (prompt + changed paths) → load the ledger → `HistorySlot` → prepend the "## Prior work …" block. Without this, every other item below polishes data nobody reads.
- `T-1` **Validate the bracket against the registry (correctness).** In `parse.go`/`enrich.go`, only treat bracket-2 as `Confidence=high` when the key **exists in `FEATURE-KEYS.md`** (or a CA `§13` block). An unknown key is **"unverified," not "wrong"** — it loses high-confidence and falls through to inference, and is flagged for assist/gate. This closes Finding B without rejecting genuinely-new features.
- `T-2` **Assist with a path-anchored suggestion.** Populate `Feature.FileGlobs` (learn paths-per-key from ledger history, reusing `enrich.go` `topLevelKey`/`pathFeatureKey`; seed from module dirs), then expose `SuggestKey(changedPaths, message)` that ranks candidates **deterministically from the files actually touched** — not from the AI's typed bracket. The AI confirms the top candidate instead of inventing one.
- `T-3` **Promote the contract from soft to hard (gate).** Add a `flowgate` trigger (`commit_feature_key_missing`): on a code-changing turn whose commit lacks a `[feature]` bracket or uses an unverified key, **reprompt** (not warn-only) with the `SuggestKey` shortlist **and** an explicit "or register a new key" path — never a bare reject. Bounded to the existing ≤2 reprompt budget, honors `gate_mode`.
- `T-4` **Curate the registry.** Grow `FEATURE-KEYS.md` beyond its 8 seeds; honor `superseded by` so the resolver never offers a retired key as a fresh candidate. New keys enter as a reviewable git diff — consistency over perfect naming (**same feature → same key** is the goal, not the "ideal" name).
- `T-5` **Measure.** Emit a key-confidence metric (% commits high-confidence validated bracket vs path-inferred low-confidence) to engine status / audit log, so the value of history retrieval is observable and the gate's effect is visible over time.
- `T-7` **Make the Q2 answer two-tier (content depth).** Keep the commit subject as the always-on **Tier 1** (pure git, works on any repo per `BR-4`). Add **Tier 2**: when a commit's `source_doc_id` maps to a CA note, surface a short CA excerpt (the `Scope` one-liner + the `Residual Notes`) under that entry in `HistorySlot`, so the AI sees *why* and *what was left undone*, not just *what*. Deterministic, git-synced, no embeddings. Tier 2 **degrades gracefully** to Tier 1 when no CA note exists (e.g. under `gate_mode: warn`, where `r-ca` is downgraded). (Numbered `T-7` to keep `T-6 = tests`.)

### Constraints

- Do not break the existing ledger parse/enrich priority chain (`CP-35 §4.1`); add the consumer, validation, assistance, and enforcement **around** it.
- Keep `FEATURE-KEYS.md` git-synced (committed repo file), not Drive-synced — only the derived catalog cache syncs (`CP-35 §4.8`, `SS-13 §13`).
- Injection (`T-0`) and enforcement (`T-3`) must be **non-fatal and bounded**: a ledger/catalog hiccup never blocks a turn (match the silent-swallow pattern in `ledger_live.go`); the gate reuses the ≤2 reprompt budget like `r-ca`.
- Plane C stays **vector-free** (`SD-17 D-4`): lexical + path scoring + LLM/AI confirm only. No embeddings in the history path.
- History retrieval stays **ordered and deterministic** (newest = current truth); never reorder by relevance.
- **Reserve a timeline seam for [Task-161](Task-161-Per-Feature-Chat-Summary-Timeline.md) (phase 2).** Design the per-feature history store and `HistorySlot` so a second entry type (`chat_summary` — a time-ordered summary of past *chats* about the feature, distinct from this *commit* history) can be added without restructuring. Phase 1 ships the commit timeline; phase 2 adds the discussion timeline beside it, keyed by the same `feature_key`. Keep the store and renderer entry-type-extensible.
- The Tier-2 CA excerpt (`T-7`) is **token-bounded**: cap each excerpt (default `Scope` first line + `Residual Notes`, ≤ ~400 chars) and inject excerpts only for the **most recent K entries** (default `K=3`); older entries keep Tier-1 subjects only. This caps the injected block regardless of how long the feature's history is.
- **Relationship to [Task-078](Task-078-Cross-Provider-Chat-Handoff.md) — keep both, decoupled.** The two solve *different* context needs and are both required: Task-078 transfers **what was said in a chat thread** (the conversation transcript) when the user switches provider; Task-157 injects **what changed in code for a feature** (commit history + CA "why") on every turn. Neither's content substitutes for the other's — commit history is not a conversation, and a transcript is not the code-change truth. Both write bounded context at the **same prompt-assembly seam** ([runner.go:903](../../../apps/local-runner/internal/runner/runner.go)), so a handoff **target** run's first turn receives Task-157's history **automatically** (no wiring between the tasks). Extract **one shared bounded-context-block helper** (UTF-8-safe truncation, escaping, omission markers); Task-157 reuses Task-078's budgeting discipline (`Task-078 T-6`/`T-7`). Combined-budget awareness: on a handoff first turn both blocks co-exist — Task-157's caps (`K=3`, ~400 chars) keep its block small so the total stays predictable.

### Open Questions

- `Q-1` Resolved (proposal) — the feature-key gate check is **`reprompt`**, `gate_mode`-gated (downgraded to warn under `gate_mode: warn`, like `r-ca`/`r-bug`). Warn-only as the default was rejected: the soft skill is already advisory, so warn changes nothing.
- `Q-2` Where does `T-0` resolve the feature from — the user prompt text only, the changed paths only, or both? (Proposal: both — prompt for Q1 intent, changed paths as the tiebreaker/confirmation, same signal `SuggestKey` uses.)
- `Q-3` Where does key auto-suggestion run for the gate — a commit-time hook the skill calls, or inside the runner before the gate evaluates? (Proposal: runner-side, surfaced into the reprompt text.)
- `Q-4` Acceptance bar for "history is beneficial": a target high-confidence-key ratio, or a qualitative review of injected history on N real chats? (Proposal: both — ratio as the leading metric, qualitative spot-check as the gate to invest further.)
- `Q-5` Should the resolver normalize near-synonym keys automatically, or only flag them for human registry curation? (Proposal: flag + suggest the canonical key via the gate; do not auto-rewrite the committed message.)

### Source Refs

- `SS-14 AC-3` (history awareness), `AC-10` (NL resolution), `BR-6` (runner gate is source of truth), line 72 (why RAG does not solve the historical question); `SD-17 §3.2`/`§3.3` (ledger + catalog), `D-4` (lexical + LLM pick, no vector); `CP-35 §4.1` (enrich priority), `§4.2` (planned resolver wiring), `§4.6` (`git-commit-format`).
- Code: `apps/local-runner/internal/changeledger/{parse.go,enrich.go,ledger.go}`, `apps/local-runner/internal/featurecatalog/{resolve.go,slots.go,catalog.go}`, `apps/local-runner/internal/runner/{runner.go,engine_setup.go,ledger_live.go}`, `apps/local-runner/internal/flowgate/{rules.go,evaluate.go,enforce.go}`, `apps/local-runner/internal/skillpack/flow-pack/common/git-commit-format/SKILL.md`, `change-audit/FEATURE-KEYS.md`, `change-audit/CA-*.md` (Tier-2 source, e.g. `CA-078` `Residual Notes`).

## 1. Goal

Make the `feature_key` behind the subsystem's two questions — **Q1 "what feature are we working on?"** and **Q2 "how was it changed before, and why?"** — reliably correct, and make it **actually reach the AI**. Today the key is enforced only by a soft skill the AI may ignore, validated against nothing, and the resulting history is injected into no prompt at all. The goal is: the consumer is wired, the key is validated against a curated registry, its selection is assisted from the files actually changed, its absence is gated, and its confidence is measured — so "build on the newest prior work" (`AC-3`) is trustworthy rather than a coin-flip.

## 2. Parent Links

- coding plan: `CP-35` §4.1 (P-1 enrich), §4.2 (P-2 resolver wiring — the gap this task closes), §4.6 (P-6 commit format)
- tech design: `SD-17` §3.2 (ledger), §3.3 (catalog + resolver), `D-4` (no-vector decision)
- system spec: `SS-14` `AC-3`, `AC-10`, `BR-6`
- specific upstream ids: hardens `AC-3` delivery from Task-096/Task-097

## 3. Trigger

Feature-keyed history is the mechanism behind `AC-3`. A trace of the live runner surfaced four problems that make it unreliable today:

1. **It is never consumed.** The ledger and catalog are rebuilt on bind and on every commit, but `HistorySlot`/`ResolveFeature` are called from no live path — the "prior work" block reaches no prompt. (Finding A)
2. **A wrong key is trusted as truth.** Any non-empty `[feature]` bracket is stamped high-confidence and used without checking the registry, so a typo or synonym silently fragments a feature's history. (Finding B)
3. **The assist that would prevent wrong keys is unbuilt.** `file_globs` is never populated, so the path-anchored suggestion that maps changed files → feature does not exist yet. (Finding C)
4. **The answer is too thin.** Even when the key is right, history renders one-line commit subjects only; the "why" and "what was left undone" — already written in the CA notes — is discarded. (Finding D)

When the key is wrong or missing, the ledger collapses changes into coarse path buckets, and — once the consumer is wired — the AI would be fed misleading "prior work." We need the history to actually reach the AI, the key it is filed under to be dependable, and the rendered answer to carry the CA "why."

### 3.1 How the pipeline runs (the flow to harden)

```text
                         ┌─────────────────────────── HARDEN HERE ───────────────────────────┐
commit  ──►  git-commit-format skill  ──►  parse.go        ──►  enrich.go       ──►  NDJSON ledger
(author)     writes [Type][feature]       reads bracket-2       priority chain        feature_history
             [layer]; advisory only       = feature_key         fills key when         (ordered, append)
                                           (T-1: validate         bracket absent
                                            vs registry)          (T-1: validate)
                                                                                            │
                                                                                            ▼
   AI turn  ◄── prompt + "## Prior work…" block ◄── HistorySlot ◄── Resolver (Q1) ◄── featurecatalog
   (T-0: inject at runner.go:903)        (slots.go, Q2)        (resolve.go,        (catalog.go;
                                          newest = truth         lexical + path)     T-2: populate file_globs)

   gate (T-3): on a code commit with missing/unverified key → reprompt with SuggestKey shortlist or "register new"
```

- **Q1 "what feature?"** is answered by the **Resolver** (lexical keyword/title scoring today; path-anchored after `T-2`). Its output is a `feature_key` (auto-selected above threshold, else AI picks from the ranked shortlist — `SD-17 §3.3`).
- **Q2 "how changed, why?"** is answered by `HistorySlot(feature_key)`: a direct, ordered key lookup over the ledger, rendered oldest→newest with the last entry marked `← current truth`. **Two tiers** (`T-7`): Tier 1 = commit subject (always); Tier 2 = a CA excerpt (`Scope` + `Residual Notes`) for the most recent entries, carrying the *why* and *what-was-left-undone*.

### 3.2 Runtime: what runs and when (the four moments)

Worked example — the AI is asked *"add slash-command autocomplete to the chat input"* on a repo with 3 prior `chat-ui` commits. The subsystem touches four moments in a session:

1. **Bind / engine init** (`engine_setup.go`, once per bind — **already runs today**). `changeledger.Build` walks `git log` (cursor-based) → `parse.go` → `enrich.go` → appends ordered entries to `.flowpilot/ledger/feature_history.ndjson`. `featurecatalog.Build` reads `FEATURE-KEYS.md` + SS/CP docs + ledger keys → writes `.flowpilot/catalog/features.ndjson`. The `chat-ui` history exists on disk **before the turn starts** — it is simply never read.
   - *`T-2` adds here:* fill `Feature.FileGlobs` from the ledger's historical paths-per-key (e.g. `chat-ui` → `apps/admin-web/src/chat/**`). Empty today.

2. **Turn start** ([runner.go:903](../../../apps/local-runner/internal/runner/runner.go), **every prompt** — this is where history is missing today). Today: `injectSkillContent` → `preparePromptForRequiredMcps` → `prompt.txt` → exec provider. `HistorySlot` is never called, so the AI gets the task with no memory of `chat-ui`.
   - *`T-0` adds here (prerequisite):* `injectFeatureHistory(workspace, prompt)` beside `injectSkillContent` → `LoadCatalog` → **Q1** `ResolveFeature(prompt)` → `chat-ui` → **Q2** `HistorySlot("chat-ui")` → prepend the "## Prior work … ← current truth" block. With `T-7`, the newest entries also carry a Tier-2 CA excerpt (e.g. *"Residual Notes: replayed run history still depends on the event-stream payload shape and was not expanded here"*), so the AI sees the trap the prior turn already mapped. Deterministic local file reads (no model/network, non-fatal). Proof: the block appears in `.flowpilot/runs/<id>/prompt.txt`. Now the AI builds on the newest entry instead of undoing it.

3. **Commit** (`git-commit-format` skill → `parse.go` → `enrich.go`, when work is done). The AI writes `[Feature][chat-ui][ui] …`. If it typos `[chatui]`: **today** `chatui` is stamped high-confidence and filed as a new feature → history silently splits. *`T-1`* refuses high-confidence for an unregistered key (falls through to inference); *`T-3`* reprompts before the turn closes — *"`chatui` is not registered; you changed `apps/admin-web/src/chat/**` — did you mean `chat-ui`? Use it, or register a new key."* The suggestion comes from `SuggestKey(changedPaths)` (scored from files touched, not the typed bracket). Fixed within the ≤2 reprompt budget, before it pollutes the ledger.

4. **Post-commit** (`ledger_live.go`, after each commit — **already runs today**). The `.git/hooks/post-commit` sentinel triggers `rebuildLedgerIfDirty` → incremental `changeledger.Build` + `featurecatalog.Build`, so the just-made commit is in the ledger before the next turn (no session restart). *`T-5`* adds the high- vs low-confidence counter here.

> **Model:** *build time* writes the data; *turn time* (`T-0`) finally reads it to answer Q1+Q2 into the prompt; *commit time* (`T-1`+`T-3`) keeps the key honest; *post-commit* keeps it fresh. The only "intelligence" is the AI **confirming** a code-ranked shortlist — never inventing from memory — which is why Plane C is cheap, git-only, and deterministic.

## 4. Exact Change

- `T-0` `runner` (injection — **prerequisite**) — add `injectFeatureHistory(workspace, prompt)` invoked at the prompt-assembly seam ([runner.go:903](../../../apps/local-runner/internal/runner/runner.go), beside `injectSkillContent`): `LoadCatalog` → resolve the feature from prompt + changed paths → load the ledger (`changeledger.New`) → `HistorySlot` → prepend the block to the prompt. Non-fatal: any error returns the prompt unchanged (match `ledger_live.go`'s silent-swallow). Result is observable in the run's `prompt.txt`.
- `T-1` `changeledger` (validation) — pass the loaded `knownKeys` (already read in `EnrichAll`) into the bracket decision: in `parse.go`, stop unconditionally setting `Confidence=high`; in `enrich.go`, only short-circuit (priority 0) when `FeatureKey` is non-empty **and** present in `FEATURE-KEYS.md` (or matched by a CA `§13` block). An unknown bracket key is retained as a hint but downgraded so the inference chain (CA → keyword → path → doc-id) still runs and the gate can flag it.
- `T-2` `featurecatalog` (assist) — (a) populate `Feature.FileGlobs` in `catalog.Build`: derive per-key path globs from the ledger's recorded commits (which files were historically committed under each key) plus the module-dir mapping in `enrich.go` `topLevelKey`; (b) expose `SuggestKey(changedPaths []string, message string) []Candidate` reusing `resolve.go` scoring, weighting `file_globs` matches against the changed paths; feed the top candidate(s) into the gate reprompt text.
- `T-3` `flowgate` (enforcement) — add a `commit_feature_key_missing` trigger in `rules.go`/`evaluate.go`: on a code-changing turn whose commit(s) lack a `[feature]` bracket or use an unverified key, emit a `reprompt` violation whose `remediationFor` text (in `enforce.go`) lists the `SuggestKey` candidates **and** the "register a new key in `FEATURE-KEYS.md`" path. Reuse the ≤2 reprompt budget; honor `gate_mode` (downgrade to warn under `gate_mode: warn`).
- `T-4` `FEATURE-KEYS.md` + `audit-logging`/`git-commit-format` skills — make registry growth explicit on bind/build and cross-reference the new gate from the skill; ensure the resolver and `loadKnownKeys` honor `superseded by <new-key>` (a retired key is not offered as a fresh candidate and resolves to its successor).
- `T-5` telemetry — count high-confidence (validated bracket / CA) vs low-confidence (inferred) ledger entries during `changeledger` build; expose the ratio via engine status / audit log.
- `T-7` `changeledger` + `featurecatalog` (Tier-2 content) — extend `parseCABlock` to also capture a capped CA excerpt (`Scope` first line + `Residual Notes`), build a `source_doc_id → excerpt` map alongside the existing `source_doc_id → feature_key` index, and have `HistorySlot` append the excerpt under entries whose `source_doc_id` matches — for the most recent `K=3` entries only, each ≤ ~400 chars. Tier 1 (commit subject) always renders; absence of a CA note is a silent no-op (graceful degrade).
- `T-6` tests — see §6.1 (Definition of Done) and §6.2 (End-to-End). Unit coverage: `T-0` injects the history block when a feature resolves and is a no-op when none does; `T-1` an unknown bracket is **not** high-confidence and falls through; `T-2` `SuggestKey` ranks the right key from changed paths and `file_globs` is populated; `T-3` the gate fires on missing/unknown key and passes on a verified one; `T-4` a `superseded by` key is not offered; `T-5` the confidence ratio counts correctly; `T-7` a CA excerpt is appended for a linked entry, capped, and omitted when no CA note exists.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/{runner.go,engine_setup.go}` (injection seam), `apps/local-runner/internal/featurecatalog/{resolve.go,catalog.go,slots.go}` (`slots.go` = Tier-2 render), `apps/local-runner/internal/changeledger/{parse.go,enrich.go,ledger.go}` (`enrich.go parseCABlock` = CA excerpt), `apps/local-runner/internal/flowgate/{rules.go,evaluate.go,enforce.go}`, `git-commit-format/SKILL.md` + `audit-logging` skill (cross-reference the new gate), `change-audit/FEATURE-KEYS.md`
- modules: `runner`, `featurecatalog`, `changeledger`, `flowgate`, `skillpack`
- routes: `flow_gate_violation` (new `commit_feature_key_missing` trigger); engine status / audit-log key-confidence metric
- tables: none (registry is git-synced; catalog cache local/Drive per `CP-35 §4.8`)

## 6. Acceptance Check

- **(T-0)** On a turn where a feature resolves, the assembled `prompt.txt` contains the "## Prior work on …" block ending in `← current truth`; on a turn where no feature resolves, the prompt is unchanged. This is the prerequisite — verified before key-accuracy work is judged.
- **(T-1)** A commit with `[feature][chatui]` (typo, not in registry) is **not** stamped high-confidence and falls through to inference; a commit with a registered key (e.g. `chat-ui`) stays high-confidence.
- **(T-2)** `SuggestKey` returns the key whose `file_globs` best match the changed paths on a few real commits in this repo; `file_globs` is non-empty in the built catalog.
- **(T-3)** A code-changing turn whose commit omits the `[feature]` bracket or uses an unknown key is reprompted with a concrete suggested key **and** the register-new option; a commit with a verified registered key passes. Under `gate_mode: warn`, it warns instead of reprompting.
- **(T-4)** `FEATURE-KEYS.md` growth/retirement is reflected by the resolver (a `superseded by` key is not offered as a fresh candidate).
- **(T-5)** The key-confidence ratio is observable and rises after the gate is active.
- **(T-7)** When a resolved feature's recent commit links to a CA note, the injected block carries a capped CA excerpt (`Scope` + `Residual Notes`) under that entry; when no CA note exists, only the Tier-1 subject renders (no error).
- The existing parse/enrich priority chain still works on a no-spec repo from git alone (`AC-2` not regressed).
- `go test ./internal/{runner,flowgate,featurecatalog,changeledger}/...` passes.

### 6.1 Definition of Done (DOD)

All items are true for Task-157:

- [x] **DOD-1 (T-0 wired):** `injectFeatureHistory` is called in the live prompt-assembly path ([runner.go:903](../../../apps/local-runner/internal/runner/runner.go)); a real session turn that resolves a feature writes the "## Prior work …" block into `.flowpilot/runs/<id>/prompt.txt`.
- [x] **DOD-2 (T-0 safe):** injection is non-fatal — a missing/corrupt catalog or ledger returns the original prompt unchanged and never blocks or errors the turn (verified by a fault-injection unit test).
- [x] **DOD-3 (T-1 validation):** an unregistered/typo bracket key is **not** `Confidence=high` and the inference chain still runs; a registered key stays high; a CA `§13` match still upgrades. No regression to the priority order in `CP-35 §4.1`.
- [x] **DOD-4 (T-2 globs):** `Feature.FileGlobs` is non-empty in the built `features.ndjson` for every key that has ≥1 ledger commit; `SuggestKey(changedPaths, message)` returns a ranked list whose top entry matches the dominant changed path.
- [x] **DOD-5 (T-3 gate):** the `commit_feature_key_missing` trigger reprompts on a code turn with a missing/unverified key, the reprompt text contains both the `SuggestKey` shortlist and the register-new instruction, it passes on a verified key, it is bounded to ≤2 reprompts, and it downgrades to warn under `gate_mode: warn`.
- [x] **DOD-6 (T-4 registry):** `FEATURE-KEYS.md` is grown to cover the repo's active features; a key marked `superseded by <x>` is never offered as a fresh candidate and resolves to `<x>`.
- [x] **DOD-7 (T-5 metric):** the high- vs low-confidence ratio is emitted to engine status / audit log and is readable after a build.
- [x] **DOD-8 (T-7 tier-2):** `HistorySlot` appends a capped CA excerpt for the most recent `K=3` linked entries (`Scope` + `Residual Notes`, ≤ ~400 chars each); entries with no CA note render Tier-1 only; total injected block stays bounded.
- [x] **DOD-9 (no-spec safety):** on a repo with **no** `FEATURE-KEYS.md` and **no** CA notes, build + injection still work from git alone (`AC-2`); Tier-2 is simply absent.
- [x] **DOD-10 (tests green):** unit tests for every `T-` item plus the E2E in §6.2 pass; `go test ./internal/{runner,flowgate,featurecatalog,changeledger}/...` is green.
- [x] **DOD-11 (docs):** `SD-17 §3.3` / `CP-35 §4.2` updated to record resolver→prompt injection delivered; `SD-20` note added if the gate becomes a formal rule; this task moved to `done/` with Completion Notes filled.
- [x] **DOD-12 (shared bounded-block helper):** the UTF-8-safe truncation / closing-tag-escaping helper (CP-37 §5) is extracted to one place (`internal/promptblock`) and reused for the Tier-2 CA excerpt cap rather than reimplemented; the same helper backs Task-078/161/162 blocks, so all injected context shares one budgeting discipline.
- [x] **DOD-13 (gate warn-downgrade):** under `gate_mode: warn` the `r-fk` (`commit_feature_key_missing`) check downgrades to a warning instead of a reprompt, matching `r-ca`/`r-bug`, and never hard-rejects.

### 6.2 End-to-End Test (`E2E-1`)

A single black-box test (`runner` package) that exercises the whole pipeline against a throwaway git repo — the proof the two questions reach the AI with the right, honest content.

- **Given** a temp git repo with: a `FEATURE-KEYS.md` containing `chat-ui`; 3 commits keyed `[Feature][chat-ui]…` in time order (oldest→newest); and a CA note (`CA-…md`) for the newest commit's `source_doc_id` whose `Residual Notes` say *"X was not expanded here."* Engine init has run (`changeledger.Build` + `featurecatalog.Build`).
- **Step 1 — inject (T-0/T-7):** assemble a turn whose prompt is *"improve the chat input"*.
  - **Then** the assembled prompt contains `## Prior work on "chat-ui"`, the 3 entries in oldest→newest order, the newest marked `← current truth`, and a Tier-2 CA excerpt containing the `Residual Notes` line under the newest entry.
- **Step 2 — bad key gate (T-1/T-2/T-3):** simulate the turn committing `[Feature][chatui][ui] tweak input` (typo) touching `apps/admin-web/src/chat/Input.tsx`.
  - **Then** the gate emits a `reprompt` whose text suggests `chat-ui` (from `SuggestKey` on the changed path) and offers register-new; the ledger entry for that commit is **not** `Confidence=high`.
- **Step 3 — fix + refresh (post-commit):** re-commit as `[Feature][chat-ui][ui] tweak input`.
  - **Then** the gate passes, the post-commit rebuild adds a `Confidence=high` `chat-ui` entry, and a **subsequent** turn's injected block shows the new commit as `← current truth`.
- **Step 4 — degrade (DOD-9):** repeat Step 1 against a repo with no CA note for the newest commit.
  - **Then** the block still renders Tier-1 subjects with no error and no Tier-2 excerpt.

## 7. Out of Scope

- Regression oracle execution/polyglot (Task-156) and resolution UX (Task-155).
- **Vector/embedding-based feature resolution and Plane-A RAG (`SD-10`).** Plane C stays vector-free (`SD-17 D-4`). The "why" context that Plane-A semantic chat-summary recall was imagined to supply is, for this job, **already captured in the CA note** (`T-7`) — and `r-ca` guarantees one exists per task/bugfix — so Plane-A RAG is **not required for Task-157**. Catalog embeddings remain an optional future flag for very large catalogs only, and would touch only Q1 (resolution), never Q2 (ordered history). Broader cross-feature/decision semantic recall stays Plane-A's concern, in its own track.
- GitNexus structural context (Plane B).
- **Conversation-transcript handoff across providers** — owned by [Task-078](Task-078-Cross-Provider-Chat-Handoff.md). Task-157 never reconstructs or transfers chat transcripts; it injects code/feature history only. The two compose at the shared prompt-assembly seam but are not interchangeable (see Constraints).

## 8. Completion Notes

- result: done
- follow-ups: continue watching the key-confidence metric (`T-5`) and injected-history quality, but the delivery slice itself is complete.
- upstream docs updated: `CP-35 §4.2`, `SD-17 §3.3`, and `SD-20 §2.9` now record the delivered resolver→prompt injection and feature-key gate.
