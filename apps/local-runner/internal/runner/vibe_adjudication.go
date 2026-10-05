package runner

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// vibeAdjudicationsRel is the durable adjudication ledger: operator answers
// to requirement escalations/questions, recorded as structured records so a
// later leg's verdict cites a durable record id (adj-N) instead of
// interpreting freeform prompt text (CA-1214).
const vibeAdjudicationsRel = ".flowpilot/adjudications.ndjson"

// appendVibeAdjudication projects an operator adjudication (the answer to a
// requirement-class park/question) into a durable workspace ledger.
// CA-1214 (live run-262417): the user's AC-4 deferral existed only as
// freeform pendingAgentContext text — the spec-aligner re-verdicted
// "blocked" because no durable, citable waiver record existed; the doc
// amendments that eventually landed were agent-mediated luck. Every vibe
// adjudication now gets a sequential record id legs and audits can cite.
// Fail-soft: ledger errors only log — never block the resume path.
// Returns the assigned record id ("" on failure).
func (s *InteractiveService) appendVibeAdjudication(runID, taskDocID, feedback string) string {
	s.mu.Lock()
	var cwd string
	if rs := s.runs[runID]; rs != nil {
		cwd = rs.workspaceCwd
	}
	s.mu.Unlock()
	feedback = strings.TrimSpace(feedback)
	if cwd == "" || feedback == "" {
		return ""
	}
	path := filepath.Join(cwd, filepath.FromSlash(vibeAdjudicationsRel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[adjudication] mkdir %s: %v", path, err)
		return ""
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[adjudication] open %s: %v", path, err)
		return ""
	}
	defer f.Close()
	// Sequential id: count existing lines (file is small, append-only).
	seq := 1
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) != "" {
				seq++
			}
		}
	}
	id := fmt.Sprintf("adj-%d", seq)
	rec := map[string]any{
		"id":        id,
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"run":       runID,
		"task_doc":  taskDocID,
		"feedback":  feedback,
		"projected": "operator_adjudication",
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return ""
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		log.Printf("[adjudication] write %s: %v", path, err)
		return ""
	}
	return id
}
