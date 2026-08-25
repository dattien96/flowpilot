package changecontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// CP-43 F1 gap: the verify step requires BOTH frozen_contracts.ndjson and
// frozen_contract_events.ndjson to carry records (feature_key, intent,
// declared_paths, version on the payload; contract_id + status on the event).
// Existing tests exercised the store API (SaveFrozen / GetFrozenForStep /
// AppendStatus-rejections) but none asserted the durable FILE contents the
// operator checks in the sandbox. Additive — preflight_test.go untouched.

func TestFrozenStoreFilesContainF1Fields(t *testing.T) {
	ws := t.TempDir()
	store, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	rec := freezeTestRecord("run-f1", "coder", 1, "f1-abc")
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus(rec.ContractID, ContractStatusFrozen, "", time.Now()); err != nil {
		t.Fatal(err)
	}

	contractsRaw, err := os.ReadFile(filepath.Join(ws, ".flowpilot", "contracts", "frozen_contracts.ndjson"))
	if err != nil {
		t.Fatalf("read frozen_contracts.ndjson: %v", err)
	}
	eventsRaw, err := os.ReadFile(filepath.Join(ws, ".flowpilot", "contracts", "frozen_contract_events.ndjson"))
	if err != nil {
		t.Fatalf("read frozen_contract_events.ndjson: %v", err)
	}

	var got FrozenContractRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(contractsRaw))), &got); err != nil {
		t.Fatalf("contracts line must be a valid record: %v", err)
	}
	if got.ContractID != "f1-abc" || got.Version != 1 || got.RunID != "run-f1" || got.CoderStepID != "coder" {
		t.Fatalf("frozen record identity fields = %+v", got)
	}
	if got.FeatureKey != "calc-core" || got.Intent != "fix" {
		t.Fatalf("frozen record feature_key/intent = %q/%q", got.FeatureKey, got.Intent)
	}
	if len(got.DeclaredPaths) != 1 || got.DeclaredPaths[0] != "src/calc.go" {
		t.Fatalf("frozen record declared_paths = %v", got.DeclaredPaths)
	}

	var ev ContractStatusEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(eventsRaw))), &ev); err != nil {
		t.Fatalf("events line must be a valid status event: %v", err)
	}
	if ev.ContractID != "f1-abc" || ev.Status != ContractStatusFrozen {
		t.Fatalf("status event = %+v, want contract_id=f1-abc status=frozen", ev)
	}
}

func TestFrozenStoreEventsAppendInOrder(t *testing.T) {
	ws := t.TempDir()
	store, err := NewFrozenStore(ws)
	if err != nil {
		t.Fatal(err)
	}
	rec := freezeTestRecord("run-f1", "coder", 1, "f1-abc")
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	at1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	at2 := at1.Add(time.Minute)
	if err := store.AppendStatus(rec.ContractID, ContractStatusFrozen, "", at1); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus(rec.ContractID, ContractStatusAccepted, "flow done", at2); err != nil {
		t.Fatal(err)
	}

	eventsRaw, err := os.ReadFile(filepath.Join(ws, ".flowpilot", "contracts", "frozen_contract_events.ndjson"))
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(eventsRaw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("events file must append one line per status, got %d lines: %q", len(lines), eventsRaw)
	}
	var e1, e2 ContractStatusEvent
	if err := json.Unmarshal([]byte(lines[0]), &e1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &e2); err != nil {
		t.Fatal(err)
	}
	if e1.Status != ContractStatusFrozen || e2.Status != ContractStatusAccepted {
		t.Fatalf("event order = %s, %s; want frozen then accepted", e1.Status, e2.Status)
	}
	if e1.At.After(e2.At) {
		t.Fatal("event timestamps must be append-ordered")
	}
	if e2.Reason != "flow done" {
		t.Fatalf("accepted event reason = %q, want flow done", e2.Reason)
	}
}