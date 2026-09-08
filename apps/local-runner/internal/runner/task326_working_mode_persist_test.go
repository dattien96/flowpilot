package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func localStoreSvc(t *testing.T) (*InteractiveService, *httptest.Server, *localFileSessionStore) {
	t.Helper()
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	return svc, srv, store
}

func persistStart(t *testing.T, srvURL string, in StartRunInput, client string) string {
	t.Helper()
	status, body := doJSON(t, "POST", srvURL+"/client/workflow-runs", in, xClient(client))
	if status != http.StatusOK {
		t.Fatalf("start status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil || h.RunID == "" {
		t.Fatalf("handle body=%s err=%v", body, err)
	}
	return h.RunID
}

// Scenario: stamped vibe survives local store round-trip.
func TestPersist_VibeModeRoundTrip(t *testing.T) {
	svc, srv, store := localStoreSvc(t)
	_ = svc
	id := persistStart(t, srv.URL, StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", FlowRef: "vibe-ingest",
	}, "desktop")
	st, ok, err := store.GetProviderSession(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("get session ok=%v err=%v", ok, err)
	}
	if st.WorkingMode != workingmode.Vibe {
		t.Fatalf("store WorkingMode=%q, want vibe", st.WorkingMode)
	}
}

// Scenario: omitted mode persists as dev.
func TestPersist_MissingModePersistsDev(t *testing.T) {
	_, srv, store := localStoreSvc(t)
	id := persistStart(t, srv.URL, StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, FlowRef: "task-harness",
	}, "tui")
	st, ok, err := store.GetProviderSession(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("get session ok=%v err=%v", ok, err)
	}
	if st.WorkingMode != workingmode.Dev {
		t.Fatalf("store WorkingMode=%q, want dev", st.WorkingMode)
	}
}

// Scenario: legacy run JSON without the field loads as dev.
func TestPersist_LegacyRecordDefaultsDev(t *testing.T) {
	store, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := ProviderSessionState{RunID: "run-legacy", ProjectID: "proj", Status: RunStatusIdle}
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetProviderSession(context.Background(), "run-legacy")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got.WorkingMode != "" && got.WorkingMode != workingmode.Dev {
		t.Fatalf("legacy WorkingMode=%q", got.WorkingMode)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs, apiErr := svc.reconstructRun(got)
	if apiErr != nil {
		t.Fatalf("reconstruct: %s", apiErr.msg)
	}
	mode := rs.workingMode
	if mode == "" {
		mode = workingmode.Dev
	}
	if mode != workingmode.Dev {
		t.Fatalf("reconstructed mode=%q", mode)
	}
}

// Scenario: reopen after restart keeps stamped vibe.
func TestPersist_RestartReplaysStampedMode(t *testing.T) {
	_, srv, store := localStoreSvc(t)
	id := persistStart(t, srv.URL, StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", FlowRef: "vibe-ingest",
	}, "tui")
	svc2 := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	st, ok, err := store.GetProviderSession(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	rs, apiErr := svc2.reconstructRun(st)
	if apiErr != nil {
		t.Fatalf("reconstruct: %s", apiErr.msg)
	}
	if rs.workingMode != workingmode.Vibe {
		t.Fatalf("replay WorkingMode=%q, want vibe", rs.workingMode)
	}
}

// Scenario: flipping session default does not mutate the live run.
func TestPersist_SessionToggleDoesNotMutateLiveRun(t *testing.T) {
	svc, srv, _ := localStoreSvc(t)
	id := persistStart(t, srv.URL, StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, FlowRef: "task-harness",
	}, "tui")
	// Session default flip is client-side; the live run must stay dev.
	svc.mu.Lock()
	got := svc.runs[id].workingMode
	svc.mu.Unlock()
	if got != workingmode.Dev {
		t.Fatalf("live run mode=%q after session default would flip", got)
	}
}
