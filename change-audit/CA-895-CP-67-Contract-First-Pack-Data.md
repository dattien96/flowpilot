# CA-895 — CP-67 P-1/P-3/P-4/P-5: contract-first pack data (faces, prompts, personas, topology)

# ---8<--- flowpilot:change-ledger
feature_key: contract-first-tdd
source_doc_id: CP-67
change_type: feature
summary: Declares the CP-67 pack layer — submit_scaffold_outcome / submit_coder_outcome logical faces, the scaffold-architect persona, the contract-first scaffold and scaffold-body prompts, the agent.scaffold behavior, and the synthesis_negotiation hub topology in task-harness + vibe-sprint — with submit_review_outcome widened as the single provider transport (B-1/B-2: no per-face adapter tool)
# --->8---

## Why

CP-67 Contract-First Scaffold TDD needs the pack layer to exist before the
runner can enforce it: the scaffold architect writes stubs + a RED suite,
the coder fills bodies under a signature lock, and a signature that cannot
satisfy the spec must route through a bounded Main-Agent negotiation phase
instead of a silent signature edit.

B-1/B-2 resolution: the two new faces are DECLARED faces — their schemas and
status maps are canonical pack data, but no provider adapter wires a
per-face tool constant. The coder's batch rides `submit_review_outcome`
(status=renegotiate_signatures → generic continue + batch_signature_requests
payload passthrough); the scaffold outcome is gate-read deterministically
from the workspace. The prompts name the logical face for the contract and
the real transport for the call, so no provider ever sees a phantom tool.

## Change

- `flow-pack/tools/submit-scaffold-outcome.yaml`, `submit-coder-outcome.yaml`
  (new): declared faces — scaffold_ready/blocked and
  completed/renegotiate_signatures/blocked status enums, stubs/test_suite and
  batch_signature_requests schemas, statusMap to generic flow-control.
- `flow-pack/tools/submit-review-outcome.yaml`: +renegotiate_signatures
  status (→continue), +batch_signature_requests payload passthrough.
- `flow-pack/agents/scaffold-architect.md` (new) +
  `prompts/scaffold-contract-tdd.md`, `prompts/implement-scaffold-body.md`
  (new): contract-first personas — canonical stub whitelist, RED-suite
  proof, the FOUR prohibitions, and the Accumulate & Batch renegotiation
  contract naming submit_review_outcome as the real transport.
- `behaviors/registry.yaml` + `behavior_registry*.go`: agent.scaffold
  behavior registered.
- `flows/task-harness.yaml`, `flows/vibe-sprint.yaml`: test_signatures/tdd
  become agent.scaffold nodes (high-reasoning model), coder/implement become
  agent.code, and the synthesis_negotiation hub.inline node plus its
  continue-forward / continue-back / done-forward edges are declared.
- `manifest.yaml`, `pack.go`, `flow_safety_topology.go`: face inventory,
  scaffold node in the acceptance topology, model field on scaffold nodes.
- agentpack tests: scaffold_architect/scaffold_tool_face/
  scaffold_flow_topology (new) + updated pack inventory tests pinning the
  new nodes, faces, and edge set.
