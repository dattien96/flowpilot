# CA-715 — Read-only posture approves compound read-only bash; `find` write primaries denied (BUG-344)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-344
change_type: bugfix
summary: make the scan/plan exec classifier compositional (quote-aware ; | && || segments, only 2>/dev/null redirect) so compound read-only bash like Grok's run_terminal_command is approved, and deny find delete/exec/write primaries (find . -delete no longer passes)
# --->8---

## Problem

The read-only posture's exec classifier (`isReadOnlyCommand`) denied ANY command
containing a metacharacter (`>|;&`$`) — a fail-closed whole-command gate. Grok
batches exploration into one compound bash call (`rg … | head; ls …; find …
2>/dev/null`), so its legitimate read commands were denied every time in
scan/plan; the denied turn then aborted blank (BUG-342). Same gate also had a
hole: `find` was allowlisted wholesale, so `find . -delete` passed.

## What changed (`chat_posture_policy.go`)

- Replaced the flat metachar gate with a **compositional, quote-aware**
  classifier (BUG-344 D-1/D-2/D-5):
  - `scanShellCommandSyntax`: quote-aware; denies backtick / `$(` / `${`
    substitution (still active inside double quotes, matching shell);
    quotes and parentheses must balance; the ONLY allowed redirect is the
    exact `2>/dev/null` / `2> /dev/null`, which is stripped — every other
    `>` shape (>, >>, 2>file, 1>, &>, <>, 2>&1) denies.
  - `splitReadOnlySegments`: splits on top-level `;`, `|`, `&&`, `||`
    (separators inside quotes are literal — `rg "B4|inplace"` keeps its
    pattern pipe); single top-level `&` (backgrounding) denies.
  - `isReadOnlyCommandSegment`: per-segment classification using the existing
    binary allowlist + `sudo` handling + `isReadOnlyGit`; one failing segment
    denies the whole command.
  - `isReadOnlyFind` (BUG-344 D-4): denies `find`'s delete/exec/write action
    primaries (`-delete -exec -execdir -ok -okdir -fls -fprint -fprint0
    -fprintf`); pure search/print predicates pass.
- Header comment updated with the composition invariant.
- `$` alone is still allowed (regex anchor, e.g. `rg 'foo$' src`); only
  `$(` / `${` are substitution and denied.

## R1 — old-suite regression evidence

- **Disclosed + operator-approved assertion flip (Q-4):**
  `chat_posture_policy_test.go` `pipe to write` (`ls | grep foo`, deny) is now
  `pipe read` in the approve matrix — that deny was the exact whole-command
  gate being fixed. Kept a replacement deny case `pipe to tee` (`ls | tee out`,
  `tee` is not allowlisted). No other legacy assertion changed.
- All legacy posture tests PASS unchanged (`TestReadOnlyApprovalDecision_*`,
  `TestIsReadOnlyToolName_*`).
- Full `./internal/runner` run: FAIL set is a **strict subset** of the
  pre-existing baseline failures (verified by diffing `--- FAIL` lists against
  the pre-BUG-344 run — zero new failures; one flaky timing test
  `TestMultiWorkspaceRunsIndependent` even flipped green). The pre-existing
  failures (flow-engine residual/gate harness, skills scope-drift, provider
  inventory, supabase/firebase) are unrelated and reproduce on a clean stash.

## R2 — provider parity

The classifier is **provider-agnostic**: `readOnlyApprovalDecision` inspects
only `ApprovalDetails{Kind:"exec", Command}`. `TestBug344ClassifierIsProviderAgnostic`
runs the same read/write commands through the exec path and asserts identical
decisions — the same code serves Claude, Codex, and Grok exec requests
(Grok `run_terminal_command`, Codex exec, Claude bash all map to
`Kind:"exec"` before reaching the policy). No adapter code touched.

## R3 — new coverage (`bug344_readonly_compound_exec_test.go`, additive)

- **Reported repro:** the real run-464841 compound Grok exploration command →
  approve.
- **Approve matrix:** pure pipes, `;` chains, `&&`, sudo pipe, `2>/dev/null`,
  `find -name … -print`, regex `$` anchor, single-quoted `$(…)` literal.
- **Deny matrix:** `;`/`&&`/pipe to write, every redirect shape (`>`, `>>`,
  `2>`, `1>`, `2>&1`-style), substitution (`$(`/backtick/`${}`), `tee`, git
  writes chained, `find -delete/-exec/-execdir/-ok/-fls/-fprint/-fprintf`,
  background `&`, empty trailing segment.
- **Fail-closed:** unbalanced quotes/parens deny.

## Honest gaps

- Post-fix LIVE runner E2E still pending (rerun run-464841 prompt in scan:
  compound read approved + turn completes with an answer; `git commit`/`rm`
  still denied → BUG-342 notice if landed). Unit matrix is the R1–R3 evidence
  for this commit.
- Conservative false-negatives accepted: `find . -name '-delete'` (literal
  pattern arg) is denied; a `head -n 2>/dev/null`-style adjacency is parsed as
  a stderr redirect. Both only deny reads, never let writes through.
- `$`-anchor allowance is a deliberate refinement over the draft doc (blanket
  `$` deny would break `rg 'foo$'`); substitution is still fully blocked.

## Prior CA not undone

- CA-714 (BUG-343 always-ask env / no `--always-approve`) intact — the deny
  path this policy feeds is unchanged in shape.
- CA-713 (opencode no-reply notice) untouched.
- `isReadOnlyGit` byte-for-byte identical; binary allowlist unchanged (only
  composition + `find` args differ).
