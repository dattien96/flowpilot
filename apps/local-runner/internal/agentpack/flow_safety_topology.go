package agentpack

import (
	"fmt"
	"strings"
)

// Canonical behavior ids ValidateFlowSafetyTopology treats as CP-55 writer /
// freeze semantics. Plain strings, not the runner package's typed
// BehaviorID: agentpack cannot import runner (runner already imports
// agentpack), and every node's Behavior field is normalized through
// NormalizeBehaviorID before comparison anyway, exactly like the
// unknown-behavior check ValidateFlowDefinition already performs.
const (
	behaviorIDAgentCode      = "agent.code"
	// behaviorIDAgentScaffold (CP-67 P-3, B-3): the Contract-First Scaffold
	// TDD node is a frozen-contract writer like agent.code — its stubs land
	// inside the freeze-dominated DeclaredPaths and its red test files feed
	// the acceptance boundary, so it carries the same topology invariants.
	behaviorIDAgentScaffold  = "agent.scaffold"
	behaviorIDContractFreeze = "contract.freeze"
	terminalDoneNodeID       = "done"
)

// ValidateFlowSafetyTopology is the CP-55 P-1 static safety pass over a
// parsed FlowDefinition. A flow with no agent.code node always passes — this
// is the P-1 compatibility strategy that lets every pre-CP-55 built-in or
// user-defined flow keep loading unchanged until it is explicitly migrated
// (CP-55 P-8). Once a flow does declare an agent.code node, four invariants
// are enforced:
//
//  1. the writer is never an entry node;
//  2. some contract.freeze node dominates the writer on every forward path
//     from a real, reachable entry (no path may bypass freeze, and a writer
//     that cannot be reached from any entry at all — e.g. stranded in a
//     forward cycle disconnected from every entry — is rejected rather than
//     vacuously "dominated");
//  3. the flow declares at least one AcceptanceNodes id naming a real,
//     non-writer node — CP-55's Flow-defined acceptance boundary;
//  4. at least one forward path from the writer reaches the terminal "done"
//     state (a writer with no successor, or whose only reachable paths loop
//     forever without ever reaching "done", fails this), and every forward
//     path that does reach "done" crosses at least one declared acceptance
//     node before doing so — a single branch reaching "done" without ever
//     crossing acceptance fails this too, even if a sibling branch loops
//     forever or crosses acceptance correctly.
func ValidateFlowSafetyTopology(def FlowDefinition) error {
	writers, freezes := classifyWriterAndFreezeNodes(def)
	if len(writers) == 0 {
		return nil
	}

	nodeIDs := nodeIDSet(def)
	entries := entryNodeIDs(def)
	fwd := forwardAdjacency(def)

	for _, writer := range writers {
		if _, isEntry := entries[writer]; isEntry {
			return fmt.Errorf("flow %q: agent.code node %q cannot be an entry node", def.ID, writer)
		}

		dominated := false
		for _, freeze := range freezes {
			if forwardDominatesFrom(fwd, entries, nodeIDs, freeze, writer) {
				dominated = true
				break
			}
		}
		if !dominated {
			return fmt.Errorf("flow %q: agent.code node %q has no contract.freeze node that dominates every forward path from a reachable entry (missing entirely, the writer is unreachable from any entry, or a branch bypasses every freeze)", def.ID, writer)
		}
	}

	acceptance, err := validateAcceptanceNodes(def, nodeIDs, writers)
	if err != nil {
		return err
	}
	for _, writer := range writers {
		ok, reachedDone := everyForwardPathFromWriterCrossesAcceptance(fwd, acceptance, writer)
		if ok {
			continue
		}
		if !reachedDone {
			return fmt.Errorf("flow %q: agent.code node %q has no forward path that reaches the terminal %q state at all (no successor, or every reachable path loops forever), so it never crosses a declared acceptance_nodes entry on any path there", def.ID, writer, terminalDoneNodeID)
		}
		return fmt.Errorf("flow %q: agent.code node %q has a forward path to the terminal %q state that never crosses a declared acceptance_nodes entry", def.ID, writer, terminalDoneNodeID)
	}
	return nil
}

// ForwardDominates reports whether every forward path from any entry node
// (a node with no incoming forward edge) to targetNodeID passes through
// dominatorNodeID. Kind=="back" edges (retry/reinvoke loops) are excluded
// from the traversal, matching how ValidateFlowDefinition already treats
// back edges as a return-and-retry mechanism rather than a fresh
// entry-to-target path.
//
// Fails closed rather than vacuously "dominating" in every case the P-1
// review flagged: dominatorNodeID/targetNodeID must both name a node
// actually declared in def.Nodes, and targetNodeID must be reachable from at
// least one real entry by walking fwd without treating dominatorNodeID as a
// wall. A flow with zero entries, or a writer/freeze pair stranded in a
// forward cycle disconnected from every real entry, has an unreachable
// target under that check and therefore never dominates — the previous
// implementation returned true for exactly this shape, since its BFS seeded
// from (zero or disconnected) entries simply never visited the target
// either way, and "never visited" was wrongly read as "dominated".
func ForwardDominates(def FlowDefinition, dominatorNodeID string, targetNodeID string) bool {
	return forwardDominatesFrom(forwardAdjacency(def), entryNodeIDs(def), nodeIDSet(def), dominatorNodeID, targetNodeID)
}

// EveryDonePathIncludesAcceptance reports whether at least one forward path
// from writerNodeID's own outgoing edges reaches the terminal "done" state,
// and every forward path that does reach "done" crosses at least one of
// def's declared AcceptanceNodes first. A writer with no reachable "done" at
// all (no successor, or every reachable path loops forever) returns false,
// the same as a writer where some path reaches "done" without crossing
// acceptance first. An acceptance node visited only before writerNodeID does
// not count, since the walk begins at the writer's own successors and never
// looks upstream. This function trusts
// def.AcceptanceNodes as given — it does not itself verify that every entry
// names a declared, non-writer node; ValidateFlowSafetyTopology enforces
// that separately (validateAcceptanceNodes) before relying on this check.
func EveryDonePathIncludesAcceptance(def FlowDefinition, writerNodeID string) bool {
	acceptance := make(map[string]struct{}, len(def.AcceptanceNodes))
	for _, id := range def.AcceptanceNodes {
		acceptance[id] = struct{}{}
	}
	ok, _ := everyForwardPathFromWriterCrossesAcceptance(forwardAdjacency(def), acceptance, writerNodeID)
	return ok
}

func classifyWriterAndFreezeNodes(def FlowDefinition) (writers, freezes []string) {
	for _, node := range def.Nodes {
		canonical, ok := NormalizeBehaviorID(node.Behavior)
		if !ok {
			continue
		}
		switch canonical {
		case behaviorIDAgentCode, behaviorIDAgentScaffold:
			writers = append(writers, node.ID)
		case behaviorIDContractFreeze:
			freezes = append(freezes, node.ID)
		}
	}
	return writers, freezes
}

func nodeIDSet(def FlowDefinition) map[string]struct{} {
	ids := make(map[string]struct{}, len(def.Nodes))
	for _, node := range def.Nodes {
		ids[node.ID] = struct{}{}
	}
	return ids
}

func entryNodeIDs(def FlowDefinition) map[string]struct{} {
	hasIncomingForward := make(map[string]struct{}, len(def.Nodes))
	for _, edge := range def.Edges {
		if isBackEdge(edge) {
			continue
		}
		hasIncomingForward[edge.To] = struct{}{}
	}
	entries := make(map[string]struct{})
	for _, node := range def.Nodes {
		if _, ok := hasIncomingForward[node.ID]; !ok {
			entries[node.ID] = struct{}{}
		}
	}
	return entries
}

func forwardAdjacency(def FlowDefinition) map[string][]string {
	adj := make(map[string][]string, len(def.Nodes))
	for _, edge := range def.Edges {
		if isBackEdge(edge) {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	return adj
}

func isBackEdge(edge FlowEdge) bool {
	return strings.EqualFold(strings.TrimSpace(edge.Kind), "back")
}

// forwardDominatesFrom reports whether dominatorNodeID sits on every forward
// path from a real, reachable entry to targetNodeID. See ForwardDominates
// for the fail-closed contract this implements: both ids must be declared
// (present in nodeIDs), and targetNodeID must first be proven reachable from
// some entry — walking fwd with no wall at all — before any dominance
// verdict is returned. Only once that reachability is proved does the
// function re-walk fwd with dominatorNodeID treated as an impassable wall;
// if that walk still reaches targetNodeID, some path bypasses the dominator.
// The visited set in both walks bounds them regardless of any (non-back-kind)
// cycle in fwd.
func forwardDominatesFrom(fwd map[string][]string, entries map[string]struct{}, nodeIDs map[string]struct{}, dominatorNodeID, targetNodeID string) bool {
	if _, ok := nodeIDs[dominatorNodeID]; !ok {
		return false
	}
	if _, ok := nodeIDs[targetNodeID]; !ok {
		return false
	}
	if !reachableFromAnyEntry(fwd, entries, targetNodeID) {
		return false
	}
	if dominatorNodeID == targetNodeID {
		return true
	}

	visited := map[string]bool{dominatorNodeID: true}
	queue := make([]string, 0, len(entries))
	for entry := range entries {
		if visited[entry] {
			continue
		}
		visited[entry] = true
		queue = append(queue, entry)
	}
	for i := 0; i < len(queue); i++ {
		node := queue[i]
		if node == targetNodeID {
			return false
		}
		for _, next := range fwd[node] {
			if visited[next] {
				continue
			}
			visited[next] = true
			queue = append(queue, next)
		}
	}
	return true
}

// reachableFromAnyEntry reports whether targetNodeID can be reached by
// walking fwd forward from some node in entries, with no node treated as a
// wall. forwardDominatesFrom calls this before its own wall-BFS so that a
// target with zero real entries to reach it from (an empty entries set, or a
// writer/freeze pair stranded in a forward cycle disconnected from every
// real entry) fails closed instead of being read as vacuously "dominated".
func reachableFromAnyEntry(fwd map[string][]string, entries map[string]struct{}, targetNodeID string) bool {
	visited := make(map[string]bool, len(entries))
	queue := make([]string, 0, len(entries))
	for entry := range entries {
		if visited[entry] {
			continue
		}
		visited[entry] = true
		queue = append(queue, entry)
	}
	for i := 0; i < len(queue); i++ {
		node := queue[i]
		if node == targetNodeID {
			return true
		}
		for _, next := range fwd[node] {
			if visited[next] {
				continue
			}
			visited[next] = true
			queue = append(queue, next)
		}
	}
	return false
}

// validateAcceptanceNodes enforces CP-55's Flow-defined acceptance boundary
// at the flow level: a flow with any agent.code writer must declare at least
// one AcceptanceNodes entry, and every declared entry must name a real,
// non-writer node (the acceptance boundary is a distinct step the writer's
// own output must still pass through — not the writer declaring itself
// accepted). Blank entries and duplicates are rejected deterministically
// rather than silently trimmed/deduplicated, since either shape signals a
// malformed `acceptance_nodes` declaration the author should fix, not one the
// validator should quietly paper over. Returns the validated set keyed by
// node id for the traversal check to consult.
func validateAcceptanceNodes(def FlowDefinition, nodeIDs map[string]struct{}, writers []string) (map[string]struct{}, error) {
	if len(def.AcceptanceNodes) == 0 {
		return nil, fmt.Errorf("flow %q: declares an agent.code node, so acceptance_nodes must declare at least one acceptance node", def.ID)
	}
	writerSet := make(map[string]struct{}, len(writers))
	for _, w := range writers {
		writerSet[w] = struct{}{}
	}
	acceptance := make(map[string]struct{}, len(def.AcceptanceNodes))
	for _, id := range def.AcceptanceNodes {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("flow %q: acceptance_nodes contains a blank entry", def.ID)
		}
		if _, ok := nodeIDs[id]; !ok {
			return nil, fmt.Errorf("flow %q: acceptance_nodes entry %q does not name a node declared in this flow", def.ID, id)
		}
		if _, ok := writerSet[id]; ok {
			return nil, fmt.Errorf("flow %q: acceptance_nodes entry %q names an agent.code writer node, which cannot be its own acceptance step", def.ID, id)
		}
		if _, dup := acceptance[id]; dup {
			return nil, fmt.Errorf("flow %q: acceptance_nodes entry %q is declared more than once", def.ID, id)
		}
		acceptance[id] = struct{}{}
	}
	return acceptance, nil
}

// everyForwardPathFromWriterCrossesAcceptance walks every forward path
// starting at writerNodeID's own outgoing edges, carrying whether the path
// has already passed a node in acceptance, and rejects any path that reaches
// the terminal "done" state before crossing one. An acceptance node reached
// only upstream of the writer does not count, because the walk never starts
// before writerNodeID's successors. State is (node, hasCrossedAcceptance)
// rather than just node: the same node can be reached once having already
// crossed acceptance on one branch and once not having crossed it on
// another, so both must be explored independently for a mixed-branch path to
// done to be caught. That pair also bounds the walk (at most 2*len(fwd)
// states), so a forward-declared cycle downstream of the writer still
// terminates exactly like the dominance walk above.
//
// A writer whose reachable states never include "done" at all — no
// successor, or every reachable path loops forever without ever reaching
// done — is also rejected: reachedDone tracks whether "done" was visited by
// ANY explored state, and the function returns that flag (as its second
// result) rather than defaulting to true when the walk simply runs out of
// states to explore. Without this, a writer with zero successors, or one
// whose only paths are a cycle that never reaches done, would vacuously
// "pass" for the same reason forwardDominatesFrom used to vacuously
// "dominate" an unreachable target: never finding a violation was wrongly
// read as proof of safety.
//
// The two results let a caller distinguish *why* ok is false — reachedDone
// is false when no path reaches "done" at all (nothing to report crossing
// acceptance on), and true when some path did reach "done" but at least one
// such path bypassed acceptance first. Conflating the two into a single bool
// previously left ValidateFlowSafetyTopology unable to tell them apart, so
// its error message claimed a bypassing path existed even when reachedDone
// was false and no path to "done" existed at all.
func everyForwardPathFromWriterCrossesAcceptance(fwd map[string][]string, acceptance map[string]struct{}, writerNodeID string) (ok bool, reachedDone bool) {
	type step struct {
		node    string
		crossed bool
	}
	visited := make(map[step]bool)
	queue := make([]step, 0, len(fwd[writerNodeID]))
	for _, next := range fwd[writerNodeID] {
		s := step{node: next, crossed: isAcceptanceNode(acceptance, next)}
		if !visited[s] {
			visited[s] = true
			queue = append(queue, s)
		}
	}
	for i := 0; i < len(queue); i++ {
		cur := queue[i]
		if cur.node == terminalDoneNodeID {
			if !cur.crossed {
				return false, true
			}
			reachedDone = true
			continue
		}
		for _, next := range fwd[cur.node] {
			s := step{node: next, crossed: cur.crossed || isAcceptanceNode(acceptance, next)}
			if !visited[s] {
				visited[s] = true
				queue = append(queue, s)
			}
		}
	}
	return reachedDone, reachedDone
}

func isAcceptanceNode(acceptance map[string]struct{}, id string) bool {
	_, ok := acceptance[id]
	return ok
}
