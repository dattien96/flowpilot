# Task-454: Interactive Runner Subsystem Architecture Review

- Document ID: `Task-454`
- Title: `Exhaustive architecture review of the local-runner interactive dispatch/resume/quota/cohort subsystem`
- Phase: `task`
- Status: `done`
- Owner: `devin`
- Reviewers: ``
- Created: `2026-09-28`
- Last Updated: `2026-09-28`
- Parent Documents: `CP-51`, `CP-58`, `BUG-288 series`
- Child Documents: ``
- Related Documents: `apps/local-runner/AGENTS.md` (package contract)
- Replaces: ``
- Tags: `review`, `runner`, `durability`, `dispatch`, `resume`, `quota`, `cohort`

## AI Quick View

### Summary

- Complete line-by-line read (21,881 lines) of the six interactive-subsystem
  files: `interactive_handlers.go` (2,324), `interactive_service.go` (12,194),
  `dispatch_live.go` (876), `quota_gate.go` (738), `cohort_stall.go` (500),
  `interactive_resume.go` (5,249).
- Per-file deliverable produced in-conversation: (a) exported-function
  inventory with one-line purposes, (b) verbatim hard-contract/invariant
  comment blocks plus a line-range index of all remaining invariant-bearing
  blocks, (c) every durability/persistence call site with line numbers.
- Byte-exact verbatim extraction files retained at `/tmp/inv_svc.txt`,
  `/tmp/inv_med.txt`, `/tmp/inv_small.txt`.

### Current Ask

- None — review delivered; no code changes requested or made.

### Key Decisions

- `T-1` Durable state is the single source of truth; every mutation funnels
  through `persistProviderSession` / `persistEvent` before or atomically with
  the in-memory commit (persist-first ordering at stop fence, gate settle,
  block/reprompt, post-gate completion, card create).
- `T-2` Delivery outcomes are three-way only: terminal, safely retryable, or
  explicitly uncertain (`send_started`/`provider_accepted` post-crash is the
  uncertain class — never guessed).
- `T-3` Send/Stop effects linearize through durable CAS
  (`send_claimed → send_started` before the first provider byte; Stop fence
  written before any RAM cancel).
- `T-4` Provider sessions pin per run/leg; quota rotation closes a leg and
  opens a new one — live sessions are never migrated.

### Constraints

- Read-only review: no source, test, store, or schema changes permitted.
- Every referenced `BUG-*`/`CP-*`/`Task-*` block must be quoted verbatim, not
  paraphrased.

### Open Questions

- ~22 callers discard `persistProviderSession` errors (BUG-499 — logged once
  at the funnel); approval-expiry persist failure is loud-logged but not
  retried (outbox noted out of scope); post-gate completion-event persist
  failure is observable-but-not-fail-closed.

### Source Refs

- `apps/local-runner/internal/runner/*.go` (six files above), package contract
  `apps/local-runner/AGENTS.md`.

## 1. Goal

Produce a complete, verifiable architecture inventory of the interactive
run/dispatch/resume/quota/cohort surfaces sufficient to reason about restart
safety, dispatch correctness, flow orchestration, provider-session handling,
durable state transitions, and recovery behavior — without modifying code.

## 2. Parent Links

- coding plan: `CP-51` (durable dispatch V2), `CP-58` (park/cancel semantics)
- tech design: `SD-27` (worktree), `SD-26 §5.3` (chat transcript SSOT)
- system spec: package contract in `apps/local-runner/AGENTS.md`
- specific upstream ids: BUG-288 series (R11/R13/R15–R20, P0/P1/P2), V9/V10R4
  durability rounds, Task-239/240/241/242/244/249/251/254 cohort & step-log
  work

## 3. Trigger

A drift-detection flag plus the need for a trustworthy map of the ~22k-line
subsystem before further work: the durable contract surface was scattered
across ~460 invariant comment blocks and six files too large to reason about
without a verified inventory.

## 4. Exact Change

- `T-1` Read all six files completely (chunked reads with overlap; gap-filling
  verified line coverage 100% per file).
- `T-2` Enumerate every exported function per file (1 / 20 / 5 / 4 / 0 / 1).
- `T-3` Extract every invariant-bearing comment block verbatim into
  `/tmp/inv_{svc,med,small}.txt`; quote the hard-contract blocks inline and
  index the rest by line range.
- `T-4` List every durability/persistence call site per file with line
  numbers (persistProviderSession/persistEvent/persistParentSession/
  StoreApproval/StoreQuestion/ApplyStepTransition/SetRunStatus/turn-log,
  flow-events, dispatch CAS/commit, transcript, leg claims).

## 5. Touched Areas

- files: none modified; review covers `internal/runner/{interactive_handlers,
  interactive_service,dispatch_live,quota_gate,cohort_stall,
  interactive_resume}.go`
- modules: `apps/local-runner` interactive service
- routes: read-only
- tables: none

## 6. Code Guide Signatures

No production code landed — review-only task. Exported-surface inventory is
the deliverable and is recorded in the conversation output.

```go
// <apps/local-runner/internal/runner> — unchanged
// Exported surface confirmed: RegisterInteractiveRoutes; NewInteractiveService
// (+WithRegistry/With/WithStore); SetCatalogStore; AttachRunner;
// SetFlowDefinitionStore; (*apiErr).Error; (*turnBridge).{Emit,RequestApproval,
// AskQuestion,AskQuestionCtx,SpawnAgent,SubmitFlowControl,Accepted,Terminal};
// SubmitApprovalDecision; AnswerQuestion; AskWorkflowQuestion; Interrupt;
// SetActiveAccount; ActiveAccount; DispatchV2EnvEnabled; SetDispatchStore;
// ScanDispatchRecoveryOnBoot; ResolveQuotaPreflight; CommitQuotaResolution;
// RotateUsageBudgetRun; ResumeQuotaGate; RequeueBlockedIntent.
```

## 7. Test Signatures

None — review-only task; no new behavior to test. (Reading correctness was
verified by 100% line coverage per file and overlap-boundary checks.)

## 8. Acceptance Check

- All six files covered end-to-end with zero unread line ranges.
- Every exported function listed; every invariant block either quoted verbatim
  or indexed by exact line range; every persist call site listed with line
  numbers.

## 9. Out of Scope

- Source edits, refactors, or fixes of any kind.
- Review of files outside the six named files (e.g. `agent_orchestrator.go`,
  `flow_step_runtime.go`, adapters) — flagged as follow-up surfaces.

## 10. Definition of Done

- [x] All six files read completely (line coverage verified per file)
- [x] Exported-function inventory complete (31 exported symbols across six
  files)
- [x] Hard-contract invariant blocks quoted verbatim; remaining blocks indexed
  by line range with byte-exact extraction files retained
- [x] Durability/persist call-site lists complete per file
- [x] No production code or tests modified

## 11. Completion Notes

- result: six-section review delivered in-conversation covering exported
  functions, verbatim hard-contract blocks, indexed invariant-block coverage,
  and persist call sites; extraction artifacts at `/tmp/inv_svc.txt`,
  `/tmp/inv_med.txt`, `/tmp/inv_small.txt`.
- follow-ups: BUG-499 error-discard surface, approval-expiry outbox, post-gate
  completion-event fail-open are documented soft spots for future hardening;
  orchestrator/step-runtime/adapter files remain unreviewed.
- upstream docs updated: this file.
