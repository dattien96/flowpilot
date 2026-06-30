package runner

// ReviewLoopFlowConfig returns the canonical 3-node review-loop FlowDefinition fragment:
//
//	coder(reinvoke) → reviewer(join:all, dependsOn:coder) → synthesis(run:inline, join:all)
//	back-edge: synthesis → coder when continue, cap:3, onCap:escalate
//
// This is the first FlowDefinition template (CP-36 P-8 / Task-094). Domain node names
// live here, not in agent_orchestrator.go, so the generic coordinator remains domain-free.
func ReviewLoopFlowConfig() ([]FlowNode, []FlowEdge, FlowPolicy) {
	nodes := []FlowNode{
		{ID: "coder", Run: "delegate", Lifecycle: "reinvoke", Join: "all"},
		{ID: "reviewer", Run: "delegate", Lifecycle: "reinvoke", Join: "all"},
		{ID: "synthesis", Run: "inline", Lifecycle: "once", Join: "all"},
	}
	edges := []FlowEdge{
		{From: "coder", To: "reviewer", When: "done", Kind: "forward"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
	}
	policy := FlowPolicy{Cap: 3, OnCap: "escalate", ExtendBy: 2, ExtendMax: 2}
	return nodes, edges, policy
}
