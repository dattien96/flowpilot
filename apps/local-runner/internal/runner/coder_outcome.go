package runner

import (
	"fmt"
	"sort"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

// CP-67 P-1 (Task-378): the coder side of Contract-First Scaffold TDD.
// B-1/B-2 resolution — submit_scaffold_outcome / submit_coder_outcome are
// harness-declared logical faces: registered in the pack, rendered into the
// node prompts, and mapped here through the same FlowControl bridge the
// review loop already uses. No per-face MCP constant is wired into any
// provider adapter; renegotiation rides the existing flow_control transport
// and the batch is buffered record-only for the synthesis_negotiation hub.

// CoderBatchSignatureRequest is one row of the coder's Accumulate & Batch
// renegotiation set (submit-coder-outcome.yaml batch_signature_requests).
// Every field is required — the schema validator enforces rationale, because
// the Main Agent adjudicates the batch and a row without a reason is noise.
type CoderBatchSignatureRequest struct {
	Symbol            string `json:"symbol"`
	File              string `json:"file"`
	CurrentSignature  string `json:"current_signature"`
	ProposedSignature string `json:"proposed_signature"`
	Rationale         string `json:"rationale"`
}

// coderOutcomeStatuses are the domain statuses of the submit_coder_outcome
// face (canonical term per Task-378 T-5: renegotiate_signatures — never
// renegotiate_requested).
var coderOutcomeStatuses = map[string]bool{
	"completed":              true,
	"renegotiate_signatures": true,
	"blocked":                true,
}

// coderOutcomeFace returns the declared face for submit_coder_outcome, pack
// data first with the same pack-load fallback reviewOutcomeFace uses.
func coderOutcomeFace() FlowControlFace {
	if face, ok, err := agentpack.LoadBuiltinToolFace("submit_coder_outcome"); err == nil && ok && len(face.StatusMap) > 0 {
		return FlowControlFace{Tool: face.ID, Map: face.StatusMap}
	}
	return FlowControlFace{
		Tool: "submit_coder_outcome",
		Map: map[string]string{
			"completed":              "done",
			"renegotiate_signatures": "continue",
			"blocked":                "escalate",
		},
	}
}

// parseCoderBatchSignatureRequests extracts the batch_signature_requests
// array from a flow-control body. Nil when absent; error when a row is
// incomplete — a partial batch would silently drop renegotiation needs.
// Accepts both the JSON-decoded []any shape (HTTP body / tool args) and the
// typed []CoderBatchSignatureRequest (reviewOutcomeToFlowControl payload).
func parseCoderBatchSignatureRequests(body map[string]any) ([]CoderBatchSignatureRequest, error) {
	raw, ok := body["batch_signature_requests"]
	if !ok || raw == nil {
		return nil, nil
	}
	if typed, ok := raw.([]CoderBatchSignatureRequest); ok {
		for i, req := range typed {
			if req.Symbol == "" || req.File == "" || req.CurrentSignature == "" || req.ProposedSignature == "" || req.Rationale == "" {
				return nil, fmt.Errorf("submit_coder_outcome: batch_signature_requests[%d] requires symbol, file, current_signature, proposed_signature and rationale", i)
			}
		}
		return typed, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("submit_coder_outcome: batch_signature_requests must be an array")
	}
	out := make([]CoderBatchSignatureRequest, 0, len(arr))
	for i, item := range arr {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("submit_coder_outcome: batch_signature_requests[%d] must be an object", i)
		}
		req := CoderBatchSignatureRequest{
			Symbol:            strings.TrimSpace(stringFieldAny(row, "symbol")),
			File:              strings.TrimSpace(stringFieldAny(row, "file")),
			CurrentSignature:  strings.TrimSpace(stringFieldAny(row, "current_signature")),
			ProposedSignature: strings.TrimSpace(stringFieldAny(row, "proposed_signature")),
			Rationale:         strings.TrimSpace(stringFieldAny(row, "rationale")),
		}
		if req.Symbol == "" || req.File == "" || req.CurrentSignature == "" || req.ProposedSignature == "" || req.Rationale == "" {
			return nil, fmt.Errorf("submit_coder_outcome: batch_signature_requests[%d] requires symbol, file, current_signature, proposed_signature and rationale", i)
		}
		out = append(out, req)
	}
	return out, nil
}

// sortedKeys returns the map's keys in deterministic order so buffered batch
// reads are reproducible across steps.
func sortedKeys(m map[string][]CoderBatchSignatureRequest) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stringFieldAny reads a string field off a decoded JSON map.
func stringFieldAny(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// isSignatureLockedCoderChild reports whether rs is a delegate coder child
// (agent.code node) on a flow-driven parent whose frozen contract pins a
// SignatureHash — the only turn shape that can legitimately file a
// renegotiation batch (CP-67 P-1; mirrors gate_hook's coderSignaturesLocked).
func (s *InteractiveService) isSignatureLockedCoderChild(rs *interactiveRun) bool {
	if s == nil || rs == nil || rs.parentRunID == "" || !s.isFlowEngineDriven(rs.parentRunID) {
		return false
	}
	node, ok := flowNodeForRun(s, rs)
	if !ok {
		return false
	}
	canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior)
	if !ok || canonical != "agent.code" {
		return false
	}
	rec, ok := s.frozenContractForRun(rs.workspaceCwd, rs.parentRunID)
	return ok && strings.TrimSpace(rec.SignatureHash) != ""
}

// looksLikeCoderOutcome reports whether a decoded flow-control body carries
// the coder face's domain status vocabulary, so handleSubmitFlowControl can
// route it through the coder face mapping (the review face keeps priority —
// approved/changes_requested/blocked are unambiguous).
func looksLikeCoderOutcome(status string) bool {
	return coderOutcomeStatuses[status]
}

// bufferCoderBatchSignatures records a child coder's batch request set for
// the synthesis_negotiation hub. Record-only: it never mutates flow state —
// the hub reads the buffer when it mediates the renegotiation back-edge
// (CP-67 P-1; same buffer-then-mediate shape as pendingReviewVerdictByLabel).
func (s *InteractiveService) bufferCoderBatchSignatures(parentRunID string, reqs []CoderBatchSignatureRequest) {
	if s == nil || parentRunID == "" || len(reqs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parent := s.runs[parentRunID]
	if parent == nil {
		return
	}
	if parent.pendingBatchSignatureByStep == nil {
		parent.pendingBatchSignatureByStep = make(map[string][]CoderBatchSignatureRequest)
	}
	stepID := parent.stepID
	parent.pendingBatchSignatureByStep[stepID] = append(parent.pendingBatchSignatureByStep[stepID], reqs...)
}

// snapshotCoderBatchSignatures returns a copy of the buffered batch requests
// for the run — the hub's mediation read. The batch belongs to the coder's
// flow node, not the parent's volatile turn-scoped stepID: a park/resume or
// reinvoke between the coder's submit and its completion re-stamps stepID
// (run-2062497), which would orphan a batch read under only the current key.
func (s *InteractiveService) snapshotCoderBatchSignatures(parentRunID string) []CoderBatchSignatureRequest {
	if s == nil || parentRunID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parent := s.runs[parentRunID]
	if parent == nil {
		return nil
	}
	var out []CoderBatchSignatureRequest
	for _, key := range sortedKeys(parent.pendingBatchSignatureByStep) {
		out = append(out, parent.pendingBatchSignatureByStep[key]...)
	}
	return out
}

// consumeCoderBatchSignatures returns and clears every buffered batch for the
// run — the hub calls this once it has adjudicated the batch so a
// renegotiation round is never replayed. Clears all step keys for the same
// reason snapshot reads them: the step key at buffer time is not stable.
func (s *InteractiveService) consumeCoderBatchSignatures(parentRunID string) []CoderBatchSignatureRequest {
	if s == nil || parentRunID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parent := s.runs[parentRunID]
	if parent == nil {
		return nil
	}
	var out []CoderBatchSignatureRequest
	for _, key := range sortedKeys(parent.pendingBatchSignatureByStep) {
		out = append(out, parent.pendingBatchSignatureByStep[key]...)
	}
	clear(parent.pendingBatchSignatureByStep)
	return out
}

// renderNegotiationBatchPrompt renders a consumed renegotiation batch into
// the synthesis_negotiation hub prompt. The hub adjudicates every row as the
// Main Agent mediator (CP-67: coder and scaffold architect never negotiate
// peer-to-peer), then answers through submit_review_outcome:
//   - changes_requested -> its back-edge re-enters the scaffold node so the
//     architect revises the pinned signatures (the adjudicated rows belong
//     in issues/feedback so the re-entry prompt carries them);
//   - approved          -> negotiation closes, flow continues past the hub.
func renderNegotiationBatchPrompt(batch []CoderBatchSignatureRequest) string {
	var b strings.Builder
	b.WriteString("\n\n[CP-67 signature renegotiation] The coder submitted a batched renegotiation request " +
		"(status=renegotiate_signatures). Adjudicate EVERY row below — you mediate; the coder and the " +
		"scaffold architect never negotiate peer-to-peer.\n")
	for i, r := range batch {
		fmt.Fprintf(&b, "\n%d. `%s` in `%s`\n   current:   %s\n   proposed:  %s\n   rationale: %s\n",
			i+1, r.Symbol, r.File, r.CurrentSignature, r.ProposedSignature, r.Rationale)
	}
	b.WriteString("\nThen call submit_review_outcome: status=changes_requested to send the approved rows back " +
		"to the scaffold step (put the adjudicated rows in issues/feedback so the scaffold architect sees " +
		"exactly which signatures to revise), or status=approved to reject the batch and resume the flow.")
	return b.String()
}

// maybeDispatchNegotiationHubForBufferedBatch routes a buffered renegotiation
// batch to the declared negotiation hub WITHOUT waiting for the submitter's
// node to complete. The completion route in advanceOrNotifyHub deadlocks when
// the submitting leg is itself parked — live run-502144: the TDD leg
// (agent.scaffold) buffered RaspEngine.h signature rows, then parked on a
// contract-frozen write permission card; it could never complete, so the
// batch sat through three renegotiation_recorded rounds with no adjudication.
// Hub-only routing is preserved: the batch still goes to the hub.inline
// adjudicator, never peer-to-peer. With no declared negotiation hub the batch
// stays buffered for the completion route — nothing is silently dropped.
// Self-gating and idempotent: an empty buffer is a no-op, and consume-once
// means a racing second caller can never double-dispatch rows.
func (s *InteractiveService) maybeDispatchNegotiationHubForBufferedBatch(parentRunID string) {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.parentRunID != "" || !rs.flowEngineDriven {
		s.mu.Unlock()
		return
	}
	nodes := append([]agentpack.FlowNode(nil), rs.activeFlowNodes...)
	edges := append([]agentpack.FlowEdge(nil), rs.activeFlowEdges...)
	s.mu.Unlock()
	hubNode, ok := negotiationHubNodeFor(edges, nodes)
	if !ok {
		return
	}
	batch := s.consumeCoderBatchSignatures(parentRunID)
	if len(batch) == 0 {
		return
	}
	s.dispatchHubNotifyNodeWithPrompt(parentRunID, hubNode,
		composeHubNotifyPrompt(hubNode)+renderNegotiationBatchPrompt(batch))
	s.flowDiagLog(parentRunID, "flow_negotiation_hub_dispatch_on_park",
		"buffered renegotiation batch routed to negotiation hub while a leg is parked",
		"hub_node_id", hubNode.ID,
		"batch_rows", len(batch),
	)
}
