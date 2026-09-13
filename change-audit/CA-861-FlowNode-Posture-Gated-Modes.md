# CA-861 — Task-349: flow-node posture forces gated provider modes (fix Task-340 P0)

# ---8<--- flowpilot:change-ledger
feature_key: node-isolation
source_doc_id: Task-349
change_type: task
summary: fix the CP-62 review P0 — read_only/verdict_only flow nodes were structurally unenforced because flow runs are product-locked YOLO=true (BUG-299) and bypass provider modes never emit permission requests; TurnRequest gains FlowNodePosture, resolveYoloPostureForFlowNode (YOLO SSOT) forces gated modes (Claude default, Codex untrusted/workspace-write, Grok/Opencode "") AND RunnerAutoApprove=false for gated postures, so every tool call reaches turnBridge.RequestApproval where the posture matrix decides; isVerdictToolCall now matches wrapped MCP names and read_only allows the verdict face
# --->8---

## Why
Task-340's bridge matrix was correct but unreachable: ForceShellBridge excludes reviewer children and bypass modes swallow permission requests — the declared postures were dead letters end-to-end.

## Change
- `provider_registry.go`: TurnRequest.FlowNodePosture (+comment); single build site in interactive_service.go sets it from flowNodePostureFor(rs).
- `yolo_resolver.go`: IsGatedFlowNodePosture + resolveYoloPostureForFlowNode (gated ⇒ same modes as read-only chat postures + RunnerAutoApprove=false).
- `claude_adapter.go`/`codex_adapter.go` resolve through the new SSOT (codexYoloDeriveForFlowNode added); `grok_adapter.go` yoloModes also cleared for gated postures.
- `node_isolation.go`: isVerdictToolCall accepts `__submit_review_outcome`-suffixed wrapped MCP names; `chat_posture_policy.go` readOnlyApprovalDecision approves the verdict face on mcp/other (runner-hosted, content enforced at SubmitFlowControl).

## Tests
`flow_node_gating_test.go` (5): gated postures force all provider modes + auto-approve off; standard keeps YOLO bypass; wrapped verdict names recognized (Reason+Command); read_only approves wrapped verdict face; verdict_only matrix with wrapped names. Posture/YOLO/approval + isolation/profile suites green with -race.

## Providers
Parity by construction — all adapters resolve through the one SSOT; the resolver table test covers claude/codex/grok/opencode outputs at once.

## Prior claims intact
BUG-299 flow YOLO lock untouched for standard children; ForceShellBridge semantics unchanged; cp-harness/rag-harness clone pins still green.
