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

## 7. Validation & E2E Test Guide

Validation has two layers:

- **§7.1 Automated pre-E2E sweeps** — run these first; they back most of the unit-level `V-###` cases.
- **§7.3 Manual E2E walkthrough (Tests A–G)** — a hands-on, click-through pass in the **real desktop app** against the prepared `gate-sandbox` fixture (§7.2). This is the followable "verify behavior end-to-end" layer.

Every formal `V-###` case maps to one of these — see the **§7.4 case index**. (A standalone copy of the walkthrough also lives at `D:\working\gate-sandbox\CP-37-E2E-TEST-GUIDE.md`.)

### 7.1 Automated pre-E2E sweeps (run first)

- **Runner:** `cd apps/local-runner && go test ./internal/changeledger ./internal/featurecatalog ./internal/flowgate ./internal/runner ./internal/promptblock ./internal/contextsync`
- **Desktop:** `cd apps/desktop-flowpilot && npm run typecheck`

These cover the sweep cases (`V-157-08`, `V-161-09`, `V-078-07`, `V-162-06`) and the unit-level cases called out by name in the walkthrough below (`V-157-03/04`, `V-161-04/05`, `V-078-02/03/04/05`, `V-162-03`, etc.).

### 7.2 Prepared test fixture (gate-sandbox)

**Two clean features are seeded** so the pivot / no-mixing cases have somewhere to pivot to:

| Fixture | State | Powers |
|---------|-------|--------|
| `change-audit/FEATURE-KEYS.md` | registers `calc-core` **and** `calc-format` | key validation (high-confidence) |
| Git history — feature A | 2 commits `[Feature][calc-core][logic] …` on `calc.go` (`add multiply`, then `Task-900 add divide with zero guard`) | ordered "Prior work" for calc-core |
| Git history — feature B | 2 commits `[Feature][calc-format][logic] …` on `format.go` (`add Sign helper`, then `Task-901 add Clamp range helper`) | ordered "Prior work" for calc-format |
| `change-audit/CA-900-*.md`, `CA-901-*.md` | `## Scope` + `## Residual Notes`, linked via `source_doc_id: Task-900 / Task-901` | Tier-2 CA "why" excerpts |
| `.flowpilot/ledger/feature_history.ndjson` | calc-core: multiply→divide (high, `ca_excerpt`); calc-format: Sign→Clamp (high, `ca_excerpt`) | injected commit history |
| `.flowpilot/catalog/features.ndjson` | `calc-core` (globs `calc.go`) and `calc-format` (globs `format.go`) with keywords | feature resolution + SuggestKey |
| `.flowpilot/ledger/chat_summary.ndjson` | 2 `calc-core` + 1 `calc-format` seeded discussion summaries | injected "Prior discussion" |
| `.flowpilot/settings/gate-config.json` | `gate_mode: enforce` | the feature-key gate reprompts (not warn-only) |

> **Prompt wording matters.** Feature resolution needs a score ≥ 5, so a prompt must contain a distinctive feature keyword (or the key itself), not just the bare word "calc":
> - `calc-core` ← include **arithmetic / multiply / divide / operations** (e.g. "the calc-core arithmetic divide").
> - `calc-format` ← include **number / formatting / format / Sign / Clamp**.
>
> A vague prompt like "improve calc" resolves to nothing (no block injected) — that is correct behavior, not a bug.

> **Inspect the real injected prompt (on by default).** Every turn writes the fully-composed prompt under the **FlowPilot tool workspace** (where the runner runs — *not* inside the target project), namespaced by project id: `<runner-workspace>\.flowpilot\runs\<projectId>\<runId>\prompt-<turnId>.txt`, latest at `…\<projectId>\last-prompt.txt`. Open `last-prompt.txt` to confirm the `## Prior work on …` / `## Prior discussion on …` blocks were injected. (Disable with `FLOWPILOT_LOG_PROMPT=0`.)

**Prerequisites & bind**

- FlowPilot desktop app running; a **Claude** account connected (chat provider *and* the cheap-model summarizer); a **second** provider (e.g. **Codex**) for the handoff tests (D); *(optional)* Google Drive chat sync for C3.
- Optional: `FLOWPILOT_SUMMARIZER_MODEL` to override the cheap model; `FLOWPILOT_SUMMARY_IDLE_MS` to shorten the idle-summary window for F2.
- Bind the project at `D:\working\gate-sandbox`, let engine init finish, pick **Claude** + **Normal** chat mode.

### 7.3 Manual E2E walkthrough (Tests A–G)

#### Test A — Feature-history + CA "why" injection (Task-157)

**Do:** in a new Normal chat, send: `What would it take to add a safe arithmetic divide operation to calc-core?`

**Expect:** the assistant already knows `calc-core` has `Add`/`Subtract`/`Multiply`/`Divide`, treats the **divide-with-zero-guard** commit as the current state, and references the **residual note** (divide-by-zero returns 0; error propagation deferred) instead of re-proposing what exists. Confirm via `last-prompt.txt`: a `## Prior work on "calc-core"` block, oldest→newest, divide marked `← current truth`, with the Tier-2 CA excerpt under it.

**Negative control:** send `write a haiku about the sea` → no "Prior work" block (feature does not resolve; safe fallback).

*Covers: `V-157-01` (happy path), `V-157-06` (Tier-2 excerpt), `V-157-02` (safe fallback / no-resolve).*

#### Test B — Feature-key gate reprompt (Task-157)

**Do:** ask the chat to make a small change and commit it with a **bad** key:
> `Add an Abs(a int) int helper to calc.go and commit it with exactly the message "[Feature][calculator][logic] add abs".` (`calculator` is not registered.)

**Expect:** the post-turn flow gate fires a **reprompt** (`commit_feature_key_missing` / `r-fk`) inline; the remediation suggests the verified key **`calc-core`** (derived from the changed path `calc.go`) **and** offers "register a new key in `FEATURE-KEYS.md`". Re-committing as `[Feature][calc-core][logic] add abs` passes.

**Variant (warn mode):** set `gate-config.json` to `{"gate_mode":"warn"}`, repeat → a **warning**, not a blocking reprompt. Restore `enforce`.

*Covers: `V-157-05` (gate reprompt), `V-157-04` (path-anchored SuggestKey), `V-157-03` (key validation).*

#### Test C — Prior-discussion injection + rolling summary + sync (Task-161)

**C1 — discussion injected.** New Normal chat: `Picking the calc-core arithmetic divide back up — where did we land on error handling?`
→ the assistant reflects the **seeded discussion** (integer semantics, float division rejected, divide-by-zero returns 0, open `DivideChecked` question). Confirm a `## Prior discussion on "calc-core"` block in `last-prompt.txt`.

**C2 — rolling summary recorded.** Have a 1–2 turn exchange about calc-core, then let the chat go idle. Inspect `.flowpilot/ledger/chat_summary.ndjson` → a row for `calc-core` with a fresh `run_id`, `state_key`, increasing `created_at`. Re-opening with no new turn must **not** duplicate it (state-key cache). *(See Test F for the trigger that produces this.)*

**C3 — Drive sync (optional).** With Google Drive chat sync connected: Drive `context-engine/` contains `chat_summary.ndjson` beside `feature_history.ndjson`/`features.ndjson`, and `.flowpilot/manifest.json` lists `ledger/chat_summary.ndjson`. With Drive disconnected: the local row still appears, the turn shows no error (sync "skipped", not "failed").

*Covers: `V-161-02` (discussion injection), `V-161-01/03` (append + unknown suppression), `V-161-06/07/08` (sync + degrade); summarizer-failure degrade `V-161-04` and reuse `V-161-05` are unit-level.*

#### Test D — Cross-provider handoff: raw floor + summary hybrid (Task-078 · Task-162)

**Setup:** continue a Claude chat with ≥3 turns about calc-core.

**D1 — switch provider.** While **idle**, click the **Codex** chip → a **confirmation modal** opens (source/target/model/source-run; current run unchanged). Click **"Start new chat with Codex"**.
**Expect:** exactly **one** new Codex run; old Claude run preserved/re-openable; the first user turn is the handoff prompt (ordered raw `User:`/`Assistant:` inside `<previous_conversation>`, no hidden/system/tool/reasoning content). A system line reports **"Handoff from Claude used `<mode>` context"** — `hybrid` (cached summary present, `<conversation_summary>` precedes raw) / `target_summary` (no summary + history truncated → self-summarize instruction) / `raw` (no summary, whole convo fit). All three succeed; the switch never blocks.

**D2 — force hybrid.** Have a couple of committed turns so a `calc-core` rolling summary exists with a matching `state_key`, then switch → system line reads **`used hybrid context`** and the first Codex turn shows `<conversation_summary>` + recent raw.

**D3 — guards.** Switching **while a turn runs** → chips disabled. **Double-click** confirm → still exactly one run. **Empty** chat → switches immediately, no modal. A **Gemini-source** chat is rejected (`handoff_source_provider_unsupported`); Gemini is fine as a target.

*Covers: `V-078-01/06` (switch + orchestration), `V-078-05` (Gemini-source rejection), `V-162-01` (hybrid), `V-162-02` (degrade), `V-162-05` (Task-157/161 context still injected on the target's first turn); `V-078-02/03/04` (reconstruction, 64 KiB bound, oversized-turn) and `V-162-03/04` (run-scoped + state refresh) are unit-level.*

#### Test E — Conversation-sticky resolution + explicit pivot (Task-161)

**E1 — low-signal inherits the feature.** In a `calc-core` chat (e.g. "improve the calc-core arithmetic divide"), after a turn or two send a low-signal turn: **`try again`** or **`continue`**.
→ the next prompt still injects the `calc-core` blocks (resolution scans back to the last substantive prompt), and a summary is still recorded under `calc-core`, not `unknown`.

**E2 — pivot re-resolves.** In the same chat send **`now add number formatting helpers`** → the injected blocks switch to `calc-format` (Sign→Clamp + its CA "why" + discussion), not clinging to calc-core.

*Covers: `V-161-10` (record-path inherit), `V-161-11` (inject fallback + pivot).*

#### Test F — Generation triggers (Task-163)

**F1 — manual "Gen summary" button.** With a `calc-core` chat **idle/completed**, the **Gen summary** button near the YOLO toggle is enabled (disabled while running). Press → system line "Chat summary updated." and the `calc-core` row is written/refreshed. Press again with no new turn → "Chat summary unchanged" (hash match, no model call).

**F2 — idle timer + reset.** Launch with `FLOWPILOT_SUMMARY_IDLE_MS=15000`. Complete a `calc-core` turn, wait 15 s with no new turn → a summary appears on its own. In another chat, send a turn then another before 15 s → the countdown resets; the summary lands ~15 s after the *last* turn. No summary while a turn is in flight.

**F3 — upsert.** Across several `calc-core` turns in one chat, `chat_summary.ndjson` keeps **one** `calc-core` row, rewritten in place (not one per turn); `state_key` is a fixed-length hash that changes only when the transcript changes.

**F4 — startup backfill.** Stop the runner, delete the `calc-core` row, restart (`serve`) → the one-shot background scan regenerates it without user action; hash-matched rows are left untouched.

*Covers: `V-161-12` (manual button), `V-161-13` (idle timer + reset), `V-161-15` (upsert + hashed state_key), `V-161-16` (startup backfill).*

#### Test G — No cross-feature mixing (Task-163)

In **one** chat, discuss `calc-core` for a couple of turns, then pivot to `calc-format`. Trigger a summary (idle or the button) and inspect `chat_summary.ndjson`.
→ the `calc-format` row summarizes only the format discussion (no divide/zero-guard leakage); the `calc-core` row only the arithmetic discussion. Each feature's summary is built from its own turns.

*Covers: `V-161-14` (per-feature bucketing).*

### 7.4 Formal case index (`V-###`)

| Case | Covered by |
|------|-----------|
| `V-157-01`, `V-157-02`, `V-157-06` | Test A |
| `V-157-03`, `V-157-04` | Test B (+ automated sweep) |
| `V-157-05` | Test B |
| `V-157-07` (no-spec degrade) | automated / fixture without `FEATURE-KEYS.md`+CA |
| `V-157-08` | §7.1 runner sweep |
| `V-161-01`, `V-161-02`, `V-161-03` | Test C |
| `V-161-04`, `V-161-05` | automated (summarizer degrade / handoff reuse) |
| `V-161-06`, `V-161-07`, `V-161-08` | Test C3 |
| `V-161-09` | §7.1 runner sweep |
| `V-161-10`, `V-161-11` | Test E |
| `V-161-12`, `V-161-13`, `V-161-15`, `V-161-16` | Test F |
| `V-161-14` | Test G |
| `V-078-01`, `V-078-06` | Test D1 |
| `V-078-02`, `V-078-03`, `V-078-04` | automated (`internal/runner` handoff tests) |
| `V-078-05` | Test D3 |
| `V-078-07` | §7.1 desktop typecheck |
| `V-162-01` | Test D2 |
| `V-162-02`, `V-162-05` | Test D1 |
| `V-162-03`, `V-162-04` | automated (`internal/runner` handoff tests) |
| `V-162-06` | §7.1 runner + desktop sweeps |

### 7.5 Reset / re-seed (sandbox)

- The four feature commits (`calc-core` ×2, `calc-format` ×2) + `CA-900`/`CA-901` are the durable fixture; do not squash them.
- The runner **upserts one row per `(run, feature)`** in `chat_summary.ndjson` and may rewrite it — keep a copy of the seeded baseline before experimenting and restore it to reset prior-discussion.
- Fixture/maintenance commits use the `sandbox-meta` key and the ledger cursor is pinned, so they never pollute `calc-core`/`calc-format` history. To rebuild cleanly from git, re-bind the project (engine init is cursor-based / incremental).

## 8. Out of Scope

- RAG / semantic retrieval / vector memory — stays `SD-10` / `CP-10` (broader cross-chat semantic recall), a separate track.
- The detailed implementation of each phase — owned by the four tasks; this CP coordinates, it does not re-specify.
- GitNexus structural context (Plane B), regression oracle (Task-156), resolution UX (Task-155).

## 9. Completion Notes

- result: planned — coordination plan for the four context-injection tasks; sequences `P-1 → P-2 → P-3 → P-4` across `CP-35` and `CP-18` without reopening them.
- follow-ups: confirm the `SD-17` plane note for P-2 (`Q-1`); resolve cross-vendor handoff-summary review (`Q-3`); record in `CP-35 §4.2` that resolver→prompt injection is delivered (P-1) and in `CP-18 §5.7` that the AI-summary handoff is delivered (P-4).
- upstream docs updated: none yet.
