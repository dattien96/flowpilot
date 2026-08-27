package runner

import (
	"context"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/changecontract"
)

// ContextSourceChangeContract — Task-247 (CP-50 P-4).
const ContextSourceChangeContract ContextSourceID = "change.contract"

type changeContractSource struct{ priority int }

func (s *changeContractSource) ID() string          { return string(ContextSourceChangeContract) }
func (s *changeContractSource) Priority() int       { return s.priority }
func (s *changeContractSource) Deterministic() bool { return true }

func (s *changeContractSource) Fetch(_ context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	if strings.TrimSpace(hints.WorkflowRunID) == "" || hints.Workspace == "" {
		return section, nil
	}
	c, ok := latestContractForRun(hints.Workspace, hints.WorkflowRunID)
	if !ok {
		return section, nil
	}
	body := changecontract.RenderContractBlock(c)
	if body == "" {
		return section, nil
	}
	section.SourceRef = filepath.ToSlash(filepath.Join(hints.Workspace, ".flowpilot", "contracts", "contracts.ndjson"))
	section.Body = body
	return section, nil
}
