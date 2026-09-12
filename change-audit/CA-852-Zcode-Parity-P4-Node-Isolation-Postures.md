# CA-852 — CP-62 P-4: per-node read-only enforcement (postures at the approval bridge)

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-340
change_type: task
summary: FlowNode.posture (flow-YAML data) enforced at the provider-neutral approval bridge (turnBridge.RequestApproval) BEFORE YOLO — read_only reuses the BUG-344 read-only machinery (reads + per-command-classified Bash approve; writes/compound deny), verdict_only (owners, Q-3) has no Bash at all + only the verdict face for MCP; denial emits node_isolation_write_denied; scout/reviewer nodes across all harness flows + vibe-owner-debate/sprint declare postures; the oracle rule (SS-14 AC-6) becomes structural, not prompted
# --->8---

## Why

Reviewer/owner personas declared `tools:` but the field had no consumer in the spawn path; the read-only posture engine deliberately excluded flow runs — review nodes were only *asked* not to write, so test-weakening (SS-14 AC-6) was prompt-enforced only.

## Change

- `agentpack/pack.go`: `FlowNode.Posture` + YAML parse (`posture:`).
- `runner/node_isolation.go` (new): posture constants; `flowNodePostureFor` (parent activeFlowNodes ↔ child stepID/label; hubs/roots never gated); `evaluateFlowNodeApproval` matrix; `evaluateVerdictOnlyApproval` (no Bash incl. read-only, per Q-3); `isVerdictToolCall`; `EventNodeIsolationWriteDenied`.
- `runner/interactive_service.go`: `turnBridge.RequestApproval` consults `decideFlowNodePosture` after the flow-coding-commit deny, BEFORE the chat posture/YOLO branches; denials emit the audit event.
- Flow YAMLs: `posture: read_only` on scout/reviewer/plan_reviewer/cp_reviewer (task-harness, bug-plan-harness, cp-harness, bug-harness, rag-harness) + vibe-sprint scout; `posture: verdict_only` on owner_1/2 (vibe-owner-debate).
- Doc fix: `go vet`/`go build` are WRITE per the BUG-344 classifier (build cache) — examples updated to git log/rg in CP-62 + Task-340.

## Tests

`runner/node_isolation_test.go` (7: reviewer write deny, read-allow incl. go-vet-is-write pin, bash write deny, owner no-bash + verdict-only MCP matrix, standard-not-handled, run→node posture resolution incl. root-run, pack posture parse). R1 event: `TestBugHarnessPackClone` pins bug-harness ≡ rag-harness node-for-node → fixed by giving rag-harness the same postures (production data); the old test is untouched. agentpack suite green; runner pinned suites (vibe/gate/CP61/CP53/picker/flowAllowed) green.

## Providers

Case 1 by construction — enforcement sits in the provider-neutral `turnBridge.RequestApproval` every provider's approvals funnel through; `node_isolation.go` references no provider key.

## Prior claims intact

BUG-344 read-only composition invariant reused verbatim (no classifier edits); Task-242 flow-coding-commit deny ordering preserved (posture check runs after it); BUG-369 child working-mode stamping untouched; Task-326 pack inventory (11 flows / 8 agents) unchanged — only additive node fields.
