# CA-886 — BUG-373 headless `--print` stepId + offline projectId fix

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-373
change_type: fix
summary: runHeadless sends runner-minted stepId (resolveTurnStepID parity with interactive) and synthesizes stable local projectId offline; 3 new tests green; live --print prints PRINT_FIX_OK on grok
# --->8---

## Why

`just chat-print` (the only headless/operator scripting path) was 100%
broken: every invocation died with 400 `stepId is required`, and after that
was unblocked, with 502 `dispatch_prepare_failed` offline. TUI users never
saw either (interactive sends stepId; project picker supplies projectId).

## Change

- **`internal/tui/app/app.go` (`runHeadless` only, interactive untouched)**:
  seed `m.runHandle`/`m.stepID` from the fresh StartRun/ResumeRun handle,
  send `StepID: m.resolveTurnStepID()` (same helper the interactive path
  and Desktop parity use — resume-without-stepId falls back identically).
- **Same function**: when no catalog project resolves (offline), send
  `ProjectID: localProjectID(cwd)` (new tiny helper, fnv64a of cwd —
  deterministic per workspace, no new deps). Same cwd → same id → same
  per-project dispatch log.
- Callers of `runHeadless`: exactly one (`Run()` print branch) — no other
  path affected (grep).

## Tests

- 3 new tests in `internal/tui/app/run_headless_stepid_test.go` (fake
  httptest runner capturing the turn envelope): minted-stepId sent,
  resume-without-stepId falls back to `chat-<runId>`, offline projectId
  synthesized stable/non-empty. All green.
- Full `tui/app`: 1428 pass; only
  `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle` +
  `TestApprovalBarAndStopAreClickable` fail — proven pre-existing at
  pristine HEAD via detached worktree (no stash), files untouched by this
  fix, same pair already documented in CA-869/870.
- Full `tui/client`: green.
- Live proof (grok-4.5, per operator Grok-only constraint):
  `chat --print 'Reply with exactly: PRINT_FIX_OK' --provider grok`
  → stdout `PRINT_FIX_OK`, RC=0, tui.log `Run() headless done err=<nil>`.
- R1: zero legacy edits; stop-on-old-fail respected (2 reds proven old).
- R2: Case-1 agnostic — turn-envelope fix, no providerKey branch in the
  hunk; live-verified on grok.
- R3: happy (minted stepId), near-miss (resume fallback), degraded
  (offline projectId), lifecycle (print-to-stdout unchanged).

## Prior CA claims kept intact

- CA-867/868 (cli-tui): onboarding/modal/gate-init behavior untouched —
  only the headless branch changed; interactive suites re-green.
- No CP-63→66 claim touched (different packages; their suites not rerun
  here beyond the untouched guarantee).
