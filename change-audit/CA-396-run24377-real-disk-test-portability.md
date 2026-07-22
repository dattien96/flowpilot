# CA-396: run-24377 "real disk" regression test made portable (CP-51 Phase A audit)

## Summary

Found during the 2026-07-22 CP-51 Phase A (A1–A13) coverage audit: `TestRun24377RealDiskStoreResumeOrder` (`run24377_real_disk_verify_test.go`) unconditionally `t.Skip`s unless `.flowpilot/chats/run-24377-turns.ndjson` exists under the hardcoded absolute path `/Users/tiendat/Desktop/flowpilot/flowpilot` — the original author's own machine. On every other machine (including this repo/CI) it always skips, silently, and looked like coverage without ever actually running.

## Fix

Additive-only, input-sourcing change, zero assertion changes: when the hardcoded disk path exists, parse it exactly as before (unchanged real-capture verification path). When it does not, fall back to constructing the same `entries`/`children`/`hubNodes` inputs from literals — the exact same run-24377 incident data already embedded in the sibling `run24377_live_fixture_resume_order_test.go` (`TestRun24377LiveFixtureResumeOrderIsCorrect`). Every downstream assertion in the test body (no system-prompt leak, no child pollution, agent-before-first-synthesis, no consecutive same-child coder activations, no agent card between a post-flow follow-up and its answer, no mixed timed/untimed reorder risk) is untouched — only the two input variables' source changed from "disk-or-skip" to "disk-or-embedded-literal", so the test now always executes.

User-authorized: this edits a pre-existing test file, gated by the project's `additive-tests-only` convention (assertions preserved; only the never-reachable disk dependency was replaced with an always-available fallback), explicitly approved via AskUserQuestion before editing.

## Cross-provider parity

N/A — test-only change, Grok-fixture data, no runtime/provider code touched.

## Verification

- `go vet ./internal/runner/`: clean.
- `go test ./internal/runner/ -run 'TestRun24377' -v -count=1`: **3/3 pass, 0 fail** (`TestRun24377FlowHubPrefersTurnLogWhenSharedGrokSessionPolluted`, `TestRun24377LiveFixtureResumeOrderIsCorrect`, `TestRun24377RealDiskStoreResumeOrder` — the last one now actually running its full body via the fallback branch instead of skipping).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: run-24377
change_type: test
summary: Make TestRun24377RealDiskStoreResumeOrder portable (embedded fallback when the author's hardcoded disk path is absent) so it actually runs on every machine/CI instead of silently skipping.
# --->8---
