# BUG-344 — Scan/Plan read-only policy rejects legitimate compound read-only bash: SCAN unusable for Grok

## Metadata

- Document ID: `BUG-344`
- Title: `Scan/Plan read-only policy denies compound read-only bash (ls | head; rg … 2>/dev/null) — Grok's run_terminal_command always hits the metachar gate, making SCAN unusable`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-02`
- Last Updated: `2026-09-02`
- Parent Documents: [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/CP-56-Terminal-TUI-Chat-And-Flow-Client.md), [CP-46: Grok Build Controlled Adapter Over ACP](../../07-Coding-Plan/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- Child Documents: `none`
- Related Documents: [BUG-342](BUG-342-Grok-Turn-Ends-Blank-After-Scan-Deny-No-NoReply-Notice.md) (the blank-turn UX this deny produces), [CA-713](../../../change-audit/CA-713-opencode-no-reply-notice-denied-turn.md), run-464841 (`.flowpilot/cli-runner.log` 2026-09-01 17:36–17:38 UTC), `chat_posture_policy.go`, `chat_posture_policy_test.go`
- Replaces: `none`
- Tags: `ai-providers, scan-plan-code, read-only-posture, approval-gate, exec-classifier, severity-high`

## AI Quick View

### Summary

- The read-only posture policy (`chat_posture_policy.go` `isReadOnlyCommand`, line 115) rejects **any** exec command containing a metacharacter `> | ; & ` $` — even when every segment is a pure read. Live `run-464841`: Grok's `run_terminal_command` `rg "B4|…" … | head -80; tail -20 …; ls … 2>/dev/null; find …` (100% read-only) was auto-denied (`posture_read_only_scan` → `reject-once`), aborting the whole turn (see BUG-342).
- Grok batches its exploration into **one** compound bash tool call (`;`/`|` chained), so in practice almost every Grok exec request in scan/plan hits the metachar gate → SCAN mode cannot explore with Grok at all. Claude/opencode use dedicated read tools (Read/Grep/Glob) and are unaffected — the bug is exec-policy, not provider.
- Root design tension: the gate exists to stop `ls; rm -rf /`-style write smuggling (fail-closed, comment at lines 113–117). The fix must keep that property **while** approving compound commands whose **every segment** is unambiguously read-only.
- Bonus finding while auditing: `find` is today allowlisted **wholesale** — `find . -delete` would pass the current classifier. The fix should tighten `find` args (deny `-delete/-exec/-execdir/-ok/-fls/…`) in the same change, making the new policy strictly safer than today.

### Current Ask

- Replace the whole-command metachar gate with **segment-aware parsing**: split on `; | && ||`, allow only `/dev/null` redirects (`2>/dev/null`), forbid `> >> 2>` (non-null), backticks and `$()` substitution, then require **every segment** to pass the existing `isReadOnlyCommand` allowlist (with `find` arg tightening). Any segment that fails → deny the whole command (fail closed).
- Ship a full approve/deny matrix test (R3), prove provider-neutrality of the classifier (it already inspects only `ApprovalDetails` — Claude/Grok/Codex share it), and keep BUG-342's deny→notice path working (it keys on the deny outcome, which still exists for genuinely-write commands).

### Key Decisions

- `D-1` **Segment soundness rule**: shell executes `A | B ; C` as A, B, C independently — the compound is read-only **iff** every segment is read-only and no segment introduces a write side-channel (redirect, substitution, hidden exec). This is the safety invariant the fix implements; it is provable and testable.
- `D-2` Approved separators: `;`, `|`, `&&`, `||`. Allowed stderr redirect: exactly `2>/dev/null` (also tolerate `2>/dev/null` with surrounding spaces); **nothing else** — `>`, `>>`, `2>` (non-null), `1>`, `&>`, `<>`, backtick, `$(` are hard denies (each can write or smuggle).
- `D-3` Each segment is classified by the **existing** `isReadOnlyCommand` (same binary allowlist, same `sudo` handling, same `isReadOnlyGit` subcommand rules) — no new binary trust is introduced; only composition rules change.
- `D-4` `find` tightening ships in the same change (it is a real hole in today's allowlist): deny `-delete`, `-exec`, `-execdir`, `-ok`, `-okdir`, `-fls`, `-fprint*` (arg-level check inside `isReadOnlyCommand` for the `find` branch).
- `D-5` Fail closed on parse: any metacharacter outside the approved set, unbalanced quotes/parens, empty segment, or unknown binary in any segment → deny the entire command. One deny still means one (now-informed) notice per BUG-342.

### Constraints

- additive-tests-only: **add** approve/deny cases to `chat_posture_policy_test.go`; do not delete or rewrite the existing deny cases (`ls | grep foo` is currently in the deny list at line 53 — it must move to approve by design change, which is a policy change the doc owns; the test line's *assertion* flips as the fix's contract, and that flip is the bug fix itself — flag in the CA note and confirm with the operator before touching the line).
- oracle-rule: if an old test fails after the fix, fix production code or escalate; the ONLY expected old-assertion flip is the intentional `ls | grep foo`/`pipe to write` policy change (must be called out, not silent).
- No new binaries/verbs may be added to the allowlist beyond today's set (scope discipline) — composition only.
- Parity: the classifier is shared across providers (exec kind from Claude/Grok/Codex adapters) — tests must cover the Grok `run_terminal_command` shape AND Claude/Codex exec shapes; provider-agnostic evidence required (R2).
- BUG-342's deny→notice must keep working: the deny outcome (and its `ApprovalDetails` content) is unchanged for genuinely-write commands.

### Open Questions

- `Q-1` Is `2>/dev/null` the only stderr suppression worth allowing? (`2>&1` requires an fd-dup understanding — reject for now; `> /dev/null` stdout-null is harmless and may be worth allowing — default: reject to stay conservative, revisit with operator.)
- `Q-2` Should `head/tail/grep/rg` remain unrestricted as segment binaries, or add arg checks (e.g. `grep` can't write, but `tee` is NOT in the list — confirm no allowlisted binary has a hidden write flag like `find -delete`; audit each allowlisted binary's write flags in the fix).
- `Q-3` Grok's own tool metadata sends `"read_only":false` for this compound command (run-464841 wire) — can FlowPilot ever trust the provider's `read_only` flag as a hint, or should the classifier remain the sole authority? (Recommended: classifier remains sole authority — provider hints are heuristics.)
- `Q-4` The existing test at `chat_posture_policy_test.go:53` (`pipe to write`, `ls | grep foo` → deny) flips to approve under the new contract — confirm the operator's sign-off on this single intentional assertion change (or keep a separate deny case `ls | grep foo | tee out` to preserve the "pipe can write" guard).

### Source Refs

- `run-464841` wire (`.flowpilot/cli-runner.log` line ~81525 region): `session/request_permission` for `run_terminal_command` — `_meta.x.ai/tool = {kind:"execute", name:"run_terminal_command", read_only:false}`, `rawInput.command = rg -n "B4|inplace|in-place|ask_user|spawn" .flowpilot change-audit requirements 2>/dev/null | head -80; tail -20 .flowpilot/gate-metrics.ndjson; ls requirements/07-Coding-Plan/todo requirements/08-Task/todo 2>/dev/null; find . -iname '*b4*' 2>/dev/null; …` → bridge `posture_read_only_scan` deny → `optionId:"reject-once"` → prompt cancelled.
- Code: `apps/local-runner/internal/runner/chat_posture_policy.go:108-149` (`isReadOnlyCommand` — metachar gate at line 115), `:156-228` (`isReadOnlyGit`), `grok_permission.go:55-84` (`grokPermissionKind` maps kind `execute` → `"exec"`), `chat_posture_policy_test.go:13-64` (existing approve/deny matrix; line 53 `ls | grep foo` deny case).
- Prior design intent: `chat_posture_policy.go:5-17` comment ("gated + silent … reads -> approve, writes -> deny, unknown -> deny (fail closed)").

## 1. Issue Summary

The scan/plan read-only posture must approve reads and deny writes, silently. Its exec classifier today denies **any** command containing `|`, `;`, `&`, `>`, backtick or `$` — a fail-closed choice that treats every compound command as potentially-write. Grok's agent batches exploration into a single chained bash call (`rg … | head; ls …; find … 2>/dev/null`), so its legitimate read commands are denied every time; the turn then aborts with no answer (BUG-342). Net effect: **SCAN mode cannot do its core job (read-only exploration) with Grok**, while the deny list is simultaneously too loose in one spot (`find . -delete` passes today).

## 2. Parent Links

- impacted coding plan: [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (Scan/Plan/Code postures + read-only policy), [CP-46: Grok Build Controlled Adapter Over ACP](../../07-Coding-Plan/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- impacted tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- impacted system spec: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)

## 3. Environment and Reproduction

- environment: local-runner + any chat client; provider grok (`grok-4.5`); scan posture; gate-sandbox or any repo.
- reproduction steps:
  1. Scan chat, Grok. Ask a read-heavy question ("find B4-inplace references and list recent change-audit files").
  2. Grok issues one `run_terminal_command` chaining reads with `|`/`;`/`2>/dev/null`.
  3. Bridge auto-denies (`posture_read_only_scan`); turn aborts cancelled (BUG-342 blank).
  4. Repeat: essentially every Grok exploration turn in scan dies the same way.
- frequency: ~100% for Grok compound read commands (run-464841 turn-464843); Claude/opencode unaffected (native read tools pass `isReadOnlyToolName`).

## 4. Expected vs Actual

- expected: a compound command whose **every segment** is a read (e.g. `rg x | head -80; ls dir; find . -name '*b4*' 2>/dev/null`) is approved; any segment that can write (`rm`, `git push`, `echo > file`, `find -delete`, `$(…)`) denies the whole command.
- actual: `ls -lt change-audit/ | head -30; ls -la *.txt *.go 2>/dev/null; rg … | head -50` → deny purely because of `|`, `;`, `>`. And separately, `find . -delete` → approve (hole).

## 5. Impact

- users affected: every Grok user of scan/plan posture (reported provider); the same policy path serves Claude/Codex exec requests (their compound execs also denied — hidden same-class issue).
- workflows affected: chat-mode scan/plan exploration — the core use case of the posture; scan becomes a dead posture for Grok.
- severity: high (feature unusable in its primary scenario + a latent `find -delete` allowlist hole in the same classifier).

## 6. Root Cause

- hypothesis: the metachar gate is whole-command, not compositional; Grok's batching style makes the gate fire constantly.
- confirmed cause: `isReadOnlyCommand` line 115 `strings.ContainsAny(cmd, ">|;&`$")` returns false before binary classification — no compositional analysis exists. Line 140 allowlists `find` with no arg check, so `find . -delete` is approved.
- evidence: run-464841 wire (Source Refs); `chat_posture_policy_test.go:52-53` explicitly locks the current behavior (`pipe to write: ls | grep foo → deny`); `grokPermissionKind` correctly routes kind `execute` → `"exec"` (so the classifier, not the mapping, is the choke point).

## 7. Fix Strategy

- `F-1` Segment parser in `isReadOnlyCommand` (replaces the flat metachar gate):
  1. Reject if the raw command contains `$` ` ` `"` `'` (unbalanced), or any char outside a small safe set.
  2. Reject `>` / `>>` / `2>` / `1>` / `&>` / `<>` **except** the exact pattern `2>/dev/null` (and `2> /dev/null`) which is stripped before segmentation.
  3. Reject backtick and `$(`/`)` substitution anywhere.
  4. Split on `;`, `|`, `&&`, `||` (trimmed; empty segment → deny).
  5. Every segment must pass `isReadOnlyCommand` **without** the metachar gate (refactor the binary/subcommand classification into `isReadOnlyCommandSegment`) — one failure → deny whole.
- `F-2` Tighten `find`: in the `find` branch, parse flags after the path predicates; deny any of `-delete -exec -execdir -ok -okdir -fls -fprint -fprint0 -fprintf` (and `-quit` stays read). Add unit cases.
- `F-3` Keep `isReadOnlyGit`, `sudo` handling, and the binary allowlist byte-for-byte identical; only composition + `find` args change.
- `F-4` Update the classifier's doc comment (lines 5-17 / 105-117) to state the compositional invariant (D-1) and the approved separator/redirect set (D-2).

## 8. Validation

- `V-1` Approve matrix (new, additive in `chat_posture_policy_test.go` or a new `bug343_*_test.go`):
  `ls | head`, `rg x -g '!node_modules' . | head -80`, `ls -lt d | head -30; ls -la *.go`, `tail -20 f; ls a b`, `git status; git log --oneline -5`, `find . -name '*.go' 2>/dev/null`, `rg a . 2>/dev/null | head -10 && ls`, `cat a.go | grep x | head -5`, `sudo cat /etc/hosts | head -1`.
- `V-2` Deny matrix (new cases; existing deny cases stay green except the intentional flip in `Q-4`):
  `ls; rm -rf /`, `ls && rm x`, `echo hi > out.txt`, `ls 2>err.txt`, `echo x > /tmp/f; ls`, `$(rm -rf /)`, `` `rm -rf /` ``, `ls | tee out`, `find . -delete`, `find . -exec rm {} \;`, `git push; ls`, `ls; git commit -m x`, `cat a | grep x && rm b`.
- `V-3` Provider parity (R2): the classifier is shared — run the full matrix once with exec-shaped details from each adapter mapping (Claude exec, Grok `run_terminal_command`, Codex exec) asserting identical decisions (provider-agnostic evidence in the CA note); Claude/opencode native read tools path (`isReadOnlyToolName`) unchanged, existing tool-name tests green.
- `V-4` Live: rerun the run-464841 prompt in scan with Grok → the compound read command is **approved** and the turn completes with an answer; rerun a write command (`git commit`, `rm`) → still denied, and BUG-342's notice (if fixed first) shows.

## 9. Regression Guard

- tests: full matrix above in new files/append (additive); existing `chat_posture_policy_test.go` approve/deny cases remain green with ONE documented exception (`ls | grep foo` deny → approve, `Q-4` — get operator sign-off and record in the CA note); `isReadOnlyGit` cases untouched; BUG-342 tests (if landed) unaffected since the deny outcome shape is unchanged.
- alerts: none new; greppable classifier decisions via existing bridge logs (`posture_read_only_scan` / approve).
- audit checks: feature_key `ai-providers`; prior claims CA-712/CA-713 (opencode notice) intact; the `find` hole fix is part of this bug's scope (document in CA note as a same-file hardening, not scope creep).

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-56 posture policy wording (if it documents the metachar gate); SD-09 Approval Gates (exec classification composition rule) — flag, do not silently redefine.
- notes left unchanged on purpose: BUG-342's notice plan (depends on deny existing, not on which commands are denied); `grokPermissionKind` mapping; opencode/Claude adapter read-tool paths.