package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CA-1214 (live run-262417): operator adjudications answered on a question/
// escalate card existed only as freeform pendingAgentContext text — the
// spec-aligner re-verdicted "blocked" because no durable, citable waiver
// record existed. appendVibeAdjudication projects the answer into
// .flowpilot/adjudications.ndjson with a sequential record id legs cite.
func TestCA1214_AdjudicationLandsInDurableLedger(t *testing.T) {
	svc, _ := newTestServer(t)
	ws := t.TempDir()
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].workspaceCwd = ws
	svc.mu.Unlock()

	id1 := svc.appendVibeAdjudication(parent.RunID, "Task-041", "defer AC-4 to CP-10 device gate")
	if id1 != "adj-1" {
		t.Fatalf("first adjudication id = %q, want adj-1", id1)
	}
	id2 := svc.appendVibeAdjudication(parent.RunID, "Task-041", "waiver stands for instrumented race")
	if id2 != "adj-2" {
		t.Fatalf("second adjudication id = %q, want adj-2", id2)
	}

	data, rerr := os.ReadFile(filepath.Join(ws, filepath.FromSlash(vibeAdjudicationsRel)))
	if rerr != nil {
		t.Fatalf("ledger missing: %v", rerr)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("ledger has %d records, want 2", len(lines))
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("record not valid json: %v", err)
	}
	if rec["id"] != "adj-1" || rec["task_doc"] != "Task-041" || rec["run"] != parent.RunID {
		t.Fatalf("record fields wrong: %v", rec)
	}
	if !strings.Contains(rec["feedback"].(string), "CP-10") {
		t.Fatalf("feedback not recorded: %v", rec["feedback"])
	}
}

// Missing workspace / empty feedback is a no-op — never blocks the resume path.
func TestCA1214_AdjudicationFailSoft(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if id := svc.appendVibeAdjudication(parent.RunID, "Task-041", "some answer"); id != "" {
		t.Fatalf("no workspace → must no-op, got %q", id)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].workspaceCwd = t.TempDir()
	svc.mu.Unlock()
	if id := svc.appendVibeAdjudication(parent.RunID, "Task-041", "   "); id != "" {
		t.Fatalf("empty feedback → must no-op, got %q", id)
	}
}
