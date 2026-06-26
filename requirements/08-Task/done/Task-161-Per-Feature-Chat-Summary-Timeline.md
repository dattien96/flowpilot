# Task-161: Per-Feature Chat-Summary Timeline (Context Hardness Phase 2)

## Metadata

- Document ID: `Task-161`
- Title: `Per-Feature Chat-Summary Timeline`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [CP-37: Prompt Context Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md), [Task-157: Feature-Key Accuracy For History Context](Task-157-Improve-Context-Hardness.md), [Task-096: Commit-History Ledger](../done/Task-096-Commit-History-Ledger.md), [Task-097: Feature Catalog And Resolver](../done/Task-097-Feature-Catalog-And-Resolver.md), [Task-162: Summary-Based Cross-Provider Handoff](Task-162-Summary-Based-Cross-Provider-Handoff.md), [Task-078: Cross-Provider Chat Handoff](Task-078-Cross-Provider-Chat-Handoff.md), [Task-163: Chat-Summary Generation Triggers](Task-163-Chat-Summary-Generation-Triggers.md)
- Replaces: `None`
- Tags: `changeledger, featurecatalog, chat-summary, history-timeline, cross-chat-memory, summarizer, no-rag, phase-2`

## AI Quick View

### Summary

- **The third context question.** : after Task-157 we got data for answer Q1+Q2 but we missed something like that
``
. But a lot of useful context never becomes a commit — "we tried X and rejected it," "the user prefers Y," "this is blocked on Z." That lives only in past chat transcripts
``
This task improved and answer Q3 'what did we discuss about this feature in past chats?'
Basically now we have
- Q1: what feature, and what changed for it? -> Handled in **Task-157** by feature/commit history
- Q2: Why changed it? Handled in **Task-157** by the ledger + CA. Ca file always exist cause we have gate r-ca
- Q3: what was discussed about this feature in past chats ? Handled in current **Task-161** with per-feature chat-discussion history

- **It is a new *entry type* in 157's existing per-feature timeline, not a new subsystem.** Reuse the Feature Resolver ([resolve.go](../../../apps/local-runner/internal/featurecatalog/resolve.go)) to file a chat under a `feature_key`/bug, and the ordered ledger model to store chat summaries by time. When a *new* chat on a feature starts, inject the recent chat-summaries alongside 157's commit history.
- **No RAG — "via time," per feature/bug.** Resolution is by `feature_key` (lexical/path, as Task-157); ordering is by commit/chat time; retrieval is a direct key lookup. No embeddings, no similarity search. This keeps the Change plane deterministic (`SD-17 D-4`).
- **The content is AI-generated, the placement is deterministic — exactly like a CA note.** A cheap summarizer compresses each past chat; the *summary text* is non-deterministic, but its `feature_key` and time-order are deterministic, so the Plane-C "newest = truth" guarantee holds.
- **Builds the shared summarizer capability** that [Task-162](Task-162-Summary-Based-Cross-Provider-Handoff.md) (cross-provider handoff phase-2) reuses. The summarizer is built once here and consumed in two places: this timeline, and the live handoff.

### Current Ask

- This task is delivered: each feature/bug now has a time-ordered chat-discussion history beside its commit history, so a new chat starts with the prior discussion context already available.

### Key Decisions

- `T-1` **Extend, don't fork.** Add a `chat_summary` entry type to 157's per-feature timeline (the seam Task-157 reserves). Same store, same key (`feature_key`), same ordering. Injection appends a "## Prior discussion on …" block beside 157's "## Prior work on …" commit block.
- `T-2` **Summarize each chat with a cheap model** (e.g. Haiku), focused on *discussion not captured by commits/CA*: goals, decisions, rejected approaches, user preferences, open/blocked items. Do **not** re-summarize code changes — Task-157's commit/CA history owns "what was done."
- `T-3` **Resolve each chat to a feature/bug** via the existing resolver (prompt + changed paths). A chat with no confident feature resolves to `unknown` and is not injected (avoid noise).
- `T-4` **No RAG.** Retrieval is a deterministic key lookup; bounding is "most recent N chat-summaries for this feature," capped in chars (mirror Task-157 `T-7`: `N=3`, ≤ ~400 chars). Time-ordered, never similarity-ranked.
- `T-5` **The summarizer is the shared capability.** Build it as a reusable component (input: ordered turns; output: capped summary) so Task-162 reuses the same code for live-chat handoff. Rolling vs chat-end generation is an open question (`Q-2`).

### Constraints

- Plane C stays **deterministic and ordered** (`SD-17 D-4`): only the summary *text* is AI-generated; `feature_key` and time-order are not.
- Summarization is **best-effort and non-fatal**: a summarizer failure leaves the commit history intact and simply omits the discussion block (match the silent-swallow pattern in `ledger_live.go`).
- Do **not** duplicate committed history: the chat-summary focuses on discussion *not* already in commits/CA notes.
- Token-bounded injection (recent `N`, capped chars), shared with Task-157's budgeting helper.
- Per-project, local-first storage like the ledger; no cross-project recall.

### Open Questions

- `Q-1` Upstream: cross-chat conversation memory blurs the Change/Decision plane line. Does this warrant a small `SD-17 §3` amendment (add `chat_summary` to the Change plane as a deterministic, time-ordered entry) per `SS-14 BR-2`, or does it belong under `SD-10` (Decision plane)? Proposal: keep it in Plane C (it is per-feature, deterministic, time-ordered — not semantic), with a one-line `SD-17` note.
- `Q-2` When is a chat summarized — **rolling** (updated as the chat progresses, ready for Task-162's zero-budget handoff) or **at chat end** (cheaper, simpler, but unavailable mid-chat)? Proposal: rolling, so Task-162 can reuse it under the token-limit case.
- `Q-3` Which model/account runs the summarizer (cheap shared model vs the chat's own provider)? Proposal: a cheap, separate summarizer so it does not drain the primary provider's budget.
- `Q-4` Storage: a new `chat_summary` NDJSON beside `feature_history.ndjson`, or a new record kind in the same ledger? Proposal: a sibling NDJSON keyed by `feature_key`, time-ordered.
- `Q-5` Relationship to CA notes: if a chat *did* produce a CA note, do we still keep a separate chat-summary, or defer to the CA "why" (Task-157 `T-7`)? Proposal: prefer CA when present; chat-summary covers chats with no CA (no commit produced).

### Source Refs

- Builds on `SS-14 AC-3`/`AC-10`, `SD-17 §3.2`/`§3.3`/`D-4`, `CP-35 §4.1`/`§4.2`, and Task-157 (timeline + resolver + injection seam).
- Code: `apps/local-runner/internal/featurecatalog/{resolve.go,slots.go}`, `apps/local-runner/internal/changeledger/{ledger.go,query.go}`, `apps/local-runner/internal/runner/{runner.go,finalizer.go}` (a chat's turns/summary source), shared summarizer (new).

## 1. Goal

Give each feature/bug a **time-ordered chat-discussion history** as a new entry type in Task-157's per-feature timeline, so starting a new chat on a feature injects "what we discussed before" (decisions, rejected approaches, preferences, blockers) alongside "what we committed before." Deterministic and ordered (no RAG); the summary text is AI-generated by a reusable summarizer that Task-162 also consumes.

## 2. Parent Links

- coding plan: `CP-35` §4.2 (resolver→prompt injection — extended here with a new entry type)
- tech design: `SD-17` §3.2/§3.3 (ledger + catalog), `D-4` (no-vector); possible §3 amendment per `Q-1`
- system spec: `SS-14` `AC-3` (history awareness), `AC-10` (NL resolution)
- specific upstream ids: phase-2 successor to Task-157; provides the summarizer consumed by Task-162

## 3. Trigger

Task-157 gives a feature its committed history (commits + CA "why"). But discussion that never crystallized into a commit — "we tried X and rejected it," "the user prefers Y," "this is blocked on Z" — lives only in past chat transcripts and is lost when a new chat starts. The runner already produces per-turn artifacts and summaries ([finalizer.go](../../../apps/local-runner/internal/runner/finalizer.go)) and can resolve a chat to a feature (Task-097). The missing capability is a per-feature, time-ordered chat-summary log injected on a new chat — built deterministically, without embeddings.

## 4. Exact Change

- `T-1` `featurecatalog`/`changeledger` — add a `chat_summary` entry type (sibling NDJSON keyed by `feature_key`, time-ordered) and a `ChatSummarySlot(feature_key)` renderer that emits a "## Prior discussion on …" block (most recent `N`, capped chars), parallel to Task-157's `HistorySlot`.
- `T-2` summarizer (shared, new) — a reusable component `SummarizeTurns(orderedTurns) → cappedSummary` using a cheap model, prompted to capture goals/decisions/rejected-approaches/preferences/blockers and to *exclude* code-change enumeration (157 owns that). Reused by Task-162.
- `T-3` `runner` — on chat completion (or rolling, per `Q-2`), resolve the chat to a `feature_key` (reuse `ResolveFeature` on prompt + changed paths), summarize via `T-2`, and append a time-stamped `chat_summary` entry. Non-fatal.
- `T-4` `runner` (injection) — extend Task-157's `injectFeatureHistory` to also append `ChatSummarySlot` for the resolved feature, within the shared budget.
- `T-5` tests — entry stored and ordered by time; resolver files a chat under the right key; injection appends the discussion block bounded to `N`/char-cap; summarizer is best-effort (failure omits the block, never errors the turn); `unknown`-feature chats are not injected.

## 5. Touched Areas

- files: `apps/local-runner/internal/featurecatalog/slots.go`, `apps/local-runner/internal/changeledger/{ledger.go,query.go}` (or a new `chatsummary` package), `apps/local-runner/internal/runner/{runner.go,finalizer.go}`, shared summarizer (new), `requirements/06-System-Tech-Design/SD-17-*` (per `Q-1`)
- modules: `featurecatalog`, `changeledger`, `runner`, summarizer (new)
- routes: none new (injection on existing prompt-assembly seam); summary produced in the post-turn pipeline
- tables: none required (sibling NDJSON, local-first); Drive sync per `CP-35 §4.8` if shared

## 6. Acceptance Check

- Starting a new chat on a feature with prior discussion injects a "## Prior discussion on …" block, time-ordered, bounded to the recent `N`.
- The block carries decisions/rejected-approaches/preferences, not a re-list of code changes (which Task-157 already injects).
- A chat that resolves to no confident feature is not added and not injected.
- A summarizer failure omits the discussion block and never blocks or errors the turn; Task-157's commit history still injects.
- The chat-summary store is per-feature and time-ordered; retrieval is a direct key lookup (no embeddings).
- The summarizer component is reused unchanged by Task-162 (verified by a shared-component test).
- `go test ./internal/{featurecatalog,changeledger,runner}/...` passes.

### 6.1 Definition of Done (DOD)

All items are true for Task-161:

- [x] **DOD-1:** a `chat_summary` entry type is persisted per feature in time order.
- [x] **DOD-2:** `ChatSummarySlot(feature_key)` renders a bounded prior-discussion block beside the commit history.
- [x] **DOD-3:** the discussion block is injected only when a feature can be resolved confidently.
- [x] **DOD-4:** the injected summary stays token-bounded and omits the block cleanly when empty.
- [x] **DOD-5:** the shared summarizer component is available for Task-162 reuse (the cached `chat_summary` entry is consumed unchanged by the handoff path).
- [x] **DOD-6:** targeted Go tests for featurecatalog/changeledger/runner pass.
- [x] **DOD-7 (summarizer = cheap model, provider-dynamic):** the discussion summary is produced by a cheap-tier model of **the chat's own provider** via `Runner.SummarizeChatTranscript` (one-shot exec through `resolvePromptExecutionAdapter` — Claude `--print haiku`, Codex `exec`, Gemini `--model gemini-2.5-flash`), so a Codex- or Gemini-only user gets a real summary instead of a failed Claude call. The model id is overridable (`FLOWPILOT_SUMMARIZER_MODEL` global, or `FLOWPILOT_SUMMARIZER_MODEL_<PROVIDER>`), and a deterministic keyword-heuristic fallback keeps it best-effort and non-fatal when no account/CLI is reachable.
- [x] **DOD-8 (triggered generation + cache + non-blocking):** the rolling summary is generated by three non-per-turn triggers — a **5-minute idle timer** (any new turn resets it; `FLOWPILOT_SUMMARY_IDLE_MS` overrides), a **manual generate endpoint** (`POST …/chat-summary`: now/bypass-wait, 409 while running, no-op on hash match), and a **startup backfill scan** — and is skipped when the stored `state_key` already matches (no model call). All paths run off the turn path so none blocks finalization. Stored as an **upsert** (one line per run+feature, rewritten in place), and each feature's summary is built only from that feature's turns (per-feature bucketing — no cross-feature mixing). (CP-37 `V-161-12`…`V-161-16`.)
- [x] **DOD-9 (engine sync):** `ledger/chat_summary.ndjson` is part of `contextsync` `SharedFiles` — recorded in `.flowpilot/manifest.json` and best-effort synced to Drive `context-engine/` after a feature-resolvable turn; when Drive is unavailable the local entry still persists and the turn never errors (sync treated as skipped, not failed).
- [x] **DOD-10 (conversation-sticky resolution):** feature resolution for both injection and recording uses `resolveTurnsFeature`, which scans the conversation's user prompts newest→oldest and uses the first that clears the confidence threshold. A low-signal latest prompt (`continue`/`try again`, common after an AI error) inherits the established feature — so it still gets history injected and contributes to the summary — while an explicit topic change re-resolves to the new feature. The same helper is used by the handoff summary lookup, so all three paths agree. (CP-37 `V-161-10`/`V-161-11`.)

## 7. Out of Scope

- RAG / embeddings / semantic similarity retrieval (stays `SD-10`/CP-10; this task is deterministic and time-ordered).
- The live cross-provider handoff consumer — that is Task-162 (it reuses the summarizer built here).
- Cross-project / organization-wide chat recall (per-project only).
- Replacing CA notes; this complements them for chats that produced no commit.

## 8. Completion Notes

- result: done
- implementation notes: the shared summarizer is `Runner.SummarizeChatTranscript` ([summarizer.go](../../../apps/local-runner/internal/runner/summarizer.go)) — a one-shot exec on a cheap-tier model of **the chat's own provider** (Claude `--print haiku`, Codex `exec`, Gemini `gemini-2.5-flash`; ids overridable via `FLOWPILOT_SUMMARIZER_MODEL[_<PROVIDER>]`, Codex defaulting to the account's own default model). It resolves that provider's connected account home (Claude also accepts `ANTHROPIC_API_KEY`) and degrades to a deterministic keyword-heuristic (`heuristicSummarizeTurns`) when no model/account is reachable, keeping the path offline-safe and unit-testable. Generation is rolling (per finalized turn), `state_key`-cached (a SHA-256 of the run transcript), and runs in a goroutine off the finalize path so it never blocks the turn. Feature resolution is conversation-sticky via `resolveTurnsFeature` (scan newest→oldest substantive prompt), so continuation/retry prompts inherit the established feature. Resolves `Q-2` (rolling) and `Q-3` (revised: cheap-tier model of the active provider rather than a single shared model, so non-Claude setups are covered).
- follow-ups: Task-162 now consumes the shared summarizer's cached output; revisit the `SD-17` plane note only if the narrative needs a broader explanation later; cross-vendor pre-send summary review remains the shared `Q-3`/CP-37 `Q-3` follow-up.
- upstream docs updated: none required beyond the task links and the shared seam references already present.
