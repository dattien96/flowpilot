# BUG-398: r-reg regression guard treats intended TDD-RED state as violation

## Metadata

- Document ID: `BUG-398`
- Title: `Post-turn gate blocks the contracted compile-green/runtime-RED scaffold state (rules=[r-reg]) → owner-debate reprompt loop`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-49-Test-Steps](../../07-Coding-Plan/done/CP-49-Test-Steps.md)
- Feature Keys: `context-regression-engine`, `contract-first-tdd`

## AI Quick View

### Summary

- In a vibe sprint, the `tdd` step's contract is **compile-green / runtime-RED by design** — stubs plus failing tests. The post-turn gate ran `go test ./...`, saw `Tests failed: TestReverse_Empty`, and blocked with `rules=[r-reg]`, spawning an owner-debate round whose verdict was "reprompt the coder".
- Blocking on contracted-RED is what pushed run-2290 into the wedge path (owner-debate → hub verdict lost → `hub_stalled`; CP-49 BUG-LIVE-2/3 family).
- The debate remediation path is by design, but treating the intended RED state as a regression violation means every honest TDD scaffold invites a spurious gate block.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** After the `tdd` (scaffold) step produces the contract-required RED suite, `r-reg` fires `Tests failed: …` as a violation and blocks/escalates.
- **Expected:** For a signature-locked TDD scaffold step, newly-introduced failing tests that are the contracted RED state are not treated as regressions — the scaffold gate (`red_at_capture`/stub checks) owns this step, not the generic regression guard.
- **Actual:** `r-reg` sees red tests (which are new, non-baseline failures) and blocks; owner-debate verdicts "reprompt the coder" — asking the agent to undo the state the contract requires.
- **Impact:** Spurious block at every TDD scaffold step; pressure to make the RED suite green (contradicting the contract) or burn debate rounds. Contributed to run-2290's wedge and run-9's settle-`done`-with-skips path.

## Reproduction

1. Run `vibe-sprint` to the `tdd` step with a signature-locked RED contract (bed: stringutil `Reverse` stubs + `TestReverse_*` failing tests).
2. Let the post-turn gate evaluate the turn.
3. Observe `rules=[r-reg]` block citing `Tests failed: TestReverse_Empty` + `vibe-owner-debate` spawn.
- runIds: `run-2290` (runner.log 02:52:34-35 `[gate]` block + `[vibe-gate] start vibe-owner-debate`); related wedge context `run-9`, `run-1588`.

## Root cause

- The regression guard (`r-reg` in `internal/flowgate` — regressed/failed test rules evaluated in `gate_hook.go` post-turn pass) does not special-case the contracted RED state of a scaffold/tdd step: new non-baseline failures produced by the mandated RED suite are indistinguishable from accidental regressions to the rule. The suppression/`red_at_capture` logic that knows about intentional RED lives on the scaffold/reproduce rules, not in `r-reg`'s evaluation of the turn diff.

## Evidence

- `~/fp-beds/lt-evidence/cp49/RESULT.md` (§Bugs — "Observation (not separately filed): gate `r-reg` treats the intended TDD-RED state as a violation"; promoted here to a tracked bug)
- `~/fp-beds/lt-evidence/cp49/runner.log` — 02:52:34-35 `[gate]` block `rules=[r-reg]`, `Tests failed: TestReverse_Empty`, `[vibe-gate] start vibe-owner-debate`
- `~/fp-beds/lt-evidence/cp49/L49-2-run2290-events.json`, `L49-2-run2290-agentgraph-wedged.json`, `.flowpilot/gate-metrics.ndjson` (referenced in RESULT)

## Severity

- `medium` — systematic false-positive on the designed TDD-RED state; drives flows into the owner-debate wedge path but is recoverable via debate verdicts.

## Completion Notes (implemented 2026-09-23, CA-919b)

- r-tests/r-reg return nil when tr.ScaffoldExpected — contracted RED owned by r-scaffold-red. Test: bug398_scaffold_red_suppression_test.go.
