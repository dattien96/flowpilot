package runner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/featurecatalog"
)

// FlowContextConfidence indicates how reliably the feature key was resolved.
type FlowContextConfidence string

const (
	ConfidenceVerified   FlowContextConfidence = "verified"
	ConfidenceLow        FlowContextConfidence = "low"
	ConfidenceUnresolved FlowContextConfidence = "unresolved"
)

// perFileExcerptBytes caps each source file excerpt; totalExcerptBytes caps
// the aggregate across all files in one package build.
const (
	perFileExcerptBytes = 4 * 1024  // 4 KB
	totalExcerptBytes   = 16 * 1024 // 16 KB
)

// FlowContextExcerpt is a bounded, workspace-safe excerpt from one source file.
type FlowContextExcerpt struct {
	Path     string `json:"path"`
	Excerpt  string `json:"excerpt"`
	BytesCap bool   `json:"bytesCap,omitempty"` // true when truncated by per-file cap
}

// FlowContextHints is the input to BuildFlowContextPackage provided by the
// Plan step of a Flow Mode run.
type FlowContextHints struct {
	WorkflowRunID       string
	PlanStepRunID       string
	UserPrompt          string
	SourceDocID         string   // e.g. "Task-168"
	ChangedPaths        []string // from stack trace / diff
	ExplicitSourcePaths []string // user-provided or catalog-glob

	// Workspace, FeatureKey, and FeatureConfidence are populated internally by
	// BuildFlowContextPackage (feature resolution runs as a pre-processing step,
	// not a ContextSource — CP-44 P-2/Task-192 T-1) before Collect is called, so
	// every ContextSource.Fetch call can look up feature-scoped data without
	// re-resolving the feature key itself. Callers of BuildFlowContextPackage do
	// not need to set these — they are overwritten by the builder.
	Workspace         string
	FeatureKey        string
	FeatureConfidence FlowContextConfidence

	// MCPDriverRef optionally names a driver reference for the mcp.driver
	// context source (CP-44 P-5/Task-195), usually resolved at runtime for
	// this run (with a legacy saved default still allowed). Empty means no
	// MCP-backed source is configured for this run; the mcp.driver source
	// degrades to an empty, warning-free section.
	MCPDriverRef string

	// JiraIssueRef/JiraSprintRef mirror MCPDriverRef's shape for the jira.issue
	// / jira.sprint context sources (Task-229): a runtime-resolved target
	// (issue key / sprint id, or "active" for the connected board's current
	// sprint), empty when that source isn't enabled or the run-time question
	// was skipped.
	JiraIssueRef  string
	JiraSprintRef string

	// FirebaseCrashRef mirrors JiraIssueRef for firebase.crashlytics
	// (Task-231): a runtime-resolved Crashlytics crash issue id.
	FirebaseCrashRef string
}

// FlowContextPackage is the deterministic context package assembled by the Plan
// step. It carries all grounding information downstream steps need without
// broad re-retrieval. No vector DB, embedding index, or similarity search is
// used at any stage of its construction (CP-41 P-2/P-3).
//
// Sections holds the raw output of every enabled ContextSource (CP-44 P-3 /
// Task-193) — this is the extensible source of truth. HistoryBlock/
// DiscussionBlock/SourceExcerpts/Omitted are a backward-compatibility
// projection of the three built-in sections (feature.history/chat.summary/
// source.excerpt) onto the pre-CP-44 fixed fields, kept so existing consumers
// (RenderFlowContextPackage's known-section rendering, BuildAuditDraft) do
// not need to change. A newly registered source shows up in Sections and in
// the render's generic section output without any struct change here.
type FlowContextPackage struct {
	PackageID         string                `json:"packageId"`
	WorkflowRunID     string                `json:"workflowRunId"`
	PlanStepRunID     string                `json:"planStepRunId"`
	FeatureKey        string                `json:"featureKey,omitempty"`
	FeatureConfidence FlowContextConfidence `json:"featureConfidence"`
	SourceDocIDs      []string              `json:"sourceDocIds,omitempty"`
	HistoryBlock      string                `json:"historyBlock,omitempty"`
	DiscussionBlock   string                `json:"discussionBlock,omitempty"`
	SourceExcerpts    []FlowContextExcerpt  `json:"sourceExcerpts,omitempty"`
	Sections          []FlowContextSection  `json:"sections,omitempty"`
	Constraints       []string              `json:"constraints,omitempty"`
	Warnings          []string              `json:"warnings,omitempty"`
	Omitted           []string              `json:"omitted,omitempty"`
	BuiltAt           string                `json:"builtAt"`
}

// BuildFlowContextPackage assembles a deterministic FlowContextPackage for the
// Plan step of a Flow Mode run. It is a thin context.Background() wrapper
// around buildFlowContextPackage, kept for the ~30 existing call sites
// (mostly tests) that predate context propagation. Production code that has a
// live context (currently behaviorContextProduce) should call
// BuildFlowContextPackageCtx directly so a future context-aware source can
// observe caller cancellation/timeout.
func BuildFlowContextPackage(workspace string, hints FlowContextHints) (FlowContextPackage, error) {
	return buildFlowContextPackage(context.Background(), workspace, hints, nil)
}

// BuildFlowContextPackageCtx is BuildFlowContextPackage with an explicit
// context, for callers (currently behaviorContextProduce) that have a live
// ctx to propagate so a future context-aware source can observe caller
// cancellation/timeout.
func BuildFlowContextPackageCtx(ctx context.Context, workspace string, hints FlowContextHints) (FlowContextPackage, error) {
	return buildFlowContextPackage(ctx, workspace, hints, nil)
}

// BuildFlowContextPackageWithSources is BuildFlowContextPackageCtx with an
// explicit enabled-source-ID list, used when the active flow declares a
// `contexts.<name>.sources` binding (CP-44 P-4 / Task-194). A nil or empty
// sourceIDs falls back to the default built-in set, identical to
// BuildFlowContextPackageCtx.
func BuildFlowContextPackageWithSources(ctx context.Context, workspace string, hints FlowContextHints, sourceIDs []string) (FlowContextPackage, error) {
	return buildFlowContextPackage(ctx, workspace, hints, sourceIDs)
}

// buildFlowContextPackage assembles a deterministic FlowContextPackage for
// the Plan step of a Flow Mode run. It resolves the feature key from the user
// prompt (a pre-processing step, not a ContextSource — CP-44 P-2/Task-192
// T-1, since every source needs the resolved feature key), then runs the
// enabled ContextSourceRegistry sources and projects their sections onto the
// package's legacy fields (CP-44 P-3/Task-193 introduces the typed Sections
// slice on top of this without changing this projection).
//
// enabledSourceIDs selects which registered sources run; nil/empty uses
// defaultContextSourceIDs, reproducing the pre-CP-44 3-step hardcoded
// retrieval exactly (CP-44 P-4/Task-194 T-1/T-3).
//
// A missing catalog, unresolvable feature key, or absent ledger files produce
// a package with Warnings instead of an error so a Flow Mode run degrades
// gracefully (CP-41 P-2, Task-168 T-4).
func buildFlowContextPackage(ctx context.Context, workspace string, hints FlowContextHints, enabledSourceIDs []string) (FlowContextPackage, error) {
	pkg := FlowContextPackage{
		WorkflowRunID: hints.WorkflowRunID,
		PlanStepRunID: hints.PlanStepRunID,
		BuiltAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if hints.SourceDocID != "" {
		pkg.SourceDocIDs = []string{hints.SourceDocID}
	}

	// Step 1: resolve feature key via the existing deterministic resolver —
	// no vector search, no model call. This stays inline (not a ContextSource)
	// because every other source needs the resolved key/confidence as input.
	dotFP := filepath.Join(workspace, ".flowpilot")
	catalog, err := featurecatalog.LoadCatalog(dotFP)
	if err != nil {
		pkg.FeatureConfidence = ConfidenceUnresolved
		pkg.Warnings = append(pkg.Warnings, "feature catalog unavailable: "+err.Error())
	} else {
		pkg.FeatureKey, pkg.FeatureConfidence = resolvePackageFeature(hints.UserPrompt, catalog, &pkg.Warnings)
	}

	// Step 2: collect the enabled context sources (CP-44 P-2/P-4).
	ids := enabledSourceIDs
	if len(ids) == 0 {
		ids = defaultContextSourceIDs
	}
	enrichedHints := hints
	enrichedHints.Workspace = workspace
	enrichedHints.FeatureKey = pkg.FeatureKey
	enrichedHints.FeatureConfidence = pkg.FeatureConfidence

	sections, sourceWarnings := DefaultContextSourceRegistry().Collect(ctx, ids, enrichedHints)
	pkg.Sections = sections
	pkg.Warnings = append(pkg.Warnings, sourceWarnings...)
	projectContextSections(&pkg, sections)

	// Step 3: deterministic package ID.
	pkg.PackageID = fcpPackageID(hints.WorkflowRunID, hints.PlanStepRunID, pkg.FeatureKey)
	return pkg, nil
}

// projectContextSections maps registered-source output onto FlowContextPackage's
// legacy fields (HistoryBlock/DiscussionBlock/SourceExcerpts/Omitted) so
// existing consumers (RenderFlowContextPackage/Task-169, BuildAuditDraft/
// Task-171) keep working unchanged (CP-44 P-3 backward-compat projection).
func projectContextSections(pkg *FlowContextPackage, sections []FlowContextSection) {
	for _, section := range sections {
		switch ContextSourceID(section.SourceType) {
		case ContextSourceFeatureHistory:
			pkg.HistoryBlock = section.Body
		case ContextSourceChatSummary:
			pkg.DiscussionBlock = section.Body
		case ContextSourceSourceExcerpt:
			pkg.SourceExcerpts = section.Excerpts
		}
		// Omitted is collected across every source (not just source.excerpt) so
		// a future source's skipped content is still surfaced (SD-22 D-7).
		pkg.Omitted = append(pkg.Omitted, section.Omitted...)
	}
}

// resolvePackageFeature resolves the feature key from a user prompt. Returns
// key + confidence and appends a warning when resolution is low/unresolved.
func resolvePackageFeature(prompt string, catalog *featurecatalog.Catalog, warnings *[]string) (string, FlowContextConfidence) {
	candidates, err := featurecatalog.ResolveFeature(strings.TrimSpace(prompt), catalog)
	if err != nil || len(candidates) == 0 {
		*warnings = append(*warnings, "feature key unresolved for prompt")
		return "", ConfidenceUnresolved
	}
	top, ok := featurecatalog.TopCandidate(candidates, 5.0)
	if !ok {
		key := candidates[0].Key
		*warnings = append(*warnings, fmt.Sprintf("feature key low-confidence: %s (score %.1f)", key, candidates[0].Score))
		return key, ConfidenceLow
	}
	return top.Key, ConfidenceVerified
}

// readSourceExcerpts reads workspace-safe source file excerpts, capping each
// file at perFileExcerptBytes and the total at totalExcerptBytes.
// Returns collected excerpts and omission reasons for skipped files.
func readSourceExcerpts(workspace string, paths []string) (excerpts []FlowContextExcerpt, omitted []string) {
	cleanWS := filepath.Clean(workspace) + string(os.PathSeparator)
	// Resolve the workspace root through symlinks once so that the per-file
	// symlink check below compares resolved paths against the resolved root.
	resolvedWS := cleanWS
	if rws, err := filepath.EvalSymlinks(filepath.Clean(workspace)); err == nil {
		resolvedWS = rws + string(os.PathSeparator)
	}
	total := 0
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(p) {
			abs = filepath.Join(workspace, p)
		}
		clean, err := filepath.Abs(abs)
		if err != nil || !strings.HasPrefix(clean, cleanWS) {
			omitted = append(omitted, p+": outside_workspace")
			continue
		}
		// Resolve symlinks so a symlink inside the workspace that points outside
		// is caught before the file is opened (MEDIUM finding: symlink escape).
		resolved, err := filepath.EvalSymlinks(clean)
		if err != nil {
			omitted = append(omitted, p+": symlink_resolve_error")
			continue
		}
		if !strings.HasPrefix(resolved+string(os.PathSeparator), resolvedWS) {
			omitted = append(omitted, p+": outside_workspace")
			continue
		}
		if total >= totalExcerptBytes {
			omitted = append(omitted, p+": total_cap_reached")
			continue
		}
		limit := perFileExcerptBytes
		if rem := totalExcerptBytes - total; rem < limit {
			limit = rem
		}
		f, err := os.Open(clean)
		if err != nil {
			omitted = append(omitted, p+": not_found")
			continue
		}
		buf := make([]byte, limit+1)
		n, readErr := io.ReadFull(f, buf)
		f.Close()
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			omitted = append(omitted, p+": read_error")
			continue
		}
		capped := n > limit
		if capped {
			n = limit
		}
		text := string(buf[:n])
		if strings.ContainsRune(text, '\x00') {
			omitted = append(omitted, p+": binary")
			continue
		}
		total += n
		excerpts = append(excerpts, FlowContextExcerpt{Path: p, Excerpt: text, BytesCap: capped})
	}
	return excerpts, omitted
}

// RenderFlowContextPackage renders the package as a stable Markdown block
// suitable for injection into Flow Mode Coding prompts (Task-169).
// The output always contains "No vector retrieval used" for audit visibility.
func RenderFlowContextPackage(pkg FlowContextPackage) string {
	var sb strings.Builder
	sb.WriteString("## Flow Context Package\n\n")
	sb.WriteString(fmt.Sprintf("- **Package ID**: %s\n", pkg.PackageID))
	featureLabel := pkg.FeatureKey
	if featureLabel == "" {
		featureLabel = "unresolved"
	}
	sb.WriteString(fmt.Sprintf("- **Feature**: %s (confidence: %s)\n", featureLabel, pkg.FeatureConfidence))
	if len(pkg.SourceDocIDs) > 0 {
		sb.WriteString(fmt.Sprintf("- **Source doc**: %s\n", strings.Join(pkg.SourceDocIDs, ", ")))
	}
	sb.WriteString("- **No vector retrieval used**\n")

	// Task-244 (SD-21 D-3): Canonical Head leads — current truth before raw history.
	// Body already carries "## Canonical state …"; write verbatim and skip generic pass.
	for _, s := range pkg.Sections {
		if ContextSourceID(s.SourceType) == ContextSourceCanonicalHead && strings.TrimSpace(s.Body) != "" {
			sb.WriteString("\n" + strings.TrimSpace(s.Body) + "\n")
		}
	}

	if pkg.HistoryBlock != "" {
		sb.WriteString("\n### Change History\n\n")
		sb.WriteString(pkg.HistoryBlock)
		sb.WriteString("\n")
	}
	if pkg.DiscussionBlock != "" {
		sb.WriteString("\n### Prior Discussion\n\n")
		sb.WriteString(pkg.DiscussionBlock)
		sb.WriteString("\n")
	}
	for _, ex := range pkg.SourceExcerpts {
		sb.WriteString(fmt.Sprintf("\n### Source: %s\n\n```\n%s\n```\n", ex.Path, ex.Excerpt))
		if ex.BytesCap {
			sb.WriteString("_(excerpt truncated)_\n")
		}
	}
	renderGenericSections(&sb, pkg.Sections)
	if len(pkg.Warnings) > 0 {
		sb.WriteString("\n### Warnings\n\n")
		for _, w := range pkg.Warnings {
			sb.WriteString("- " + w + "\n")
		}
	}
	if len(pkg.Omitted) > 0 {
		sb.WriteString("\n### Omitted Sources\n\n")
		for _, o := range pkg.Omitted {
			sb.WriteString("- " + o + "\n")
		}
	}
	return sb.String()
}

// renderGenericSections renders any section whose SourceType is not one of
// the three built-in types already rendered above by name (feature.history/
// chat.summary/source.excerpt) — so a newly registered ContextSource (CP-44
// P-3/P-4, e.g. a future MCP-backed or Canonical Head source) appears in the
// prompt without RenderFlowContextPackage needing a code change per source.
// Sections with empty Body are skipped (nothing to show); Omitted/Warnings on
// a section are still surfaced via the caller's existing Warnings/Omitted
// blocks (projectContextSections merges them onto the package).
func renderGenericSections(sb *strings.Builder, sections []FlowContextSection) {
	for _, s := range sections {
		switch ContextSourceID(s.SourceType) {
		case ContextSourceFeatureHistory, ContextSourceChatSummary, ContextSourceSourceExcerpt, ContextSourceCanonicalHead:
			continue // already rendered by name / head-first pass above
		}
		if strings.TrimSpace(s.Body) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n### %s\n\n", s.SourceType))
		if s.SourceRef != "" {
			sb.WriteString(fmt.Sprintf("_Source: %s_\n\n", s.SourceRef))
		}
		sb.WriteString(s.Body)
		sb.WriteString("\n")
	}
}

// PersistFlowContextPackage emits an EventFlowContextPackage event so the
// package is durable in workflow_provider_events, keyed by WorkflowRunID +
// PlanStepRunID, and survives a runner restart without a new table (Task-168 T-6).
func PersistFlowContextPackage(ctx context.Context, store InteractiveStateStore, pkg FlowContextPackage) error {
	return store.AppendEvent(ctx, ProviderEvent{
		Type:               EventFlowContextPackage,
		WorkflowRunID:      pkg.WorkflowRunID,
		WorkflowStepRunID:  pkg.PlanStepRunID,
		FlowContextPackage: &pkg,
	})
}

// fcpPackageID returns a short deterministic ID for the context package.
func fcpPackageID(runID, stepID, featureKey string) string {
	h := sha256.Sum256([]byte(runID + "|" + stepID + "|" + featureKey))
	return fmt.Sprintf("fcp-%x", h[:4])
}

// fcpDedup returns paths with duplicates removed, preserving order.
func fcpDedup(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}
