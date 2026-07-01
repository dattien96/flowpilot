package runner

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// domainHardcodeGuardFiles are the "active execution path" files CP-42/
// Task-180 requires to stay free of *new* semantic role/step-name hardcodes.
var domainHardcodeGuardFiles = []string{
	"interactive_service.go",
	"agent_catalog.go",
	"review_loop_config.go",
	"flow_context_handoff.go",
	"agent_orchestrator.go",
}

// domainHardcodeWords are the semantic strings CP-42 P-1/Task-180 T-1 flags:
// role names and step-type names that a generic flow engine must not branch
// on directly.
var domainHardcodeWords = []string{
	"reviewer", "coder", "synthesizer",
	"plan", "planning", "design",
	"coding", "implementation", "code",
	"review-loop",
}

// domainHardcodeBaseline is the audited count of quoted string-literal
// occurrences of each forbidden word, per file, as of CA-152/Task-180. This
// is a frozen inventory, not an endorsement of the remaining hits:
//
//   - flow_context_handoff.go / agent_orchestrator.go: 0. These are clean —
//     flow_context_handoff.go's isPlanStepType/isCodingStepType route
//     through agentpack.NormalizeBehaviorID (CA-147) instead of literal
//     string comparisons.
//   - agent_catalog.go (6): the legacy Go built-in agent literals
//     (coder/reviewer/synthesizer name+role), kept as the documented
//     emergency fallback per Task-174 T-3 until pack-backed loading is
//     proven stable enough to remove them.
//   - review_loop_config.go (7): the legacy hardcoded review-loop topology,
//     used only when the embedded pack fails to load, plus the
//     `def.ID != "review-loop"` pack-flow-ID match used to select which
//     parsed pack flow to convert to the legacy FlowNode/FlowEdge shape.
//   - interactive_service.go (1): the four separate `isAgentRole(rs, "coder")`
//     call sites were consolidated into the single named `isCoderRun` helper
//     (Task-180 T-3: isolate an unavoidable compatibility check into one
//     place). As of Task-180's edge-driven follow-up, `isCoderRun` is now
//     only the *fallback* path in `maybeReinvokeCoderForContinue`: a run
//     started from a resolved flowRef tracks the flow's edges
//     (`interactiveRun.activeFlowEdges`) and resolves the "continue" reinvoke
//     target from the synthesis→coder back-edge's `To` node id, matched
//     against a spawned child's `label` — never consulting `isCoderRun` at
//     all in that case. The literal role-name check remains only for runs
//     with no tracked flow edges (the AI-driven `spawn_agent` path, which
//     never resolves a flowRef and so has no edge data to read). See
//     CA-158/the CP-42 progress notes for the entry-node spawn this builds on.
//
// A change in any count here must come with an updated baseline AND a
// change-audit note explaining why (new legacy shim vs. hardcode regression).
var domainHardcodeBaseline = map[string]int{
	"interactive_service.go":  1,
	"agent_catalog.go":        6,
	"review_loop_config.go":   7,
	"flow_context_handoff.go": 0,
	"agent_orchestrator.go":   0,
}

// TestDomainHardcodeGuardMatchesFrozenBaseline fails if a new semantic
// role/step-name string literal appears in one of the generic-executor files
// beyond the audited baseline, without requiring every existing hardcode to
// already be removed (Task-180 T-5: "regression guard test that scans runner
// execution files for forbidden semantic branching patterns").
func TestDomainHardcodeGuardMatchesFrozenBaseline(t *testing.T) {
	for _, file := range domainHardcodeGuardFiles {
		count, err := countDomainHardcodeHits(file)
		if err != nil {
			t.Fatalf("scan %s: %v", file, err)
		}
		want, ok := domainHardcodeBaseline[file]
		if !ok {
			t.Fatalf("no baseline recorded for %s; add one and document why in domainHardcodeBaseline", file)
		}
		if count != want {
			t.Errorf("%s: found %d domain-literal hits, baseline expects %d — "+
				"a semantic hardcode was added or removed without updating "+
				"domainHardcodeBaseline and its change-audit note", file, count, want)
		}
	}
}

func countDomainHardcodeHits(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		for _, w := range domainHardcodeWords {
			if strings.Contains(line, `"`+w+`"`) {
				count++
			}
		}
	}
	return count, scanner.Err()
}
