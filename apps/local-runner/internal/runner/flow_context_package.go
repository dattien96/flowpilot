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

	"flowpilot-runner/internal/changeledger"
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
	perFileExcerptBytes  = 4 * 1024  // 4 KB
	totalExcerptBytes    = 16 * 1024 // 16 KB
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
}

// FlowContextPackage is the deterministic context package assembled by the Plan
// step. It carries all grounding information downstream steps need without
// broad re-retrieval. No vector DB, embedding index, or similarity search is
// used at any stage of its construction (CP-41 P-2/P-3).
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
	Constraints       []string              `json:"constraints,omitempty"`
	Warnings          []string              `json:"warnings,omitempty"`
	Omitted           []string              `json:"omitted,omitempty"`
	BuiltAt           string                `json:"builtAt"`
}

// BuildFlowContextPackage assembles a deterministic FlowContextPackage for the
// Plan step of a Flow Mode run. It resolves the feature key from the user
// prompt, loads change history and chat summaries from existing ledger seams,
// and reads workspace-safe source excerpts.
//
// A missing catalog, unresolvable feature key, or absent ledger files produce
// a package with Warnings instead of an error so a Flow Mode run degrades
// gracefully (CP-41 P-2, Task-168 T-4).
func BuildFlowContextPackage(workspace string, hints FlowContextHints) (FlowContextPackage, error) {
	pkg := FlowContextPackage{
		WorkflowRunID: hints.WorkflowRunID,
		PlanStepRunID: hints.PlanStepRunID,
		BuiltAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if hints.SourceDocID != "" {
		pkg.SourceDocIDs = []string{hints.SourceDocID}
	}

	// Step 1: resolve feature key via the existing deterministic resolver —
	// no vector search, no model call.
	dotFP := filepath.Join(workspace, ".flowpilot")
	catalog, err := featurecatalog.LoadCatalog(dotFP)
	if err != nil {
		pkg.FeatureConfidence = ConfidenceUnresolved
		pkg.Warnings = append(pkg.Warnings, "feature catalog unavailable: "+err.Error())
	} else {
		pkg.FeatureKey, pkg.FeatureConfidence = resolvePackageFeature(hints.UserPrompt, catalog, &pkg.Warnings)
	}

	// Step 2: load history and discussion only for verified features.
	// Low / unresolved confidence must NOT inject the wrong feature's history.
	if pkg.FeatureConfidence == ConfidenceVerified && pkg.FeatureKey != "" {
		pkg.HistoryBlock, pkg.DiscussionBlock = loadFlowFeatureBlocks(dotFP, pkg.FeatureKey)
		if pkg.HistoryBlock == "" {
			pkg.Warnings = append(pkg.Warnings, "no change history found for feature: "+pkg.FeatureKey)
		}
	}

	// Step 3: workspace-safe source excerpts from hints.
	allPaths := fcpDedup(append(hints.ChangedPaths, hints.ExplicitSourcePaths...))
	pkg.SourceExcerpts, pkg.Omitted = readSourceExcerpts(workspace, allPaths)

	// Step 4: deterministic package ID.
	pkg.PackageID = fcpPackageID(hints.WorkflowRunID, hints.PlanStepRunID, pkg.FeatureKey)
	return pkg, nil
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

// loadFlowFeatureBlocks loads the HistorySlot and ChatSummarySlot for featureKey
// from .flowpilot. Returns ("", "") when ledgers are unavailable — never errors.
func loadFlowFeatureBlocks(dotFP, featureKey string) (history, discussion string) {
	ledger, err := changeledger.New(dotFP)
	if err != nil {
		return "", ""
	}
	history = strings.TrimSpace(featurecatalog.HistorySlot(featureKey, ledger))
	if summaryLedger, err := changeledger.NewChatSummaryLedger(dotFP); err == nil {
		discussion = strings.TrimSpace(featurecatalog.ChatSummarySlot(featureKey, summaryLedger))
	}
	return history, discussion
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
