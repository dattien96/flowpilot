# FlowPilot fix session — live run-3362 review (vibe flow on PrivateVault production)

Working repo: /Users/macmini/code/flowpilot. PrivateVault is READ-ONLY evidence.

## Change contract (frozen file list)

- apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml
- apps/local-runner/internal/agentpack/flow-pack/prompts/scaffold-contract-tdd.md
- apps/local-runner/internal/runner/vibe_sprint_boundary.go
- apps/local-runner/internal/runner/vibe_gate.go
- apps/local-runner/internal/runner/interactive_service.go
- apps/local-runner/internal/runner/flow_validate_audit_dispatch.go (freeze scope / audit finalize)
- apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx
- apps/desktop-flowpilot/src/components/FlowAwaitingUserCard.tsx
- apps/desktop-flowpilot/src/components/flowAwaitingUserDrift.ts (if needed)
- New additive test files only (never edit existing tests)
- change-audit/CA-1092+ entries

## Issues (from user review of live run-3362)

- A1: desktop shows no "task x/y <name>" chip (TUI has vibeTaskChip; contract fields exist in AgentLoopState).
- A2: vibe_sprint_boundary card button says "Retry" — must be "Continue" (advances next task).
- B1: tdd/scaffold leg wrote prod files — incl. MODIFYING files that existed at freeze base (39751f2: settings.gradle.kts, libs.versions.toml, build.gradle.kts). DECISION (user): bounded stubs — create NEW prod files with whitelist stub bodies OK; modifying pre-existing prod files forbidden for tdd. Frozen scope for tdd must mark existing-at-base paths read-only.
- B2: vibe-sprint has NO reviewer node (coder→validate→synthesis). Add reviewer per task-harness pattern: delegate, read_only, cohort=review, join=all, prompts/review-safe-fix-contract.md; edges validate->reviewer, reviewer->synthesis, validate->coder(continue/back). Covers all entries (vibe-tasks/cp-ingest/ingest all funnel into vibe-sprint).
- B3: sprint boundary parks a user gate between tasks — must auto-continue. User gates only: cp_lock + debate cap. Keep stop/cancel + budget/locked semantics.
- C1: joined owner note reached synthesis without content — owners are verdict_only; note renders only FinalMessage (truncated 1500); machine verdicts not rendered into note; note also must reach the debate_synthesis reinvoke reliably.
- C2: violation-routed gate blocks during a mounted debate re-mount ANOTHER debate (nested). CA-1063's inDebate suppression only covers drift-only path; applyVibeGateResolver's vibeGateOwnerDebate branch lacks the guard. Also: parent hub turns got r-tests/r-reg blocks on the still-red suite mid-remediation -> debate churn engine (12 blocks, 15 owner pairs).

## Evidence base (PrivateVault, read-only)

- .flowpilot/contracts/frozen_contracts.ndjson: coder_step_id=tdd bound full prod declared_paths, every sprint.
- .flowpilot/gate-metrics.ndjson: tdd children (run-6823/9093/11448/15514) = gate_blind red_at_capture (warn, contract-first downgrade); 12 r-tests+r-reg blocks ALL on parent run-3362 hub turns.
- sessions.ndjson: owner verdict content DID reach parent store (preflight_draft_result: "reprompt the coder, fill three TODO bodies") but synthesis kept escalating "joined note missing".
- Sprint-5 coder leg NEVER spawned (coders: run-6017/8425/10211/12553 only = 4); run settled done via flow_audit_vibe_missing_key_auto with openIssues=3 — audit auto-finalize bypassed failing gate.

## Invariants

- Old tests untouched & green; additive tests only; reproduce-first per bug.
- [Type][feature] commit format; CA entry per change; gofmt+vet; provider parity proven or tested (runner paths are provider-agnostic where possible).
- Hub-only routing; durable state; bounded loops; fail-closed gates.

---

# Live-run captures — run-100368 (PrivateVault vibe-tasks, 2026-10-02 ~20:00-21:15)

Runner PID 67586 (port 4317). Parent run-100368 = vibe CP-02 chain: Task-025 settled DONE (sprint boundary crossed, doc → requirements/08-Task/done/), Task-026 sprint in progress (tdd leg run-127453 churning).

## Captured bugs (deferred — fix after live run)

- **BUG-567** debate overlay stays mounted after restart: `vibeParkedNodes`/`activeFlowNodes` not restored → `submit_review_outcome` never offered (flowRequiresHubMachineVerdict checks debate node set, no `review` cohort) → reviewer verdicts cannot land until overlay resolves. Workaround: continue + guidance routes `continue → debate_trigger`, fresh debate resolves → `restoreVibeFlowAfterDebate` fires.
- **BUG-568** debate restore/reseed wipes completed step statuses — `coder` step reseeded DONE→PENDING → `vibeSprintEvidenceComplete` false → audit `blocked_missing_feature_key` can't auto-finalize. Also wiped preflight/context/tdd → needless re-walk.
- **BUG-569** `maybeReinvokeCoderForContinue` matches child legs by node ID `coder`, but existing children are labelled `task025_coder*` → silent no-op, step RUNNING with no leg. Workaround: manual `/spawn-agent` with label `coder`.
- **BUG-570** remediation reprompt loop on tdd leg: turn ends zero_delta → post-turn gate routes owner-debate → debate resolves with no verdict text → identical reprompt → repeat. run-127453 hit ~4 rounds.
- **BUG-571** `pending_flow_gate_settle` wedges on both legs AND parent hub run — settle eval never completes; only `agent-loop/continue` clears it.
- **BUG-572** audit defer→PENDING after last cohort join → nothing re-dispatches it (defer relies on join/settle trigger that already passed). Needed operator `flow-control done` pump.
- **BUG-573** (owner_2 surfaced): `IsTestFile` misses `*_test.cpp` → stub-whitelist false positive on C++ suites.
- **BUG-574** (owner_2 surfaced): gate result parser only recognizes `go test`/npm/pytest output — red gtest+bash run reads as green (false green) even though `test_baseline.json` records `suite_passed: false`.
- **BUG-575** (desktop UI): opening a vibe/flow-armed chat from HISTORY does not switch to Flow Mode — `store.ts` ~3829 `isWorkflowHistoryItem` checks `runKind !== "chat"`, but vibe runs persist `runKind="chat"` with `flowArm="started"` + `chat_flow_ref` set → chatMode coerced to `normal_chat`, board/timeline never render. Fix direction: treat `flowArm==="started"`/`flowRef` as workflow surfaces regardless of runKind.
- **BUG-576** (desktop UI, verify): top bar chip showed `Completed run-100368` while backend status was `running` — possible stale status chip/notification. Confirm repro before fixing.

## Live workarounds used (HTTP, no UI click)

- `POST /client/workflow-runs/{id}/agent-loop/continue` {"feedback": "..."} — clears settle-gate wedge, unblocks escalate parks, re-drives stalled nodes.
- `POST /client/workflow-runs/{id}/flow-control` {"status":"done","summary":"..."} — operator path bypasses CA-1087 stale-hub guard, dispatches audit/next node.
- `POST /client/workflow-runs/{id}/spawn-agent` {label:"coder",...} — manual leg spawn for BUG-569 label mismatch.
- `POST /client/questions/{qid}/answer` — answer reviewer/gate questions to unblock legs.
- `POST /client/workflow-runs/{id}/agent-loop/stop` — force-settle orphaned legs (waiting_user_approval with no real card).
- **BUG-577** sprint boundary does not reseed downstream step statuses — sprint-2 inherited sprint-1's DONE on coder/validate/reviewer/audit → walker skips all of them → auto-finalize false-green risk (Task-026 could stamp done with only scaffold stubs). Escalate chain side-effect: operator `flow-control done` resolved against synthesis → escalate → tdd leg FAILED.
- **BUG-578** escalate-Retry on debate gate re-drives `debate_synthesis` without re-attaching the join note and without re-driving the owner cohort → guaranteed re-escalate; UI Retry button can never succeed on this gate.
- **BUG-579** orphaned leg stuck `waiting_user_approval` forever: leg cancelled when parent parks but status never settles; `/interrupt` cannot settle (no turn goroutine to consume cancel); stall-sweep skips member when pending gate fields set; child turn fenced while parent parks + parent parks because child open = circular deadlock.
- **BUG-580** cohort verdict loss, two layers: (a) `snapshotReviewCohortVerdictsLocked` replace-all on ANY cohort join wipes `lastReviewCohortVerdicts` — an owner-debate join erased the recorded reviewer verdict; (b) `appendCohortResult` `cohortDrained` tombstone consumes the verdict from the pending map then drops it → verdict can never land again.
- **BUG-581** phantom `waiting_question`/`waiting_user_approval` status on parent/leg with no pending question or approval card in admin lists — status field diverges from durable record.
- **BUG-582** (desktop timeline): step list renders ~15 rows with blank entries — sprint nodes + debate-overlay nodes (owner_1/owner_2/debate_trigger/debate_synthesis) merge into one runtime list; after the overlay unmounts the rows stay and/or render unlabeled. Step count is wrong vs the actual sprint's node set.
- **BUG-583** (desktop timeline): step status is not accurate across turns — observed flapping synthesis DONE→WAITING_USER_APPROVAL→DONE, coder RUNNING with no live leg (BUG-569), tdd FAILED from a misrouted operator flow-control, stale DONE inherited across sprint boundary (BUG-577). Needs follow-up: displayed status must reflect real leg/node state per turn.
- **BUG-584** (desktop liveness): "live — last event X ago" chip only sees the parent run's own SSE stream. Leg/sub-agent internal events (message_delta, tool_started/completed, turn events) flow on each leg's OWN run stream which the app never subscribes → chip reads "quiet" during a busy 20-min leg turn. Contract: an event from ANY leg/sub-agent must bump the run's live time (aggregate agentRuns' streams or runner-side fan-in to parent stream).
- **BUG-581 evidence+**: `user_question_required` seq 4903 emitted on run-100368 while `/admin/.../questions` returns [] — confirmed phantom question surface.

## Fix discipline for ALL captured bugs

Every fix MUST follow /safe-fix-contract: reproduce-first red assertion before touching prod; never edit/weaken existing tests (additive only); prove or test Claude+Codex+Grok parity; change-contract scope block before mutating; CA entry + [Type][feature] commit per change; gofmt/vet clean; `go test -count=1 ./internal/runner/...` green for runner changes.
