package runner

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

	// ResolvedFeatureKey lets a caller that already knows the feature key with
	// certainty skip step 1's NL-based resolution entirely (CP-55 P-8): a
	// Flow's first coder context build, right after a contract has just been
	// frozen, knows its FrozenContractRecord.FeatureKey directly — re-deriving
	// it via fuzzy matching against UserPrompt (which for this call is only
	// the frozen contract's one-sentence Intent, not necessarily anything the
	// feature catalog resolves confidently) would be strictly less reliable
	// than the value already in hand, and — critically — a low/unresolved
	// confidence from that fuzzy match would make every feature-scoped
	// ContextSource (feature.history, chat.summary) skip entirely (their own
	// `hints.FeatureConfidence != ConfidenceVerified` guard), silently
	// degrading feature-history ranking back to nothing on exactly the turn
	// P-8's own guarantee needs it to work. Empty (the default for every
	// other caller) preserves the existing NL-resolution behavior unchanged.
	ResolvedFeatureKey string

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
	// CP-55 P-8: a caller-supplied ResolvedFeatureKey skips the fuzzy NL
	// resolution below entirely — see its own doc comment on FlowContextHints.
	//
	// CORRECTION (CP-55 P-8 Claude-agent review, Important Finding 3):
	// ResolvedFeatureKey used to be marked ConfidenceVerified unconditionally,
	// with no cross-check against the feature catalog — runContractFreezeNode
	// deliberately does not allowlist the planner's own feature_key either
	// ("trusted for now"), so a planner LLM that hallucinated or mis-selected
	// a feature_key colliding with a real, but wrong, existing feature would
	// silently and confidently route feature.history/chat.summary to the
	// wrong feature, with no warning anywhere in the pipeline. Rejecting an
	// unrecognized key outright would also break two legitimate cases: a
	// planner naming a genuinely NEW feature not in the catalog yet, and a
	// project with no catalog built at all yet (a normal early-project state,
	// not a validation signal against this specific key) — so this only
	// downgrades confidence to ConfidenceLow when the catalog loads
	// successfully AND positively lacks this key; a catalog load failure
	// (missing/unbuilt) keeps the caller-supplied value trusted exactly as
	// before, since there is no actual signal against it in that case. A
	// brand-new feature has no history to rank anyway, so nothing is lost by
	// downgrading it; an existing-but-wrong collision at least no longer
	// masquerades as independently verified.
	if strings.TrimSpace(hints.ResolvedFeatureKey) != "" {
		key := strings.TrimSpace(hints.ResolvedFeatureKey)
		pkg.FeatureKey = key
		pkg.FeatureConfidence = ConfidenceVerified
		dotFP := filepath.Join(workspace, ".flowpilot")
		if catalog, err := featurecatalog.LoadCatalog(dotFP); err == nil {
			if _, ok := catalog.Get(key); !ok {
				pkg.FeatureConfidence = ConfidenceLow
				pkg.Warnings = append(pkg.Warnings, fmt.Sprintf(
					"resolved feature key %q is not a known catalog entry — treating as low-confidence (new feature or unverified planner value)", key))
			}
		}
	} else {
		dotFP := filepath.Join(workspace, ".flowpilot")
		catalog, err := featurecatalog.LoadCatalog(dotFP)
		if err != nil {
			pkg.FeatureConfidence = ConfidenceUnresolved
			pkg.Warnings = append(pkg.Warnings, "feature catalog unavailable: "+err.Error())
		} else {
			pkg.FeatureKey, pkg.FeatureConfidence = resolvePackageFeature(hints.UserPrompt, catalog, &pkg.Warnings)
		}
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
	// Resolve the workspace root through symlinks ONCE, up front, so the root
	// itself may legitimately be a symlink (e.g. /tmp on macOS). This does NOT
	// resolve any per-file path — doing that (the pre-P1-11 approach) is what
	// created the TOCTOU gap: EvalSymlinks validates a snapshot of the path,
	// then the file is reopened by string path, so an intermediate directory
	// swapped for a symlink between validation and open could escape the
	// workspace (BUG-288 P1-11). Every per-file open below instead walks its
	// own path component-by-component from this resolved root via
	// openWorkspaceRegularFile, which re-validates every component against
	// TOCTOU immediately before descending into it.
	resolvedWS := cleanWS
	if rws, err := filepath.EvalSymlinks(filepath.Clean(workspace)); err == nil {
		resolvedWS = rws + string(os.PathSeparator)
	}
	wsRoot := strings.TrimSuffix(resolvedWS, string(os.PathSeparator))
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
		// Lexical containment check computed against the UNRESOLVED workspace
		// root (matching how `clean` itself was built above) so a workspace
		// root that is itself behind a symlink (e.g. /tmp -> /private/tmp on
		// macOS) does not spuriously reject legitimate files — the relative
		// path is then re-anchored onto the resolved root for the actual open
		// below. This is intentionally cheap/approximate (it does not itself
		// resolve symlinks) — openWorkspaceRegularFile is the actual
		// TOCTOU-safe boundary enforcement, walking every component with
		// O_NOFOLLOW.
		cleanWSNoSep := strings.TrimSuffix(cleanWS, string(os.PathSeparator))
		rel, relErr := filepath.Rel(cleanWSNoSep, clean)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
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
		// BUG-288 P1-11: component-walk open (openat O_NOFOLLOW on unix; best-
		// effort top-down Lstat walk elsewhere) instead of EvalSymlinks-then-
		// reopen-by-string-path, which only protected the final component.
		f, err := openWorkspaceRegularFile(wsRoot, filepath.Join(wsRoot, rel))
		if err != nil {
			switch {
			case errors.Is(err, errNotRegularFile):
				omitted = append(omitted, p+": not_regular")
			// BUG-288 R13-22: wrapped *os.PathError needs errors.Is / fs.ErrNotExist.
			case errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err):
				omitted = append(omitted, p+": not_found")
			default:
				omitted = append(omitted, p+": symlink_resolve_error")
			}
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
//
// Section body order is strictly by FlowContextSection.Priority (ascending),
// then SourceType. Legacy fields (HistoryBlock/SourceExcerpts/DiscussionBlock)
// are synthesized as sections at their built-in priorities when Sections does
// not already carry them — so callers that only set legacy fields still render
// correctly, and MCP/Jira/Firebase (priority 6–9) never hard-code before
// source.excerpt (priority 4) (CP-50 residual P1).
func RenderFlowContextPackage(pkg FlowContextPackage) string {
	applyFlowContextPackBudget(&pkg)
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

	for _, s := range sectionsForRender(pkg) {
		renderFlowContextSection(&sb, s, pkg)
	}

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

// sectionsForRender merges package Sections with legacy projection fields,
// then sorts by Priority ascending (tie-break SourceType).
func sectionsForRender(pkg FlowContextPackage) []FlowContextSection {
	sections := append([]FlowContextSection(nil), pkg.Sections...)
	has := map[ContextSourceID]bool{}
	for _, s := range sections {
		has[ContextSourceID(s.SourceType)] = true
	}
	// Synthesize legacy-only fields so tests/callers that skip Sections still
	// participate in priority ordering.
	if !has[ContextSourceFeatureHistory] && strings.TrimSpace(pkg.HistoryBlock) != "" {
		sections = append(sections, FlowContextSection{
			SourceType: string(ContextSourceFeatureHistory),
			Priority:   2,
			Body:       pkg.HistoryBlock,
		})
	}
	if !has[ContextSourceSourceExcerpt] && len(pkg.SourceExcerpts) > 0 {
		sections = append(sections, FlowContextSection{
			SourceType: string(ContextSourceSourceExcerpt),
			Priority:   4,
			Excerpts:   pkg.SourceExcerpts,
		})
	}
	if !has[ContextSourceChatSummary] && strings.TrimSpace(pkg.DiscussionBlock) != "" {
		sections = append(sections, FlowContextSection{
			SourceType: string(ContextSourceChatSummary),
			Priority:   5,
			Body:       pkg.DiscussionBlock,
		})
	}
	// Fill default priorities for sections that omit Priority (older payloads).
	for i := range sections {
		if sections[i].Priority != 0 {
			continue
		}
		switch ContextSourceID(sections[i].SourceType) {
		case ContextSourceCanonicalHead:
			sections[i].Priority = 1
		case ContextSourceFeatureHistory:
			sections[i].Priority = 2
		case ContextSourceChangeContract:
			sections[i].Priority = 3
		case ContextSourceDependence:
			sections[i].Priority = 3
		case ContextSourceSourceExcerpt:
			sections[i].Priority = 4
		case ContextSourceChatSummary:
			sections[i].Priority = 5
		case ContextSourceMCPDriver:
			sections[i].Priority = 6
		case ContextSourceJiraIssue:
			sections[i].Priority = 7
		case ContextSourceJiraSprint:
			sections[i].Priority = 8
		case ContextSourceFirebaseCrashlytics:
			sections[i].Priority = 9
		default:
			// Unknown sources without Priority go after built-ins.
			sections[i].Priority = 50
		}
	}
	sort.SliceStable(sections, func(i, j int) bool {
		if sections[i].Priority != sections[j].Priority {
			return sections[i].Priority < sections[j].Priority
		}
		return sections[i].SourceType < sections[j].SourceType
	})
	return sections
}

// renderFlowContextSection writes one section with its canonical heading style.
func renderFlowContextSection(sb *strings.Builder, s FlowContextSection, pkg FlowContextPackage) {
	switch ContextSourceID(s.SourceType) {
	case ContextSourceChangeContract:
		if strings.TrimSpace(s.Body) == "" {
			return
		}
		sb.WriteString("\n### change.contract\n")
		// Embed trusted run-scoped marker so appendChangeContractIfAny skips
		// without trusting the user-forgeable heading alone (V10R4 P1).
		if pkg.WorkflowRunID != "" {
			sb.WriteString(changeContractTrustedMarker(pkg.WorkflowRunID) + "\n")
		}
		sb.WriteString("\n")
		if s.SourceRef != "" {
			sb.WriteString(fmt.Sprintf("_Source: %s_\n\n", s.SourceRef))
		}
		sb.WriteString(s.Body)
		sb.WriteString("\n")
	case ContextSourceCanonicalHead:
		if strings.TrimSpace(s.Body) == "" {
			return
		}
		// Body already carries "## Canonical state …"; write verbatim.
		sb.WriteString("\n" + strings.TrimSpace(s.Body) + "\n")
	case ContextSourceFeatureHistory:
		body := s.Body
		if body == "" {
			body = pkg.HistoryBlock
		}
		if strings.TrimSpace(body) == "" {
			return
		}
		sb.WriteString("\n### Change History\n\n")
		sb.WriteString(body)
		sb.WriteString("\n")
	case ContextSourceSourceExcerpt:
		excerpts := s.Excerpts
		if len(excerpts) == 0 {
			excerpts = pkg.SourceExcerpts
		}
		for _, ex := range excerpts {
			sb.WriteString(fmt.Sprintf("\n### Source: %s\n\n```\n%s\n```\n", ex.Path, ex.Excerpt))
			if ex.BytesCap {
				sb.WriteString("_(excerpt truncated)_\n")
			}
		}
	case ContextSourceChatSummary:
		body := s.Body
		if body == "" {
			body = pkg.DiscussionBlock
		}
		if strings.TrimSpace(body) == "" {
			return
		}
		sb.WriteString("\n### Prior Discussion\n\n")
		sb.WriteString(body)
		sb.WriteString("\n")
	default:
		if strings.TrimSpace(s.Body) == "" {
			return
		}
		sb.WriteString(fmt.Sprintf("\n### %s\n\n", s.SourceType))
		if s.SourceRef != "" {
			sb.WriteString(fmt.Sprintf("_Source: %s_\n\n", s.SourceRef))
		}
		sb.WriteString(s.Body)
		sb.WriteString("\n")
	}
}

// renderGenericSections is retained for tests that call it directly; production
// render goes through sectionsForRender (priority-ordered).
func renderGenericSections(sb *strings.Builder, sections []FlowContextSection) {
	for _, s := range sections {
		switch ContextSourceID(s.SourceType) {
		case ContextSourceFeatureHistory, ContextSourceChatSummary, ContextSourceSourceExcerpt, ContextSourceCanonicalHead:
			continue
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
