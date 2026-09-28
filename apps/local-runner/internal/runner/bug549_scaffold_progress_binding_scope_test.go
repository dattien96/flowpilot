package runner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"testing"
)

// BUG-549: the Desktop scaffold feed is keyed by projectId only, so a scaffold
// that ran on binding "mac" still renders when the operator selects binding
// "Primary". The progress endpoint must accept an optional workingDirectory
// scope: a filter that does not match the run's workspace returns an empty,
// inactive feed, and a restart-time replay reads the filtered directory's
// persisted log instead of always resolving project.Path (the first binding).

func TestBug549_ProgressFilterHidesOtherWorkspaceFeed(t *testing.T) {
	dirA := newScaffoldWorkspace(t)
	dirB := newScaffoldWorkspace(t)
	recipe := realScaffoldRecipe(t, "true")
	_, srv := newScaffoldTestService(t, dirA, "react-native", dispatcherForRecipe(&recordingScaffoldExecutor{}, recipe))

	// Scaffold runs on binding A's directory.
	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold",
		ScaffoldDispatchRequestBody{WorkingDirectory: dirA, Platform: "react-native", ProviderKey: "codex"}, nil)
	if status != http.StatusOK {
		t.Fatalf("dispatch status = %d body=%s", status, body)
	}

	// Binding-scoped poll for B must not surface A's transcript.
	_, body = doJSON(t, http.MethodGet,
		fmt.Sprintf("%s/client/projects/proj-rn/scaffold/progress?workingDirectory=%s", srv.URL, neturl.QueryEscape(dirB)), nil, nil)
	snapB := scaffoldProgressFromResponse(t, body)
	if len(snapB.Events) != 0 {
		t.Fatalf("filtered feed for dir B returned %d events from dir A's run, want 0", len(snapB.Events))
	}
	if snapB.Result != nil {
		t.Fatalf("filtered feed for dir B returned result %+v from dir A's run, want nil", snapB.Result)
	}

	// The run's own binding still sees the transcript.
	_, body = doJSON(t, http.MethodGet,
		fmt.Sprintf("%s/client/projects/proj-rn/scaffold/progress?workingDirectory=%s", srv.URL, neturl.QueryEscape(dirA)), nil, nil)
	snapA := scaffoldProgressFromResponse(t, body)
	if len(snapA.Events) == 0 {
		t.Fatal("filtered feed for dir A returned no events, want the run's transcript")
	}
	if snapA.Result == nil || snapA.Result.Status != ScaffoldStatusDone {
		t.Fatalf("filtered feed for dir A result = %+v, want done", snapA.Result)
	}

	// No param at all keeps the legacy project-scoped feed (TUI, Projects page).
	_, body = doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/progress", nil, nil)
	if snap := scaffoldProgressFromResponse(t, body); len(snap.Events) == 0 {
		t.Fatal("unfiltered feed returned no events, want the legacy project-level transcript")
	}
}

func TestBug549_ProgressFilterReplaysFilteredWorkspaceTail(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	// Persist a finished transcript under binding B only — the catalog row still
	// points at binding A (project.Path = first binding), which is exactly the
	// multi-binding shape that loses the replay today.
	for i, text := range []string{"AI scaffold turn started", "compiler gate PASS"} {
		ev := ScaffoldProgressEvent{Seq: int64(i + 1), Kind: "phase", Phase: "done", Text: text}
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dirB, ".flowpilot", scaffoldProgressFileName)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	result := ScaffoldProgressEvent{Seq: 3, Kind: "result", Phase: "done",
		Result: &ScaffoldDispatchResult{Status: ScaffoldStatusDone, Message: "done"}}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dirB, ".flowpilot", scaffoldProgressFileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Fresh service (no in-memory hub) whose project.Path is binding A.
	catalog := newInteractiveCatalog()
	catalog.projects = []Project{{ID: "proj-rn", Name: "App", Path: dirA, Platform: "react-native"}}
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, nil)
	svc.AttachRunner(&Runner{workspace: t.TempDir()})
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, body := doJSON(t, http.MethodGet,
		fmt.Sprintf("%s/client/projects/proj-rn/scaffold/progress?workingDirectory=%s", srv.URL, neturl.QueryEscape(dirB)), nil, nil)
	snap := scaffoldProgressFromResponse(t, body)
	if len(snap.Events) == 0 {
		t.Fatal("filtered replay for dir B returned no events — the persisted log under B must be readable")
	}
	if snap.Result == nil || snap.Result.Status != ScaffoldStatusDone {
		t.Fatalf("filtered replay result = %+v, want done", snap.Result)
	}

	// Without a filter the replay still resolves project.Path (dir A) — no file
	// there, so the legacy path stays empty rather than leaking B's transcript.
	_, body = doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/progress", nil, nil)
	if snap := scaffoldProgressFromResponse(t, body); len(snap.Events) != 0 {
		t.Fatalf("unfiltered replay returned %d events, want 0 (project.Path dir A has no log)", len(snap.Events))
	}
}
