# CA-916: Scaffold Live Progress Feed for TUI and Desktop

## Summary

The AI Scaffold Turn ran as a synchronous POST that returned only a final
`ScaffoldDispatchResult` — TUI showed "Starting AI Scaffold…" then silence, and
Desktop's create-project auto-trigger was invisible entirely. Users could not
tell whether the AI was actually running (CA-910 added only a busy spinner).

The runner now owns a per-project **scaffold progress feed**: phase milestones
(`started`/`recipe`/`ai_turn`/`gate`/`heal`) plus live provider-stdout deltas,
persisted to `<workspace>/.flowpilot/scaffold-progress.ndjson` and served at
`GET /client/projects/{id}/scaffold/progress?after=<seq>`.

- `PromptExecutionRequest.OnStdoutDelta` — `ExecutePrompt` tails the provider's
  `stdout.txt` artifact every 150 ms while the process runs, so output streams
  before the process exits. Gemini's Agy capture buffers stdout in memory, so
  its deltas arrive as a single flush at turn end; every file-stdout provider
  (claude/codex/grok/opencode/devin) streams identically.
- `ScaffoldRequest.OnProgress` — dispatcher emits recipe/ai_turn/gate/heal
  milestones; each `executeTurn` wires `OnStdoutDelta` → `output` events.
- `beginScaffoldProgress`/`endScaffoldProgress` bracket both dispatch paths
  (HTTP POST and the create-project auto-trigger). The feed is marked active
  before the create-project response returns so an immediately-polling client
  sees the turn as live. A nil `Dispatch` result still closes the feed with an
  error event so pollers never hang.
- TUI: `ScaffoldProgress` client call piggybacks on the existing thinking
  ticker (every ~8 ticks ≈ 720 ms while `scaffoldBusy`, guarded by
  `scaffoldProgressInFlight`). Output events append through
  `appendAssistantDelta` — literally the chat streaming path — and the current
  phase labels the busy line. A final fetch after the POST result flushes
  stragglers. `cmdDispatchScaffold`'s signature and the returned
  `EngineScaffoldMsg` are unchanged (CA-910 contract preserved).
- Desktop: `fetchScaffoldProgress` + pure `applyScaffoldSnapshot` reducer +
  `ScaffoldActivity` card mounted in ProjectsSettings (selected/created
  project) and EngineSettings (selected project). It polls until terminal or
  ~7 s of inactivity with no events, then hides — no card churn for
  non-capable platforms.

## Tests

- `TestScaffoldProgress_DispatchStreamsPhaseAndOutputEventsHTTP` — POST → poll:
  `started → recipe → ai_turn` phases, both stdout deltas, `result=done`,
  monotonic seq.
- `TestScaffoldProgress_AfterCursorReturnsOnlyNewerEventsHTTP` — `?after=`
  filters strictly, `nextSeq` consistent.
- `TestScaffoldProgress_SkippedDispatchEmitsResultEventHTTP` — skipped dispatches
  still close the feed with a result event.
- `TestScaffoldProgress_ReplaysPersistedLogAfterHubLossHTTP` — a fresh service
  over the same workspace replays the NDJSON tail (durable restart contract).
- `TestTailPromptStdout_StreamsAppendsBeforeClose` — tailer emits mid-run
  appends and flushes the remainder on stop.
- TUI: 6 tests — output→assistant stream, phase on busy line, tick-scheduled
  poll, in-flight guard, server-backed fetch, final flush.
- Desktop: 5 reducer tests — delta concat, milestones, terminal/skipped,
  monotonic cursor, hidden-when-empty.
- Existing scaffold/init/busy suites all green; runner package retains only the
  known env-dependent baseline (`TestIsFlowPlannerExcludedPath…` — gitnexus,
  untouched by this change).

## Live validation (Devin SWE-2 only)

Real runner (`/tmp/fp-runner`, port 47935, client-managed) + real workspace:
`POST /engine/init` installed 100 skills → `POST /scaffold
providerKey=devin modelName=devin/swe-2-high` → concurrent
`GET /scaffold/progress` polls showed `active=true phase=ai_turn` with streamed
Devin stdout ("I'll start by reading all four skill files…") while the POST was
still in flight; the turn scaffolded a real RN monorepo (apps/, packages/,
turbo.json, pnpm-workspace.yaml) and the feed closed with the gate verdict.

## Provider parity

Shared-logic case: `OnStdoutDelta` is consumed by `ExecutePrompt` only via the
stdout artifact file — no `providerKey` branch in the tailer. The single
divergence is Gemini (in-memory Agy capture → one flush at end), documented and
intentional; Devin live-verified, other file-stdout providers identical by
construction.

## GitNexus

`gitnexus impact` ran on `Dispatch` (HIGH, 2 callers — both updated),
`handleDispatchScaffold` (LOW), `appendAssistantDelta` (LOW — reused unmodified).
`detect_changes` is MCP-only and unavailable via CLI; scope verified through
`git status`/`git diff` against the declared contract.

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: CP-68
change_type: feature
summary: live scaffold progress feed (phases + stdout deltas) polled by TUI and Desktop so the AI scaffold turn renders like a chat run
# --->8---
