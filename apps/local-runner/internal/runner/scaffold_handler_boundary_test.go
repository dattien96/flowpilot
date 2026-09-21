package runner

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// CP-68 scaffold review (M1): create-project engine init keeps CA-890's
// soft-skip semantics for every resolve error EXCEPT the boundary gate — only
// working_directory_outside_boundary becomes a 400-after-create.

func TestCreateProject_SoftSkipsEngineInitForNonexistentAbsoluteDirectory(t *testing.T) {
	// A nonexistent ABSOLUTE directory is not a boundary escape: the project is
	// still created (201) and engine init is skipped, exactly as before CP-68.
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
	_, srv := newScaffoldTestService(t, t.TempDir(), "react-native", dispatcher)
	dir := filepath.Join(t.TempDir(), "does-not-exist-yet")

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects", CreateProjectInput{
		Name:          "Future App",
		DirectoryPath: dir,
		Platform:      "react-native",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("status = %d body=%s, want 201 soft-skip (CA-890 semantics)", status, body)
	}

	// Soft skip means no engine init state and no scaffold turn, even async.
	time.Sleep(200 * time.Millisecond)
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want 0 — init was soft-skipped", executor.count())
	}
	if _, err := os.Stat(filepath.Join(dir, ".flowpilot", "engine-init.json")); !os.IsNotExist(err) {
		t.Fatalf("engine-init.json should not exist for a soft-skipped init: %v", err)
	}
}

func TestCreateProject_RejectsEscapingRelativeDirectoryWithBoundaryCode(t *testing.T) {
	// A RELATIVE path containing ".." that climbs out of the process cwd is the
	// one resolve error that stays a 400-after-create — and the project row was
	// still created (the gate is after-create by design).
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
	_, srv := newScaffoldTestService(t, t.TempDir(), "react-native", dispatcher)

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects", CreateProjectInput{
		Name:          "Escaping App",
		DirectoryPath: filepath.Join("..", "..", "..", "..", "..", "outside-runner-root"),
		Platform:      "react-native",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400 for a cwd-escaping relative path", status, body)
	}
	var errBody struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errBody); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, body)
	}
	if errBody.Error.Code != "working_directory_outside_boundary" {
		t.Fatalf("error code = %q, want working_directory_outside_boundary (%s)", errBody.Error.Code, body)
	}

	time.Sleep(200 * time.Millisecond)
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want 0 — rejected before any AI call", executor.count())
	}
}

func TestCreateProject_AutoTriggerIsDebouncedPerProject(t *testing.T) {
	// S6: two autoTriggerScaffold calls for the SAME project must spawn exactly
	// one background AI turn. The fake executor blocks until released so the
	// first claim is provably still in flight when the second trigger fires.
	release := make(chan struct{})
	executor := &recordingScaffoldExecutor{
		onCall: func(_ int, req PromptExecutionRequest) (PromptExecutionResult, error) {
			<-release
			return PromptExecutionResult{
				Status:      "success",
				RunID:       "scaffold-run-1",
				ProviderKey: req.ProviderKey,
				ExitCode:    0,
			}, nil
		},
	}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
	svc, _ := newScaffoldTestService(t, t.TempDir(), "react-native", dispatcher)
	dir := t.TempDir()

	svc.autoTriggerScaffold("proj-rn", dir, "react-native", "")

	// Wait until the first turn is actually executing (claim held), then fire a
	// second trigger for the same project.
	deadline := time.Now().Add(5 * time.Second)
	for executor.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if executor.count() != 1 {
		close(release)
		t.Fatalf("executor calls = %d, want the first auto-trigger running", executor.count())
	}
	svc.autoTriggerScaffold("proj-rn", dir, "react-native", "")
	time.Sleep(200 * time.Millisecond)
	close(release)

	// Let the released turn finish, then confirm the claim was released and the
	// executor still saw exactly one call.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		inFlight := svc.scaffoldInFlight["proj-rn"]
		svc.mu.Unlock()
		if !inFlight {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if executor.count() != 1 {
		t.Fatalf("executor calls = %d, want exactly 1 — the duplicate trigger must be debounced", executor.count())
	}
	svc.mu.Lock()
	released := !svc.scaffoldInFlight["proj-rn"]
	svc.mu.Unlock()
	if !released {
		t.Fatal("scaffoldInFlight claim was not released after the turn finished")
	}

	// A trigger AFTER completion is a new decision and is allowed through. It
	// uses a fresh workspace so the replay guard (scaffold-status.json written
	// by the first turn) does not skip it before the executor.
	svc.autoTriggerScaffold("proj-rn", t.TempDir(), "react-native", "")
	deadline = time.Now().Add(5 * time.Second)
	for executor.count() == 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if executor.count() != 2 {
		t.Fatalf("executor calls = %d, want 2 once the first turn completed", executor.count())
	}

	// Drain the second goroutine before teardown so t.TempDir cleanup never
	// races an in-flight scaffold-status.json write.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		inFlight := svc.scaffoldInFlight["proj-rn"]
		svc.mu.Unlock()
		if !inFlight {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	svc.mu.Lock()
	stillInFlight := svc.scaffoldInFlight["proj-rn"]
	svc.mu.Unlock()
	if stillInFlight {
		t.Fatal("second auto-trigger did not finish within the deadline")
	}
}
