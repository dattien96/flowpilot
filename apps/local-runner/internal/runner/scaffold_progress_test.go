package runner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// CA-916 scaffold observability: the AI scaffold turn must be visible like a
// chat run — phase milestones and provider stdout deltas flow through a durable
// per-project feed exposed at GET /client/projects/{id}/scaffold/progress.

func scaffoldProgressFromResponse(t *testing.T, body []byte) ScaffoldProgressSnapshot {
	t.Helper()
	var out ScaffoldProgressSnapshot
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode scaffold progress: %v (%s)", err, body)
	}
	return out
}

func scaffoldPhaseSeq(events []ScaffoldProgressEvent) []string {
	var phases []string
	for _, ev := range events {
		if ev.Kind == "phase" || ev.Kind == "result" {
			phases = append(phases, ev.Phase)
		}
	}
	return phases
}

func scaffoldOutputText(events []ScaffoldProgressEvent) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Kind == "output" {
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}

func TestScaffoldProgress_DispatchStreamsPhaseAndOutputEventsHTTP(t *testing.T) {
	dir := newScaffoldWorkspace(t)
	executor := &recordingScaffoldExecutor{
		onCall: func(call int, req PromptExecutionRequest) (PromptExecutionResult, error) {
			if req.OnStdoutDelta != nil {
				req.OnStdoutDelta("scaffold writing Step 0… ")
				req.OnStdoutDelta("created src/App.tsx")
			}
			return PromptExecutionResult{
				Status:      "success",
				RunID:       fmt.Sprintf("scaffold-run-%d", call),
				ProviderKey: req.ProviderKey,
				ExitCode:    0,
			}, nil
		},
	}
	recipe := realScaffoldRecipe(t, "true")
	svc, srv := newScaffoldTestService(t, dir, "react-native", dispatcherForRecipe(executor, recipe))

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold",
		ScaffoldDispatchRequestBody{WorkingDirectory: dir, Platform: "react-native", ProviderKey: "codex"}, nil)
	if status != http.StatusOK {
		t.Fatalf("dispatch status = %d body=%s", status, body)
	}
	result := scaffoldResultFromResponse(t, body)
	if result.Status != ScaffoldStatusDone {
		t.Fatalf("scaffold status = %q want done (%s)", result.Status, result.Message)
	}

	status, body = doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/progress", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("progress status = %d body=%s", status, body)
	}
	snap := scaffoldProgressFromResponse(t, body)
	if snap.Active {
		t.Fatal("feed still active after dispatch completed")
	}
	if snap.Result == nil || snap.Result.Status != ScaffoldStatusDone {
		t.Fatalf("snapshot result = %+v, want done", snap.Result)
	}
	if len(snap.Events) == 0 {
		t.Fatal("no progress events recorded")
	}
	for i := 1; i < len(snap.Events); i++ {
		if snap.Events[i].Seq <= snap.Events[i-1].Seq {
			t.Fatalf("seq not monotonic: %+v after %+v", snap.Events[i], snap.Events[i-1])
		}
	}
	phases := scaffoldPhaseSeq(snap.Events)
	joined := strings.Join(phases, ",")
	for _, want := range []string{"started", "ai_turn", "done"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing phase %q in %v", want, phases)
		}
	}
	if got := scaffoldOutputText(snap.Events); !strings.Contains(got, "scaffold writing Step 0") || !strings.Contains(got, "created src/App.tsx") {
		t.Fatalf("output text = %q, want both stdout deltas", got)
	}
	_ = svc
}

func TestScaffoldProgress_AfterCursorReturnsOnlyNewerEventsHTTP(t *testing.T) {
	dir := newScaffoldWorkspace(t)
	recipe := realScaffoldRecipe(t, "true")
	_, srv := newScaffoldTestService(t, dir, "react-native", dispatcherForRecipe(&recordingScaffoldExecutor{}, recipe))

	if status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold",
		ScaffoldDispatchRequestBody{WorkingDirectory: dir, Platform: "react-native"}, nil); status != http.StatusOK {
		t.Fatalf("dispatch status = %d body=%s", status, body)
	}
	_, body := doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/progress", nil, nil)
	full := scaffoldProgressFromResponse(t, body)
	if len(full.Events) < 2 {
		t.Fatalf("need >=2 events for cursor test, got %d", len(full.Events))
	}
	cut := full.Events[1].Seq
	_, body = doJSON(t, http.MethodGet, fmt.Sprintf("%s/client/projects/proj-rn/scaffold/progress?after=%d", srv.URL, cut), nil, nil)
	tail := scaffoldProgressFromResponse(t, body)
	for _, ev := range tail.Events {
		if ev.Seq <= cut {
			t.Fatalf("after=%d returned stale event %+v", cut, ev)
		}
	}
	if want := len(full.Events) - 2; len(tail.Events) != want {
		t.Fatalf("tail events = %d, want %d", len(tail.Events), want)
	}
	if tail.NextSeq != full.NextSeq {
		t.Fatalf("nextSeq = %d, want %d", tail.NextSeq, full.NextSeq)
	}
}

func TestScaffoldProgress_SkippedDispatchEmitsResultEventHTTP(t *testing.T) {
	dir := t.TempDir()
	_, srv := newScaffoldTestService(t, dir, "unsupported-os", dispatcherForRecipe(&recordingScaffoldExecutor{}, nil))

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold",
		ScaffoldDispatchRequestBody{WorkingDirectory: dir, Platform: "unsupported-os"}, nil)
	if status != http.StatusOK {
		t.Fatalf("dispatch status = %d body=%s", status, body)
	}
	_, body = doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/progress", nil, nil)
	snap := scaffoldProgressFromResponse(t, body)
	if snap.Active {
		t.Fatal("feed active after skipped dispatch")
	}
	if snap.Result == nil || snap.Result.Status != ScaffoldStatusSkipped {
		t.Fatalf("result = %+v, want skipped", snap.Result)
	}
}

func TestScaffoldProgress_ReplaysPersistedLogAfterHubLossHTTP(t *testing.T) {
	dir := newScaffoldWorkspace(t)
	recipe := realScaffoldRecipe(t, "true")
	_, srv := newScaffoldTestService(t, dir, "react-native", dispatcherForRecipe(&recordingScaffoldExecutor{}, recipe))

	if status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold",
		ScaffoldDispatchRequestBody{WorkingDirectory: dir, Platform: "react-native"}, nil); status != http.StatusOK {
		t.Fatalf("dispatch status = %d body=%s", status, body)
	}
	srv.Close()

	// Simulate runner restart: a brand-new service over the same workspace has
	// no in-memory hub, but the persisted NDJSON log must still replay.
	catalog := newInteractiveCatalog()
	catalog.projects = []Project{{ID: "proj-rn", Name: "App", Path: dir, Platform: "react-native"}}
	svc2 := newInteractiveService(DefaultProviderRegistry(), catalog, nil)
	svc2.AttachRunner(&Runner{workspace: t.TempDir()})
	mux := http.NewServeMux()
	svc2.RegisterInteractiveRoutes(mux)
	srv2 := httptest.NewServer(mux)
	t.Cleanup(srv2.Close)

	status, body := doJSON(t, http.MethodGet, srv2.URL+"/client/projects/proj-rn/scaffold/progress", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("progress status = %d body=%s", status, body)
	}
	snap := scaffoldProgressFromResponse(t, body)
	if snap.Active {
		t.Fatal("replayed feed should not be active")
	}
	phases := scaffoldPhaseSeq(snap.Events)
	if !strings.Contains(strings.Join(phases, ","), "done") {
		t.Fatalf("persisted replay missing terminal phase, events=%v", phases)
	}
}

func TestTailPromptStdout_StreamsAppendsBeforeClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdout.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var mu sync.Mutex
	var got strings.Builder
	stop := tailPromptStdout(t.Context(), path, func(chunk string) {
		mu.Lock()
		got.WriteString(chunk)
		mu.Unlock()
	})

	if _, err := f.WriteString("chunk-one "); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		seen := got.String()
		mu.Unlock()
		if strings.Contains(seen, "chunk-one") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := f.WriteString("chunk-two"); err != nil {
		t.Fatal(err)
	}
	stop()
	mu.Lock()
	final := got.String()
	mu.Unlock()
	if !strings.Contains(final, "chunk-one") || !strings.Contains(final, "chunk-two") {
		t.Fatalf("tailer captured %q, want both chunks (incl. post-stop flush)", final)
	}
}
