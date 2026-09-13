package runner

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Task-346 (CP-62 P-6 enrichment): the sprint handoff consumes the parked
// decision card (question + the human's choice + remaining options as
// alternatives) and the oracle guard's tampered test files (weakened_tests).
// CP-49 hard ceiling: only verified state is recorded; prose answers leave
// the choice empty and empty sources keep the file byte-identical.

func TestHandoffEnrichment_CardWithChoice(t *testing.T) {
	svc, rs, _ := handoffRun(t)
	svc.emitUserDecisionCard(rs.id, UserDecisionCard{
		Question:    "JWT or Session?",
		Recommended: "opt_jwt",
		Detail:      "Distributed system.",
		Options: []DecisionCardOption{
			{ID: "opt_jwt", Label: "Stateless JWT", Consequence: "No Redis dependency."},
			{ID: "opt_session", Label: "Redis Session", Consequence: "Instant revoke."},
		},
	})
	svc.captureDecisionChoice(rs.id, "opt_session")
	path := svc.emitSprintHandoff(rs)
	data := readHandoffForTest(t, path)
	if !strings.Contains(string(data), "user chose opt_session — JWT or Session?") {
		t.Fatalf("chosen decision missing: %s", data)
	}
	if !strings.Contains(string(data), "Redis Session — Instant revoke.") {
		t.Fatalf("chosen why missing: %s", data)
	}
	var parsed struct {
		Decisions []SprintDecision `json:"decisions"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("handoff must stay valid JSON: %v", err)
	}
	found := false
	for _, dec := range parsed.Decisions {
		if strings.HasPrefix(dec.What, "user chose opt_session") {
			found = true
			if len(dec.Alternatives) != 1 || dec.Alternatives[0] != "Stateless JWT" {
				t.Fatalf("alternatives = %v, want [Stateless JWT]", dec.Alternatives)
			}
		}
	}
	if !found {
		t.Fatalf("chosen decision entry missing: %s", data)
	}
}

func TestHandoffEnrichment_CardWithoutChoiceFallsBackToRecommended(t *testing.T) {
	svc, rs, _ := handoffRun(t)
	svc.emitUserDecisionCard(rs.id, UserDecisionCard{
		Question:    "SQLite or Postgres?",
		Recommended: "opt_sqlite",
		Options: []DecisionCardOption{
			{ID: "opt_sqlite", Label: "SQLite", Consequence: "Embedded."},
			{ID: "opt_pg", Label: "Postgres", Consequence: "Server."},
		},
	})
	path := svc.emitSprintHandoff(rs)
	data := readHandoffForTest(t, path)
	if !strings.Contains(string(data), "recommended: opt_sqlite") {
		t.Fatalf("recommended fallback missing: %s", data)
	}
	if strings.Contains(string(data), "user chose") {
		t.Fatalf("prose feedback must not guess a choice: %s", data)
	}
}

func TestHandoffEnrichment_TamperedTestsRecorded(t *testing.T) {
	svc, rs, _ := handoffRun(t)
	svc.setTamperedTestPaths(rs, []string{"stringutil/reverse_test.go"})
	path := svc.emitSprintHandoff(rs)
	data := readHandoffForTest(t, path)
	if !strings.Contains(string(data), `"path": "stringutil/reverse_test.go"`) && !strings.Contains(string(data), `"path":"stringutil/reverse_test.go"`) {
		t.Fatalf("weakened_tests path missing: %s", data)
	}
	if !strings.Contains(string(data), "oracle guard flagged") {
		t.Fatalf("weakened_tests justification missing: %s", data)
	}
}

func TestHandoffEnrichment_NoSourcesOmitsFields(t *testing.T) {
	svc, rs, _ := handoffRun(t)
	svc.mu.Lock()
	rs.lastFlowVerdicts = nil
	svc.mu.Unlock()
	path := svc.emitSprintHandoff(rs)
	data := readHandoffForTest(t, path)
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("handoff must stay valid JSON: %v", err)
	}
	for _, field := range []string{"decisions", "weakened_tests", "risks", "alternatives"} {
		if _, ok := parsed[field]; ok {
			t.Fatalf("field %q must be omitted with no sources: %s", field, data)
		}
	}
}

func readHandoffForTest(t *testing.T, path string) []byte {
	t.Helper()
	if path == "" {
		t.Fatalf("handoff not written")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return data
}
