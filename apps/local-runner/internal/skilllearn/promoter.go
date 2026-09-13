package skilllearn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LessonCandidatesFileName is the CP-23 §5 workspace artifact: lesson
// candidates are persisted as JSON at
// <workspace>/.flowpilot/workflow_lesson_candidates.json.
const LessonCandidatesFileName = "workflow_lesson_candidates.json"

// PromoterService provides the human-approval API over the persisted
// LessonCandidate store (Task-336 §11 Code Guide). It is a library service:
// Task-336 ships no runner/TUI wiring — TUI/Desktop review surfaces come
// later and will call these methods.
type PromoterService struct {
	// StorePath is the path of the candidates JSON file. Typically
	// <workspace>/.flowpilot/workflow_lesson_candidates.json (see
	// NewPromoterService).
	StorePath string
}

// NewPromoterService returns a PromoterService rooted at the given workspace
// (StorePath = <workspaceRoot>/.flowpilot/workflow_lesson_candidates.json).
func NewPromoterService(workspaceRoot string) *PromoterService {
	return &PromoterService{
		StorePath: filepath.Join(strings.TrimSpace(workspaceRoot), ".flowpilot", LessonCandidatesFileName),
	}
}

// ListCandidates returns every candidate currently persisted in the store
// (Task-336 §11 Code Guide). A missing store file is tolerated as an empty
// list; a corrupt file is returned as an error (never a panic).
// ctx is accepted for the Code Guide signature and reserved for cancellation
// by future TUI callers; the current implementation is synchronous local I/O.
func (p *PromoterService) ListCandidates(ctx context.Context) ([]LessonCandidate, error) {
	return loadCandidates(p.StorePath)
}

// UpsertCandidates ingests aggregation output (e.g. AggregateDriftEvents)
// into the store. This is the ONLY write path for new candidates and it
// enforces the T-1 hard rule: ingested candidates are always persisted as
// StatusCandidate — no code path can create an approved/promoted entry
// without the explicit Approve/Reject methods below.
//
// Upsert semantics (deterministic):
//   - an existing entry with the same TriggerPattern (case-insensitive) and
//     StatusCandidate is refreshed: RepeatCount and the SupportingRunIDs union
//     are updated from the newer aggregation, while ID, Title, Group,
//     AntiPattern, PreferredBehavior are preserved (they may carry human edits
//     made via UpdateCandidate);
//   - an existing entry with the same TriggerPattern whose status is
//     approved/rejected/promoted is skipped — a human decision is never
//     overturned or duplicated by re-aggregation;
//   - otherwise the candidate is appended (forced to StatusCandidate; an
//     empty Title or TriggerPattern is rejected with an error).
func (p *PromoterService) UpsertCandidates(ctx context.Context, incoming []LessonCandidate) error {
	existing, err := loadCandidates(p.StorePath)
	if err != nil {
		return err
	}

	for _, candidate := range incoming {
		if strings.TrimSpace(candidate.Title) == "" {
			return fmt.Errorf("skilllearn: cannot upsert candidate %q without a title", candidate.ID)
		}
		if strings.TrimSpace(candidate.TriggerPattern) == "" {
			return fmt.Errorf("skilllearn: cannot upsert candidate %q without a trigger pattern", candidate.ID)
		}
		candidate.Status = StatusCandidate // T-1: ingestion never pre-approves

		idx := -1
		for i := range existing {
			if strings.EqualFold(strings.TrimSpace(existing[i].TriggerPattern), strings.TrimSpace(candidate.TriggerPattern)) {
				idx = i
				break
			}
		}
		if idx < 0 {
			if candidate.ID == "" {
				candidate.ID = generateCandidateID(candidate.TriggerPattern)
			}
			existing = append(existing, candidate)
			continue
		}
		if existing[idx].Status != StatusCandidate {
			continue // human already decided; keep the store stable
		}
		// Refresh the evidence, preserve identity and human-edited content.
		existing[idx].RepeatCount = candidate.RepeatCount
		existing[idx].SupportingRunIDs = unionStrings(existing[idx].SupportingRunIDs, candidate.SupportingRunIDs)
	}

	return saveCandidates(p.StorePath, existing)
}

// UpdateCandidate refines a pending candidate before approval (Task-336 DOD
// item 3 — the Edit right). Only the editable content fields of edited are
// applied (empty string = keep the stored value): Title, Group, TriggerPattern,
// AntiPattern, PreferredBehavior. The evidence fields (ProjectID, RepeatCount,
// SupportingRunIDs) and the status are preserved — the candidate stays
// StatusCandidate (status transitions happen only via Approve/Reject).
// edited.ID, when set, must match candidateID.
func (p *PromoterService) UpdateCandidate(ctx context.Context, candidateID string, edited LessonCandidate) error {
	if strings.TrimSpace(edited.ID) != "" && edited.ID != candidateID {
		return fmt.Errorf("skilllearn: edited candidate id %q does not match %q", edited.ID, candidateID)
	}
	existing, err := loadCandidates(p.StorePath)
	if err != nil {
		return err
	}
	idx := indexByID(existing, candidateID)
	if idx < 0 {
		return fmt.Errorf("skilllearn: candidate %q not found", candidateID)
	}
	if existing[idx].Status != StatusCandidate {
		return fmt.Errorf("skilllearn: candidate %q is %q; only pending candidates can be edited", candidateID, existing[idx].Status)
	}
	if strings.TrimSpace(edited.Title) != "" {
		existing[idx].Title = edited.Title
	}
	if strings.TrimSpace(edited.Group) != "" {
		existing[idx].Group = edited.Group
	}
	if strings.TrimSpace(edited.TriggerPattern) != "" {
		existing[idx].TriggerPattern = edited.TriggerPattern
	}
	if strings.TrimSpace(edited.AntiPattern) != "" {
		existing[idx].AntiPattern = edited.AntiPattern
	}
	if strings.TrimSpace(edited.PreferredBehavior) != "" {
		existing[idx].PreferredBehavior = edited.PreferredBehavior
	}
	return saveCandidates(p.StorePath, existing)
}

// ApproveCandidateAndExport approves a candidate and exports it as a SKILL.md
// file (Task-336 §11 Code Guide). Status flow (no silent loss):
//
//	candidate → approved (persisted immediately) → export → promoted (persisted)
//
// If the export fails (including a *ConflictError that needs
// SkillExportOptions.Force=true), the candidate STAYS approved with the error
// returned — the approval is never lost, and re-calling this method with the
// same ID retries the export. Only pending (candidate) or previously-failed
// (approved) candidates may be approved; rejected and promoted candidates are
// refused.
func (p *PromoterService) ApproveCandidateAndExport(ctx context.Context, candidateID string, opts SkillExportOptions) error {
	existing, err := loadCandidates(p.StorePath)
	if err != nil {
		return err
	}
	idx := indexByID(existing, candidateID)
	if idx < 0 {
		return fmt.Errorf("skilllearn: candidate %q not found", candidateID)
	}

	switch existing[idx].Status {
	case StatusCandidate:
		existing[idx].Status = StatusApproved
		if err := saveCandidates(p.StorePath, existing); err != nil {
			return err
		}
	case StatusApproved:
		// Previous export attempt failed after approval — retry below.
	case StatusRejected:
		return fmt.Errorf("skilllearn: candidate %q was rejected and cannot be approved", candidateID)
	case StatusPromoted:
		return fmt.Errorf("skilllearn: candidate %q was already promoted", candidateID)
	default:
		return fmt.Errorf("skilllearn: candidate %q has unknown status %q", candidateID, existing[idx].Status)
	}

	if err := PromoteCandidateToSkill(existing[idx], opts); err != nil {
		// Keep StatusApproved (already persisted) so a retry (e.g. with
		// Force=true after a conflict) resumes cleanly — no silent loss.
		return err
	}
	existing, err = loadCandidates(p.StorePath)
	if err != nil {
		return err
	}
	idx = indexByID(existing, candidateID)
	if idx < 0 {
		return fmt.Errorf("skilllearn: candidate %q disappeared during export", candidateID)
	}
	existing[idx].Status = StatusPromoted
	return saveCandidates(p.StorePath, existing)
}

// RejectCandidate rejects a candidate (Task-336 §11 Code Guide): status →
// StatusRejected, persisted immediately. Idempotent for already-rejected
// candidates; promoted candidates are refused (an exported skill must be
// retracted out of band, not silently un-rejected).
func (p *PromoterService) RejectCandidate(ctx context.Context, candidateID string) error {
	existing, err := loadCandidates(p.StorePath)
	if err != nil {
		return err
	}
	idx := indexByID(existing, candidateID)
	if idx < 0 {
		return fmt.Errorf("skilllearn: candidate %q not found", candidateID)
	}
	switch existing[idx].Status {
	case StatusRejected:
		return nil // idempotent
	case StatusPromoted:
		return fmt.Errorf("skilllearn: candidate %q was already promoted and cannot be rejected", candidateID)
	case StatusCandidate, StatusApproved, "":
		existing[idx].Status = StatusRejected
		return saveCandidates(p.StorePath, existing)
	default:
		return fmt.Errorf("skilllearn: candidate %q has unknown status %q", candidateID, existing[idx].Status)
	}
}

// indexByID returns the index of the candidate with the given ID, or -1.
func indexByID(candidates []LessonCandidate, id string) int {
	for i := range candidates {
		if candidates[i].ID == id {
			return i
		}
	}
	return -1
}

// generateCandidateID derives a stable deterministic ID from the trigger
// pattern text (same "lc-" + 12-hex sha256 shape as AggregateDriftEvents) for
// hand-built candidates that arrive without an ID.
func generateCandidateID(triggerPattern string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(triggerPattern))))
	return "lc-" + hex.EncodeToString(sum[:])[:12]
}

// unionStrings merges b into a preserving first-appearance order and
// de-duplicating.
func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, s := range list {
			if seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// loadCandidates reads the store. Contract (Task-336): a missing file is an
// empty list; a corrupt file is an error (never a panic); an empty file is an
// empty list.
func loadCandidates(path string) ([]LessonCandidate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("skilllearn: read candidate store %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var candidates []LessonCandidate
	if err := json.Unmarshal(data, &candidates); err != nil {
		return nil, fmt.Errorf("skilllearn: corrupt candidate store %s: %w", path, err)
	}
	return candidates, nil
}

// saveCandidates persists the store atomic-ish: write to "<path>.tmp" in the
// same directory then rename over the target, so a crash never leaves a
// half-written store.
func saveCandidates(path string, candidates []LessonCandidate) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("skilllearn: candidate store path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("skilllearn: mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil {
		return fmt.Errorf("skilllearn: marshal candidates: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("skilllearn: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("skilllearn: rename %s: %w", tmp, err)
	}
	return nil
}
