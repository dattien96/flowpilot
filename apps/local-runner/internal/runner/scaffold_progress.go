package runner

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CP-68 scaffold observability (Task-385 follow-up): the AI Scaffold Turn runs
// synchronously inside Dispatch, which used to leave TUI/Desktop blind until the
// final ScaffoldDispatchResult. This file adds a per-project progress feed —
// phase milestones plus live provider-stdout deltas — persisted to
// <workspace>/.flowpilot/scaffold-progress.ndjson so a late-joining or
// post-restart client can still replay what the AI did.
//
// Provider parity: output deltas come from tailing the provider's stdout
// artifact, which every file-stdout provider (claude/codex/grok/opencode/devin)
// produces identically. Gemini's Agy capture buffers stdout in memory, so its
// deltas arrive as a single flush at turn end — phases still report liveness.

const (
	scaffoldProgressKeep       = 400
	scaffoldProgressFileName   = "scaffold-progress.ndjson"
	scaffoldProgressPhaseStart = "started"
)

// ScaffoldProgressEvent is one observable scaffold moment. Kind distinguishes
// milestones ("phase"), streamed AI text ("output") and the terminal "result"
// marker. Seq is a per-project monotonically increasing cursor for `after`
// polling — it survives buffer trims so clients never replay old events.
type ScaffoldProgressEvent struct {
	Seq     int64                   `json:"seq"`
	Time    string                  `json:"time"`
	Kind    string                  `json:"kind"`
	Phase   string                  `json:"phase"`
	Attempt int                     `json:"attempt,omitempty"`
	Text    string                  `json:"text,omitempty"`
	Result  *ScaffoldDispatchResult `json:"result,omitempty"`
}

// ScaffoldProgressSnapshot is GET /client/projects/{id}/scaffold/progress.
type ScaffoldProgressSnapshot struct {
	ProjectID string                  `json:"projectId"`
	Active    bool                    `json:"active"`
	Phase     string                  `json:"phase,omitempty"`
	Attempt   int                     `json:"attempt,omitempty"`
	Events    []ScaffoldProgressEvent `json:"events"`
	NextSeq   int64                   `json:"nextSeq"`
	Result    *ScaffoldDispatchResult `json:"result,omitempty"`
}

// scaffoldProgressHub is the in-memory ring for one project's feed. Workspace is
// captured at begin so emits never need to re-resolve the project directory.
type scaffoldProgressHub struct {
	workspace string
	events    []ScaffoldProgressEvent
	nextSeq   int64
	// runStartSeq is the seq of the current run's first event. nextSeq never
	// resets across runs, so hubFloor > runStartSeq is the only true signal that
	// the ring trimmed THIS run's head.
	runStartSeq int64
	active      bool
	result      *ScaffoldDispatchResult
}

// beginScaffoldProgress starts a fresh feed run: clears the previous run's
// buffer, marks the hub active and emits the `started` phase. Seq keeps
// increasing across runs so a client mid-poll never sees a seq go backwards.
func (s *InteractiveService) beginScaffoldProgress(projectID, workspace string) {
	s.mu.Lock()
	if s.scaffoldProgress == nil {
		s.scaffoldProgress = map[string]*scaffoldProgressHub{}
	}
	hub := s.scaffoldProgress[projectID]
	if hub == nil {
		hub = &scaffoldProgressHub{nextSeq: 1}
		s.scaffoldProgress[projectID] = hub
	}
	hub.workspace = workspace
	hub.events = nil
	hub.active = true
	hub.result = nil
	hub.runStartSeq = hub.nextSeq
	s.mu.Unlock()
	s.emitScaffoldProgress(projectID, "phase", scaffoldProgressPhaseStart, 0, "AI scaffold turn started")
}

// emitScaffoldProgress appends one event and persists it to the workspace
// progress log. Lock-free ordering is guaranteed by doing seq assignment under
// s.mu; the NDJSON append happens outside the lock so disk latency never blocks
// the dispatcher or subscribers.
func (s *InteractiveService) emitScaffoldProgress(projectID, kind, phase string, attempt int, text string) {
	s.emitScaffoldProgressEvent(projectID, ScaffoldProgressEvent{
		Kind:    kind,
		Phase:   phase,
		Attempt: attempt,
		Text:    text,
	})
}

func (s *InteractiveService) emitScaffoldProgressEvent(projectID string, ev ScaffoldProgressEvent) {
	s.mu.Lock()
	hub := s.scaffoldProgress[projectID]
	if hub == nil {
		s.mu.Unlock()
		return
	}
	ev.Seq = hub.nextSeq
	hub.nextSeq++
	ev.Time = time.Now().UTC().Format(time.RFC3339Nano)
	hub.events = append(hub.events, ev)
	if len(hub.events) > scaffoldProgressKeep {
		hub.events = append([]ScaffoldProgressEvent(nil), hub.events[len(hub.events)-scaffoldProgressKeep:]...)
	}
	if ev.Result != nil {
		hub.result = ev.Result
	}
	workspace := hub.workspace
	s.mu.Unlock()

	if workspace != "" {
		appendScaffoldProgressLine(workspace, ev)
	}
}

// endScaffoldProgress emits the terminal result event and marks the feed
// inactive. The POST handler result remains authoritative; this mirrors it into
// the feed so pollers learn the outcome through the same channel.
func (s *InteractiveService) endScaffoldProgress(projectID string, result *ScaffoldDispatchResult) {
	s.mu.Lock()
	hub := s.scaffoldProgress[projectID]
	if hub != nil {
		hub.active = false
	}
	s.mu.Unlock()
	if result == nil {
		// Dispatch returned a transport error: still close the feed so pollers
		// never hang on an "active" feed that will never produce a result.
		result = &ScaffoldDispatchResult{Status: ScaffoldStatusError, Message: "scaffold: dispatch failed"}
	}
	s.emitScaffoldProgressEvent(projectID, ScaffoldProgressEvent{
		Kind:   "result",
		Phase:  result.Status,
		Text:   result.Message,
		Result: result,
	})
}

// scaffoldProgressSnapshot returns events with seq > after plus the feed's
// current head state. When the hub is empty (runner restarted, or a dispatch
// from before this feature ran), the persisted NDJSON tail is replayed so the
// client still sees the last run's transcript.
func (s *InteractiveService) scaffoldProgressSnapshot(projectID string, after int64) ScaffoldProgressSnapshot {
	snap := ScaffoldProgressSnapshot{ProjectID: projectID, Events: []ScaffoldProgressEvent{}}
	var hubFloor, runStartSeq int64
	s.mu.Lock()
	hub := s.scaffoldProgress[projectID]
	if hub != nil {
		runStartSeq = hub.runStartSeq
		snap.Active = hub.active
		snap.NextSeq = hub.nextSeq
		snap.Result = hub.result
		if len(hub.events) > 0 {
			hubFloor = hub.events[0].Seq
		}
		for _, ev := range hub.events {
			if ev.Seq > after {
				snap.Events = append(snap.Events, ev)
			}
		}
		for i := len(hub.events) - 1; i >= 0; i-- {
			if hub.events[i].Kind == "phase" {
				snap.Phase = hub.events[i].Phase
				snap.Attempt = hub.events[i].Attempt
				break
			}
		}
	}
	s.mu.Unlock()

	if hub == nil || len(hub.events) == 0 {
		if persisted := s.loadScaffoldProgressTail(projectID); len(persisted) > 0 {
			filtered := persisted[:0]
			for _, ev := range persisted {
				if ev.Seq > after {
					filtered = append(filtered, ev)
				}
			}
			snap.Events = filtered
			snap.NextSeq = persisted[len(persisted)-1].Seq + 1
			for i := len(persisted) - 1; i >= 0; i-- {
				if persisted[i].Kind == "phase" {
					snap.Phase = persisted[i].Phase
					snap.Attempt = persisted[i].Attempt
					break
				}
			}
			for i := len(persisted) - 1; i >= 0; i-- {
				if persisted[i].Result != nil {
					snap.Result = persisted[i].Result
					break
				}
			}
		}
		return snap
	}

	// Ring-trim gap: the in-memory hub keeps only the last ~400 events. When the
	// caller's cursor sits below the retained floor AND the trim dropped events
	// from THIS run (hubFloor > runStartSeq), prepend the persisted log for the
	// missing head so a late joiner still sees the full transcript.
	if hubFloor > runStartSeq && after < hubFloor {
		if persisted := s.loadScaffoldProgressTail(projectID); len(persisted) > 0 {
			head := make([]ScaffoldProgressEvent, 0, len(persisted)+len(snap.Events))
			for _, ev := range persisted {
				if ev.Seq > after && ev.Seq < hubFloor && ev.Seq >= runStartSeq {
					head = append(head, ev)
				}
			}
			snap.Events = append(head, snap.Events...)
		}
	}
	return snap
}

// loadScaffoldProgressTail replays the persisted NDJSON log for a project whose
// hub is empty. Best-effort: an unreadable or unresolvable workspace yields nil.
func (s *InteractiveService) loadScaffoldProgressTail(projectID string) []ScaffoldProgressEvent {
	project, ok := s.lookupProject(projectID)
	if !ok {
		return nil
	}
	dir, err := s.resolveEngineWorkingDirectory(strings.TrimSpace(project.Path))
	if err != nil || dir == "" {
		return nil
	}
	// Read more than the in-memory ring so the merge path can recover a trimmed
	// transcript head; NDJSON lines are small so a few thousand is cheap.
	return readScaffoldProgressTail(filepath.Join(dir, ".flowpilot", scaffoldProgressFileName), scaffoldProgressKeep*8)
}

// appendScaffoldProgressLine writes one event as an NDJSON line. The log lives
// beside scaffold-status.json so it follows the workspace, not the runner.
func appendScaffoldProgressLine(workspace string, ev ScaffoldProgressEvent) {
	path := filepath.Join(workspace, ".flowpilot", scaffoldProgressFileName)
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

// readScaffoldProgressTail returns the last `limit` events from the log.
func readScaffoldProgressTail(path string, limit int) []ScaffoldProgressEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var events []ScaffoldProgressEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev ScaffoldProgressEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
		if len(events) > limit {
			events = events[len(events)-limit:]
		}
	}
	return events
}

// handleScaffoldProgress serves GET /client/projects/{projectId}/scaffold/progress.
func (s *InteractiveService) handleScaffoldProgress(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	var after int64
	if v := r.URL.Query().Get("after"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			after = n
		}
	}
	writeInteractiveJSON(w, http.StatusOK, s.scaffoldProgressSnapshot(projectID, after))
}
