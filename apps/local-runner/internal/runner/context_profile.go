package runner

import (
	"context"
	"fmt"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Task-341 (CP-62 P-5): per-node context profile runtime support — the
// profile's token budget rides the Task-334 Budget Packer (never a second
// packer), and the catalog tier keeps one-line metadata for pruned sections
// so the model sees WHAT exists even when bodies were dropped.

// flowNodeProfileFor resolves the ContextProfile of the flow node the run is
// currently executing (false when no profile applies). Mirrors
// flowNodePostureFor's node resolution (parent activeFlowNodes matched on
// stepID/label); the profile map itself resolves from the builtin pack by the
// parent's flow ref — hubs/roots and pack-less flows have no profile.
func (s *InteractiveService) flowNodeProfileFor(rs *interactiveRun) (agentpack.ContextProfile, bool) {
	if s == nil || rs == nil || strings.TrimSpace(rs.parentRunID) == "" {
		return agentpack.ContextProfile{}, false
	}
	s.mu.Lock()
	parent := s.runs[rs.parentRunID]
	if parent == nil || len(parent.activeFlowNodes) == 0 {
		s.mu.Unlock()
		return agentpack.ContextProfile{}, false
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
		return agentpack.ContextProfile{}, false
	}
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		return agentpack.ContextProfile{}, false
	}
	bare := workingmode.BareFlowID(flowRef)
	for i := range pack.Flows {
		if pack.Flows[i].ID != bare {
			continue
		}
		if profile, ok := pack.Flows[i].ContextProfiles[profileName]; ok {
			return profile, true
		}
	}
	return agentpack.ContextProfile{}, false
}

// flowNodeProfileBudgetFor returns the estimated prompt token budget declared
// by the node's context profile (0 = no profile budget).
func (s *InteractiveService) flowNodeProfileBudgetFor(rs *interactiveRun) int {
	profile, ok := s.flowNodeProfileFor(rs)
	if !ok {
		return 0
	}
	return profile.MaxEstPromptTokens
}

// flowContextProfilesFor returns the run's active flow definition's
// ContextProfiles map (CP-62 P-5). BUG-421: produce paths resolve the
// consuming node's profile through this — chatFlowRef first, then the
// workflowID pack ref (workflow-launched runs carry it instead). Resolution
// goes through the standard resolver so store-backed mirrors and the
// embedded pack both work; any failure returns nil and callers fall back to
// the pre-profile precedence unchanged (typed degradation, never a block).
func (s *InteractiveService) flowContextProfilesFor(ctx context.Context, parentRunID string) map[string]agentpack.ContextProfile {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	store := s.flowDefinitionStore
	var ref string
	if rs != nil {
		ref = strings.TrimSpace(rs.chatFlowRef)
		if ref == "" {
			ref = strings.TrimSpace(rs.workflowID)
		}
	}
	s.mu.Unlock()
	if ref == "" {
		return nil
	}
	rec, err := NewFlowDefinitionResolver(store).ResolveFlowRef(ctx, ref)
	if err != nil {
		return nil
	}
	return rec.Definition.ContextProfiles
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
