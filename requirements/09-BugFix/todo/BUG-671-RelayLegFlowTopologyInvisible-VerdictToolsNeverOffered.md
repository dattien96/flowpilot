# BUG-671 — Relay-Leg Parents Hide Flow Topology: Verdict Tools Never Offered To Cohort Legs

## Metadata

- Document ID: `BUG-671`
- Title: `flow topology reads resolve on the relay leg (empty activeFlowNodes) — submit_review_outcome/spawn gates invisible on Devin+Grok legs`
- Phase: `bugfix`
- Status: `FIXED — CA-1238`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Last Updated: `2025-01-24`
- Parent Documents: `BUG-628` (grok outcome tools), `BUG-669` (relay-leg discriminator), `CP-53 P-2`, `CP-61 P-1`, `BUG-504`, `BUG-635`
- Related Documents: `BUG-667`, `BUG-669`, `BUG-670`
- Tags: `chat-legs, verdict-tools, cohort-barrier, provider-parity, severity:critical`

## AI Quick View

### Summary

- Extends BUG-628's original scope (Grok legs missing outcome tools) — live run-752742 proved the gap is **not provider-specific**: Devin AND Grok legs both reported `submit_review_outcome` / `spawn_agent` absent (`q-815828`, `q-832954`, leg transcripts `run-821096`, `run-821102`).
- Root cause is topology resolution, not the adapters: every verdict-offer predicate reads the RUN'S OWN `activeFlowNodes` / `activeFlowAcceptanceNodes` — or its `parentRunID`'s — but a **reattach/relay leg carries none** (BUG-669: inherit stamps `flowEngineDriven` without `activeFlowNodes`).
- When the cohort parent IS a relay leg (live: leg5 `run-817740` spawned reviewers `run-821096`/`run-821102`), all of these resolve on empty topology:
  - `flowNodePostureFor` → `verdictOnlyChild=false`
  - `flowNodeForRunLocked` → `delegateClassChild=false`
  - `flowRequiresSynthesisMachineVerdict(parent)` / `flowRequiresHubMachineVerdict(parent,…)` → cohort-membership offer fails
  - `flowHasVerdictOnlyNode(rs.activeFlowNodes)` on the hub relay leg itself → host-side offer fails
- Result: cohort legs run the review correctly and emit a verdict in prose, but the machine face was never registered — the engine records nothing, the cohort barrier reports missing verdicts, and the only escapes are question+replay or manual `/flow-control` POST (both observed live).
- Same gap on the read side: `synthesisDoneVerdictError`, `hubDoneVerdictError`, `missingVerdictLabelsWithLiveMember`, `hubProseVerdictDerivesFlowStatus` all read `parent.activeFlowNodes` — a relay-leg parent yields "no review cohort nodes configured" even when the launcher leg's real graph declares them.

### Current Ask

- Introduce a single topology-owner seam: a flow-relay leg (`legIsFlowRelay`) resolves flow topology (`activeFlowNodes`, `activeFlowAcceptanceNodes`, acceptance/cohort reads, verdict maps) through the chat's launcher leg — the same `flowLoopOwnerForLocked` resolution BUG-669 added for loop state.
- Verdict record/read symmetry: `recordReviewCohortMemberVerdict` and the verdict readers must key on the topology owner so verdicts recorded under a relay parent remain visible to gates.

### Key Decisions

- `T-1` One resolver (`flowTopologySourceLocked`), reused at every flow-semantic read/write site — per-site ad-hoc fallbacks will drift.
- `T-2` Non-relay runs resolve to themselves — zero behavior change on the non-relay path.
- `T-3` This is provider-agnostic shared-path work (turn offer flags), satisfying the parity contract with one evidence point: the offer flag reaches `TurnRequest.OfferReviewOutcomeTool`, which every adapter (claude/codex/devin/grok/opencode) already consumes identically.

## Evidence

### Live evidence — run-752742 / chat cht_3078718bf299

- Leg `run-816045` (devin reattach): minted with `flowArm=started`, topology empty; its synthesis turn's tools list exposed `approve`/`ask_user`/`spawn_agent` but NOT `submit_review_outcome` — the leg literally could not record its assembled `changes_requested` verdict; resolved via question `q-815828` `replay_inline`.
- Cohort legs `run-821096`/`run-821102` (spawned by relay leg5 `run-817740`): both completed full reviews (spec_align SYNCED T-1..T-5 + reviewer APPROVED) but reported the outcome tool absent — engine recorded nothing; leg5 had to self-POST `/flow-control`.
- `spawn_agent` absence on the same legs ("loop is stopped" / not advertised) is the paired tool-gap half of this defect — the offer path for spawn faces shares the topology-read class.

### Code evidence

- `interactive_service.go:10426-10460` — offer ladder; every predicate reads own/parent `activeFlowNodes`.
- `gate_hook.go:2421` `flowNodeForRunLocked` — `findFlowNode(parent.activeFlowNodes, label)` fails when parent is a relay leg.
- `node_isolation.go:36` `flowNodePostureFor` — returns "" when `len(parent.activeFlowNodes) == 0`.
- `review_done_verdict.go` — `flowRequiresSynthesisMachineVerdict`, `flowRequiresHubMachineVerdict`, `synthesisDoneVerdictError`, `hubDoneVerdictError`, `missingVerdictLabelsWithLiveMember`, `hubProseVerdictDerivesFlowStatus` — all read `rs`/`parent.activeFlowNodes`.

## Root Cause

A reattach leg is a session host: `flowEngineDriven` is inherited for launch-dedup, but `activeFlowNodes`/`activeFlowAcceptanceNodes` are only populated by an actual launch (`flow_executor.go`). BUG-669 fixed loop-state ownership for these legs; the same stale-owner class persists on the **topology** axis — every topology read still resolves on whichever leg happens to hold the run row, so the relay leg's empty graph makes verdict tools and cohort gates silently disappear.

## Fix Direction

- Add `flowTopologySourceLocked(rs)` next to `flowLoopOwnerForLocked` in `chat_switch.go`: relay legs resolve to `flowLoopOwnerForLocked(rs)` (launcher leg), everything else to itself.
- Route every flow-semantic read/write through it:
  - `flowNodeForRunLocked` (hub-self + parent paths)
  - `flowNodePostureFor`
  - offer ladder (parent predicates, `flowHasVerdictOnlyNode`, `runHasFlowNode` for `OfferVibeRequirementTool`)
  - `flowRequiresSynthesisMachineVerdict`, `flowRequiresHubMachineVerdict`
  - `synthesisDoneVerdictError`, `hubDoneVerdictError`, `missingVerdictLabelsWithLiveMember`, `hubProseVerdictDerivesFlowStatus`
  - `recordReviewCohortMemberVerdict`, `snapshotReviewCohortVerdictsLocked` (write side — verdicts land on the topology owner)
- Tests: relay parent + launcher leg + cohort child → `OfferReviewOutcomeTool=true`; hub turn on relay leg → tool offered when launcher topology declares verdict nodes; verdict recorded on a relay parent remains visible to `synthesisDoneVerdictError` on the same parentRunID.

## Declared Paths

- `apps/local-runner/internal/runner/chat_switch.go` — add `flowTopologySourceLocked`
- `apps/local-runner/internal/runner/gate_hook.go` — `flowNodeForRunLocked`
- `apps/local-runner/internal/runner/node_isolation.go` — `flowNodePostureFor`
- `apps/local-runner/internal/runner/review_done_verdict.go` — topology reads + verdict map keying
- `apps/local-runner/internal/runner/interactive_service.go` — offer ladder
- `apps/local-runner/internal/runner/bug671_relay_leg_tool_offer_test.go` — new red→green tests
