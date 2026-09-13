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

// Task-350: takeNextVibeSprintLocked phải reset cross-sprint verified state —
// verdict rows / tampered paths / card+choice / AC cache thuộc về sprint cũ.
func TestHandoffEnrichment_SprintAdvanceResetsVerifiedState(t *testing.T) {
	svc, rs, _ := handoffRun(t)
	svc.mu.Lock()
	rs.vibeTaskPlan = []string{"requirements/08-Task/todo/Task-a.md", "requirements/08-Task/todo/Task-b.md"}
	rs.vibeSprintIndex = 1
	rs.expectedACsCache = []string{"AC-1"}
	rs.expectedACsResolved = true
	rs.decisionCardChosen = "opt_stale"
	svc.mu.Unlock()

	svc.mu.Lock()
	d := svc.takeNextVibeSprintLocked(rs)
	svc.mu.Unlock()
	if !d.Start || d.Sprint != 2 {
		t.Fatalf("decision = %+v, want Start sprint 2", d)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.lastFlowVerdicts != nil || rs.lastTamperedTestPaths != nil || rs.decisionCard != nil ||
		rs.decisionCardChosen != "" || rs.expectedACsCache != nil || rs.expectedACsResolved {
		t.Fatalf("cross-sprint state not reset: verdicts=%d tampered=%d chosen=%q cache=%v",
			len(rs.lastFlowVerdicts), len(rs.lastTamperedTestPaths), rs.decisionCardChosen, rs.expectedACsCache)
	}
}

// Task-351: emitSprintHandoffAt pin sprint index — index advance giữa quyết
// định và ghi file không được ghi nhầm dữ liệu sprint N vào handoff N+1.
func TestHandoffEnrichment_EmitAtPinnedIndex(t *testing.T) {
	svc, rs, cwd := handoffRun(t)
	svc.mu.Lock()
	rs.vibeSprintIndex = 3 // cursor advanced concurrently after the decision
	svc.mu.Unlock()
	path := svc.emitSprintHandoffAt(rs, 2)
	if path == "" {
		t.Fatalf("handoff not written")
	}
	want := cwd + "/requirements/.flowpilot/vibe/handoffs/handoff-sprint-2.yaml"
	if path != want {
		t.Fatalf("path = %q, want %q (pinned sprint 2)", path, want)
	}
}

// Task-352: captureDecisionChoice khớp cả LABEL (không chỉ id) — đúng hợp đồng
// TUI/desktop gửi label khi user trả lời bằng tên option.
func TestHandoffEnrichment_CaptureMatchesLabel(t *testing.T) {
	svc, rs, _ := handoffRun(t)
	svc.emitUserDecisionCard(rs.id, UserDecisionCard{
		Question: "DB?",
		Options: []DecisionCardOption{
			{ID: "opt_sqlite", Label: "SQLite", Consequence: "Embedded."},
		},
	})
	svc.captureDecisionChoice(rs.id, "  sqlite ") // case-insensitive + trim
	svc.mu.Lock()
	chosen := rs.decisionCardChosen
	svc.mu.Unlock()
	if chosen != "opt_sqlite" {
		t.Fatalf("chosen = %q, want opt_sqlite via label match", chosen)
	}
}

// Task-352: production integration — decision_card payload qua applyFlowControl
// (không qua helper) phải emit event + stamp run state; payload hỏng → prose stays.
func TestHandoffEnrichment_ApplyFlowControlEmitsCard(t *testing.T) {
	svc, rs, _ := handoffRun(t)
	in := FlowControlInput{
		Status: "escalate",
		Payload: map[string]any{
			"decision_card": map[string]any{
				"question": "Go left or right?",
				"options": []any{
					map[string]any{"id": "left", "label": "Left", "consequence": "Faster."},
					map[string]any{"id": "right", "label": "Right", "consequence": "Safer."},
				},
				"recommended": "left",
			},
		},
	}
	if _, err := svc.applyFlowControl(rs.id, in); err != nil {
		t.Fatalf("applyFlowControl: %v", err)
	}
	svc.mu.Lock()
	card := rs.decisionCard
	var found bool
	for _, ev := range rs.events {
		if ev.Type == EventUserDecisionCardRequested {
			found = true
		}
	}
	svc.mu.Unlock()
	if card == nil || card.Question != "Go left or right?" {
		t.Fatalf("card not stamped from payload: %+v", card)
	}
	if !found {
		t.Fatalf("user_decision_card_requested not emitted from applyFlowControl")
	}
}
