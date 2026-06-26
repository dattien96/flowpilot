# CP-37: Prompt Context Continuity — Injection And Cross-Provider Handoff

## Metadata

- Document ID: `CP-37`
- Title: `Prompt Context Continuity — Injection And Cross-Provider Handoff`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: [Task-157: Feature-Key Accuracy For History Context](../../08-Task/done/Task-157-Improve-Context-Hardness.md) (P-1), [Task-161: Per-Feature Chat-Summary Timeline](../../08-Task/done/Task-161-Per-Feature-Chat-Summary-Timeline.md) (P-2), [Task-078: Cross-Provider Chat Handoff](../../08-Task/done/Task-078-Cross-Provider-Chat-Handoff.md) (P-3), [Task-162: Summary-Based Cross-Provider Handoff](../../08-Task/done/Task-162-Summary-Based-Cross-Provider-Handoff.md) (P-4), [Task-163: Chat-Summary Generation Triggers](../../08-Task/done/Task-163-Chat-Summary-Generation-Triggers.md) (P-2.1)
- Related Documents: [CP-35: Context And Regression Engine Rollout](../done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-18: Refactor Workflow With Session](../done/CP-18-Refactor-Workflow-With_Session.md), [CP-10: Integrations, Memory & Context Intelligence](../done/CP-10-Integrations-Hardening.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SD-10: Context Resolver & RAG](../../06-System-Tech-Design/SD-10-Context-Resolver-RAG.md)
- Replaces: `None`
- Tags: `context, prompt-injection, history, cross-provider, handoff, summarizer, no-rag, coordination, local-runner`

## AI Quick View

### Summary

- **One theme, one seam.** Everything here is about getting the *right prior context* into the AI's prompt at the **shared prompt-assembly seam** ([runner.go:903](../../../apps/local-runner/internal/runner/runner.go), beside `injectSkillContent`). This CP coordinates four tasks across two existing design lineages (the context engine `SD-17`/`CP-35`, and the cross-provider handoff `SD-12`/`CP-18`) into one ordered rollout.
- **Three context sources, all deterministic, no RAG:**
  - **Q1/Q2 — feature/commit history** ("what feature, and what changed for it, in order — newest = truth"): the ledger + CA "why" → **P-1 (Task-157)**.
  - **Q3 — per-feature chat-discussion history** ("what was *discussed* about this feature in past chats, by time"): a new time-ordered entry type on the same timeline → **P-2 (Task-161)**.
  - **Conversation handoff** ("carry *this* live thread to a new provider on switch"): raw transcript transfer, then summary → **P-3 (Task-078)** then **P-4 (Task-162)**.
- **Two shared mechanisms** the four tasks build once and reuse:
  - a **bounded-context-block helper** (UTF-8-safe truncation, escaping, omission markers) — first specced in Task-078, reused everywhere;
  - a **summarizer** (cheap model, rolling, time-ordered) — built in **P-2 (Task-161)** and reused by **P-4 (Task-162)**. Built once, consumed twice.
- **No RAG anywhere in this CP.** The "why"/discussion context is captured deterministically — CA notes (already guaranteed by `r-ca`) and time-ordered summaries — not by semantic/vector retrieval. RAG (`SD-10`) and the broader semantic-recall pipeline stay in `CP-10` as a separate, later track.
- **Raw is the floor; summary is an enhancement.** The handoff (P-3) ships raw transfer that always works even at zero budget (the common token-limit switch case); the summary (P-4) layers quality on top with graceful degradation. The switch never fails on a model call.

### Current Ask

- Sequence and coordinate the four context-injection tasks so the AI reliably starts every turn (and every provider switch) with the right prior context — committed history, prior discussion, and the live conversation — using shared, deterministic mechanisms and no RAG.

### Key Decisions

- `D-1` **Shared seam.** All injection happens at the runner prompt-assembly seam; tasks write *beside* each other, not through each other. A cross-provider handoff target run's first turn therefore receives the feature history automatically (P-1 fires for free) — no coupling between handoff and engine code.
- `D-2` **No RAG in this CP.** Deterministic, ordered, git/summary-based context only (`SD-17 D-4`, `SS-14` line 72). The CA note (`r-ca`-guaranteed) supplies the committed "why"; a time-ordered summarizer supplies discussion/handoff context. Semantic retrieval stays in `CP-10`/`SD-10`.
- `D-3` **Summarizer built once, reused twice.** P-2 builds the rolling, cheap-model summarizer for the per-feature discussion timeline; P-4 reuses the identical component for live handoff. No second summarizer.
- `D-4` **Raw floor never removed.** The handoff's raw transcript transfer (P-3) is the guaranteed zero-budget fallback; the summary (P-4) is layered on with a degrade ladder (hybrid → target self-summary → raw).
- `D-5` **Extensible timeline seam.** P-1 designs the per-feature history store/renderer so P-2's `chat_summary` entry type slots in without restructuring.
- `D-6` **Coordination, not re-parenting.** P-1/P-2 remain owned by `CP-35`/`SD-17`; P-3/P-4 by `CP-18`/`SD-12`. This CP sequences the context-injection theme across them without reopening those plans.

### Constraints

- Injection and summarization are **non-fatal and bounded**: a ledger/catalog/summarizer hiccup never blocks a turn or a switch (match `ledger_live.go` silent-swallow).
- All injected blocks are **token-bounded** (recent-N, char caps) and share one budgeting helper; combined-budget awareness on a handoff first turn (feature history + handoff content co-exist).
- History/discussion retrieval stays **ordered and deterministic** (newest = truth); never reorder by relevance.
- Per-project, local-first storage; no cross-project recall.

### Open Questions

- `Q-1` Does the per-feature chat-discussion timeline (P-2) warrant a one-line `SD-17 §3` amendment (add `chat_summary` as a deterministic Change-plane entry) per `SS-14 BR-2`? (Tracked as Task-161 `Q-1`.)
- `Q-2` Summarizer model/account and rolling-vs-chat-end generation (Task-161 `Q-2`/`Q-3`).
- `Q-3` Cross-vendor pre-send review/redaction for handoff summaries (Task-078 `Q-2` / Task-162 `Q-1`).

### Source Refs

- `SD-17 §3.2`/`§3.3`/`D-4` (ledger, catalog, no-vector); `SD-12 §3.4` (provider-neutral transcript extraction + provenance); `SS-14 AC-3`/`AC-10`/`BR-6`; `SS-11 §5.1`/`§6`.
- Existing plans: `CP-35` (context engine slices P-1..P-8 — P-2 planned the resolver→prompt injection this rollout finishes), `CP-18 §5.6`/`§5.7` (handoff), `CP-10` (Plane-A/RAG track, deferred).
- Code: `apps/local-runner/internal/runner/runner.go` (`injectSkillContent` seam), `internal/featurecatalog/{resolve.go,slots.go,catalog.go}`, `internal/changeledger/{parse.go,enrich.go,ledger.go}`, `internal/runner/{interactive_handlers.go,transcript_loader.go,finalizer.go}`.

## 1. Goal

Coordinate the delivery of reliable prompt-context continuity: every AI turn starts with the right *committed* history (P-1) and *prior discussion* (P-2) for the feature in play, and every cross-provider switch carries the *live conversation* forward (P-3 raw, P-4 summary). Achieve this with two shared, deterministic mechanisms (a bounded-block helper and a summarizer) at one shared injection seam, with no RAG.

## 2. Input Documents

- `SD-17` (context engine — feature history, resolver, no-vector decision), `SD-12`/`SD-14` (cross-provider transcript extraction + session rules), `SS-14`/`SS-11` (acceptance).
- Existing rollouts this coordinates: `CP-35` (P-2 planned the resolver→prompt injection finished here), `CP-18` (handoff lineage), `CP-10` (semantic-recall track kept separate).

## 3. Implementation Strategy

- **Prerequisites:** Go runner builds; the ledger/catalog (Task-096/097) and the handoff transcript extractors (Task-078 source) exist or are built in their owning tasks.
- **Order (execution sequence):** `P-1 (Task-157)` → `P-2 (Task-161)` → `P-3 (Task-078)` → `P-4 (Task-162)`.
- **Parallelism / critical path:**
  - `P-1` and `P-3` are **independent** and may run in parallel (engine injection vs. raw handoff).
  - `P-2` depends on `P-1` (it adds an entry type to P-1's timeline and builds the summarizer).
  - `P-4` depends on **both** `P-3` (the slot) **and** `P-2` (the summarizer).
  - Critical path: `P-1 → P-2 → P-4`; `P-3` joins before `P-4`.
- **Shared-build rule:** the bounded-block helper is extracted when the first consumer needs it (P-1 or P-3, whichever lands first) and reused; the summarizer is built in P-2 and reused in P-4 (`D-3`).

## 4. Phases

Each phase is fully specified in its task; this CP records scope, order, and the shared-mechanism touchpoints.

- **P-1 — [Task-157](../../08-Task/done/Task-157-Improve-Context-Hardness.md): Feature-Key Accuracy + History Injection (done).** Wire the consumer (inject feature history at the seam), validate the feature key against the registry, assist key selection from changed paths, gate missing/unknown keys, measure key confidence, and render two-tier history (commit subject + CA "why"). **Reserves the timeline seam** for P-2. Owned by `CP-35`/`SD-17`.
- **P-2 — [Task-161](../../08-Task/done/Task-161-Per-Feature-Chat-Summary-Timeline.md): Per-Feature Chat-Summary Timeline (done).** Add a `chat_summary` entry type to P-1's timeline (Q3: time-ordered prior *discussion* per feature/bug, no RAG) and **build the shared rolling summarizer**. Inject the discussion block beside P-1's commit block. Owned by `CP-35`/`SD-17` (`SD-17 §3.2` note added, resolving `Q-1`).
- **P-2.1 — [Task-163](../../08-Task/done/Task-163-Chat-Summary-Generation-Triggers.md): Chat-Summary Generation Triggers (done).** Refine P-2's *generation*: replace per-turn summarizing with a 5-min idle timer + a manual "Gen summary" control + a startup backfill scan; store one upserted row per `(run, feature)` (hashed `state_key`); bucket turns per feature so summaries don't mix. Reuses P-2's summarizer unchanged; validated by `§7.2 V-161-12…V-161-16`.
- **P-3 — [Task-078](../../08-Task/done/Task-078-Cross-Provider-Chat-Handoff.md): Cross-Provider Chat Handoff (done).** On a confirmed provider switch, reconstruct a provider-neutral transcript and send a bounded **raw** handoff prompt to a new run; preserve and link the source run. Ships the **raw floor + the slot** the summary plugs into. Owned by `CP-18`/`SD-12`.
- **P-4 — [Task-162](../../08-Task/done/Task-162-Summary-Based-Cross-Provider-Handoff.md): Summary-Based Handoff (done).** Fill P-3's slot with the AI-summary **hybrid** (summary of older turns + recent raw), **reusing P-2's summarizer**, with the degrade ladder (hybrid → target self-summary → raw). Owned by `CP-18`/`SD-12`; supersedes the vague `CP-10` AI-summary-handoff follow-up.

## 5. Shared Mechanisms (the unifying core)

- **The injection seam** ([runner.go:903](../../../apps/local-runner/internal/runner/runner.go)). P-1 and P-2 prepend feature-history / discussion blocks here on every turn; P-3/P-4 produce the handoff prompt that becomes a new run's first turn — which then *also* flows through this seam, so the handoff target automatically inherits P-1/P-2 context (`D-1`).
- **The bounded-context-block helper.** UTF-8-safe truncation, `</…>`-tag escaping, omission markers, char/recent-N caps. One implementation, shared by every block (feature history, discussion, handoff).
- **The summarizer.** Cheap-model, rolling, cache-keyed by transcript state (`hash(last_committed_turn_id + turn_count)`), focused on discussion/decisions (not code changes — P-1 owns those). Built in P-2, reused in P-4.

## 6. Acceptance Check (program-level)

- P-1's feature-history block appears in a turn's `prompt.txt` when a feature resolves; P-2's discussion block appears beside it for features with prior chats; both bounded.
- A cross-provider switch (P-3) always produces a working handoff (even at zero source budget); P-4 upgrades long-chat handoffs to summary+recent-raw and degrades gracefully when no summary exists.
- The summarizer is implemented once (P-2) and reused unchanged by P-4 (shared-component test).
- No embeddings/RAG in any of the four tasks; retrieval is deterministic key/time lookup throughout.
- Each task's own acceptance checks pass; `go test ./internal/{runner,featurecatalog,changeledger,flowgate}/...` green.

## 7. Validation Matrix

This section lists the **pre-E2E** test cases that should be run to validate the four delivered tasks before the final real-app manual E2E pass. The later "click through the real desktop app and verify behavior end-to-end" pass is intentionally left to the manual validation the user will run separately.

> **Manual E2E walkthrough.** A hands-on, click-through guide that exercises all four tasks against a prepared sandbox lives at `D:\working\gate-sandbox\CP-37-E2E-TEST-GUIDE.md` (Tests A–D map back to the `V-###` cases below). The sandbox already has the fixture seeded — a `calc-core` feature with two `[Feature][calc-core]…` commits, a `CA-900` note carrying `## Scope`/`## Residual Notes`, and two prior chat-discussion summaries — so the only setup is binding the project in the desktop app.

### 7.1 Task-157 Validation

- `V-157-01` **Feature history injection happy path**
  Setup: use a repo/workspace whose `.flowpilot/catalog/features.ndjson` and `.flowpilot/ledger/feature_history.ndjson` already contain a known feature with at least 2 history entries.
  Run: trigger prompt assembly for a turn whose text clearly matches that feature.
  Verify: the generated `prompt.txt` starts with `## Prior work on "<feature>"`, entries render oldest→newest, and the last entry is marked `← current truth`.

- `V-157-02` **Feature history injection safe fallback**
  Setup: temporarily remove or corrupt `.flowpilot/catalog/features.ndjson` or `.flowpilot/ledger/feature_history.ndjson`.
  Run: trigger prompt assembly for the same turn as `V-157-01`.
  Verify: the turn still proceeds, the prompt body is unchanged except for the missing history block, and no runner error is surfaced to the user.

- `V-157-03` **Commit-key validation**
  Setup: prepare one commit subject with an unregistered key like `[Feature][chatui][ui] ...` and one with a registered key like `[Feature][chat-ui][ui] ...`.
  Run: exercise parse/enrich logic or the corresponding unit tests.
  Verify: the typo key stays low-confidence and falls through to inference; the registered key becomes high-confidence and is preserved.

- `V-157-04` **Path-anchored suggestion**
  Setup: collect changed paths that clearly belong to one historical feature, for example paths under one known module or screen.
  Run: call `SuggestKey(changedPaths, message)` using a neutral message plus those paths.
  Verify: the top-ranked candidate matches the expected feature and its score is driven by the file-glob match, not just message wording.

- `V-157-05` **Feature-key gate reprompt**
  Setup: create a code-changing turn that commits without a verified `[feature]` bracket.
  Run: let the post-turn flow gate evaluate that turn.
  Verify: `commit_feature_key_missing` emits a reprompt, the remediation text includes the suggested shortlist and the "register new key" instruction, and a follow-up committed turn with a registered key passes cleanly.

- `V-157-06` **Tier-2 CA excerpt rendering**
  Setup: ensure the most recent history entries link to CA notes with `## Scope` and `## Residual Notes`.
  Run: render `HistorySlot(featureKey, ledger)`.
  Verify: only the recent entries include the capped CA excerpt lines, older entries remain subject-only, and the excerpt is truncated safely when long.

- `V-157-07` **No-spec degrade path**
  Setup: test against a repo or fixture with no `FEATURE-KEYS.md` and no CA notes.
  Run: rebuild the ledger/catalog and trigger prompt injection.
  Verify: feature history still derives from git-only fallback signals, no crash occurs, and Tier-2 excerpt content is simply absent.

- `V-157-08` **Automated package sweep**
  Run: `cd apps/local-runner && go test ./internal/changeledger ./internal/featurecatalog ./internal/flowgate ./internal/runner`
  Verify: all targeted packages pass.

### 7.2 Task-161 Validation

- `V-161-01` **Chat-summary ledger append**
  Setup: complete a chat turn on a feature-resolvable run with at least one user prompt and one assistant response.
  Run: let `recordChatSummaryIfNeeded` execute at turn finalization.
  Verify: `.flowpilot/ledger/chat_summary.ndjson` receives a new line with the expected `feature_key`, `run_id`, `turn_id`, and increasing `created_at`.

- `V-161-02` **Discussion block injection**
  Setup: seed `chat_summary.ndjson` with prior summaries for one feature.
  Run: start a new turn whose prompt resolves to that feature.
  Verify: the injected prompt includes `## Prior discussion on "<feature>"` after the commit-history block and only the most recent bounded summary entries are shown.

- `V-161-03` **Unknown-feature suppression**
  Setup: use a prompt that does not resolve confidently to any feature.
  Run: finalize the turn and then start another turn.
  Verify: no new `chat_summary` entry is appended for that ambiguous turn and no prior-discussion block is injected on the next turn.

- `V-161-04` **Summarizer failure degrade**
  Setup: simulate a missing/unreadable chat-summary ledger or force summary generation to return empty.
  Run: finalize the turn and then trigger prompt injection.
  Verify: the turn still completes normally and only Task-157 feature history remains in the prompt.

- `V-161-05` **Shared summarizer reuse seam**
  Setup: record a summary for a feature and inspect the emitted summary format.
  Run: feed that same run into the Task-162 handoff path.
  Verify: the handoff path can reuse the summary without reformatting or schema translation.

- `V-161-06` **E2E context-engine sync includes chat summaries**
  Setup: bind a project with Google Drive chat sync connected, then create `.flowpilot/ledger/chat_summary.ndjson` beside existing `.flowpilot/ledger/feature_history.ndjson` and `.flowpilot/catalog/features.ndjson`.
  Run: trigger engine context sync through project bind/init or the shared context-sync helper.
  Verify: Drive `context-engine/` contains `chat_summary.ndjson` alongside `feature_history.ndjson`, `features.ndjson`, and `flow-rules.json` when present; local `.flowpilot/manifest.json` includes a SHA256/size entry for `ledger/chat_summary.ndjson`.

- `V-161-07` **E2E post-turn chat-summary sync trigger**
  Setup: use a Drive-connected project and complete a normal chat turn that resolves confidently to a feature.
  Run: let `recordChatSummaryIfNeeded` append the summary during turn finalization.
  Verify: the same completed turn leaves the local summary durable and best-effort sync uploads the updated `chat_summary.ndjson` to Drive `context-engine/` without requiring a later engine init or manual chat-session sync.

- `V-161-08` **E2E Drive-unavailable sync degrade**
  Setup: disconnect or omit the project Google Drive chat-sync binding, then complete a feature-resolvable chat turn.
  Run: let chat-summary append and context-engine sync attempt execute.
  Verify: `.flowpilot/ledger/chat_summary.ndjson` still receives the local entry, the turn remains completed, no user-facing error is emitted, and the context-sync result treats Drive as skipped rather than failed.

- `V-161-10` **Low-signal latest prompt inherits the feature (record path)**
  Setup: a feature-resolvable chat (a turn that clearly matches a registered feature), followed by a low-signal continuation/retry turn whose own text does not resolve to any feature (e.g. `try again`, `continue`, `go on`).
  Run: let the chat-summary recorder run after the continuation turn.
  Verify: a `chat_summary` entry is still recorded under the **established feature** (resolution scans back from the latest prompt to the most recent substantive prompt), not dropped as `unknown`; no entry is filed under a wrong/empty key. (Unit: `TestRecordChatSummaryResolvesFeatureFromEarlierTurnOnLowSignalPrompt`.)

- `V-161-11` **Low-signal prompt still gets history injected; explicit pivot re-resolves (inject path)**
  Setup: an ongoing chat already resolved to feature A with prior commit/discussion history.
  Run: (a) send a low-signal turn (`continue` / `try again`); then (b) send a turn that clearly names a *different* registered feature B.
  Verify: on (a) the assembled prompt still contains feature A's `## Prior work on "A"` / `## Prior discussion on "A"` blocks (inherited via fallback); on (b) the prompt re-resolves and injects feature B's history instead of clinging to A. A brand-new chat whose first prompt is low-signal (no prior turns) injects nothing. (Unit: `TestInjectFeatureHistoryFallsBackToPriorTurnFeature`.)

- `V-161-12` **Manual "Gen summary" button (now / busy / no-op)**
  Setup: an existing chat resolved to a feature.
  Run: with the chat **running**, observe the Gen-summary control; then with the chat **idle**, press it; then press it again with no new turn.
  Verify: the button is disabled while running and `POST …/chat-summary` returns `chat_summary_run_busy` (409) if forced; when idle it generates immediately (bypassing the idle wait) and reports `generated:true`; a second press with an unchanged transcript returns `generated:false, skipped:true` (hash match). (Unit: `TestGenerateChatSummaryNow`.)

- `V-161-13` **Idle-timer trigger + reset**
  Setup: set a short idle window via `FLOWPILOT_SUMMARY_IDLE_MS`; complete a feature-resolvable turn.
  Run: (a) wait the window with no new turn; (b) in a second run, send a new turn before the window elapses, then wait.
  Verify: (a) the summary is generated once the window passes; (b) the new turn resets the countdown — the summary fires only after the window from the *last* turn, not the first. No summary is produced while a turn is in flight.

- `V-161-14` **No cross-feature mixing (per-feature bucketing)**
  Setup: one chat that discusses feature A for several turns, then pivots to feature B.
  Run: let the recorder run after the B turns.
  Verify: feature B's `chat_summary` entry summarizes only the B turns (no A discussion leaks in), and A's entry covers only the A turns; low-signal turns attach to the running feature. (Unit: `TestBucketTurnsByFeatureSeparatesFeatures`.)

- `V-161-15` **Rolling upsert (one line per run+feature, hashed state_key)**
  Setup: a chat resolved to a feature; complete several content-changing turns.
  Run: inspect `.flowpilot/ledger/chat_summary.ndjson` after each turn.
  Verify: the run keeps exactly **one** line per feature, rewritten in place as the chat grows (not one line per turn); the `state_key` is a fixed-length hash that changes when the transcript changes and matches (skips regen) when it does not. (Unit: `TestRecordChatSummaryRefreshesInPlaceAfterNewTurn`.)

- `V-161-16` **Startup backfill scan**
  Setup: a persisted chat with no `chat_summary` entry (or a stale hash) and the runner stopped.
  Run: start the runner (`serve`).
  Verify: the one-shot background scan generates the missing/stale summary for that chat without a user action; chats whose stored hash already matches are left untouched (no redundant model call); live in-memory runs are skipped (owned by their idle timer/button).

- `V-161-09` **Automated package sweep**
  Run: `cd apps/local-runner && go test ./internal/changeledger ./internal/featurecatalog ./internal/runner`
  Verify: all targeted packages pass.

### 7.3 Task-078 Validation

- `V-078-01` **Idle-chat provider switch**
  Setup: open a normal chat with an existing idle run in desktop state.
  Run: select a different provider chip.
  Verify: the confirmation modal opens, the current `runId` does not change yet, and the source timeline remains visible until confirmation.

- `V-078-02` **Source transcript reconstruction**
  Setup: prepare one Claude-source run and one Codex-source run with multi-turn history.
  Run: call the handoff-context endpoint for each run.
  Verify: the reconstructed prompt contains ordered raw user prompts and visible assistant responses only; hidden/system/tool payload content is excluded.

- `V-078-03` **Bounded raw handoff packing**
  Setup: use a source transcript whose raw content exceeds 64 KiB.
  Run: build the handoff prompt.
  Verify: the newest complete turns are retained, the omission marker appears, `includedTurnCount` is non-zero, and `omittedTurnCount` reflects the dropped older turns.

- `V-078-04` **Single oversized turn fallback**
  Setup: create a source transcript where the newest single turn alone exceeds the byte budget.
  Run: build the handoff prompt.
  Verify: that turn is still included in truncated form, the per-turn truncation marker is present, and the request does not fail with `handoff_context_unavailable`.

- `V-078-05` **Unsupported-source rejection**
  Setup: use a Gemini-source run.
  Run: call `POST /client/workflow-runs/{runId}/handoff-context`.
  Verify: the runner returns `handoff_source_provider_unsupported` and no target run is created.

- `V-078-06` **Desktop orchestration**
  Setup: confirm the modal on an idle source chat.
  Run: let the desktop store request handoff context, create the target run, and send the first turn.
  Verify: exactly one target run is created, the first user turn is the handoff prompt, the source run remains in history, and a failed handoff keeps the source run intact and retryable.

- `V-078-07` **Type and store safety**
  Run: `cd apps/desktop-flowpilot && npm run typecheck`
  Verify: the desktop contract/store code compiles cleanly.

### 7.4 Task-162 Validation

- `V-162-01` **Hybrid handoff happy path**
  Setup: create a source run with a cached same-run summary in `chat_summary.ndjson`.
  Run: request handoff context for that run.
  Verify: the prompt contains `<conversation_summary>` followed by `<previous_conversation>`, and the response reports `handoffMode = "hybrid"`.

- `V-162-02` **Raw/target-summary degrade path**
  Setup: use a source run with no cached summary for that run.
  Run: request handoff context.
  Verify: the request still succeeds, no unrelated summary is pulled in, and the mode reflects the degrade path rather than falsely claiming hybrid context.

- `V-162-03` **Run-scoped summary selection**
  Setup: create two runs on the same feature, with a newer summary recorded on the other run.
  Run: request handoff for the older run.
  Verify: the handoff uses only the summary whose `run_id` matches the source run and does not import the newer other-run summary.

- `V-162-04` **Transcript-state refresh**
  Setup: request handoff twice without new committed turns, then again after one more committed turn.
  Run: inspect the summary selected across those calls.
  Verify: the first two requests reuse the cached summary; the later request after a new committed turn reflects refreshed content/state.

- `V-162-05` **Feature-history coexistence**
  Setup: perform a provider handoff for a feature with both commit history and prior discussion history.
  Run: inspect the target run's first assembled prompt.
  Verify: the handoff prompt content is present and the shared prompt-assembly seam still prepends Task-157/Task-161 context without blowing past bounded prompt packing.

- `V-162-06` **Cross-surface verification sweep**
  Run: `cd apps/local-runner && go test ./internal/changeledger ./internal/featurecatalog ./internal/flowgate ./internal/runner`
  Run: `cd apps/desktop-flowpilot && npm run typecheck`
  Verify: both the runner and desktop validation sweeps pass.

## 8. Out of Scope

- RAG / semantic retrieval / vector memory — stays `SD-10` / `CP-10` (broader cross-chat semantic recall), a separate track.
- The detailed implementation of each phase — owned by the four tasks; this CP coordinates, it does not re-specify.
- GitNexus structural context (Plane B), regression oracle (Task-156), resolution UX (Task-155).

## 9. Completion Notes

- result: planned — coordination plan for the four context-injection tasks; sequences `P-1 → P-2 → P-3 → P-4` across `CP-35` and `CP-18` without reopening them.
- follow-ups: confirm the `SD-17` plane note for P-2 (`Q-1`); resolve cross-vendor handoff-summary review (`Q-3`); record in `CP-35 §4.2` that resolver→prompt injection is delivered (P-1) and in `CP-18 §5.7` that the AI-summary handoff is delivered (P-4).
- upstream docs updated: none yet.
