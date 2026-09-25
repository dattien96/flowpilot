---
id: BUG-454
title: Baseline suite has ~19 deterministic red tests that pre-date the bug-fix wave
status: done
version: 1
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-445]
---

## AI Quick View
- **What**: Nineteen tests fail deterministically on both the working tree and clean baseline `d191004f` — pre-existing production/test drift, not wave regressions.
- **Why**: Each is a real defect or stale expectation needing its own reproduce-first fix; none may be weakened or mass-waived.
- **Key constraint**: Reproduce-first per defect; baseline-identical ≠ resolved. These block any "suite green" completion claim.

## 1. Metadata
- Document ID: `BUG-454`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `dev-infra`
- Parent Documents: [BUG-445](./BUG-445-Preexisting-Red-Suite-Declared-Done-Without-Green-Gate.md)

## 2. Symptom and Impact
`go test -count=1 ./internal/...` fails on a clean checkout of `d191004f` with the
same set as on the wave tree. Each row below fails **identically** on baseline
(verified 2026-09-23, same machine) — proof they pre-date the wave — but they are
deterministic defects, not flakes.

| Test | Package | Failure signature | Class |
|---|---|---|---|
| TestResumeFlowWithFeedbackAfterEscalate | runner | `after resume hub = "WAITING_USER_APPROVAL", want RUNNING` | prod defect |
| TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget | runner | bail=false expected on hub.inline target | prod defect |
| TestFinalizerHookSurfacesArtifacts | runner | `"3 files changed"` vs `"3 file(s) changed"` | stale expectation |
| TestIsFlowPlannerExcludedPathCoversSkillpackScaffold | runner | `isFlowPlannerExcludedPath("docs/report.md")=true` | prod defect (over-broad exclusion) |
| TestTask330_ResumeFromTddStartsSprintWhenNoTddOutput | runner | `chatFlowRef="flowpilot-core-flow-pack/vibe-ingest"` | prod defect |
| TestSpawnChildEmitsGraphAndBusEvents | runner | graph/bus event types missing | prod defect |
| TestLocalFileSessionStoreAgentMetadataRoundTrip | runner | child session not found after round-trip | prod defect |
| TestListRemoteChatSessionsIncludesRecordsFromSameDriveRootWithDifferentProjectIDs | runner | summaries returned but assertion mismatch | prod defect / stale expectation |
| TestSupabaseCatalogStoreShaping | runner | projects endpoint URL mismatch | stale expectation |
| TestSkillsMergeClaudeProjectAndProviderHomeWithPrecedence | runner | `flowpilot-only` loaded for claude project skills | prod defect |
| TestFlowEngineSynthesisPromptIncludesJoinedReviewerNote | runner | waitLoop 3s timeout on synthesis prompt | defect or env-timing — investigate |
| TestE2EReviewLoopApprovedPathPersistsTerminalLoopStateAfterHubTurnFinishes | runner | waitLoop 8s timeout | defect or env-timing — investigate |
| TestE2EReviewLoopMultiRoundChangesThenApprovedCompletes | runner | waitLoop 8s timeout | defect or env-timing — investigate |
| TestE2EReviewLoopMultiRoundSlowSynthesisStillCompletes | runner | waitLoop 8s timeout | defect or env-timing — investigate |
| TestRepoNameFromDirUsesBasename | structure | `RepoNameFromDir("C:\\working\\flowpilot")` returns full string on POSIX | prod defect (Windows path) |
| TestScaffoldYAMLIsNotTreatedAsSkill | skillpack | react-native skills=25, want 23 | stale expectation (skill count drift) |
| TestInstall_CommonOnlyForNonePlatform | skillpack | installed=60, want 52 | stale expectation (count drift) |
| TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle | tui/app | in-flight follow-up does not keep [stop] armed (claude/codex/grok) | prod defect |
| TestApprovalBarAndStopAreClickable | tui/app | expected clickable [stop] | prod defect |

## 3. Reproduction and Evidence
- Tree: `go test -count=1 ./internal/...` → same set red.
- Baseline `/private/tmp/fp-baseline @ d191004f` (pre-wave HEAD): identical
  failures, byte-equal signatures — verified by running the same `-run` set
  there on 2026-09-23.
- Separately, **env-dependent** reds (not defects — machine config differs):
  `TestDetectProvidersPopulatesInventoryShape` (4 vs 6 providers — devin/
  opencode installed here), `TestResolveGoogleDriveMcpProviderStatuses_`-
  `AllNotStarted` (same 4-vs-6), `TestCleanupSessionsTearsDownProviderPools`
  (live claude process reaping), `TestFirebaseToolsMcpAdapterFetchEndToEnd`
  (MCP/network), `TestGitNexusDependentsSmokeScopeDiff` (repo not indexed in
  GitNexus registry), `TestCatalogStoreForFallsBackToFake` (machine has a real
  supabase config, so fallback does not trigger).
- **Flakes** (pass isolated / TempDir-cleanup race family):
  `TestVibeSprintFreezeSpawnsTddThenCoder` (unlinkat cleanup),
  `TestRunContractFreezeNodeBindsContractToCoderStep`,
  `TestListProviderAccountsRecoversManagedCodexSlotsFromDisk`,
  `TestGrokPreflightContractUsesSharedParserAndGate`,
  `TestRunContractFreezeNodeReuseBindsMissingSibling`,
  `TestRunContractFreezeNodeAllowsPreExistingDirtyWorktree`.

## 4. Acceptance and Verification
- Each deterministic row gets a reproduce-first fix (or a linked, expiry-bound
  waiver); env rows get documented waivers; flakes get a tracked cleanup-race
  investigation.
- Do NOT modify old assertions to force green; do NOT claim `/safe-fix-contract`
  satisfied for the suite until every row is green or explicitly waived.

## 5. Resolution (2026-09-23)

All 19 deterministic rows resolved — 5 real production fixes, 14 stale
fixture/expectation corrections (each verified against the commit that
changed the contract). CA-936b / CA-937b / CA-938b.

| Test | Resolution |
|---|---|
| TestResumeFlowWithFeedbackAfterEscalate | PROD FIX — `resumeFlowWithFeedback` restamps hub when `lastEscalatedInlineNodeID==""` (hub self-escalation). CA-936b |
| TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget | STALE — `hub.inline` dispatches hub-notify inline since CP-58 `16e78b20`; bail coverage moved to `user.confirm` (control scope) |
| TestFinalizerHookSurfacesArtifacts | PROD FIX — `handleListArtifacts` stops serving `fakeArtifacts` once a turn completed. CA-936b |
| TestIsFlowPlannerExcludedPathCoversSkillpackScaffold | STALE — `*.md` exclusion intentional since `e39d261d`; fixture now uses `docs/report.go` |
| TestTask330_ResumeFromTddStartsSprintWhenNoTddOutput | STALE — Task-327 requires the locked SS artifact; fixture seeds `SS-1` + `vibeLockedSS` |
| TestSpawnChildEmitsGraphAndBusEvents | STALE — bus event emits on async child completion; test polls event endpoint |
| TestLocalFileSessionStoreAgentMetadataRoundTrip | STALE — hardcoded `UpdatedAt` drifted past 90-day `sessionStoreMaxAge`; relative timestamps (ece72e4d pattern) |
| TestListRemoteChatSessionsIncludesRecordsFromSameDriveRootWithDifferentProjectIDs | STALE — `ece72e4d` moved codex seed to `time.Now()`; claude `UpdatedAt` now relative so ordering holds |
| TestSupabaseCatalogStoreShaping | STALE — `platform` column added by CP-56 `4bb804e1` |
| TestSkillsMergeClaudeProjectAndProviderHomeWithPrecedence | STALE — `.agents/skills` common pack loads for all providers since BUG-062 F-2 `bbca52dc` |
| TestFlowEngineSynthesisPromptIncludesJoinedReviewerNote | STALE — reviewer fake must call `submit_review_outcome` (CP-67 machine verdict) |
| TestE2EReviewLoop* x3 | PROD FIX — child reprompt on transient `turn_in_progress` parked undrainable `pendingHubReinvoke` on the child; retry now reschedules via `scheduleChildTurn`. CA-936b + red test `bug454_child_reprompt_retry_test.go` |
| TestRepoNameFromDirUsesBasename | PROD FIX — `repoNameFromDir` splits `\` too. CA-938b |
| TestScaffoldYAMLIsNotTreatedAsSkill / TestInstall_CommonOnlyForNonePlatform | STALE — `flow-harness-contract` + `vibe-lanes` intentionally added by Task-412/CA-903 `dd217c75`; counts updated |
| TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle | PROD FIX — `turnIsActive` BUG-371 guard now exempts in-flight turns (`turnSendPending`/open `turnStream`). CA-937b |
| TestApprovalBarAndStopAreClickable | STALE — CA-826 `8d3162d4` intentionally hides [stop] under an approval; test now asserts that contract |

Post-fix full `./internal/...` run: remaining reds are only the six
env-waived tests plus the documented TempDir-cleanup/order flakes
(`TestBug360FreezeProceedsFromStashWithoutScoutChild`,
`TestPlanPhaseRoundResetOnResumeApprove`, `TestRun75035_…SiblingPollution`
all pass isolated — the last is an order-dependent env-shared-state flake,
same bucket).
