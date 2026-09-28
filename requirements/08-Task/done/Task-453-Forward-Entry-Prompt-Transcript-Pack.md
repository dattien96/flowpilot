# Task-453: Forward Entry Prompt — Bounded Transcript Pack

- Document ID: `Task-453`
- Title: `Entry-node prompt = forward text + bounded settled chat transcript, packed via promptpacker`
- Phase: `task`
- Status: `done`
- Created: `2026-09-28`
- Parent Documents: `CP-89`, `Task-452`
- Related Documents: `Task-341` (context profiles), `promptpacker`, `CP-89-Test-Steps`
- Tags: `flow`, `promptpacker`, `context-budget`

## AI Quick View

### Summary

CP-89 §3.2 requires the entry node's prompt to be the forward text plus the
settled chat transcript — so `ingest_reader` sees what user and hub already
agreed. Raw concat is unbounded (F-6). This task builds the packed forward
prompt through the existing `promptpacker` path with a hard cap.

### Current Ask

`buildForwardPromptPackage(rs, forwardText)` — collects settled user+hub
turns oldest→newest, packs under the entry node's context budget, emits a
deterministic prompt the forward seam passes to `startResolvedFlow`.

### Key Decisions

- Only **settled** turns are included: a still-streaming or failed turn is
  excluded (three-outcome contract — never fold half-turns).
- Roles: `user` + `hub` assistant messages only; system/tool frames never
  leak into the child prompt.
- The pack rides `promptpacker` with a declared cap derived from the entry
  node's `contextProfile`/budget (Task-341 schema) — overflow degrades
  oldest-first with a `context_degraded` note in the flow audit, matching
  existing degrade-soft semantics.
- Forward text (if any) is always last and never truncated away — it is the
  instruction the transcript informs.
- Deterministic ordering: turn index ascending; identical input → identical
  package (tests assert byte equality on replay).

### Constraints

- Child prompt budget is a hard bound — transcript must not evict the
  forward instruction or the node system contract.
- Transcript source is the runner's own chat store (SSOT per AGENTS §2), not
  provider-side session folders.
- No new store: the package is assembled at forward time and passed in-line;
  audit fields (turn count included, bytes) ride existing flow audit.

## 1. Goal

The entry child sees the agreed chat context inside a hard budget — the
"already discussed" half of chat_then_forward actually lands.

## 2. Parent Links

- coding plan: `CP-89` §3.2(2), §8 (F-6)

## 3. Trigger

Note requires transcript fold-in; nothing today packages multi-turn chat
context into an entry prompt.

## 4. Exact Change

- `T-1` `forwardPromptPackage` builder: enumerate settled chat turns for the
  run (user + hub), assemble sections `[chat transcript]` + `[forward
  instruction]`.
- `T-2` Budget: resolve entry node's context budget; pack via
  `promptpacker` API with transcript as a degradable source and forward
  text as a pinned source.
- `T-3` Wire: `forwardPinnedFlow` (Task-452) calls the builder and passes
  the packaged prompt as the entry-node input; `flowStartOnly` unchanged.
- `T-4` Audit: emit `forward_prompt_packed` flow-audit entry with
  `turns_included`, `bytes`, `degraded` flags.

## 5. Touched Areas

- files: `interactive_service.go` (forward seam call site), new
  `forward_prompt.go`, `promptpacker` (consume only)
- modules: `runner`, `promptpacker`
- routes: none
- tables: none new

## 6. Code Guide Signatures

```go
type ForwardPromptPackage struct {
    Prompt         string // final child-prompt string
    TurnsIncluded  int
    Bytes          int
    Degraded       bool   // transcript truncated oldest-first
}
func (s *InteractiveService) buildForwardPromptPackage(rs *interactiveRun, forwardText string, budget int64) (ForwardPromptPackage, *apiErr)
func settledChatTurnsForRun(rs *interactiveRun) []ChatTurn // runner SSOT store only
```

## 7. Test Signatures

- `TestTask453_PackageContainsForwardAndTranscript` — child prompt contains
  the forward text and a marker line from a settled chat turn.
- `TestTask453_OnlySettledTurnsIncluded` — in-flight/failed turn excluded.
- `TestTask453_RolesFiltered` — only user+hub frames; no system/tool text.
- `TestTask453_BudgetCapTruncatesOldestFirst` — N large chat turns → prompt
  ≤ budget; forward text intact; `Degraded` true.
- `TestTask453_EmptyTranscriptForwardsTextOnly` — first-turn forward works.
- `TestTask453_BareForwardEmptyTextStillPacks` — transcript-only package is
  valid.
- `TestTask453_DeterministicReplay` — same state → byte-identical Prompt.
- `TestTask453_AuditEntryRecorded` — `forward_prompt_packed` fields present.

## 8. Acceptance Check

Live: chat 3 turns on a `pending` run, forward — the spawned entry child's
dispatched prompt contains the forward line and at least one settled chat
line, under budget.

## 9. Out of Scope

- `forwardFlow` admission/fences (Task-452). Summary-compression of the
  transcript (future — this task packs, never summarizes).

## 10. Definition of Done

- [ ] §6 signatures landed or deviation documented
- [ ] §7 tests additive + green; package suite green
- [ ] Child prompt provably ≤ entry budget on oversized transcripts
- [ ] Deterministic across replay
- [ ] CA entry + commit `[Feature][forward-prompt] ...`
