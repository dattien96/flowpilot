# CA-420: flow node agent ref with a Windows absolute path is reduced to its basename

## Summary

Found while diagnosing why reopening an already-restored BUG-320 chat
(`run-55348`) still showed `my-coder` as PENDING even though its child
session record existed on disk. That record had no `Label` (restored under
pre-Label-preservation code), so `matchFlowNodeForSession` fell back to
matching by `AgentName`/`Role` against `flowNodeAgentName(node)`. The node's
`agent:` ref was a Windows absolute path
(`C:\Users\dat.nguyen\.codex\agents\coder-agent.toml`), and
`flowNodeAgentName` -> `agentNameFromRef` used Go's POSIX-only `path`
package, so `path.Base` returned the whole string and only `.toml` was
stripped -- the fallback compared `coder-agent` against the full path and
never matched. Full analysis in
[BUG-321](../requirements/09-BugFix/done/BUG-321-Agent-Ref-Windows-Absolute-Path-Not-Reduced-To-Basename.md).

Root cause: `path.Base`/`path.Ext` (package `path`) only recognize `/` as a
separator. On a backslash-separated Windows path they see one element with no
directory, so the directory is left in place.

## Change

- `agentNameFromRef` (flow_executor.go) now does
  `agent = strings.ReplaceAll(agent, "\\", "/")` after trimming and before
  `path.Base`/`path.Ext`. Everything else is unchanged.
- Deliberately NOT `path/filepath`: that package is OS-dependent (on Linux/CI
  it treats `\` as an ordinary filename character), which would make the
  derivation differ between the Windows dev host and Linux CI. An
  unconditional string replace + the always-`/` `path` package is
  deterministic on every OS.
- Pure widening: for any ref without a backslash (100% of built-in flow packs,
  every pre-existing test, every POSIX path) the output is byte-identical. The
  only inputs whose result changes are backslash-bearing refs, which today
  produce a definitely-wrong value.

## Provider parity

Provider-agnostic string handling: `agentNameFromRef` does not branch on
provider. The manifestation is provider-independent (any provider's flow node
can reference an agent file by Windows absolute path). The leaf table test
covers `.codex`/`.claude`/`.grok` agent-dir path shapes as parity evidence,
and the resume-match test covers a Codex coder + a Claude reviewer.

## additive-tests-only compliance

New test file (`bug321_agent_name_from_windows_path_test.go`, 3 test
functions) only. No pre-existing test edited -- in particular the original
`TestFlowNodeAgentNameDerivesFromFilePath` (the POSIX-only table) stays
untouched and green.

R3 matrix coverage: reported leaf repro (Codex Windows absolute path) plus
mixed-separator, UNC, relative-backslash, and no-extension shapes, and the
pre-existing POSIX shapes as an in-table regression guard; the resume
step-timeline match (matchFlowNodeForSession's Label-less AgentName/Role
fallback -- the live symptom); the gate reviewer-tier detection
(isFlowReviewerChild) -- the two HIGH-risk downstream processes GitNexus
flagged.

## Verification

- Red-first: `TestAgentNameFromRefStripsWindowsAndMixedSeparators` (7 of 12
  rows fail pre-fix), `TestMatchFlowNodeForSessionResolvesWindowsPathAgentByFallback`
  (no match pre-fix), and `TestIsFlowReviewerChildDetectsWindowsPathReviewerNode`
  (reviewer mis-detected pre-fix) -- all green after.
- `go build ./...`, `go vet ./internal/runner/...`: clean.
- Pre-existing `TestFlowNodeAgentNameDerivesFromFilePath`: green, unmodified.
- Full-package sweep vs `git stash` baseline at the same HEAD (production edit
  stashed + new test file held aside so the baseline is a true pre-BUG-321
  tree): fix and baseline FAIL sets differ only by pre-existing
  order-dependent flakes in files this change never touches; no changed-area
  test regressed. Fix = 2343 passed / 18 failed / 21 skipped; baseline
  (fix stashed, new test held aside) = 2322 passed / 21 failed / 21 skipped.
  In-fix-not-baseline = EMPTY (no regression); 17 stable failures common to
  both are pre-existing environment-specific; the 3 baseline-only failures
  (finalizer-hook, grok-spawn-prompt, concurrent-index-merge) pass 3/3 in
  isolation on the fixed tree (order-dependent flakes).
- Real `provider-accounts.json` verified unchanged across the sweep.

## Impact analysis (GitNexus)

- `agentNameFromRef` (the edited leaf): upstream risk LOW -- 14 hits, 2 direct
  callers.
- `flowNodeAgentName` (its wrapper, shares the behavior): upstream risk HIGH --
  5 direct callers, 3 processes affected (`runFlowGateAtEpoch`,
  `runChildArtifactOutputGateAtEpoch`, `handleSubmitFlowControl`). The HIGH
  blast radius is the reason the two HIGH-risk processes (gate reviewer
  detection, resume step-timeline match) each get a dedicated regression test;
  the fix is a pure widening that cannot change any non-backslash input.

## Not fixed by recent commits

BUG-256 introduced the Label-preferred match with the AgentName/Role fallback,
and BUG-320 preserved Label on restore so the primary match succeeds first for
fresh restores -- but neither touched `agentNameFromRef`, and the derived name
is also used outside resume (flow spawn, model resolution, gate tier), so this
defect predates and is orthogonal to both.

## Known limits (documented, out of scope)

- Live end-to-end re-verification is pending a runner rebuild/restart (the fix
  loads at build/boot; the running binary predates it).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-321
change_type: bugfix
summary: agentNameFromRef now normalizes Windows backslash separators to "/" before the POSIX path package reduces a flow node's agent ref to its catalog name, so a node whose agent is a Windows absolute path (e.g. C:\Users\me\.codex\agents\coder-agent.toml) yields "coder-agent" instead of the whole path minus its extension; fixes every consumer of the derived name for such nodes -- most visibly resume step-timeline matching and gate reviewer-tier detection -- with zero change for the "/"-based and bare-name refs every built-in flow pack already uses.
# --->8---
