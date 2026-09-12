package runner

import (
	"fmt"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Task-341 (CP-62 P-5): per-node context profile runtime support — the
// profile's token budget rides the Task-334 Budget Packer (never a second
// packer), and the catalog tier keeps one-line metadata for pruned sections
// so the model sees WHAT exists even when bodies were dropped.

// flowNodeProfileBudgetFor returns the max token budget declared by the
// profile of the flow node the run is currently executing (0 = no profile
// budget). Mirrors flowNodePostureFor's node resolution (parent
// activeFlowNodes matched on stepID/label); the profile map itself resolves
// from the builtin pack by the parent's flow ref — hubs/roots and
// pack-less flows return 0, keeping behavior unchanged.
func (s *InteractiveService) flowNodeProfileBudgetFor(rs *interactiveRun) int {
	if s == nil || rs == nil || strings.TrimSpace(rs.parentRunID) == "" {
		return 0
	}
	s.mu.Lock()
	parent := s.runs[rs.parentRunID]
	if parent == nil || len(parent.activeFlowNodes) == 0 {
		s.mu.Unlock()
		return 0
	}
	stepID, label := strings.TrimSpace(rs.stepID), strings.TrimSpace(rs.label)
	profileName := ""
	for _, n := range parent.activeFlowNodes {
		if n.ID == stepID || (label != "" && n.ID == label) {
			profileName = strings.TrimSpace(n.ContextProfile)
			break
		}
	}
	flowRef := parent.chatFlowRef
	s.mu.Unlock()
	if profileName == "" {
		return 0
	}
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		return 0
	}
	bare := workingmode.BareFlowID(flowRef)
	for i := range pack.Flows {
		if pack.Flows[i].ID != bare {
			continue
		}
		if profile, ok := pack.Flows[i].ContextProfiles[profileName]; ok {
			return profile.MaxTokens
		}
	}
	return 0
}

// catalogSummaryHeading marks the CP-62 P-5 catalog tier block appended to a
// packed prompt whose sections were pruned.
const catalogSummaryHeading = "### Context Catalog (pruned sections — request by title if needed)"

// buildCatalogSummary renders one line per dropped item so the model keeps a
// cheap index of what exists (progressive disclosure: metadata always, body
// on demand). One line each — never the bodies.
func buildCatalogSummary(droppedItems []string) string {
	if len(droppedItems) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(catalogSummaryHeading + "\n")
	for _, item := range droppedItems {
		line := strings.TrimSpace(item)
		if line == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", line)
	}
	return strings.TrimRight(b.String(), "\n")
}
