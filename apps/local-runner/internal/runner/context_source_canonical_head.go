package runner

import (
	"context"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/changecontract"
)

// ContextSourceCanonicalHead — Task-244 (CP-50 P-1, CP-43 P-5, CP-44 DOD-7).
const ContextSourceCanonicalHead ContextSourceID = "canonical.head"

type canonicalHeadSource struct{ priority int }

func (s *canonicalHeadSource) ID() string          { return string(ContextSourceCanonicalHead) }
func (s *canonicalHeadSource) Priority() int       { return s.priority }
func (s *canonicalHeadSource) Deterministic() bool { return true }

func (s *canonicalHeadSource) Fetch(_ context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	// Gate giống featureHistorySource: chỉ tra khi feature verified —
	// hint không chắc chắn không bao giờ được inject Head của feature khác.
	if hints.FeatureConfidence != ConfidenceVerified || hints.FeatureKey == "" {
		return section, nil
	}
	head, found, err := changecontract.LoadHead(hints.Workspace, hints.FeatureKey)
	if err != nil || !found {
		return section, nil // degrade AC-9: không error, không warning
	}
	// G-8 fix: filepath.ToSlash cho SourceRef nhất quán trên Windows.
	section.SourceRef = filepath.ToSlash(filepath.Join(hints.Workspace, ".flowpilot", "canonical", hints.FeatureKey+".json"))
	section.Body = strings.TrimSpace(changecontract.RenderHeadBlock(head))
	return section, nil
}
