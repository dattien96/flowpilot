package runner

// CP-81 Task-415: protected-work inventory + durable stop-all for the shared
// runner lifecycle. LiveWorkSnapshot feeds the lifecycle manager's
// WorkSnapshot provider; StopAllForSystemAction is the confirmed-drain
// executor that reuses the existing CA-913/CP-51 durable stop fencing
// (stopAgentLoop) — it deliberately does not invent a second run-state model.

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"flowpilot-runner/internal/lifecycle"
)

// StopAllResult summarizes a confirmed drain: which roots were stopped, which
// scaffold turns were cancelled, and any per-item errors (best-effort — a
// single wedged run must not leave the rest running).
type StopAllResult struct {
	StoppedRoots       []string `json:"stoppedRoots"`
	CancelledScaffolds []string `json:"cancelledScaffolds"`
	Errors             []string `json:"errors,omitempty"`
}

// AttachLifecycle lets the interactive service consult the lifecycle manager —
// currently the drain gate that rejects new work once draining begins (CP-81
// D-3). nil is fine: unmanaged contexts never drain.
func (s *InteractiveService) AttachLifecycle(m *lifecycle.Manager) {
	s.mu.Lock()
	s.lifecycleMgr = m
	s.mu.Unlock()
}

// drainingErr returns the typed 409 when the attached lifecycle manager is in
// a draining/stopped phase; nil when unmanaged or still serving.
func (s *InteractiveService) drainingErr() *apiErr {
	s.mu.Lock()
	m := s.lifecycleMgr
	s.mu.Unlock()
	if m == nil {
		return nil
	}
	snap, err := m.Snapshot(context.Background())
	if err != nil {
		return nil
	}
	if snap.Phase.IsDraining() {
		return newAPIErr(http.StatusConflict, lifecycle.ErrRunnerDraining,
			fmt.Sprintf("runner is %s; new work rejected", snap.Phase))
	}
	return nil
}

// LiveWorkSnapshot enumerates every unit of protected work the lifecycle
// manager must block idle shutdown on (SD-28 R-5 kind enumeration):
// in-flight turns, post-turn gates, inline flow nodes, running agent loops,
// in-flight scaffolds. Warm provider sessions/pools are NOT work — they hold
// no execution and are reclaimed by CleanupSessions, not by drain.
func (s *InteractiveService) LiveWorkSnapshot(ctx context.Context) (lifecycle.WorkloadSnapshot, error) {
	_ = ctx // inventory is in-memory; ctx reserved for future bounded I/O
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]lifecycle.WorkloadItem, 0)
	loopSeen := map[string]bool{}

	for _, rs := range s.runs {
		provider := string(rs.providerKey)
		if rs.turnInFlight || rs.turnCancel != nil {
			items = append(items, lifecycle.WorkloadItem{
				Kind:        "turn",
				RunID:       rs.id,
				ProjectID:   rs.projectID,
				ProviderKey: provider,
				Cancellable: true,
				Detail:      "provider turn in flight",
			})
		}
		if rs.postTurnGateCancel != nil {
			items = append(items, lifecycle.WorkloadItem{
				Kind:        "flow",
				RunID:       rs.id,
				ProjectID:   rs.projectID,
				ProviderKey: provider,
				Cancellable: true,
				Detail:      "post-turn gate in flight",
			})
		}
		if rs.flowInlineCancel != nil {
			items = append(items, lifecycle.WorkloadItem{
				Kind:        "flow",
				RunID:       rs.id,
				ProjectID:   rs.projectID,
				ProviderKey: provider,
				Cancellable: true,
				Detail:      "inline flow node in flight",
			})
		}
		// Agent loop: root runs only; running/paused loops hold pending work
		// the next client turn would resume — count them as protected so a
		// detached runner does not silently drop a live orchestration.
		if rs.parentRunID == "" && !loopSeen[rs.id] {
			if st := s.agentOrchestrator.loopStateFor(rs.id); st.Status == "running" || st.Status == "paused" {
				loopSeen[rs.id] = true
				items = append(items, lifecycle.WorkloadItem{
					Kind:        "agent",
					RunID:       rs.id,
					ProjectID:   rs.projectID,
					ProviderKey: provider,
					Cancellable: true,
					Detail:      fmt.Sprintf("agent loop %s round=%d", st.Status, st.Round),
				})
			}
		}
	}

	for projectID := range s.scaffoldInFlight {
		items = append(items, lifecycle.WorkloadItem{
			Kind:        "scaffold",
			ProjectID:   projectID,
			Cancellable: true,
			Detail:      "scaffold turn in flight",
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].RunID < items[j].RunID
	})
	return lifecycle.WorkloadSnapshot{Items: items}, nil
}

// StopAllForSystemAction durably stops every protected workload before the
// process exits (CP-81 forced drain). It reuses stopAgentLoop per root so the
// durable Stop fence, gate epochs, child terminalization, and persistence
// rules all stay identical to a user-facing Stop. Scaffold turns are cancelled
// via their armed cancel funcs.
func (s *InteractiveService) StopAllForSystemAction(ctx context.Context, reason string) (StopAllResult, error) {
	res := StopAllResult{}

	// Collect roots to stop: any non-terminal or actively-executing root run,
	// plus the root ancestor of any active child run.
	s.mu.Lock()
	roots := map[string]bool{}
	activeMarkers := func(rs *interactiveRun) bool {
		return rs.turnInFlight || rs.turnCancel != nil ||
			rs.postTurnGateCancel != nil || rs.flowInlineCancel != nil
	}
	for _, rs := range s.runs {
		isActive := activeMarkers(rs) ||
			(rs.status != RunStatusCompleted && rs.status != RunStatusFailed && rs.status != RunStatusCancelled)
		if !isActive {
			continue
		}
		root := rs
		for depth := 0; depth < 16 && root.parentRunID != ""; depth++ {
			parent, ok := s.runs[root.parentRunID]
			if !ok {
				break
			}
			root = parent
		}
		roots[root.id] = true
	}
	scaffoldCancels := make([]context.CancelFunc, 0, len(s.scaffoldCancels))
	for _, cancel := range s.scaffoldCancels {
		scaffoldCancels = append(scaffoldCancels, cancel)
	}
	scaffoldIDs := make([]string, 0, len(s.scaffoldCancels))
	for projectID := range s.scaffoldCancels {
		scaffoldIDs = append(scaffoldIDs, projectID)
	}
	s.mu.Unlock()

	for _, cancel := range scaffoldCancels {
		cancel()
	}
	res.CancelledScaffolds = scaffoldIDs

	rootIDs := make([]string, 0, len(roots))
	for id := range roots {
		rootIDs = append(rootIDs, id)
	}
	sort.Strings(rootIDs)
	for _, runID := range rootIDs {
		_, stopErr := s.stopAgentLoop(runID)
		if stopErr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", runID, stopErr.msg))
			log.Printf("[lifecycle] stop-all run=%s failed: %s", runID, stopErr.msg)
			continue
		}
		res.StoppedRoots = append(res.StoppedRoots, runID)
	}
	log.Printf("[lifecycle] stop-all reason=%q roots=%d scaffolds=%d errors=%d",
		reason, len(res.StoppedRoots), len(res.CancelledScaffolds), len(res.Errors))

	if len(res.Errors) > 0 {
		return res, fmt.Errorf("stop-all incomplete: %s", strings.Join(res.Errors, "; "))
	}
	return res, nil
}

// armScaffoldCancel registers the cancel func for an in-flight scaffold so
// StopAllForSystemAction can terminate it. Call after claimScaffold succeeds.
func (s *InteractiveService) armScaffoldCancel(projectID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scaffoldCancels == nil {
		s.scaffoldCancels = map[string]context.CancelFunc{}
	}
	s.scaffoldCancels[projectID] = cancel
}

// scaffoldBusyProjectIDs returns in-flight scaffold claims — the live-busy
// surface the TUI close dialog and lifecycle inventory share.
func (s *InteractiveService) scaffoldBusyProjectIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.scaffoldInFlight))
	for id := range s.scaffoldInFlight {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
