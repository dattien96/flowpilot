package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// TestInitSuggestions_NoScaffoldRow pins the CP-68 single-command contract: the
// `/init` Tab picker keeps exactly `skill` and `all` — there is no `scaffold`
// subcommand, for any platform (the picker is platform-independent by design).
func TestInitSuggestions_NoScaffoldRow(t *testing.T) {
	for _, input := range []string{"/init", "/init ", "/init s", "/init a", "/init scaffold", "/init sc"} {
		sugg := filterInitSuggestions(input)
		for _, s := range sugg {
			if s.value == "scaffold" {
				t.Fatalf("input %q must never offer a scaffold row, got %+v", input, sugg)
			}
		}
	}
	sugg := filterInitSuggestions("/init ")
	if len(sugg) != 2 {
		t.Fatalf("/init space want 2 rows, got %d: %+v", len(sugg), sugg)
	}
	values := map[string]bool{}
	for _, s := range sugg {
		values[s.value] = true
	}
	if !values["skill"] || !values["all"] {
		t.Fatalf("/init picker = %+v, want exactly skill and all", sugg)
	}
}

// newInitTestModel binds a model to a test runner URL and a project row.
func newInitTestModel(t *testing.T, srvURL, projectID, platform string) *AppModel {
	t.Helper()
	m := New(config.ChatConfig{}, srvURL)
	m.project = &client.Project{ID: projectID, Name: "App", Path: t.TempDir(), Platform: platform}
	return m
}

// newInitTestServer serves the two endpoints the single-command flow touches:
// engine/init always succeeds; scaffold records hits and answers per-test.
func newInitTestServer(t *testing.T, scaffoldHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/client/projects/p1/engine/init":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"projectId":"p1","initialized":true,"lastInit":{"status":"success","steps":[{"step":"skillpack_install","outcome":"ok","detail":"23 installed, 0 skipped, 0 errors"}],"install":{"installedPaths":[],"skippedPaths":[],"errors":[]}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/client/projects/p1/scaffold":
			if scaffoldHandler == nil {
				t.Errorf("unexpected scaffold call")
				http.NotFound(w, r)
				return
			}
			scaffoldHandler(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestInitEngine_AutoTriggersScaffoldForCapablePlatform(t *testing.T) {
	var scaffoldHits int32
	srv := newInitTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&scaffoldHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"done","platform":"react-native","skillsAttached":["react-native-scaffold-bootstrap","react-native-mobile-plumbing","react-native-core-ui-tokens","react-native-screen-archetypes"],"message":"scaffold: done — compiler gate PASS (pnpm install && pnpm tsc --noEmit)","compilerGate":{"passed":true,"exitCode":0}}`))
	})

	m := newInitTestModel(t, srv.URL, "p1", "react-native")
	cmd := m.cmdInitEngine("all")
	if cmd == nil {
		t.Fatal("cmdInitEngine(all) = nil cmd")
	}
	initMsg, ok := cmd().(EngineInitMsg)
	if !ok {
		t.Fatalf("cmd msg type = %T, want EngineInitMsg", cmd())
	}
	if initMsg.Err != nil {
		t.Fatalf("engine init failed: %v", initMsg.Err)
	}

	// Capable platform ⇒ the init handler must return a scaffold dispatch cmd.
	_, scaffoldCmd := m.handleEngineInitMsg(initMsg)
	if scaffoldCmd == nil {
		t.Fatal("capable platform must auto-trigger the AI Scaffold turn")
	}
	started := m.messages[len(m.messages)-1].Content
	if !strings.Contains(started, "Starting AI Scaffold turn") {
		t.Fatalf("pre-dispatch message = %q", started)
	}

	scaffoldMsg, ok := scaffoldCmd().(EngineScaffoldMsg)
	if !ok {
		t.Fatalf("scaffold cmd msg type = %T, want EngineScaffoldMsg", scaffoldCmd())
	}
	if scaffoldMsg.Err != nil {
		t.Fatalf("scaffold dispatch failed: %v", scaffoldMsg.Err)
	}
	if scaffoldMsg.Result == nil || scaffoldMsg.Result.Status != "done" {
		t.Fatalf("scaffold result = %+v, want done", scaffoldMsg.Result)
	}
	if atomic.LoadInt32(&scaffoldHits) != 1 {
		t.Fatalf("scaffold endpoint hits = %d, want 1", scaffoldHits)
	}

	final, _ := m.handleEngineScaffoldMsg(scaffoldMsg)
	last := final.(*AppModel).messages[len(final.(*AppModel).messages)-1].Content
	if !strings.Contains(last, "Compiler Gate: PASS") {
		t.Fatalf("final scaffold message = %q, want the gate verdict", last)
	}
}

func TestInitEngine_SkipsScaffoldForUnsupportedPlatform(t *testing.T) {
	var scaffoldHits int32
	srv := newInitTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&scaffoldHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"done"}`))
	})

	m := newInitTestModel(t, srv.URL, "p1", "vuejs")
	initMsg := m.cmdInitEngine("all")().(EngineInitMsg)
	if initMsg.Err != nil {
		t.Fatalf("engine init failed: %v", initMsg.Err)
	}

	// Not capable ⇒ graceful ignore: static init succeeded, no AI call, and the
	// exact operator-specified skip line is shown.
	_, scaffoldCmd := m.handleEngineInitMsg(initMsg)
	if scaffoldCmd != nil {
		t.Fatal("vuejs must not auto-trigger a scaffold turn")
	}
	last := m.messages[len(m.messages)-1].Content
	if !strings.Contains(last, "scaffold: skipped (no verified recipe)") {
		t.Fatalf("last message = %q, want the graceful-ignore line", last)
	}
	if atomic.LoadInt32(&scaffoldHits) != 0 {
		t.Fatalf("scaffold endpoint hits = %d, want 0", scaffoldHits)
	}
}

func TestInitEngine_SkillKindNeverTriggersScaffold(t *testing.T) {
	var scaffoldHits int32
	srv := newInitTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&scaffoldHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"done"}`))
	})

	// Even on the most capable platform, `/init skill` stays 100% CP-34: static
	// skill install only, never an AI call.
	m := newInitTestModel(t, srv.URL, "p1", "react-native")
	cmd := m.cmdInitEngine("skill")
	if cmd == nil {
		t.Fatal("cmdInitEngine(skill) = nil cmd")
	}
	initMsg := cmd().(EngineInitMsg)
	if initMsg.Kind != "skill" {
		t.Fatalf("kind = %q, want skill", initMsg.Kind)
	}
	_, scaffoldCmd := m.handleEngineInitMsg(initMsg)
	if scaffoldCmd != nil {
		t.Fatal("/init skill must never trigger the AI Scaffold turn")
	}
	for _, message := range m.messages {
		if strings.Contains(message.Content, "AI Scaffold") {
			t.Fatalf("/init skill leaked a scaffold message: %q", message.Content)
		}
	}
	if atomic.LoadInt32(&scaffoldHits) != 0 {
		t.Fatalf("scaffold endpoint hits = %d, want 0", scaffoldHits)
	}
}

func TestInitEngine_ScaffoldFailureRendersErrorWithLog(t *testing.T) {
	srv := newInitTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"scaffold_dispatch_failed","message":"compiler gate FAILED after 3 attempt(s)"}}`))
	})

	m := newInitTestModel(t, srv.URL, "p1", "react-native")
	initMsg := m.cmdInitEngine("all")().(EngineInitMsg)
	_, scaffoldCmd := m.handleEngineInitMsg(initMsg)
	if scaffoldCmd == nil {
		t.Fatal("capable platform must auto-trigger the scaffold turn")
	}

	scaffoldMsg := scaffoldCmd().(EngineScaffoldMsg)
	if scaffoldMsg.Err == nil {
		t.Fatalf("scaffold error = nil, want the HTTP failure (%+v)", scaffoldMsg.Result)
	}
	final, _ := m.handleEngineScaffoldMsg(scaffoldMsg)
	last := final.(*AppModel).messages[len(final.(*AppModel).messages)-1].Content
	if !strings.Contains(last, "scaffold: failed") || !strings.Contains(last, "FAILED after 3 attempt(s)") {
		t.Fatalf("failure message = %q, want the surfaced runner detail", last)
	}
}

func TestInitEngine_BareInitNormalizesToAllAndTriggersScaffold(t *testing.T) {
	// Bare `/init` (no subcommand) defaults to kind "all" (CP-34), so it must go
	// through the same single-command scaffold path.
	srv := newInitTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"done","platform":"react-native","message":"scaffold: done","compilerGate":{"passed":true,"exitCode":0}}`))
	})
	m := newInitTestModel(t, srv.URL, "p1", "react-native")

	initMsg := m.cmdInitEngine("  ")().(EngineInitMsg)
	if initMsg.Kind != "all" {
		t.Fatalf("normalized kind = %q, want all", initMsg.Kind)
	}
	_, scaffoldCmd := m.handleEngineInitMsg(initMsg)
	if scaffoldCmd == nil {
		t.Fatal("bare /init must auto-trigger the scaffold turn on a capable platform")
	}
}
