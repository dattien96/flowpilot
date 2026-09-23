package runner

// Task-333 T-4 / T-5 (CP-49 P-3 / P-4): the SS-Lock gate — a non-bypassable
// workflow pause that hands SS approval to a human, plus the post-approval
// handoff of the drafts to the CP-48 docscan machinery (Task-332).
//
// Topology mirrors the CP-60 Vibe Mode `user.confirm` lock (internal/runner/
// vibe_lock.go): park the run in a waiting state, emit a confirm-required
// event carrying the draft for the client modal, and only ever resume from the
// dedicated human-driven confirm endpoint
// (POST /client/workflow-runs/{id}/confirm). The AI has no code path past the
// gate:
//
//  1. Reverse-doc runs NEVER publish to requirements/ phase roots — the only
//     publisher is HandleSSLockConfirm, and it requires the gate to be in the
//     `waiting_user_confirm` state (transition guarded by the gate mutex).
//  2. While the gate is locked, POST /client/workflow-runs/{id}/turns is
//     rejected with 409 `ss_lock` (see the fence installed at the top of
//     handleStartTurn in interactive_handlers.go), so no AI/provider
//     continuation can advance the run.
//  3. HandleSSLockConfirm / HandleSSLockReject are reachable only from the
//     confirm HTTP endpoint, i.e. human-initiated, and the gate is one-shot:
//     after completed/cancelled any further confirm/reject fails.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/docscan"
)

// EventUserConfirmRequired is the SS-Lock pause event (Task-333 Key Decision
// T-3): clients show the SS review modal from its payload and answer via
// POST /client/workflow-runs/{id}/confirm.
const EventUserConfirmRequired ProviderEventType = "user_confirm_required"

// EventSSLockConfirmed / EventSSLockRejected close the gate lifecycle.
const (
	EventSSLockConfirmed ProviderEventType = "ss_lock_confirmed"
	EventSSLockRejected  ProviderEventType = "ss_lock_rejected"
)

// SS-Lock gate lifecycle statuses (Task-333 §10: "waiting_user_confirm",
// "resuming", "cancelled").
const (
	ssLockWaiting   = "waiting_user_confirm"
	ssLockResuming  = "resuming"
	ssLockCompleted = "completed"
	ssLockCancelled = "cancelled"
)

// SSLockState is the gate state surfaced to clients (Task-333 Code Guide).
type SSLockState struct {
	RunID       string `json:"run_id"`
	DraftSSPath string `json:"draft_ss_path"`
	Locked      bool   `json:"locked"`
	UserEdits   string `json:"user_edits,omitempty"`
}

// StandardizeEvent is one SS-Lock lifecycle record kept on the gate and served
// to clients via GET /client/workflow-runs/{runId}/ss-lock. The pause event
// embeds the full draft SS content so the modal renders it without a second
// fetch (CP-60 parity: the modal previews the draft).
type StandardizeEvent struct {
	Type           ProviderEventType `json:"type"`
	OccurredAt     string            `json:"occurred_at"`
	Status         string            `json:"status"`
	DraftSSPath    string            `json:"draft_ss_path,omitempty"`
	DraftSDPath    string            `json:"draft_sd_path,omitempty"`
	DraftSSContent string            `json:"draft_ss_content,omitempty"`
}

// SSLockSnapshot is the full gate view returned by the GET endpoint and used
// by tests to assert the gate lifecycle.
type SSLockSnapshot struct {
	State      SSLockState         `json:"state"`
	Status     string              `json:"status"`
	Events     []StandardizeEvent  `json:"events"`
	Published  []string            `json:"published_paths,omitempty"`
	ScanReport *docscan.ScanReport `json:"scan_report,omitempty"`
}

// ssLockGate is the server-side pause record for one standardize run.
type ssLockGate struct {
	mu sync.Mutex

	runID         string
	featureBase   string // deterministic published-file base name, e.g. "Auth"
	workspaceRoot string

	status         string
	draftSSPath    string
	draftSDPath    string
	draftSSContent string
	draftSDContent string
	userEdits      string
	events         []StandardizeEvent
	published      []string
	scanReport     *docscan.ScanReport
}

// ssLockKey scopes gates per service instance so concurrent services (tests,
// embedders) never collide on synthetic run ids.
type ssLockKey struct {
	svc   *InteractiveService
	runID string
}

var (
	ssLockGatesMu sync.Mutex
	ssLockGates   = map[ssLockKey]*ssLockGate{}
)

func (s *InteractiveService) registerSSLockGate(g *ssLockGate) {
	ssLockGatesMu.Lock()
	defer ssLockGatesMu.Unlock()
	ssLockGates[ssLockKey{svc: s, runID: g.runID}] = g
}

func (s *InteractiveService) ssLockGateFor(runID string) *ssLockGate {
	ssLockGatesMu.Lock()
	defer ssLockGatesMu.Unlock()
	return ssLockGates[ssLockKey{svc: s, runID: runID}]
}

// SSLockSnapshot returns the current gate view for a run.
func (s *InteractiveService) SSLockSnapshot(runID string) (*SSLockSnapshot, bool) {
	g := s.ssLockGateFor(runID)
	if g == nil {
		return nil, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	snap := &SSLockSnapshot{
		State: SSLockState{
			RunID:       g.runID,
			DraftSSPath: g.draftSSPath,
			Locked:      g.status == ssLockWaiting,
			UserEdits:   g.userEdits,
		},
		Status:     g.status,
		Events:     append([]StandardizeEvent{}, g.events...),
		Published:  append([]string{}, g.published...),
		ScanReport: g.scanReport,
	}
	return snap, true
}

// ssLockTurnFence is the non-bypassability fence for the turn endpoint: while
// a run is locked at SS-Lock, no new AI/provider turn may start. The ONLY way
// forward is the human confirm endpoint. Nil means "not fenced".
func (s *InteractiveService) ssLockTurnFence(runID string) *apiErr {
	g := s.ssLockGateFor(runID)
	if g == nil {
		return nil
	}
	g.mu.Lock()
	status := g.status
	g.mu.Unlock()
	if status != ssLockWaiting {
		return nil
	}
	return newAPIErr(http.StatusConflict, "ss_lock",
		"run is locked at the non-bypassable SS-Lock gate; approve or reject via POST /client/workflow-runs/{id}/confirm before starting a new turn")
}

// EmitSSLockEvent transitions the workflow run into the `waiting_user_confirm`
// pause and emits EventUserConfirmRequired with the draft SS content as its
// payload (Task-333 Code Guide). Clients resume the run only through
// HandleSSLockConfirm.
func (s *InteractiveService) EmitSSLockEvent(ctx context.Context, state SSLockState) error {
	g := s.ssLockGateFor(state.RunID)
	if g == nil {
		return newAPIErr(http.StatusNotFound, "ss_lock_not_found",
			fmt.Sprintf("no ss-lock gate registered for run %q", state.RunID))
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != ssLockWaiting {
		return newAPIErr(http.StatusConflict, "ss_lock_state",
			fmt.Sprintf("ss-lock gate for run %q is %q, cannot emit the pause event", state.RunID, g.status))
	}
	// CP-60 parity: parking is idempotent — a re-emit while already waiting
	// must not duplicate the pause event (the client modal would render twice).
	for _, ev := range g.events {
		if ev.Type == EventUserConfirmRequired {
			return nil
		}
	}
	g.events = append(g.events, StandardizeEvent{
		Type:           EventUserConfirmRequired,
		OccurredAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Status:         g.status,
		DraftSSPath:    g.draftSSPath,
		DraftSDPath:    g.draftSDPath,
		DraftSSContent: g.draftSSContent,
	})
	log.Printf("[ss-lock] run=%q parked waiting_user_confirm draft_ss=%q draft_sd=%q", state.RunID, g.draftSSPath, g.draftSDPath)
	return nil
}

// HandleSSLockConfirm processes human approval at the SS-Lock gate (Task-333
// Code Guide): user edits (if any) replace the draft SS content, both drafts
// are formatted via the CP-48 AutoFix and published into the requirements/
// phase roots, the todo/ drafts are removed, and the whole requirements tree
// is handed to the CP-48 scanner for the final conformance report (T-5).
// Errors leave the gate back in waiting_user_confirm so the confirm can be
// retried. Never called by AI: reachable only from the confirm endpoint.
func (s *InteractiveService) HandleSSLockConfirm(ctx context.Context, runID string, userEdits string) error {
	g := s.ssLockGateFor(runID)
	if g == nil {
		return newAPIErr(http.StatusNotFound, "ss_lock_not_found",
			fmt.Sprintf("no ss-lock gate registered for run %q", runID))
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != ssLockWaiting {
		return newAPIErr(http.StatusConflict, "ss_lock_state",
			fmt.Sprintf("ss-lock gate for run %q is %q, not %q; refusing confirm", runID, g.status, ssLockWaiting))
	}
	g.status = ssLockResuming
	g.userEdits = userEdits

	// 1. Merge user edits into the SS content (CP-60 write-back semantics: a
	// non-empty edit payload replaces the draft).
	ssContent := g.draftSSContent
	if strings.TrimSpace(userEdits) != "" {
		ssContent = userEdits
	}

	// 2. Publish the human-approved SS (AutoFix-formatted) into the phase
	// root; never overwrite an existing approved document (CP-49 constraint).
	publishedSS, err := publishApprovedDoc(g.workspaceRoot, "05-System-Specs", "SS", g.featureBase, ssContent, docscan.PhaseSS)
	if err != nil {
		g.status = ssLockWaiting
		return fmt.Errorf("standardize: publishing approved SS failed: %w", err)
	}
	_ = os.Remove(g.draftSSPath) // draft consumed by publication

	publishedSD := ""
	if g.draftSDContent != "" {
		publishedSD, err = publishApprovedDoc(g.workspaceRoot, "06-System-Tech-Design", "SD", g.featureBase, g.draftSDContent, docscan.PhaseSD)
		if err != nil {
			g.status = ssLockWaiting
			return fmt.Errorf("standardize: publishing approved SD failed: %w", err)
		}
		_ = os.Remove(g.draftSDPath)
	}

	// 3. T-5 handoff: CP-48 scanner over the whole requirements tree produces
	// the final conformance report for the freshly published set.
	report, serr := docscan.ScanDirectory(filepath.Join(g.workspaceRoot, "requirements"))
	if serr != nil {
		log.Printf("[ss-lock] run=%q post-publish conformance scan failed (docs are published): %v", runID, serr)
	}

	g.published = []string{}
	if publishedSS != "" {
		g.published = append(g.published, publishedSS)
	}
	if publishedSD != "" {
		g.published = append(g.published, publishedSD)
	}
	g.scanReport = report
	g.status = ssLockCompleted
	g.events = append(g.events, StandardizeEvent{
		Type:       EventSSLockConfirmed,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		Status:     g.status,
	})
	log.Printf("[ss-lock] run=%q confirmed and published %v", runID, g.published)
	return nil
}

// HandleSSLockReject processes human rejection at the SS-Lock gate: the
// workflow ends cleanly as "cancelled" and nothing is written — the AI-authored
// drafts are removed so no unapproved content is left behind (Task-333 §10
// TestStandardize_UserRejectsSSLock_AbortsWorkflow).
func (s *InteractiveService) HandleSSLockReject(ctx context.Context, runID string) error {
	g := s.ssLockGateFor(runID)
	if g == nil {
		return newAPIErr(http.StatusNotFound, "ss_lock_not_found",
			fmt.Sprintf("no ss-lock gate registered for run %q", runID))
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != ssLockWaiting {
		return newAPIErr(http.StatusConflict, "ss_lock_state",
			fmt.Sprintf("ss-lock gate for run %q is %q, not %q; refusing reject", runID, g.status, ssLockWaiting))
	}
	if g.draftSSPath != "" {
		_ = os.Remove(g.draftSSPath)
	}
	if g.draftSDPath != "" {
		_ = os.Remove(g.draftSDPath)
	}
	g.draftSSContent, g.draftSDContent = "", ""
	g.status = ssLockCancelled
	g.events = append(g.events, StandardizeEvent{
		Type:       EventSSLockRejected,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		Status:     g.status,
	})
	log.Printf("[ss-lock] run=%q rejected by user — workflow cancelled, no files written", runID)
	return nil
}

// publishApprovedDoc AutoFix-formats approved content and writes it into
// requirements/<phaseDir>/<prefix>-<base>.md, picking a -2/-3 suffix instead
// of ever overwriting an existing approved document.
func publishApprovedDoc(workspaceRoot, phaseDir, prefix, base, content, phase string) (string, error) {
	fixed, err := docscan.AutoFixDocument(content, phase)
	if err != nil {
		return "", fmt.Errorf("autofix %s draft: %w", phase, err)
	}
	dir := filepath.Join(workspaceRoot, "requirements", phaseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.md", prefix, base))
	for i := 2; ; i++ {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%s-%d.md", prefix, base, i))
	}
	if err := os.WriteFile(path, []byte(fixed), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ---- HTTP surface ----------------------------------------------------------

// handleSSLockConfirm serves POST /client/workflow-runs/{runId}/confirm — the
// single human resume path of the SS-Lock gate (CP-60 `user.confirm` parity).
// Body: {"action": "approve"|"reject" (default approve), "edits": "..."}.
func (s *InteractiveService) handleSSLockConfirm(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	var body struct {
		Action string `json:"action,omitempty"`
		Edits  string `json:"edits,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	switch strings.ToLower(strings.TrimSpace(body.Action)) {
	case "", "approve", "confirm":
		if err := s.HandleSSLockConfirm(r.Context(), runID, body.Edits); err != nil {
			writeInteractiveError(w, asAPIErr(err))
			return
		}
	case "reject", "cancel":
		if err := s.HandleSSLockReject(r.Context(), runID); err != nil {
			writeInteractiveError(w, asAPIErr(err))
			return
		}
	default:
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request",
			fmt.Sprintf("unknown ss-lock action %q (want approve|reject)", body.Action)))
		return
	}
	// CP-84 (Task-429): the gate mutation above ran under the gate mutex —
	// mark the decision dirty now that it is released (s.mu order is safe here).
	s.markRunRealtimeDirty(runID)
	snap, ok := s.SSLockSnapshot(runID)
	if !ok {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "ss_lock_not_found", runID))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, snap)
}

// handleGetSSLock serves GET /client/workflow-runs/{runId}/ss-lock — the
// SS-Lock modal state source (status, lifecycle events, draft SS content).
func (s *InteractiveService) handleGetSSLock(w http.ResponseWriter, r *http.Request) {
	snap, ok := s.SSLockSnapshot(r.PathValue("runId"))
	if !ok {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "ss_lock_not_found", r.PathValue("runId")))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, snap)
}
