# CA-820 — ss_lock stamps SS approved; requirement park emits graph

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: BUG-365
change_type: bugfix
summary: resumeVibeLock stamps the locked SS list approved on disk (SS-*/SPRINT-PLAN*, never FORMAT-*) and parkVibeRequirement emits the blocked snapshot so the TUI shows the park instead of Thinking forever
# --->8---

## Why

Live run-646702 (`gate-sandbox` snake): the operator locked at `ss_lock`
(timeline `[DONE]`) but `requirements/05-System-Specs/SS-100/101/102` and
`SPRINT-PLAN-snake-mvp.md` still read `status: draft`. `resumeVibeLock` only
sealed in memory (`vibeSSSealed`). `cp_writer` (`prompts/vibe-cp-from-ss.md`)
read the draft, asked "fix SS status to approved first", and the operator's
"Request changes" answer left `requirements/07-Coding-Plan/todo/` empty.
`task_slicer` then wrote zero Task files; the BUG-363/364 park fired and
stamped the step DONE, but `parkVibeRequirement` only mutated the loop state
— no `emitAgentGraph` — so the TUI kept "Thinking 3m29s" with no `[Retry]`
card until the terminal was killed ("Input stalled").

## Change

- `vibe_ss_stamp.go` (new): `stampVibeSSApproved(cwd)` globs
  `requirements/05-System-Specs/SS-*.md` + `SPRINT-PLAN*.md`, skips
  `FORMAT-*`, and surgically rewrites `status: draft` (frontmatter) and
  `- Status: \`draft\`` (metadata) to `approved`. Prose mentioning draft is
  untouched.
- `vibe_lock.go` `resumeVibeLock`: when `nodeID == ss_lock` (after any edit
  write-back, before sealing) call `stampVibeSSApproved(cwd)` so `cp_writer`
  reads approved SS immediately.
- `vibe_sprint.go` `parkVibeRequirement`: capture the `mutateLoop` snapshot,
  then after `parkFlowForAwaitingUser` emit `EventAgentGraphUpdated` and
  `go persistParentSession` — exact mirror of `parkVibeLock`. CA-818
  DONE-before-park ordering untouched.
- `prompts/vibe-cp-from-ss.md`: the `SS Preview & Lock` decision is the
  approval; never stop on a leftover `status: draft` line — write the CP now.
- Explicitly NOT changed: CA-791 missing-CP join (pinned), BUG-363 park
  reason, CA-790 once-lock edges, CA-817/818 park decision + DONE stamp.

## Tests

- `bug365_ss_lock_stamps_approved_test.go` (new, additive-only): SS +
  SPRINT-PLAN both stamped (frontmatter + metadata), body sentence
  "can move from `draft` → `approved`" preserved, `FORMAT-*` untouched.
  Verified red-before (stash production edits → FAIL) / green-after.
- `bug365_requirement_park_emits_graph_test.go` (new, additive-only): after
  `parkVibeRequirement`, an `EventAgentGraphUpdated` with
  `blocked/requirement` is on the parent stream. Red-before/green-after.
- Cohorts green with the fix: BUG-363/364/365, CA-78x/79x/80x/81x,
  `TestOnVibe*`, `TestVibeSession_*`, `TestAdvanceHubDone*`, Task-321/326.
- Full `internal/runner` suite: 20 failing tests verified identical on
  baseline without the fix (pre-existing provider/environment flakes:
  real-machine Grok, MCP inventory, TempDir-vs-gitnexus-index races) — no
  new failure introduced. `internal/agentpack` green.

## Providers

Agnostic Case 1: a disk stamp plus one engine event; zero `providerKey`
references in `vibe_ss_stamp.go`, `vibe_lock.go` or the changed
`parkVibeRequirement` branch. Tests use `ProviderKeyCodex` via
`newTestServer`, same precedent as CA-796/797/798/817/818.

## Will not undo

CA-791 missing-CP-joins-slicer (function untouched; pinned test green).
CA-790 once-lock edges. CA-817 park decision + BUG-364 DONE stamp ordering.
CA-783 SS fallback. Residuals: live `V-4` re-run on a clean sandbox pending
(operator tick), and the pre-existing full-suite flakes above are out of
scope.
