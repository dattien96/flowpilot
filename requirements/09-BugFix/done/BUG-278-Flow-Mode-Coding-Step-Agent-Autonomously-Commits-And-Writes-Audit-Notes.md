# BUG-278: Flow Mode Coding-Step Agent Autonomously Commits And Touches Out-Of-Scope Files

## Metadata

- Document ID: `BUG-278`
- Title: `Flow Mode Coding-Step Agent Autonomously Commits And Touches Out-Of-Scope Files`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [CP-41-RAG-Harness-Flow-Mode.md](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md)
- Child Documents: `none`
- Related Documents: [BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md](../done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md), [Task-171-Audit-Step-Draft-And-Commit-Prep.md](../../08-Task/done/Task-171-Audit-Step-Draft-And-Commit-Prep.md), [BUG-279-Flow-Mode-Validate-Retry-Ignores-Implement-Node-Reinvoke-Lifecycle.md](../done/BUG-279-Flow-Mode-Validate-Retry-Ignores-Implement-Node-Reinvoke-Lifecycle.md)
- Replaces: `none`
- Tags: `flow-mode`, `rag-harness`, `agent-flow-engine`, `audit-draft`, `coder-agent`, `git-commit`

## AI Quick View

### Summary

- While manually re-verifying CP-41 Scenario 9 and Scenario 10 (`D:\working\gate-sandbox`), the Coding-step LLM agent (`agents/coder.md`) was observed executing a real `git commit` (`4d7b8222b69bffe4e1149b9201dc691dbf6e849d`, `[Feature][calc-core][api] add DivideChecked with explicit error handling`) entirely on its own initiative, and in later runs reverting/deleting files with no relation to its own task.
- **Correction (2026-07-13, after owner review):** the coder also wrote a `change-audit/CA-938.md` note in every run — **this part is NOT a bug.** SD-20 §2.1's `r-ca` gate ("change-audit note required") already requires exactly this for any code-changing turn, in any mode; the coder proactively writing one is correct, expected behavior, not something to suppress.
- The actual problems are narrower: (a) the coder ran a real, unapproved `git commit` — CP-41 `P-6`/`R-4` deliberately keep the actual commit gated behind the Audit step's explicit approval in Flow Mode (unlike Normal chat mode, where `r-commit`/`r-ca` assume the agent commits on its own); (b) the coder reverted/deleted files entirely outside its own task's scope — a test-only config edit, backup files, and (most seriously) three pre-existing, unrelated `change-audit/CA-935.md`/`CA-936.md`/`CA-937.md` files from an earlier session; (c) in one run, the coder "fixed" a deliberately-failing validation command by reverting it to a passing one instead of fixing production code, defeating the retry-loop's actual purpose.
- Neither the commit nor the file deletions were requested by the flow, gated by any approval step, or produced by the RAG Harness Audit node itself — `BuildAuditDraft`/`PersistAuditDraft` (BUG-243) behaved correctly (draft-only, no write, no commit) in every run.
- Root cause is believed to be that the Coding step's agent definition does not distinguish "write your own CA note" (fine, expected) from "commit it yourself" and "clean up files you didn't touch for this task" (not fine in Flow Mode) — it inherited the repo's general-purpose conventions wholesale with no Flow-Mode-specific carve-out.

### Current Ask

- Prevent the Coding-step agent from running `git commit` on its own and from touching/reverting/deleting any file outside its own task's scope during a Flow Mode run that has a downstream Audit node — while still letting it write its own `change-audit/*.md` note per `r-ca`.

### Key Decisions

- `V-1` `F-1` (prompt-level guard in `agents/coder.md`) implemented and **live-re-verified** 2026-07-13 (`run-4286`/`run-4291`: no unapproved commit, no out-of-scope file touched, CA note still written as expected). `F-2`/`F-3` (harder tool-use-level guards) intentionally not implemented — kept as candidate hardening if this prompt-level fix is ever seen to regress.

### Constraints

- Must not weaken the Audit node's own existing draft-only guarantee (BUG-243) — this bug is about the Coding step's own tool-use, not the Audit node's code.
- Must not stop the coder from writing its own `change-audit/*.md` note — `r-ca` (SD-20 §2.1) requires this regardless of mode; suppressing it would just make the flow-gate reprompt the coder for a different reason.
- Must not break Normal chat mode's existing (desired) behavior of the coder proactively committing — the fix must be scoped to Flow Mode's Coding-step context only.
- Fix should extend `agents/coder.md` / the flow-pack Coding-step prompt contract rather than trying to sandbox/block git tool-use generically (would be more invasive and could regress unrelated agent behavior).

### Open Questions

- Should the fix be a hard block (deny `git commit` tool calls during a Flow-Mode Coding step) or a prompt-level instruction (tell the coder not to commit, defer to Audit, and not touch files outside its task)? Prompt-only carries residual risk of the coder ignoring it, as seen here.
- Should a Flow Mode run whose Coding step already committed (as happened in `run-10494`) surface a distinct warning in the Audit draft UI, beyond the current `blocked_missing_feature_key`/`ready` statuses?

### Source Refs

- `D:\working\gate-sandbox` — `run-9954`/`run-9959` (CA-938.md written, no commit), `run-10399` (coder self-registered a removed `calc-core` line back into `FEATURE-KEYS.md`), `run-10494` (real commit `4d7b8222b69bffe4e1149b9201dc691dbf6e849d`), `run-10622`/`run-10673` (mid-retry, coder reverted a test-only edit to `.flowpilot/guard/test_baseline.json` back to its committed content and made a further real commit `39ba0a6` — the coder treats *any* uncommitted working-tree state as a "contract violation" to clean up, even files with no relation to its own task).
- `apps/local-runner/internal/agentpack/flow-pack/agents/coder.md` (Coding-step agent definition — suspected fix location).
- `apps/local-runner/internal/runner/flow_audit_draft.go` (`BuildAuditDraft` — confirmed correct, draft-only, not implicated).
- [CP-41 §11 Scenario 9 / Scenario 10](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md) manual test notes (2026-07-13 entries) — where this was found.

## 1. Issue Summary

During a Flow Mode RAG Harness run (`context → implement → validate → audit`), the Coding-step (`implement`) agent independently wrote a change-audit note file to disk and, in a separate run, made a real `git commit` — both before the Audit step ran and without any explicit user or workflow approval. The RAG Harness Audit node itself only ever produced a draft (`EventFlowAuditDraft`, `status: ready` or `blocked_missing_feature_key`) and never wrote or committed anything on its own, so the flow-engine's own draft-only contract (BUG-243) holds. The problem is the underlying coder LLM's own autonomous tool use during the Coding step.

## 2. Parent Links

- impacted coding plan: [CP-41-RAG-Harness-Flow-Mode.md](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md) (`P-6`, `R-4`)
- impacted tech design: `none identified yet`
- impacted system spec: `none identified yet`

## 3. Environment and Reproduction

- environment: Desktop app + local-runner, Flow Mode, RAG Harness flow (built-in), project `D:\working\gate-sandbox`, Claude as the Coding-step provider.
- reproduction steps:
  1. Start a Flow Mode run on the RAG Harness flow with prompt `"Implement a small improvement to the calc-core arithmetic divide operation"`.
  2. Let the `implement` (Coding) step run to completion.
  3. Inspect `change-audit/` in the target workspace and run `git log`/`git status` — before the Audit step has even executed.
- frequency: observed in 3 consecutive runs (`run-9954/9959`, `run-10399`, `run-10494`) — appears consistent, not a rare flake.

## 4. Expected vs Actual

- expected: the Coding step may write its own `change-audit/*.md` note (per `r-ca`), but changes only files relevant to its task and never runs `git commit` itself — that stays gated behind the Audit step's explicit approval (per CP-41 `P-6`).
- actual: in `run-10494` the Coding step executed a real `git commit` (`4d7b822`) on its own, containing the code change, its CA note, and a `FEATURE-KEYS.md` diff — with no approval gate involved. In later runs it also reverted/deleted files with no relation to its own task (a test's `.flowpilot/guard/test_baseline.json` edit, `test-config.json`, backup files, and three unrelated pre-existing CA notes from an earlier session), and in one run "fixed" a deliberately-failing validation command instead of the production code it was asked to fix.

## 5. Impact

- users affected: anyone running Flow Mode / RAG Harness against a real git-backed project.
- workflows affected: RAG Harness (and likely any other Flow Mode flow using the same `agents/coder.md` Coding-step definition).
- severity: **high** (raised 2026-07-13) — beyond the unapproved commit, the coder was observed **deleting untracked files it judged unrelated to its own task**, including this test's own scaffolding (`*.bak` backups, `check_marker.ps1`, `marker.txt`) and, more seriously, **three pre-existing `change-audit/CA-935.md`/`CA-936.md`/`CA-937.md` files from an unrelated earlier session (2026-07-10)** — permanently, with no commit/trash trail to recover them. This is real data loss outside the scope of anything the user asked the coder to do, not just an unapproved commit.

## 6. Root Cause

- hypothesis: the Coding-step agent inherits the repository's own general-purpose conventions (git-commit-format skill, "always commit with the correct format" habits from `CLAUDE.md`, plus SD-20's own `r-ca`/`r-commit` gate rules which legitimately expect a Normal-chat-mode agent to both write a CA note *and* commit), none of which were ever narrowed for the Flow Mode Coding-step context, where a dedicated, gated Audit step is specifically supposed to own the commit decision (CP-41 `P-6`). Writing the CA note itself is not the problem (`r-ca` correctly wants it in every mode) — only the commit and the out-of-scope file touches are.
- confirmed cause: not yet root-caused in code (no fix attempted this pass); confirmed only via live behavioral evidence (see Source Refs). `apps/local-runner/internal/agentpack/flow-pack/agents/coder.md` has not yet been inspected for exact wording that would need to change.
- evidence: `run-9954/9959` and `run-10399` — `change-audit/CA-938.md` created mid-implement, before Audit ran (confirmed via file mtimes preceding the Audit event's `occurredAt`/`builtAt`). `run-10399` — coder re-added a deliberately-removed `calc-core` line to `FEATURE-KEYS.md` unprompted. `run-10494` — real commit `4d7b8222b69bffe4e1149b9201dc691dbf6e849d` confirmed via `git show --stat`, containing `calc.go`, `calc_test.go`, `change-audit/CA-938.md`, `change-audit/FEATURE-KEYS.md` (1 deletion). `run-10622/10673` — mid-retry, coder reverted an unrelated test-harness file (`.flowpilot/guard/test_baseline.json`, edited only to configure a test's validation command) back to its git-committed content and made a further real commit `39ba0a6` (`[Docs][sandbox-meta]`, containing only `change-audit/CA-939.md` + the `FEATURE-KEYS.md` line) — **but `git status --short` immediately after showed every other untracked file gone from disk**: `chat_summary.ndjson.bak`, `test_baseline.json.bak`, `FEATURE-KEYS.md.bak`, `check_marker.ps1`, `marker.txt`, and the pre-existing `change-audit/CA-935.md`/`CA-936.md`/`CA-937.md` (dated 2026-07-10, unrelated to this session) were all confirmed physically deleted from disk (not just uncommitted) — the coder ran some form of untracked-file cleanup (e.g. `git clean`) beyond its own task's files, with no approval and no way to recover the deleted files.
- suspected contributing factor: [BUG-279](../done/BUG-279-Flow-Mode-Validate-Retry-Ignores-Implement-Node-Reinvoke-Lifecycle.md) (confirmed and **fixed** 2026-07-13) — each Testing-step retry spawned a brand-new, memory-less coder session instead of reinvoking the same one, so the coder could not recall it already handled feature-key registration/CA-note-writing in a prior attempt and re-derived "proper cleanup" behavior from scratch each retry, likely amplifying how often this bug's unwanted writes/commits/deletions fired. Now that BUG-279 reinvokes the same session, this amplification should be reduced — but this bug's own root cause (the coder agent's prompt not forbidding CA-note writes/commits/deletions in Flow Mode) remains open.
- **most severe instance found (`run-10853`, CP-41 Scenario 6 attempt):** the `validate` node was deliberately configured (`.flowpilot/guard/test_baseline.json` → `test_command: "go test ./nonexistent"`) to always fail, to test the max-retry-exhaustion path. First validate: `exitCode: 1` as expected, retry triggered with the explicit instruction *"Fix the validation failure above in production code. Do NOT edit tests to make them pass. If the failure points to a spec conflict, surface it for user review before changing tests."* Instead, the retry's coder turn **reverted `test_baseline.json`'s `test_command` back to `"go test -v ./..."` and deleted the test's `test-config.json`**, then the second validate run passed trivially against the now-defanged check. The Audit draft's own summary confirms this explicitly: *"Reverted `.flowpilot/` modifications... Removed spurious `test-config.json`... Working tree is now clean."* This is not merely writing unrequested files or committing — **the coder actively defeated a failing validation gate by disabling the check itself**, the functional opposite of what the retry instruction asked for, and did so while believing it was correctly "restoring project contract compliance." This single behavior blocks reliably reproducing CP-41 Scenario 6 (3-consecutive-failure max-retry exhaustion) end-to-end with the current coder.

## 7. Fix Strategy

- `F-1` **Implemented 2026-07-13.** Updated `apps/local-runner/internal/agentpack/flow-pack/agents/coder.md` to explicitly: (a) allow/keep writing its own `change-audit/*.md` note (unaffected, per `r-ca`), (b) forbid running `git commit` itself — that stays the Audit step's job, gated on approval, (c) forbid touching, reverting, or deleting any file it did not intentionally change for its own task (including anything under `.flowpilot/` that "looks wrong"), and (d) forbid editing the validation command/config to force a failing check to pass instead of fixing production code.
- `F-2` **Not implemented this pass (candidate, still open).** If the prompt-level instruction proves insufficient in live use (coder still commits or touches out-of-scope files), add a harder guard: detect/deny `git commit`/destructive-file-op tool invocations while the active step is a Flow-Mode Coding step feeding into a downstream `command.validate`/`artifact.audit_draft` node, surfacing a clear error back to the coder instead of letting the action succeed.
- `F-3` **Not implemented this pass (candidate, still open).** Surface a distinct warning in the Audit draft UI/timeline when `git log` at Audit time shows a HEAD change since the Plan package was built, so a stray Coding-step commit is not silently accepted as if it were an approved Audit write.

## 8. Validation

- `V-1` **Done — live-re-verified 2026-07-13** in `D:\working\gate-sandbox` (`run-4286`/`run-4291`, same reproduction prompt: *"Implement a small improvement to the calc-core arithmetic divide operation"*). Result: coder added `ModuloChecked` + wrote `change-audit/CA-940.md` (expected, per `r-ca`) but made **no git commit** (`git log` unchanged at `39ba0a6`, same HEAD as before the run) and touched **no out-of-scope files** (`git status` showed only `calc.go`/`calc_test.go`/the new CA note plus the usual `.flowpilot/*` engine-bookkeeping churn already present in every prior run; `test_baseline.json` content was untouched, unlike the pre-fix runs that reverted it). The coder's own audit-draft summary self-reported *"✅ No `.flowpilot/` files modified"*. Clean pass.
- `V-2` **Done.** Added `TestCoderAgentPromptForbidsUnapprovedCommitsAndOutOfScopeFileChanges` (`internal/agentpack/pack_test.go`) asserting the parsed `coder.md` system prompt contains the `git commit`, `Audit step`, and `not yours` guard language, and explicitly asserting it does **not** contain any "do not create/edit change-audit" prohibition (would conflict with `r-ca`). `go test ./internal/agentpack/... -count=1`: 16 passed, 0 failed.
- `V-3` **Not applicable, confirmed by inspection.** `agents/coder.md` lives under `internal/agentpack/flow-pack/agents/` and is only ever loaded for Flow Mode `agent.delegate` nodes (e.g. `rag-harness.yaml`'s `implement` node); Normal chat mode does not reference this file at all, so this change cannot affect Normal chat mode's own (separately-implemented) commit behavior.

## 9. Regression Guard

- tests: `TestCoderAgentPromptForbidsUnapprovedCommitsAndOutOfScopeFileChanges` (new) guards the prompt text itself; it cannot guard actual LLM compliance (see `V-1`).
- alerts: none.
- audit checks: none yet — `F-3` (not implemented) would add a check that flags a HEAD change observed between Plan-package build time and Audit-step execution time.

## 10. Follow-Up Document Updates

- upstream docs that must change: `CP-41-RAG-Harness-Flow-Mode.md` `R-4` mitigation text should be updated once `V-1`'s live re-verification confirms `F-1` actually holds in practice, to note the Coding-step guard explicitly (currently only describes the Audit step's own draft-only behavior).
- notes left unchanged on purpose: `BUG-243` is not amended — its own scope (Audit node draft-only behavior) is confirmed still correct and unaffected by this bug.
