# CA-717 — Scan allowlist: `2>&1` redirect + `go` report subcommands + stdout `curl` (BUG-344 follow-up)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-344
change_type: bugfix
summary: allow the no-file 2>&1 stderr redirect and add go (test/list/env/doc/version/help) and stdout-only curl to the read-only exec allowlist, so scan mode stops false-denying git log/status with 2>&1 and go test exploration
# --->8---

## Problem (live runs)

- `run-472208` (opencode, scan, `scan xem project nay lam gi`): the ONLY tool
  was `git log --oneline -15 2>&1 | head -20; echo "---"; git status --short
  2>&1 | head -30` — every segment is a git READ, but `2>&1` hit the
  redirect gate (BUG-344 only allowed `2>/dev/null`) → false deny → blank
  notice.
- `run-472155` (opencode, scan, `test B4-inplace, toi la Nam`): 4 compound
  read commands approved; `go test -v ./... 2>&1 | tail -n 50; echo "EXIT:$?"`
  denied because `go` is not an allowlisted binary. The user's intent was
  read-style exploration (tests don't modify source).

## What changed (`chat_posture_policy.go`)

- `scanShellCommandSyntax`: the stderr-to-stdout dup **`2>&1`** (exact form)
  is now stripped like `2>/dev/null` — it writes no file. Every other `>`
  shape still denies (`>`, `>>`, `2>file`, `1>`, `&>`, `< >`, `2>& 1`,
  `2>&2`, `2>&10`, `2>&1x`).
- `isReadOnlyCommandSegment` new allowlisted binaries, both subcommand- or
  flag-constrained (fail-closed stays):
  - `go`: bare (help), `test`, `list`, `env`, `doc`, `version`, `help`.
    `build/run/get/install/generate/mod/vet/...` deny. `go test -c/-o`
    (compiles a binary to disk) denies.
  - `curl`: stdout fetches only; `-o/-O/--output/--output-document`
    (download to disk) and `-d/-F/-T/--upload-file/-X/--request` (send data,
    mutate remote) deny. `curl | sh` still denies compositionally (`sh` not
    allowlisted).
- Header + scanner comments updated with the invariant.

Deliberate scope note: `go test` executes test binaries which may write files
inside test code, and `curl` performs network fetches — both accepted by the
operator for scan because they do not modify the project's source. This is an
allowlist widening (NOT a reject-list flip): unknown binaries still deny, so
`python`, `make`, `npm`, `rm`, `git commit`, `dd`, etc. remain blocked.

## R1 — old-suite regression evidence

- All legacy posture tests PASS unchanged (`TestReadOnlyApprovalDecision_*`,
  `TestIsReadOnlyToolName_*`, `TestBug344*`).
- Full `./internal/runner` run vs post-344: no new failure in any
  policy/posture/grok path. One unrelated flow-engine harness test flaked
  under full-suite load (`TestTryAdvanceSpawnsAgentCodeWriterWithWriterPrompt`)
  and passes in isolation (`ok 0.628s`) — same load-flake class as the three
  already-recorded harness tests; not a regression.
- No legacy test edited (additive only).

## R2 — provider parity

The classifier is provider-agnostic (only `ApprovalDetails{Kind:"exec",
Command}`); the new cases are covered by the shared matrix — the same policy
serves Grok `run_terminal_command`, opencode bash, Codex exec, and Claude
bash. No adapter touched.

## R3 — new coverage (extended `bug344_readonly_compound_exec_test.go`)

- **Approve:** exact `run-472208` command (`git log … 2>&1 | head; echo "---";
  git status --short 2>&1 | head -30`), `git diff 2>&1 | head`, exact
  `run-472155` command (`go test -v ./... 2>&1 | tail; echo "EXIT:$?"`),
  `go list/go env/go doc`, `go test -run … 2>&1 | tail`, `curl -s URL | head`,
  `curl -fsSL URL`.
- **Deny:** `go build/run/get/mod tidy/vet`, `go test -c -o`, `curl -o/-O/-d/
  -X POST/-F`, `curl | sh`, `curl > out.txt`, and the `2>&1` near-misses
  (`2>& 1`, `2>&2`, `2>&10`, `2>&1x`).

## Honest gaps

- Live rerun of run-472208/472155 scenarios post-fix still pending (expected:
  both turns complete with a model answer, no notice). Unit matrix is the R1–R3
  evidence for this commit.
- `go test` may write files inside test code; `curl` performs network fetches —
  operator-accepted scope, recorded here.
- `go vet` is denied (not in the report list) — conservative; revisit if scan
  exploration needs it.

## Prior CA not undone

- CA-715 (BUG-344 compositional parser + find tightening) intact — this
  follow-up only widens the allowlist and the redirect set.
- CA-716 (Grok no-reply notice) and CA-713 (opencode notice) untouched — the
  deny outcome shape is unchanged.
- CA-714 (always-ask env) intact.