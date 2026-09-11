package docscan

import (
	"fmt"
	"strings"
)

// Severity classifies how badly a document violates the SS-13 contract.
//
// Levels follow Task-332 T-2 / CP-48 P-1:
//   - Critical: structural breakage (no metadata block, empty document).
//   - Important: missing required data (metadata fields, AI Quick View,
//     canonical sections, Feature Keys on governing docs).
//   - Minor: format drift (section ordering, missing Open Questions sub-view).
type Severity string

const (
	SeverityCritical  Severity = "critical"
	SeverityImportant Severity = "important"
	SeverityMinor     Severity = "minor"
)

// Phase identifiers used across the package. They are the short forms of the
// requirements/ directory names and filename prefixes.
const (
	PhaseSS     = "ss"
	PhaseSD     = "sd"
	PhaseCP     = "cp"
	PhaseTask   = "task"
	PhaseBugFix = "bugfix"
)

// allPhases lists every governed phase, in requirements/ directory order.
var allPhases = []string{PhaseSS, PhaseSD, PhaseCP, PhaseTask, PhaseBugFix}

// governingPhases are the "doc quản trị" phases that must carry the optional
// `Feature Keys` metadata field (Task-332 Open Questions; BUG-280). Task and
// BugFix execution deltas are never flagged for it.
var governingPhases = map[string]bool{
	PhaseSS: true,
	PhaseSD: true,
	PhaseCP: true,
}

// ScanIssue is a single conformance violation found in one document.
type ScanIssue struct {
	FilePath   string   `json:"file_path"`
	Line       int      `json:"line"`
	Severity   Severity `json:"severity"`
	RuleID     string   `json:"rule_id"`
	Message    string   `json:"message"`
	CanAutoFix bool     `json:"can_auto_fix"`
}

// ScanReport aggregates the result of a directory scan.
type ScanReport struct {
	TotalFilesScanned int         `json:"total_files_scanned"`
	ConformingFiles   int         `json:"conforming_files"`
	Issues            []ScanIssue `json:"issues"`
}

// ConformanceRule describes one check the scanner can apply.
type ConformanceRule struct {
	ID          string   `json:"id"`
	Phases      []string `json:"phases"` // e.g. ["ss", "sd", "cp", "task", "bugfix"]
	Severity    Severity `json:"severity"`
	Description string   `json:"description"`
	CanAutoFix  bool     `json:"can_auto_fix"`
}

// Rule identifiers used by the scanner and the auto-fixer.
const (
	ruleMissingMetadataBlock   = "missing_metadata_block"
	ruleMissingAIQuickView     = "missing_ai_quick_view"
	ruleMissingAIVSubsection   = "missing_ai_quick_view_subsection"
	ruleMissingOpenQuestions   = "missing_open_questions"
	ruleMissingRequiredField   = "missing_required_field"
	ruleMissingFeatureKeys     = "missing_feature_keys"
	ruleSectionOutOfOrder      = "section_out_of_order"
	ruleMissingRequiredSection = "missing_required_section"
	ruleEmptyDocument          = "empty_document"
)

// requiredMetadataFieldsByPhase maps each phase to its required metadata block
// fields. All five FORMAT-REFERENCE-*.md samples define the same 13 mandatory
// fields (SS-13 §5.1), so the table is uniform per phase but kept per-phase so
// future divergence is a one-line change.
var requiredMetadataFieldsByPhase = func() map[string][]string {
	fields := []string{
		"Document ID",
		"Title",
		"Phase",
		"Status",
		"Owner",
		"Reviewers",
		"Created",
		"Last Updated",
		"Parent Documents",
		"Child Documents",
		"Related Documents",
		"Replaces",
		"Tags",
	}
	m := make(map[string][]string, len(allPhases))
	for _, p := range allPhases {
		m[p] = fields
	}
	return m
}()

// aiQuickViewSubsectionsByPhase maps each phase to the required `## AI Quick
// View` sub-sections. Every FORMAT-REFERENCE-*.md sample uses the same six
// sub-sections (SS-13 §5.2), derived here from those samples.
var aiQuickViewSubsectionsByPhase = func() map[string][]string {
	subs := []string{"Summary", "Current Ask", "Key Decisions", "Constraints", "Open Questions", "Source Refs"}
	m := make(map[string][]string, len(allPhases))
	for _, p := range allPhases {
		m[p] = subs
	}
	return m
}()

// canonicalSectionsByPhase maps each phase to its canonical numbered `## N.`
// section order, taken verbatim from the FORMAT-REFERENCE-{SS,SD,CP,TASK,
// BUGFIX}.md samples (SS-13 §6 "Minimum sections").
//
// Legacy docs with Vietnamese section titles (e.g. "## 1. Mục tiêu") do not
// match these canonical English names, so missing_required_section fires on
// them BY DESIGN. Translating/aliasing a legacy heading is a content decision
// owned by the /standardize flow (Task-333 / CP-48 P-3 Assisted-Fixer), never
// by this scanner — AutoFix must not rename, translate, or drop such headings.
var canonicalSectionsByPhase = map[string][]string{
	PhaseSS: {
		"Goal",
		"Problem",
		"Scope",
		"Non-Goals",
		"User Stories or Primary Use Cases",
		"Acceptance Criteria",
		"Business Rules",
		"Edge Cases",
		"Dependencies",
		"Open Questions",
		"Definition of Done",
	},
	PhaseSD: {
		"Goal",
		"Input Documents",
		"Architecture Decision",
		"Component Impact",
		"Data Model",
		"Interfaces and Contracts",
		"Execution Flow",
		"Failure and Edge Handling",
		"Security and Operational Concerns",
		"Risks and Trade-Offs",
		"Validation Strategy",
		"Traceability to Spec",
	},
	PhaseCP: {
		"Goal",
		"Input Documents",
		"Implementation Strategy",
		"Work Breakdown",
		"Touched Areas",
		"Data or Migration Steps",
		"Validation Plan",
		"Rollout and Fallback",
		"Risks",
		"Definition of Done",
	},
	PhaseTask: {
		"Goal",
		"Parent Links",
		"Trigger",
		"Exact Change",
		"Touched Areas",
		"Acceptance Check",
		"Out of Scope",
		"Completion Notes",
	},
	PhaseBugFix: {
		"Issue Summary",
		"Parent Links",
		"Environment and Reproduction",
		"Expected vs Actual",
		"Impact",
		"Root Cause",
		"Fix Strategy",
		"Validation",
		"Regression Guard",
		"Follow-Up Document Updates",
	},
}

// phaseMetadataValues is the value synthesized for a missing `Phase` metadata
// field during auto-fix; it mirrors the FORMAT-REFERENCE samples.
var phaseMetadataValues = map[string]string{
	PhaseSS:     "system_spec",
	PhaseSD:     "tech_design",
	PhaseCP:     "coding_plan",
	PhaseTask:   "task",
	PhaseBugFix: "bugfix",
}

// metadataFieldDefaults holds safe, non-destructive placeholder values used
// when the auto-fixer inserts a missing metadata field.
var metadataFieldDefaults = map[string]string{
	"document id":       "TODO",
	"title":             "TODO",
	"status":            "draft",
	"owner":             "TODO",
	"reviewers":         "TODO",
	"created":           "TODO",
	"last updated":      "TODO",
	"parent documents":  "None",
	"child documents":   "None",
	"related documents": "None",
	"replaces":          "None",
	"tags":              "TODO",
	"feature keys":      "None",
}

// phaseAliases maps accepted AutoFixDocument phase spellings (lowercased) to
// the canonical phase identifiers.
var phaseAliases = map[string]string{
	"ss":          PhaseSS,
	"system_spec": PhaseSS,
	"system spec": PhaseSS,
	"sd":          PhaseSD,
	"tech_design": PhaseSD,
	"tech design": PhaseSD,
	"cp":          PhaseCP,
	"coding_plan": PhaseCP,
	"coding plan": PhaseCP,
	"task":        PhaseTask,
	"bugfix":      PhaseBugFix,
	"bug_fix":     PhaseBugFix,
	"bug fix":     PhaseBugFix,
}

// DefaultConformanceRules returns the rule table the scanner checks documents
// against. It contains at least the rules mandated by Task-332 §10.
func DefaultConformanceRules() []ConformanceRule {
	all := append([]string{}, allPhases...)
	return []ConformanceRule{
		{
			ID:          ruleMissingMetadataBlock,
			Phases:      all,
			Severity:    SeverityCritical,
			Description: "Document is missing the required '## Metadata' block (SS-13 §5.1).",
			CanAutoFix:  true,
		},
		{
			ID:          ruleMissingAIQuickView,
			Phases:      all,
			Severity:    SeverityImportant,
			Description: "Document is missing the required '## AI Quick View' block (SS-13 §5.2).",
			CanAutoFix:  true,
		},
		{
			ID:          ruleMissingAIVSubsection,
			Phases:      all,
			Severity:    SeverityImportant,
			Description: "'## AI Quick View' is missing one of its required sub-sections (Summary, Current Ask, Key Decisions, Constraints, Source Refs) per the FORMAT-REFERENCE samples.",
			CanAutoFix:  true,
		},
		{
			ID:          ruleMissingOpenQuestions,
			Phases:      all,
			Severity:    SeverityMinor,
			Description: "'## AI Quick View' is missing the 'Open Questions' sub-section (SS-13 §5.2).",
			CanAutoFix:  true,
		},
		{
			ID:          ruleMissingRequiredField,
			Phases:      all,
			Severity:    SeverityImportant,
			Description: "Metadata block is missing a required field for the phase (SS-13 §5.1); the field list is field-specific and comes from the FORMAT-REFERENCE samples.",
			CanAutoFix:  true,
		},
		{
			ID:          ruleMissingFeatureKeys,
			Phases:      []string{PhaseSS, PhaseSD, PhaseCP},
			Severity:    SeverityImportant,
			Description: "Governing SS/SD/CP document is missing the 'Feature Keys' metadata field (BUG-280, SS-13 §5.1). Never flagged on task/bugfix docs or on FORMAT-REFERENCE writing guides.",
			CanAutoFix:  true,
		},
		{
			ID:          ruleSectionOutOfOrder,
			Phases:      all,
			Severity:    SeverityMinor,
			Description: "Numbered '## N.' sections are not in the phase's canonical order defined by the FORMAT-REFERENCE sample.",
			CanAutoFix:  true,
		},
		{
			ID:          ruleMissingRequiredSection,
			Phases:      all,
			Severity:    SeverityImportant,
			Description: "Document is missing one of the phase's canonical numbered sections (SS-13 §6 minimum sections).",
			CanAutoFix:  true,
		},
		{
			ID:          ruleEmptyDocument,
			Phases:      all,
			Severity:    SeverityCritical,
			Description: "Document content is empty (or whitespace only).",
			CanAutoFix:  false,
		},
	}
}

// NormalizePhase maps a user-supplied phase string to a canonical phase
// identifier, accepting short and long spellings case-insensitively.
func NormalizePhase(phase string) (string, error) {
	key := normalizeName(phase)
	if key == "" {
		return "", fmt.Errorf("docscan: unsupported phase %q (supported: ss, sd, cp, task, bugfix)", phase)
	}
	if p, ok := phaseAliases[key]; ok {
		return p, nil
	}
	return "", fmt.Errorf("docscan: unsupported phase %q (supported: ss, sd, cp, task, bugfix)", phase)
}

// requiredMetadataFields returns the required metadata field names for a phase.
func requiredMetadataFields(phase string) []string {
	return requiredMetadataFieldsByPhase[phase]
}

// aiQuickViewSubsections returns the required AI Quick View sub-sections for a
// phase.
func aiQuickViewSubsections(phase string) []string {
	return aiQuickViewSubsectionsByPhase[phase]
}

// canonicalSections returns the canonical numbered section names for a phase.
func canonicalSections(phase string) []string {
	return canonicalSectionsByPhase[phase]
}

// normalizeName lowercases a name and collapses whitespace so heading and
// field comparisons are stable (it also tolerates trailing CR from CRLF files).
func normalizeName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
