package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newScaffoldTestService builds an InteractiveService with a fake catalog row for
// proj-rn and (optionally) an injected scaffold dispatcher so no real provider CLI
// is ever spawned from an HTTP test.
func newScaffoldTestService(t *testing.T, dir, platform string, dispatcher *ScaffoldDispatcher) (*InteractiveService, *httptest.Server) {
	t.Helper()
	catalog := newInteractiveCatalog()
	catalog.projects = []Project{{ID: "proj-rn", Name: "App", Path: dir, Platform: platform}}
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, nil)
	svc.AttachRunner(&Runner{workspace: t.TempDir()})
	if dispatcher != nil {
		svc.scaffoldDispatcherFactory = func() *ScaffoldDispatcher { return dispatcher }
	}
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return svc, srv
}

func scaffoldStatusFromResponse(t *testing.T, body []byte) ScaffoldStatusResponse {
	t.Helper()
	var out ScaffoldStatusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode scaffold status: %v (%s)", err, body)
	}
	return out
}

func scaffoldResultFromResponse(t *testing.T, body []byte) ScaffoldDispatchResult {
	t.Helper()
	var out ScaffoldDispatchResult
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode scaffold result: %v (%s)", err, body)
	}
	return out
}

func TestScaffoldStatus_CapablePlatformReportsRecipe(t *testing.T) {
	_, srv := newScaffoldTestService(t, t.TempDir(), "react-native", nil)

	status, body := doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/status?platform=react-native", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%s", status, body)
	}
	out := scaffoldStatusFromResponse(t, body)
	if !out.Capable {
		t.Fatalf("Capable = false, want true (%+v)", out)
	}
	if out.ProjectID != "proj-rn" || out.Platform != "react-native" {
		t.Fatalf("identity = %+v", out)
	}
	if out.Recipe == nil {
		t.Fatal("Recipe = nil, want the react-native recipe")
	}
	if len(out.Recipe.ScaffoldSkills) != 4 {
		t.Fatalf("recipe skills = %v, want 4", out.Recipe.ScaffoldSkills)
	}
	if !strings.Contains(out.VerificationCommand, "pnpm tsc --noEmit") {
		t.Fatalf("VerificationCommand = %q", out.VerificationCommand)
	}
	if len(out.MissingSkills) != 0 {
		t.Fatalf("MissingSkills = %v, want none", out.MissingSkills)
	}
}

func TestScaffoldStatus_UnsupportedPlatformIsNotCapable(t *testing.T) {
	_, srv := newScaffoldTestService(t, t.TempDir(), "vuejs", nil)

	for _, platform := range []string{"vuejs", "ruby", "android", "unknown", ""} {
		status, body := doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/status?platform="+platform, nil, nil)
		if status != http.StatusOK {
			t.Fatalf("%q: status = %d body=%s", platform, status, body)
		}
		out := scaffoldStatusFromResponse(t, body)
		if out.Capable {
			t.Fatalf("%q: Capable = true, want false (Desktop must hide the option)", platform)
		}
		if out.Recipe != nil {
			t.Fatalf("%q: Recipe = %+v, want nil", platform, out.Recipe)
		}
	}
}

func TestScaffoldStatus_PlatformFallsBackToCatalogProject(t *testing.T) {
	dir := t.TempDir()
	_, srv := newScaffoldTestService(t, dir, "react-native", nil)

	// No query params at all: platform + workspace come from the catalog row.
	status, body := doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/status", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%s", status, body)
	}
	out := scaffoldStatusFromResponse(t, body)
	if out.Platform != "react-native" || !out.Capable {
		t.Fatalf("fallback status = %+v, want react-native capable", out)
	}
}

func TestScaffoldStatus_ReportsCompletedScaffoldStatusFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".flowpilot"), 0o755); err != nil {
		t.Fatal(err)
	}
	statusFile := ScaffoldStatusFile{
		Status:              ScaffoldStatusDone,
		Platform:            "react-native",
		CompletedAt:         time.Now().UTC().Format(time.RFC3339),
		SkillsAttached:      []string{"react-native-scaffold-bootstrap"},
		VerificationCommand: "pnpm install && pnpm tsc --noEmit",
	}
	raw, err := json.MarshalIndent(statusFile, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ScaffoldStatusPath(dir), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	_, srv := newScaffoldTestService(t, dir, "react-native", nil)
	status, body := doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/status?platform=react-native&workingDirectory="+dir, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%s", status, body)
	}
	out := scaffoldStatusFromResponse(t, body)
	if out.ScaffoldStatus == nil {
		t.Fatalf("ScaffoldStatus = nil, want the replay guard (%s)", body)
	}
	if out.ScaffoldStatus.Status != ScaffoldStatusDone || out.ScaffoldStatus.Platform != "react-native" {
		t.Fatalf("ScaffoldStatus = %+v, want done/react-native", out.ScaffoldStatus)
	}
}

func TestScaffoldStatus_UnknownProjectStillAnswersCapability(t *testing.T) {
	_, srv := newScaffoldTestService(t, t.TempDir(), "react-native", nil)
	status, body := doJSON(t, http.MethodGet, srv.URL+"/client/projects/nope/scaffold/status?platform=vuejs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%s", status, body)
	}
	if out := scaffoldStatusFromResponse(t, body); out.Capable {
		t.Fatalf("capable = true, want false (%s)", body)
	}
}

func TestScaffoldDispatch_CapablePlatformRunsTurnAndReportsDone(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	workspace := newScaffoldWorkspace(t)
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
	_, srv := newScaffoldTestService(t, workspace, "react-native", dispatcher)

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold", map[string]any{
		"workingDirectory": workspace,
		"platform":         "react-native",
		"providerKey":      "claude",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%s", status, body)
	}
	out := scaffoldResultFromResponse(t, body)
	if out.Status != ScaffoldStatusDone {
		t.Fatalf("Status = %q (%s), want done", out.Status, out.Message)
	}
	if len(out.SkillsAttached) != 4 {
		t.Fatalf("SkillsAttached = %v, want 4 blueprint skills", out.SkillsAttached)
	}
	if out.ProviderKey != "claude" {
		t.Fatalf("ProviderKey = %q, want the caller's explicit choice", out.ProviderKey)
	}
	if executor.count() != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.count())
	}
	if executor.turn(0).WorkingDirectory != workspace {
		t.Fatalf("turn workspace = %q, want %q", executor.turn(0).WorkingDirectory, workspace)
	}
	if _, ok := LoadScaffoldStatusFile(workspace); !ok {
		t.Fatal("expected scaffold-status.json after a done dispatch")
	}
}

func TestScaffoldDispatch_UnsupportedPlatformSkipsWithoutAICall(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	// The service is built for a vuejs project; the dispatcher factory must never
	// be reached because the recipe gate is evaluated before the AI turn.
	dispatcher := dispatcherForRecipe(executor, nil)
	_, srv := newScaffoldTestService(t, t.TempDir(), "vuejs", dispatcher)

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold", map[string]any{
		"workingDirectory": t.TempDir(),
		"platform":         "vuejs",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%s", status, body)
	}
	out := scaffoldResultFromResponse(t, body)
	if out.Status != ScaffoldStatusSkipped {
		t.Fatalf("Status = %q, want skipped", out.Status)
	}
	if out.Message != scaffoldSkippedNoRecipe {
		t.Fatalf("Message = %q, want %q", out.Message, scaffoldSkippedNoRecipe)
	}
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want zero AI calls", executor.count())
	}
}
func TestCreateProject_AutoTriggersScaffoldForCapablePlatform(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
	_, srv := newScaffoldTestService(t, t.TempDir(), "react-native", dispatcher)
	dir := t.TempDir()

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects", CreateProjectInput{
		Name:          "Mobile App",
		DirectoryPath: dir,
		Platform:      "react-native",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("status = %d body=%s", status, body)
	}

	// The passive trigger is async: it must complete shortly after the response,
	// never blocking it.
	deadline := time.Now().Add(5 * time.Second)
	for executor.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if executor.count() != 1 {
		t.Fatalf("executor calls = %d, want 1 auto-triggered scaffold turn", executor.count())
	}
	turn := executor.turn(0)
	if turn.WorkingDirectory != dir {
		t.Fatalf("turn workspace = %q, want the created project dir %q", turn.WorkingDirectory, dir)
	}
	if len(turn.SkillIds) != 4 {
		t.Fatalf("SkillIds = %v, want the 4 declared blueprint skills", turn.SkillIds)
	}
	if _, ok := LoadScaffoldStatusFile(dir); !ok {
		t.Fatal("expected scaffold-status.json after the auto-trigger")
	}
}

func TestCreateProject_SkipsScaffoldForUnsupportedPlatform(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, nil)
	_, srv := newScaffoldTestService(t, t.TempDir(), "vuejs", dispatcher)
	dir := t.TempDir()

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects", CreateProjectInput{
		Name:          "Vue App",
		DirectoryPath: dir,
		Platform:      "vuejs",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("status = %d body=%s", status, body)
	}

	time.Sleep(200 * time.Millisecond)
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want 0 — vuejs has no verified recipe", executor.count())
	}
	// The static init flow still completed normally.
	if _, err := os.Stat(filepath.Join(dir, ".flowpilot", "engine-init.json")); err != nil {
		t.Fatalf("expected engine-init.json after auto-init: %v", err)
	}
}
func TestCreateProject_AutoTriggerRejectsEscapingDirectory(t *testing.T) {
	// Even the passive Desktop trigger must respect the workspace boundary: a
	// crafted directory path that escapes the visible tree is rejected with a 4xx
	// and never reaches the (async) scaffold dispatcher.
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
	_, srv := newScaffoldTestService(t, t.TempDir(), "react-native", dispatcher)

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects", CreateProjectInput{
		Name:          "Escaping App",
		DirectoryPath: filepath.Join("..", "..", "outside-runner-root"),
		Platform:      "react-native",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400 for an escaping directory", status, body)
	}
	time.Sleep(200 * time.Millisecond)
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want 0 — rejected before any AI call", executor.count())
	}
}

func TestScaffoldDispatch_FallsBackToProjectDefaultModel(t *testing.T) {
	// The TUI /init all dispatch carries no provider/model of its own; the
	// runner must fall back to the project's stored default_model
	// (devin/swe-2-max → devin) instead of silently resolving the registry
	// default provider.
	executor := &recordingScaffoldExecutor{}
	workspace := newScaffoldWorkspace(t)
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))

	catalog := newInteractiveCatalog()
	catalog.projects = []Project{{ID: "proj-rn", Name: "App", Path: workspace, Platform: "react-native", Model: "devin/swe-2-max"}}
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, nil)
	svc.AttachRunner(&Runner{workspace: t.TempDir()})
	svc.scaffoldDispatcherFactory = func() *ScaffoldDispatcher { return dispatcher }
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold", map[string]any{
		"workingDirectory": workspace,
		"platform":         "react-native",
		"trigger":          "init_all",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%s", status, body)
	}
	if executor.count() != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.count())
	}
	turn := executor.turn(0)
	if turn.ProviderKey != "devin" {
		t.Fatalf("ProviderKey = %q, want devin resolved from the project's default_model", turn.ProviderKey)
	}
	if turn.ModelName != "devin/swe-2-max" {
		t.Fatalf("ModelName = %q, want the project's default_model", turn.ModelName)
	}
}

func TestScaffoldDispatch_SkipsWhileTurnInFlight(t *testing.T) {
	// The create_project auto-trigger already runs one scaffold turn per
	// project; a manual dispatch that lands while it runs must skip instead of
	// stacking a second concurrent AI turn writing into the same workspace.
	executor := &recordingScaffoldExecutor{}
	workspace := newScaffoldWorkspace(t)
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
	svc, srv := newScaffoldTestService(t, workspace, "react-native", dispatcher)

	svc.mu.Lock()
	svc.scaffoldInFlight = map[string]bool{"proj-rn": true}
	svc.mu.Unlock()

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold", map[string]any{
		"workingDirectory": workspace,
		"platform":         "react-native",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body=%s", status, body)
	}
	out := scaffoldResultFromResponse(t, body)
	if out.Status != ScaffoldStatusSkipped {
		t.Fatalf("Status = %q, want skipped while a scaffold turn is in flight", out.Status)
	}
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want 0 — no second concurrent AI turn", executor.count())
	}
}
