package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

// Built-in context source IDs (CP-44 P-2 / Task-192). These are the exact
// three sources BuildFlowContextPackage previously called directly; migrating
// them behind ContextSource must not change their observable output (see
// TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor).
const (
	ContextSourceFeatureHistory ContextSourceID = "feature.history"
	ContextSourceChatSummary    ContextSourceID = "chat.summary"
	ContextSourceSourceExcerpt  ContextSourceID = "source.excerpt"
)

// defaultContextSourceIDs is the enabled set a flow uses when it declares no
// explicit `contexts.sources` binding (Task-194 default). This exactly
// reproduces CP-41's pre-CP-44 behavior.
var defaultContextSourceIDs = []string{
	string(ContextSourceFeatureHistory),
	string(ContextSourceChatSummary),
	string(ContextSourceSourceExcerpt),
}

// ValidateFlowContextSources checks every declared context-source id — both
// `contexts.<name>.sources` (flow-level, Task-194) and each node's own
// `ContextSources` (step-definition-level, Task-196) — resolves to a
// registered ContextSource, failing fast at flow-load time rather than
// letting a flow silently run without an unknown source (CP-44 P-4/D-4,
// Task-194 T-2, Task-196 T-2/DOD-3). Mirrors the unknown-behavior-id check
// agentpack.ValidateFlowDefinition already does for node.Behavior, but lives
// in the runner package because ContextSourceRegistry does.
func ValidateFlowContextSources(def agentpack.FlowDefinition) error {
	registry := DefaultContextSourceRegistry()
	for name, binding := range def.Contexts {
		for _, id := range binding.Sources {
			if _, err := registry.Resolve(id); err != nil {
				return fmt.Errorf("flow %q context %q declares unknown source %q: %w", def.ID, name, id, err)
			}
		}
	}
	for _, node := range def.Nodes {
		for _, id := range node.ContextSources {
			if _, err := registry.Resolve(id); err != nil {
				return fmt.Errorf("flow %q node %q declares unknown context source %q: %w", def.ID, node.ID, id, err)
			}
		}
	}
	return nil
}

// resolveEnabledContextSourceIDs resolves a context-producing node's enabled
// source set by precedence: (a) CP-45/SD-23 D-6 — a bound `context_artifact`
// input artifact instance's config_json.sources, the framework's highest
// precedence tier; else (b) the node's own ContextSources (step-definition-
// level, Task-196) when set; else (c) the flow-level
// `contexts.<name>.sources` binding the node fills — found by matching one of
// the node's declared Outputs keys against def.Contexts (the same key
// convention rag-harness.yaml uses: node output "main_context" binds to
// contexts.main_context); else (d) nil, meaning "use the runner's default
// built-in set" (CP-44 P-4, Task-194 T-1/T-4). CP-45 only adds tier (a) on
// top of the pre-existing (b)/(c)/(d) chain — a node with no artifact
// binding resolves exactly as it did before CP-45 (SD-23 D-6 soft
// migration, F-4).
func resolveEnabledContextSourceIDs(def agentpack.FlowDefinition, node agentpack.FlowNode) []string {
	if ids, ok := resolveArtifactBoundContextSources(node); ok {
		return ids
	}
	if len(node.ContextSources) > 0 {
		return node.ContextSources
	}
	for outputKey := range node.Outputs {
		if binding, ok := def.Contexts[outputKey]; ok && len(binding.Sources) > 0 {
			return binding.Sources
		}
	}
	return nil
}

// registerBuiltinContextSources adds the three migrated built-in sources plus
// the mcp.driver external source to r. mcp.driver is registered (so a flow
// can validate/enable it via `contexts.<name>.sources`, Task-194) but is not
// part of defaultContextSourceIDs — it only runs when a flow opts in.
func registerBuiltinContextSources(r *ContextSourceRegistry) {
	mustRegisterContextSource(r, &featureHistorySource{priority: 2})
	mustRegisterContextSource(r, &chatSummarySource{priority: 5})
	mustRegisterContextSource(r, &sourceExcerptSource{priority: 4})
	mustRegisterContextSource(r, &mcpDriverSource{priority: 6})
}

// mustRegisterContextSource panics on a registration conflict among the
// fixed, compile-time set of built-in sources; a duplicate here is a
// programming error, not runtime input (mirrors mustRegister in
// behavior_registry_builtin.go).
func mustRegisterContextSource(r *ContextSourceRegistry, src ContextSource) {
	if err := r.Register(src); err != nil {
		panic(err)
	}
}

// featureHistorySource wraps the existing HistorySlot seam (CA + commit
// ledger, "newest = current truth"). It only loads history for a verified
// feature key — low/unresolved confidence must never inject the wrong
// feature's history (Task-168 T-1), so an unverified hint yields an empty,
// warning-free section rather than a lookup.
type featureHistorySource struct{ priority int }

func (s *featureHistorySource) ID() string          { return string(ContextSourceFeatureHistory) }
func (s *featureHistorySource) Priority() int       { return s.priority }
func (s *featureHistorySource) Deterministic() bool { return true }

func (s *featureHistorySource) Fetch(_ context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	if hints.FeatureConfidence != ConfidenceVerified || hints.FeatureKey == "" {
		return section, nil
	}
	dotFP := filepath.Join(hints.Workspace, ".flowpilot")
	section.SourceRef = dotFP + "/ledger/feature_history.ndjson"

	ledger, err := changeledger.New(dotFP)
	var history string
	if err == nil {
		history = strings.TrimSpace(featurecatalog.HistorySlot(hints.FeatureKey, ledger))
	}
	section.Body = history
	if history == "" {
		// Preserves the original warning text/condition exactly: fires whenever
		// the ledger is unavailable OR has no entries for this feature.
		section.Warnings = []string{"no change history found for feature: " + hints.FeatureKey}
	}
	return section, nil
}

// chatSummarySource wraps the existing ChatSummarySlot seam (per-feature
// long-term chat memory).
//
// Deliberate behavior change vs. the pre-CP-44 code (flagged per CP-44 P-9 —
// sources must be independent, not chained): the old loadFlowFeatureBlocks
// only loaded the chat-summary ledger when the *unrelated* regular
// changeledger.New(dotFP) call also succeeded, because both blocks shared one
// function body. That accidental coupling meant a corrupted/unreadable main
// ledger silently suppressed valid chat-summary data too. This source now
// loads independently. This is untested/unexercised in the pre-refactor
// suite (no fixture has a healthy chat-summary ledger alongside a broken main
// ledger), so it does not change any existing test's outcome, and it removes
// a cross-source dependency this CP is explicitly designed to avoid.
type chatSummarySource struct{ priority int }

func (s *chatSummarySource) ID() string          { return string(ContextSourceChatSummary) }
func (s *chatSummarySource) Priority() int       { return s.priority }
func (s *chatSummarySource) Deterministic() bool { return true }

func (s *chatSummarySource) Fetch(_ context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	if hints.FeatureConfidence != ConfidenceVerified || hints.FeatureKey == "" {
		return section, nil
	}
	dotFP := filepath.Join(hints.Workspace, ".flowpilot")
	section.SourceRef = dotFP + "/ledger/chat_summary.ndjson"

	summaryLedger, err := changeledger.NewChatSummaryLedger(dotFP)
	if err != nil {
		return section, nil
	}
	section.Body = strings.TrimSpace(featurecatalog.ChatSummarySlot(hints.FeatureKey, summaryLedger))
	return section, nil
}

// sourceExcerptSource wraps the existing readSourceExcerpts seam: explicit,
// workspace-safe source-file reads from stack-trace/changed/user-provided
// paths. It never reads outside the workspace and never follows a symlink
// escape (see readSourceExcerpts).
type sourceExcerptSource struct{ priority int }

func (s *sourceExcerptSource) ID() string          { return string(ContextSourceSourceExcerpt) }
func (s *sourceExcerptSource) Priority() int       { return s.priority }
func (s *sourceExcerptSource) Deterministic() bool { return true }

func (s *sourceExcerptSource) Fetch(_ context.Context, hints FlowContextHints) (FlowContextSection, error) {
	allPaths := fcpDedup(append(hints.ChangedPaths, hints.ExplicitSourcePaths...))
	excerpts, omitted := readSourceExcerpts(hints.Workspace, allPaths)
	return FlowContextSection{
		SourceType: s.ID(),
		Priority:   s.priority,
		Excerpts:   excerpts,
		Omitted:    omitted,
	}, nil
}
