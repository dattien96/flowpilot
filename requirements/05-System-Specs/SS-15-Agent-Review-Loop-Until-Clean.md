# SS-15: Agent Review Loop (Review Until Clean)

## Metadata

- Document ID: `SS-15`
- Title: `Agent Review Loop (Multi-Reviewer, Review Until Clean)`
- Phase: `system_spec`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: `None` (product feature spec)
- Child Documents: [SD-18: Main-Hub Agent Review Loop](../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md)
- Related Documents: [SS-16: Agent Flow Engine](./SS-16-Agent-Flow-Engine.md) (this spec is the first template instance of that generic engine), [SS-12: Multiple Agents](./SS-12-Multiple-Agents.md), [SS-06: Workflow Skill Agent](./SS-06-Workflow-Skill-Agent.md), [SS-11: Workflow With Session](./SS-11-Workflow-With_Session.md), [SS-08: Approve Gate](./SS-08-Approve-Gate.md)
- Replaces: `None` (expands the SS-12 stub: "many agents communicate / 2 review agents debate")
- Tags: `multi-agent, review, orchestration, synthesis, loop, business-spec`

## AI Quick View

### Summary

- FlowPilot can already spawn sub-agents, but a user cannot yet say "review my change with several reviewers and keep iterating until there are no issues left" and have it happen automatically.
- This spec defines a **review-until-clean loop**: a coder produces a change, **multiple reviewers** assess it in parallel, the **main chat agent consolidates their findings and resolves conflicts**, and the cycle repeats until the change is clean or a safety cap is reached.
- The main chat agent is the **single coordinator** (the hub). Reviewers never talk to each other; each reports its result and the main agent decides — this keeps each agent focused and the outcome explainable.
- The loop is **bounded and honest**: it stops at a round cap, and if issues still remain it **asks the user** what to do rather than quietly declaring success.
- The whole loop is **observable** on the existing orchestration board (which agents ran, what each found, current round, open-issue count, final verdict).

### Current Ask

- Define the business behavior, rules, and acceptance criteria for an automated, bounded, multi-reviewer "review until clean" loop coordinated by the main chat agent.

### Key Decisions

- `AC-0` The main chat agent is the only coordinator; sub-agents do not communicate directly with each other.
- `AC-0b` The loop must terminate on a clean result OR ask the user at a safety cap — it must never stop silently with known issues unresolved.

### Constraints

- Must reuse FlowPilot's existing sub-agent spawning, approval gates (SS-08), and session continuity (SS-11) — not a new parallel system.
- Each agent stays isolated: only an agent's final result is shared, never its full working transcript.
- Phase 1 is chat-mode only (no durable cross-PC database for the loop); cross-PC durability is a later concern.
- The user remains in control: code is not committed or merged automatically as part of the loop.

### Open Questions

- `Q-4` Should "review until clean" be offered as a one-click action in the UI, or only via a chat instruction/skill in Phase 1? (Q-1…Q-3 are resolved — see §10.)

## 1. Goal

Let a user ask FlowPilot to **review a change with multiple reviewers and iterate until it is clean**, with the main chat agent coordinating the whole process — collecting every reviewer's findings, resolving disagreements, deciding whether another round is needed — without the user having to manually re-prompt each round, and without losing visibility or control.

## 2. Problem

Today FlowPilot can spawn sub-agents and run a single coder↔reviewer hand-off, but:

- **Only one reviewer, one pass.** There is no way to run several reviewers with different focuses and combine their findings.
- **No "until clean" loop.** The user must manually look at the result and re-prompt the coder each round; nothing automatically continues until issues are gone.
- **Fragile signaling.** The current loop infers the reviewer's verdict from keywords in its message ("changes requested"), which is brittle and breaks with multiple reviewers (each would independently try to restart the coder).
- **No conflict resolution.** When two reviewers disagree (one says "fix this", one says "this is fine"), nothing reconciles them; the user is left with contradictory advice.
- **Unbounded risk / silent stop.** Without an explicit, bounded loop there is either a risk of looping forever or of stopping without telling the user that issues remain.

Users want the workflow they'd run by hand — "have a couple of reviewers look at this, sort out their disagreements, and keep fixing until it's clean" — to be a single, trustworthy, observable command.

## 3. Scope

- **In scope:**
  - A main-chat-coordinated loop: coder → multiple parallel reviewers → main-agent synthesis → decide → repeat.
  - Multiple reviewers per round, each with a distinct review focus.
  - The main agent consolidating findings, de-duplicating, and resolving reviewer conflicts.
  - A bounded loop with a configurable round cap and an explicit "ask the user" gate when the cap is hit with issues open.
  - Observing the loop on the orchestration board (agents, findings, round, open-issue count, verdict).
- **Out of scope:**
  - Direct agent-to-agent conversation/debate (reviewers talking to each other without the hub).
  - Durable cross-PC persistence of the loop and workflow-engine integration (deferred, see SS-11 / later phase).
  - Automatic commit/merge of the resulting change.

## 4. Non-Goals

- Not building a general autonomous multi-agent debate framework; the main agent is always the arbiter.
- Not removing human approval gates; child agents still obey approval/YOLO rules (SS-08).
- Not guaranteeing a "perfect" review — the goal is a trustworthy, bounded, transparent loop, not a correctness proof.

## 5. User Stories or Primary Use Cases

- `US-1` As a developer, I want to ask FlowPilot to "review this change with a couple of reviewers and fix issues until it's clean", so that I get a vetted result without babysitting each round.
- `US-2` As a developer, I want each reviewer to focus on a different angle (e.g. correctness vs security), so that more classes of problems are caught.
- `US-3` As a developer, when two reviewers disagree, I want the main agent to resolve the conflict using the full task context and give me one clear answer, so that I'm not handed contradictory advice.
- `US-4` As a developer, I want the loop to stop after a sensible number of rounds and ask me what to do if issues remain, so that it never loops forever or silently claims success.
- `US-5` As a developer, I want to watch the loop on the board (who reviewed, what they found, which round, how many issues are left) and intervene (extend, stop, inject feedback) at any time.
- `US-6` As a developer, I want the loop to survive a server restart and continue, so that a long review isn't lost.

## 6. Acceptance Criteria

- `AC-1` A user request to "review until clean" starts a loop that spawns one coder and **two or more reviewers** running in parallel.
- `AC-2` Each reviewer's result is collected and presented to the main agent as a **single consolidated summary** that labels each reviewer's findings distinctly.
- `AC-3` The main agent produces **one de-duplicated, conflict-resolved issue list** per round (not N separate contradictory lists).
- `AC-4` If the consolidated result has **zero open issues**, the loop ends with an "approved/clean" outcome and a summary to the user.
- `AC-5` If issues remain and the round is **below the cap**, the coder is automatically re-run with the consolidated feedback and a new review round begins — **without the user re-prompting**.
- `AC-6` If the **round cap is reached with issues still open**, the loop **pauses and asks the user** (e.g. extend / accept as-is / stop); it does not silently finish.
- `AC-7` Two reviewers in the same round never cause two competing coder re-runs; exactly one consolidated decision drives the next round.
- `AC-8` The orchestration board shows, live: each agent and its status, each reviewer's findings, the current round and cap, the open-issue count, and the final verdict.
- `AC-9` The user can stop the loop at any time; after stopping, no further automatic rounds occur.
- `AC-10` The loop and its progress survive a runner restart and resume from where they left off.
- `AC-11` Existing single-agent chat and the existing single-reviewer behavior are unchanged when the review-loop is not in use.

## 7. Business Rules

- `BR-1` **Hub-only coordination.** All cross-agent information flows through the main chat agent. Reviewers receive the change to review and return findings; they are not given each other's transcripts.
- `BR-2` **Isolation preserved.** Only an agent's final result (or failure) is shared upward — never its intermediate reasoning, tool calls, or diffs (consistent with the existing multi-agent design).
- `BR-3` **Conflicts resolved by the hub.** When reviewers disagree, the main agent decides using the original task context; the resolved decision is authoritative for that round.
- `BR-4` **Bounded loop.** Every loop has a round cap (default small, e.g. 3) and can be extended only by an explicit user action.
- `BR-5` **No silent success.** Reaching the cap with open issues must surface to the user as a decision, not a completion.
- `BR-6` **Human gates intact.** Each agent's own file writes, commands, and questions still honor approval/YOLO rules; the loop never auto-approves a child's gated action.
- `BR-7` **User override.** The user can stop, extend, or inject feedback into the loop at any point, and these actions take precedence over the automatic flow.
- `BR-8` **Explainability.** Every round's findings, the consolidated decision, and the final verdict are visible and attributable to a source.
- `BR-9` **Default cohort = two lenses.** A review round runs at least two reviewers with distinct lenses — `correctness` and `security` by default. The skill may add a third lens (`regression`) when the change touches previously-tested behavior, but two is the shipped minimum.
- `BR-10` **Bounded extension guarantees termination.** At the cap the user may extend, but only in increments of 2 rounds and at most twice (a hard ceiling of cap + 4). Once the ceiling is reached the only remaining choices are Accept-as-is or Stop, so the loop always terminates.

## 8. Edge Cases

- `E-1` A reviewer fails mid-review — the loop proceeds with the remaining reviewers' findings and notes the failure.
- `E-2` Reviewers fully agree — synthesis still produces one issue list (no artificial conflict).
- `E-3` First round is already clean — the loop ends in one round with an approved outcome.
- `E-4` The cap is hit on the very first round — the user is asked, not silently stopped.
- `E-5` The user stops mid-round while a reviewer or coder is still running — in-flight work is allowed to finish or is interrupted per existing stop semantics, and no new round starts.
- `E-6` The server restarts mid-loop — the loop resumes with the same round count and pending state.
- `E-7` Only one reviewer is requested — the loop still works (degenerate "multi" case) and behaves like a bounded single-reviewer loop with explicit verdicts.

## 9. Dependencies

- Existing sub-agent spawning and the orchestration board (CP-19 / SD-16).
- Approval gates (SS-08) for child agent actions.
- Session continuity and chat persistence (SS-11) for resume.
- The main chat agent's ability to follow a defined protocol (skill) and to call tools.

## 10. Open Questions

- `Q-1` **Resolved.** The **main chat agent** performs synthesis by default (it holds the original task context, `BR-3`). A `synthesizer` built-in sub-agent ships as an optional offload the main agent may spawn for very large review sets.
- `Q-2` **Resolved.** Default cohort is **two reviewers** with `correctness` + `security` lenses; the skill may add `regression` when relevant (`BR-9`).
- `Q-3` **Resolved.** At the cap the user is offered **Extend / Accept / Stop**; "extend" is bounded to +2 rounds, at most twice (`BR-10`).
- `Q-4` Should "review until clean" be offered as a one-click action in the UI, or only via a chat instruction/skill in Phase 1? (Phase 1 ships the chat/skill path; a one-click UI affordance is a later enhancement.)

## 11. Definition of Done

- The behaviors in §6 (AC-1…AC-11) are demonstrable in the desktop app with at least two reviewers.
- A reviewer disagreement is shown being resolved into a single decision (AC-3, BR-3).
- The cap-with-open-issues path visibly asks the user (AC-6, BR-5).
- The loop is observable on the board and survives a restart (AC-8, AC-10).
- Single-agent and legacy single-reviewer flows are confirmed unchanged (AC-11).
- SD-18 (design) and CP-36 (coding plan) are linked as children and cover every AC.
