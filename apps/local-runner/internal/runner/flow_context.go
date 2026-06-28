package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

const (
	flowContextArtifactSourceKind = "flow_context_package"
	flowContextArtifactFeatureID  = "flow-context"
	flowContextDefaultMaxBytes    = 24 * 1024
	flowContextDefaultExcerptCap  = 4 * 1024
)

type FlowContextConfidence string

const (
	FlowContextConfidenceVerified FlowContextConfidence = "verified"
	FlowContextConfidenceLow      FlowContextConfidence = "low"
	FlowContextConfidenceMissing  FlowContextConfidence = "missing"
)

type FlowContextSourceRef struct {
	Kind       string `json:"kind"`
	Path       string `json:"path,omitempty"`
	DocID      string `json:"docId,omitempty"`
	FeatureKey string `json:"featureKey,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type FlowContextExcerpt struct {
	Path          string `json:"path"`
	IncludedText  string `json:"includedText,omitempty"`
	OmittedReason string `json:"omittedReason,omitempty"`
	StartLine     int    `json:"startLine,omitempty"`
	EndLine       int    `json:"endLine,omitempty"`
}

type FlowContextSection struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type FlowContextHints struct {
	WorkflowRunID       string
	PlanStepRunID       string
	Prompt              string
	SourceDocID         string
	ChangedPaths        []string
	ExplicitSourcePaths []string
	MaxBytes            int
}

type FlowContextPackage struct {
	PackageID         string                 `json:"packageId"`
	WorkflowRunID     string                 `json:"workflowRunId"`
	PlanStepRunID     string                 `json:"planStepRunId"`
	FeatureKey        string                 `json:"featureKey"`
	FeatureConfidence FlowContextConfidence  `json:"featureConfidence"`
	SourceDocIDs      []string               `json:"sourceDocIds,omitempty"`
	SourceRefs        []FlowContextSourceRef `json:"sourceRefs,omitempty"`
	HistoryBlock      string                 `json:"historyBlock,omitempty"`
	DiscussionBlock   string                 `json:"discussionBlock,omitempty"`
	SourceExcerpts    []FlowContextExcerpt   `json:"sourceExcerpts,omitempty"`
	Constraints       []string               `json:"constraints,omitempty"`
	Warnings          []string               `json:"warnings,omitempty"`
	Omitted           []string               `json:"omitted,omitempty"`
	Sections          []FlowContextSection   `json:"sections,omitempty"`
	MaxBytes          int                    `json:"maxBytes,omitempty"`
}

func BuildFlowContextPackage(workspace, prompt string, priorTurns []transcriptTurn, hints FlowContextHints) (FlowContextPackage, error) {
	dotFP := filepath.Join(workspace, ".flowpilot")
	maxBytes := hints.MaxBytes
	if maxBytes <= 0 || maxBytes > flowContextDefaultMaxBytes {
		maxBytes = flowContextDefaultMaxBytes
	}

	pkg := FlowContextPackage{
		WorkflowRunID:     hints.WorkflowRunID,
		PlanStepRunID:     hints.PlanStepRunID,
		FeatureConfidence: FlowContextConfidenceMissing,
		MaxBytes:          maxBytes,
		Constraints: []string{
			"No vector retrieval used.",
			"Attach all new state to workflow_run_id and workflow_step_run_id.",
			"Use existing ledger and summary history only.",
		},
	}

	catalog, catalogErr := featurecatalog.LoadCatalog(dotFP)
	if catalogErr != nil {
		pkg.Warnings = append(pkg.Warnings, "feature catalog unavailable: "+catalogErr.Error())
	}

	if catalog != nil {
		if top, ok := resolveInjectionFeature(prompt, priorTurns, catalog); ok {
			pkg.FeatureKey = top.Key
			pkg.FeatureConfidence = FlowContextConfidenceVerified
			if top.Score < 5.0 {
				pkg.FeatureConfidence = FlowContextConfidenceLow
			}
		}
	}

	if pkg.FeatureKey == "" && strings.TrimSpace(hints.SourceDocID) != "" {
		pkg.Warnings = append(pkg.Warnings, "feature could not be resolved from prompt or prior turns")
	}

	if strings.TrimSpace(pkg.FeatureKey) != "" {
		ledger, err := changeledger.New(dotFP)
		if err != nil {
			pkg.Warnings = append(pkg.Warnings, "change ledger unavailable: "+err.Error())
		} else {
			pkg.HistoryBlock = strings.TrimSpace(featurecatalog.HistorySlot(pkg.FeatureKey, ledger))
		}
		if summaryLedger, err := changeledger.NewChatSummaryLedger(dotFP); err == nil {
			pkg.DiscussionBlock = strings.TrimSpace(featurecatalog.ChatSummarySlot(pkg.FeatureKey, summaryLedger))
		}
		if feature, ok := catalog.Get(pkg.FeatureKey); ok {
			for _, docID := range feature.DocRefs {
				if docID != "" {
					pkg.SourceDocIDs = appendUnique(pkg.SourceDocIDs, docID)
				}
			}
		}
	}

	if docID := strings.TrimSpace(hints.SourceDocID); docID != "" {
		pkg.SourceDocIDs = appendUnique(pkg.SourceDocIDs, docID)
	}

	pkg.SourceRefs = buildFlowContextSourceRefs(pkg.FeatureKey, pkg.SourceDocIDs)
	remainingBudget := maxBytes - len(renderFlowContextPackageBody(pkg))
	for _, path := range collectFlowContextSourcePaths(workspace, pkg.FeatureKey, hints) {
		excerpt, ref, omitted := readFlowContextExcerpt(workspace, path, flowContextDefaultExcerptCap, remainingBudget)
		if ref.Kind != "" {
			pkg.SourceRefs = append(pkg.SourceRefs, ref)
		}
		if omitted != "" {
			pkg.Omitted = append(pkg.Omitted, omitted)
			continue
		}
		if strings.TrimSpace(excerpt.IncludedText) != "" {
			pkg.SourceExcerpts = append(pkg.SourceExcerpts, excerpt)
			remainingBudget -= len(excerpt.IncludedText)
		}
	}

	if pkg.HistoryBlock == "" {
		pkg.Warnings = append(pkg.Warnings, "no committed history found")
	}
	if pkg.DiscussionBlock == "" {
		pkg.Warnings = append(pkg.Warnings, "no discussion summary found")
	}
	if len(pkg.SourceExcerpts) == 0 {
		pkg.Warnings = append(pkg.Warnings, "no bounded source excerpts were included")
	}

	pkg.PackageID = flowContextPackageID(pkg)
	pkg.Sections = []FlowContextSection{
		{Title: "Source refs", Body: renderFlowContextSourceRefs(pkg.SourceRefs)},
		{Title: "History", Body: pkg.HistoryBlock},
		{Title: "Discussion", Body: pkg.DiscussionBlock},
		{Title: "Source excerpts", Body: renderFlowContextExcerpts(pkg.SourceExcerpts)},
		{Title: "Constraints", Body: renderFlowContextBulletList(pkg.Constraints)},
		{Title: "Warnings", Body: renderFlowContextBulletList(pkg.Warnings)},
		{Title: "Omitted", Body: renderFlowContextBulletList(pkg.Omitted)},
	}
	return pkg, nil
}

func boundFlowContextRenderedPackage(pkg FlowContextPackage, rendered string) string {
	maxBytes := pkg.MaxBytes
	if maxBytes <= 0 || len(rendered) <= maxBytes {
		return rendered
	}
	const marker = "\n...[truncated]"
	if maxBytes <= len(marker) {
		return rendered[:maxBytes]
	}
	return rendered[:maxBytes-len(marker)] + marker
}

func flowContextPackageID(pkg FlowContextPackage) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		pkg.WorkflowRunID,
		pkg.PlanStepRunID,
		pkg.FeatureKey,
		string(pkg.FeatureConfidence),
		pkg.HistoryBlock,
		pkg.DiscussionBlock,
		strings.Join(pkg.SourceDocIDs, ","),
		strings.Join(pkg.Warnings, ","),
		strings.Join(pkg.Omitted, ","),
	}, "\n")))
	return "flowctx_" + hex.EncodeToString(sum[:8])
}

func appendUnique(values []string, next string) []string {
	for _, existing := range values {
		if existing == next {
			return values
		}
	}
	return append(values, next)
}

func buildFlowContextSourceRefs(featureKey string, docIDs []string) []FlowContextSourceRef {
	out := make([]FlowContextSourceRef, 0, len(docIDs)+1)
	if featureKey != "" {
		out = append(out, FlowContextSourceRef{
			Kind:       "feature",
			FeatureKey: featureKey,
		})
	}
	for _, docID := range docIDs {
		out = append(out, FlowContextSourceRef{
			Kind:       "document",
			DocID:      docID,
			FeatureKey: featureKey,
		})
	}
	return out
}

func collectFlowContextSourcePaths(workspace, featureKey string, hints FlowContextHints) []string {
	candidates := append([]string{}, hints.ExplicitSourcePaths...)
	candidates = append(candidates, hints.ChangedPaths...)

	if workspace != "" && featureKey != "" {
		if cat, err := featurecatalog.LoadCatalog(filepath.Join(workspace, ".flowpilot")); err == nil {
			if feat, ok := cat.Get(featureKey); ok {
				for _, glob := range feat.FileGlobs {
					candidates = append(candidates, glob)
				}
			}
		}
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.ContainsAny(candidate, "*?[") && workspace != "" {
			matches, err := filepath.Glob(filepath.Join(workspace, candidate))
			if err == nil {
				for _, match := range matches {
					rel, relErr := filepath.Rel(workspace, match)
					if relErr != nil || strings.HasPrefix(rel, "..") {
						continue
					}
					if _, ok := seen[rel]; ok {
						continue
					}
					seen[rel] = struct{}{}
					out = append(out, rel)
				}
				continue
			}
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	sort.Strings(out)
	return out
}

func readFlowContextExcerpt(workspace, candidate string, excerptCap, remainingBudget int) (FlowContextExcerpt, FlowContextSourceRef, string) {
	ref := FlowContextSourceRef{Kind: "source_file", Path: candidate}
	if remainingBudget <= 0 {
		return FlowContextExcerpt{Path: candidate, OmittedReason: "too_large"}, ref, candidate + ": too_large"
	}
	abs, ok := resolveWorkspaceSafePath(workspace, candidate)
	if !ok {
		return FlowContextExcerpt{Path: candidate, OmittedReason: "outside_workspace"}, ref, candidate + ": outside_workspace"
	}
	info, err := os.Stat(abs)
	if err != nil {
		return FlowContextExcerpt{Path: candidate, OmittedReason: "not_found"}, ref, candidate + ": not_found"
	}
	if info.IsDir() {
		return FlowContextExcerpt{Path: candidate, OmittedReason: "not_found"}, ref, candidate + ": not_found"
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return FlowContextExcerpt{Path: candidate, OmittedReason: "not_found"}, ref, candidate + ": not_found"
	}
	if len(raw) > excerptCap || len(raw) > remainingBudget {
		return FlowContextExcerpt{Path: candidate, OmittedReason: "too_large"}, ref, candidate + ": too_large"
	}
	if bytesContainNUL(raw) {
		return FlowContextExcerpt{Path: candidate, OmittedReason: "binary"}, ref, candidate + ": binary"
	}
	return FlowContextExcerpt{
		Path:         candidate,
		IncludedText: strings.TrimSpace(string(raw)),
		StartLine:    1,
		EndLine:      lineCount(string(raw)),
	}, ref, ""
}

func resolveWorkspaceSafePath(workspace, candidate string) (string, bool) {
	if workspace == "" || candidate == "" {
		return "", false
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", false
	}
	path := candidate
	if !filepath.IsAbs(candidate) {
		path = filepath.Join(root, candidate)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return abs, true
}

func bytesContainNUL(raw []byte) bool {
	for _, b := range raw {
		if b == 0 {
			return true
		}
	}
	return false
}

func lineCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func renderFlowContextBulletList(values []string) string {
	if len(values) == 0 {
		return "None"
	}
	lines := make([]string, len(values))
	for i, value := range values {
		lines[i] = "- " + strings.TrimSpace(value)
	}
	return strings.Join(lines, "\n")
}

func renderFlowContextSourceRefs(refs []FlowContextSourceRef) string {
	if len(refs) == 0 {
		return "None"
	}
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		switch ref.Kind {
		case "feature":
			lines = append(lines, "- feature: "+ref.FeatureKey)
		case "document":
			lines = append(lines, "- doc: "+ref.DocID+" ("+ref.FeatureKey+")")
		case "source_file":
			line := "- file: " + ref.Path
			if ref.Reason != "" {
				line += " [" + ref.Reason + "]"
			}
			lines = append(lines, line)
		default:
			lines = append(lines, "- "+ref.Kind)
		}
	}
	return strings.Join(lines, "\n")
}

func renderFlowContextExcerpts(excerpts []FlowContextExcerpt) string {
	if len(excerpts) == 0 {
		return "None"
	}
	blocks := make([]string, 0, len(excerpts))
	for _, excerpt := range excerpts {
		body := excerpt.IncludedText
		if body == "" {
			body = excerpt.OmittedReason
		}
		blocks = append(blocks, strings.TrimSpace(fmt.Sprintf("### %s\n```text\n%s\n```", excerpt.Path, body)))
	}
	return strings.Join(blocks, "\n\n")
}

func RenderFlowContextPackage(pkg FlowContextPackage) string {
	var sb strings.Builder
	sb.WriteString("## Flow Context Package\n")
	sb.WriteString(fmt.Sprintf("- Package ID: %s\n", pkg.PackageID))
	sb.WriteString(fmt.Sprintf("- Workflow run ID: %s\n", pkg.WorkflowRunID))
	sb.WriteString(fmt.Sprintf("- Plan step run ID: %s\n", pkg.PlanStepRunID))
	sb.WriteString(fmt.Sprintf("- Feature key: %s\n", pkg.FeatureKey))
	sb.WriteString(fmt.Sprintf("- Feature confidence: %s\n", pkg.FeatureConfidence))
	sb.WriteString("- No vector retrieval used.\n")
	for _, section := range pkg.Sections {
		sb.WriteString("\n### " + section.Title + "\n")
		body := strings.TrimSpace(section.Body)
		if body == "" {
			body = "None"
		}
		sb.WriteString(body)
		sb.WriteString("\n")
	}
	return boundFlowContextRenderedPackage(pkg, strings.TrimSpace(sb.String()))
}

func renderFlowContextPackageBody(pkg FlowContextPackage) string {
	return RenderFlowContextPackage(pkg)
}
