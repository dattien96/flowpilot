package runner

import (
	"log"
	"strings"
)

// flowContextRenderBudgetBytes is the v1 render budget for Flow context injection
// (Task-188 T-2 / CP-43 P-5). Canonical Head is mandatory; raw history and chat
// summary are lower priority and may be dropped under pressure.
const flowContextRenderBudgetBytes = 24 * 1024

// packBudgetDropOrder lists droppable sources from first sacrificed to last.
// canonical.head and change.contract are never listed here.
var packBudgetDropOrder = []ContextSourceID{
	ContextSourceFeatureHistory,
	ContextSourceChatSummary,
	ContextSourceDependence,
	ContextSourceSourceExcerpt,
	ContextSourceMCPDriver,
	ContextSourceJiraIssue,
	ContextSourceJiraSprint,
	ContextSourceFirebaseCrashlytics,
}

// applyFlowContextPackBudget trims lower-priority section bodies so rendered
// prompt stays within budget while preserving Canonical Head (and change.contract).
// Mutates pkg in place; appends a warning and logs each drop (AC-9 degrade-mềm).
func applyFlowContextPackBudget(pkg *FlowContextPackage) {
	if pkg == nil {
		return
	}
	if estimateFlowContextRenderBytes(*pkg) <= flowContextRenderBudgetBytes {
		return
	}
	for _, id := range packBudgetDropOrder {
		if estimateFlowContextRenderBytes(*pkg) <= flowContextRenderBudgetBytes {
			return
		}
		if !dropContextSourceBody(pkg, id) {
			continue
		}
		msg := "context source " + string(id) + " omitted under context budget (Canonical Head retained — Task-188 T-2)"
		pkg.Warnings = append(pkg.Warnings, msg)
		log.Printf("[context-pack] dropped %s under budget package=%s run=%s", id, pkg.PackageID, pkg.WorkflowRunID)
	}
}

func dropContextSourceBody(pkg *FlowContextPackage, id ContextSourceID) bool {
	dropped := false
	for i := range pkg.Sections {
		if ContextSourceID(pkg.Sections[i].SourceType) != id {
			continue
		}
		if strings.TrimSpace(pkg.Sections[i].Body) != "" || len(pkg.Sections[i].Excerpts) > 0 {
			pkg.Sections[i].Body = ""
			pkg.Sections[i].Excerpts = nil
			dropped = true
		}
	}
	switch id {
	case ContextSourceFeatureHistory:
		if strings.TrimSpace(pkg.HistoryBlock) != "" {
			pkg.HistoryBlock = ""
			dropped = true
		}
	case ContextSourceChatSummary:
		if strings.TrimSpace(pkg.DiscussionBlock) != "" {
			pkg.DiscussionBlock = ""
			dropped = true
		}
	case ContextSourceSourceExcerpt:
		if len(pkg.SourceExcerpts) > 0 {
			pkg.SourceExcerpts = nil
			dropped = true
		}
	}
	return dropped
}

func estimateFlowContextRenderBytes(pkg FlowContextPackage) int {
	const headerReserve = 512
	n := headerReserve + len(pkg.PackageID) + len(pkg.FeatureKey)
	for _, s := range sectionsForRender(pkg) {
		n += len(s.SourceType) + len(s.Body) + len(s.SourceRef)
		for _, ex := range s.Excerpts {
			n += len(ex.Path) + len(ex.Excerpt)
		}
	}
	for _, w := range pkg.Warnings {
		n += len(w)
	}
	for _, o := range pkg.Omitted {
		n += len(o)
	}
	return n
}
