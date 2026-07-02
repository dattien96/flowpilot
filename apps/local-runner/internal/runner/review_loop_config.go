package runner

import "flowpilot-runner/internal/agentpack"

// ReviewLoopFlowConfig returns the built-in review-loop FlowDefinition fragment.
//
// It prefers the pack-defined built-in flow and falls back to the legacy
// hardcoded template only when the embedded pack cannot be loaded. The
// built-in pack uses the full review cohort topology:
// coder → reviewer_correctness + reviewer_security → synthesis → coder.
func ReviewLoopFlowConfig() ([]FlowNode, []FlowEdge, FlowPolicy) {
	if nodes, edges, policy, ok := reviewLoopFlowConfigFromPack(); ok {
		return nodes, edges, policy
	}

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

func reviewLoopFlowConfigFromPack() ([]FlowNode, []FlowEdge, FlowPolicy, bool) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		return nil, nil, FlowPolicy{}, false
	}
	for _, def := range pack.Flows {
		if def.ID != "review-loop" {
			continue
		}
		nodes := make([]FlowNode, 0, len(def.Nodes))
		for _, node := range def.Nodes {
			nodes = append(nodes, FlowNode{
				ID:        node.ID,
				Run:       node.Run,
				Lifecycle: node.Lifecycle,
				Join:      node.Join,
			})
		}
		edges := make([]FlowEdge, 0, len(def.Edges))
		for _, edge := range def.Edges {
			edges = append(edges, FlowEdge{
				From: edge.From,
				To:   edge.To,
				When: edge.When,
				Kind: edge.Kind,
			})
		}
		return nodes, edges, FlowPolicy{
			Cap:       def.Policy.Cap,
			OnCap:     def.Policy.OnCap,
			ExtendBy:  def.Policy.ExtendBy,
			ExtendMax: def.Policy.ExtendMax,
		}, true
	}
	return nil, nil, FlowPolicy{}, false
}
