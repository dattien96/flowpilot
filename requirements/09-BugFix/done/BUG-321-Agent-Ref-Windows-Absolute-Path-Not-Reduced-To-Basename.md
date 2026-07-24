# BUG-321: Flow Node Agent Ref With a Windows Absolute Path Is Not Reduced to Its Basename

## Metadata

- Document ID: `BUG-321`
- Title: `agentNameFromRef (and its wrapper flowNodeAgentName) used the POSIX-only "path" package, so a flow node whose agent ref is a Windows absolute path (backslash separators, e.g. C:\Users\me\.codex\agents\coder-agent.toml) was returned as the whole path minus only its extension instead of "coder-agent" -- breaking every consumer of the derived catalog name for such nodes (gate reviewer-tier detection, resume step-timeline matching, model resolution)`
- Phase: `bugfix`
- Status: `done`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-24`
- Last Updated: `2026-07-24`
- Parent Documents: `none`
- Child Documents: `none`
- Related Documents: [BUG-256](../done/BUG-256-Restarted-Flow-Step-Timeline-Loses-Reviewer-Statuses-When-Nodes-Share-Agent.md) (introduced matchFlowNodeForSession's Label-preferred match with an AgentName/Role fallback -- this bug lives in that fallback's use of the derived agent name), [BUG-320](../done/BUG-320-Restored-Stopped-Flow-Shows-Wrong-Step-Timeline-Because-Dropped-Child-Had-No-Evidence.md) (surfaced this bug: reopening a pre-Label restored run showed the coder as PENDING even though its session record existed, because the Label-less fallback match failed on the Windows-path agent ref), [CA-420](../../change-audit/CA-420-agent-ref-windows-path-basename.md)
- Replaces: `none`
- Tags: `agent-flow-engine, flow-node, agent-catalog, resume, gate, windows, cross-provider, severity-medium`

## AI Quick View

### Summary

Found while diagnosing why reopening an already-restored BUG-320 chat
(`run-55348`) still showed `my-coder` as PENDING even though its child session
record existed on disk. That record had no `Label` (it was restored under
pre-Label-preservation code), so `matchFlowNodeForSession` fell back to matching
by `AgentName`/`Role` against `flowNodeAgentName(node)`. The node's `agent:` ref
was a Windows absolute path (`C:\Users\dat.nguyen\.codex\agents\coder-agent.toml`
-- the live flow references provider CLI agent files by absolute path), and
`flowNodeAgentName` -> `agentNameFromRef` used Go's POSIX-only `path` package:
`path.Base` finds no `/`, returns the whole string, and `TrimSuffix(base,
path.Ext(base))` strips only `.toml`, yielding
`C:\Users\dat.nguyen\.codex\agents\coder-agent` instead of `coder-agent`. So the
fallback match compared `coder-agent` (the session's AgentName) against that full
path and never matched.

### Current Ask

Make `agentNameFromRef` reduce a Windows-style (backslash-separated) agent ref to
its basename exactly as it already does for POSIX (`/`-separated) refs, so every
consumer of the derived catalog name works regardless of whether the ref was
written with `\` or `/`. No behavior change for the `/`-based and bare-name refs
every built-in flow pack already uses.

### Key Decisions

- Normalize `\` -> `/` unconditionally before the POSIX `path` package parses the
  ref, rather than switching to `path/filepath`. `path/filepath` is OS-dependent
  (on Linux/CI it treats `\` as an ordinary filename character), so it would make
  the derivation differ between the Windows dev host and Linux CI. An
  unconditional string replace + the always-`/` `path` package is deterministic
  on every OS.
- The change is a pure widening: for any ref with NO backslash (100% of built-in
  flow packs, every pre-existing test case, every POSIX path) the output is
  byte-identical. The only inputs whose result changes are backslash-bearing
  refs, which today produce a definitely-wrong value.
- Scope is the single leaf `agentNameFromRef`; its wrapper `flowNodeAgentName`
  and all their callers are left untouched and inherit the fix.

### Constraints

- Additive tests only; no pre-existing test edited (the existing
  `TestFlowNodeAgentNameDerivesFromFilePath` stays green, unmodified).
- No real machine paths in tests (the Windows paths in the table are literal
  string inputs, not touched on disk).

### Open Questions

- `none`.

### Source Refs

- `apps/local-runner/internal/runner/flow_executor.go` -- `agentNameFromRef`
  (the leaf fix), `flowNodeAgentName` (its wrapper, unchanged).
- Consumers exercised by the tests: `matchFlowNodeForSession`
  (`interactive_resume.go`, resume step-timeline node matching) and
  `isFlowReviewerChild` (`gate_hook.go`, gate reviewer-tier detection).

## 1. Issue Summary

`agentNameFromRef(agentRef)` is meant to reduce a flow node's `agent:` file
reference to the agent catalog name (`agents/coder.md` -> `coder`). It used
`path.Base`/`path.Ext` from Go's POSIX-only `path` package, which only recognizes
`/` as a separator. Given a Windows absolute ref like
`C:\Users\dat.nguyen\.codex\agents\coder-agent.toml`, it returned
`C:\Users\dat.nguyen\.codex\agents\coder-agent` (the whole path minus only the
`.toml` extension). Every consumer of the derived name then failed for that node.

## 2. Parent Links

`agent-flow-engine` -- flow node agent-name derivation, used by flow spawning,
per-node model resolution, resume step-timeline matching
(`matchFlowNodeForSession`), and gate reviewer-tier detection
(`isFlowReviewerChild`).

## 3. Environment and Reproduction

Windows host; a custom flow whose nodes reference provider CLI agent files by
absolute path (e.g. `agent: C:\Users\dat.nguyen\.codex\agents\coder-agent.toml`).

1. Run such a flow so a child session persists with `AgentName = "coder-agent"`
   but (for a record written before Label preservation) no `Label`.
2. Restart / restore / reopen the run so the step timeline is rebuilt from
   persisted sessions.
3. `matchFlowNodeForSession` falls back to AgentName matching, compares
   `coder-agent` against `flowNodeAgentName(node)` = the full Windows path, and
   finds no match -> the node shows PENDING even though its session exists.

Also reproducible directly at the unit level: `agentNameFromRef` on any
backslash-terminated ref returns the un-stripped path.

## 4. Expected vs Actual

- Expected: `agentNameFromRef(C:\Users\...\coder-agent.toml)` = `coder-agent`;
  the resume match and gate reviewer detection work for Windows-path agent refs.
- Actual: it returned `C:\Users\...\coder-agent`; the AgentName/Role fallback
  match and `isFlowReviewerChild`'s node-name check both failed for such nodes.

## 5. Root Cause

`path.Base` and `path.Ext` (package `path`) operate on slash-separated paths
only. On a backslash-separated Windows path they see a single element with no
directory, so `path.Base` returns the input unchanged and only the extension is
trimmed. This is masked whenever the primary `session.Label == node.ID` match
succeeds first (BUG-256/BUG-320 Label preservation), which is why it only
surfaced on Label-less records; but the derived name is ALSO used outside resume
(flow spawn, model resolution, gate tier), so the defect is not resume-specific.

## 6. Fix Strategy

- In `agentNameFromRef`, after trimming, `agent = strings.ReplaceAll(agent,
  "\\", "/")` before calling `path.Base`/`path.Ext`. Everything else is
  unchanged. A no-op for refs without backslashes; correct basename for refs
  with them (including mixed-separator and UNC forms).

## 7. Validation

- Red-first leaf unit proof:
  `TestAgentNameFromRefStripsWindowsAndMixedSeparators` -- a 12-row table
  (Codex/Claude/Grok Windows absolute paths, mixed separators, UNC, relative
  backslash, no-extension, and the pre-existing POSIX shapes). 7 rows fail
  pre-fix (all backslash-terminal), all pass after; the POSIX/mixed rows pass on
  both sides, proving no regression to existing behavior.
- Red-first resume-match proof (the live symptom, the HIGH-risk resume process):
  `TestMatchFlowNodeForSessionResolvesWindowsPathAgentByFallback` -- a Label-less
  session matches its node by AgentName/Role when the node's agent ref is a
  Windows path. Fails pre-fix (no match), passes after. Covers Codex coder,
  Claude reviewer (by AgentName), and reviewer-by-role.
- Red-first gate proof (the second HIGH-risk process,
  `runFlowGateAtEpoch` -> `isFlowReviewerChild`):
  `TestIsFlowReviewerChildDetectsWindowsPathReviewerNode` -- a reviewer node with
  a Windows-path agent ref is recognized as a reviewer (and a coder node is not)
  even when the run's own role/agentName are empty. Fails pre-fix, passes after.
- `go build ./...`, `go vet ./internal/runner/...`: clean.
- Pre-existing `TestFlowNodeAgentNameDerivesFromFilePath` (the original
  POSIX-only table) re-run unmodified and green (R1).
- Full-package sweep vs `git stash` baseline at the same HEAD (see §8 for the
  exact FAIL-set diff): no changed-area test regressed; only pre-existing
  order-dependent flakes differ. Fix = 2343 passed / 18 failed / 21 skipped; baseline (fix stashed, new test held aside) = 2322 passed / 21 failed / 21 skipped. FIX-vs-baseline delta: ZERO tests in fix-but-not-baseline (no regression); 17 stable failures common to both are pre-existing environment-specific (Codex/Grok provider-home & path, git-commit-guard shim, skills-merge, auth-workspace command -- all depend on provider installs / machine paths absent in the test env); the 3 baseline-only failures (TestFinalizerHookSurfacesArtifacts, TestGrokSpawnPromptCompositionMatchesClaudeCodexBaseline, TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows) are order-dependent flakes that pass 3/3 in isolation on the fixed tree.
- Real `provider-accounts.json` verified unchanged across the sweep.
- Live end-to-end: PENDING -- the fix loads at runner build/boot; the running
  binary predates it. To verify, run a fresh flow whose nodes reference agent
  files by Windows absolute path, restart/reopen, and confirm every node's step
  status matches its Agents-panel status (no PENDING for a node whose child
  session exists).

## 8. Regression Guard

- `TestAgentNameFromRefStripsWindowsAndMixedSeparators` locks the leaf across
  Windows/mixed/UNC/relative-backslash/no-ext plus the pre-existing POSIX shapes.
- `TestMatchFlowNodeForSessionResolvesWindowsPathAgentByFallback` locks the
  resume step-timeline match for Windows-path agent refs.
- `TestIsFlowReviewerChildDetectsWindowsPathReviewerNode` locks gate
  reviewer-tier detection for Windows-path reviewer nodes.
- The pre-existing `TestFlowNodeAgentNameDerivesFromFilePath` stays untouched and
  green.
- Impact analysis (GitNexus): `agentNameFromRef` upstream = LOW (14 hits, 2
  direct); its wrapper `flowNodeAgentName` = HIGH (5 direct callers, 3 processes
  -- `runFlowGateAtEpoch`, `runChildArtifactOutputGateAtEpoch`,
  `handleSubmitFlowControl`). The HIGH blast radius is why the two HIGH-risk
  processes (gate + resume-match) are each covered by a dedicated test above; the
  fix itself is a pure widening that cannot alter any non-backslash input.

## 9. Follow-Up Document Updates

- CA-420 records the change.
- Live end-to-end section above needs the user's post-restart retest result.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-321
change_type: bugfix
summary: agentNameFromRef now normalizes Windows backslash separators to "/" before the POSIX path package reduces a flow node's agent ref to its catalog name, so a node whose agent is a Windows absolute path (e.g. C:\Users\me\.codex\agents\coder-agent.toml) yields "coder-agent" instead of the whole path minus its extension; this fixes every consumer of the derived name for such nodes -- most visibly resume step-timeline matching (matchFlowNodeForSession's AgentName/Role fallback) and gate reviewer-tier detection (isFlowReviewerChild) -- with zero change for the "/"-based and bare-name refs every built-in flow pack already uses.
# --->8---
