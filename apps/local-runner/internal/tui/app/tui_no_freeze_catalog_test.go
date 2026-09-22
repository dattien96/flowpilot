package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-514 regression: a catalog that times out (runner alive, Supabase slow) must
// be retried and must never be reported as "runner offline".
func TestSessionDefaults_CatalogSlow_NotReportedAsRunnerDead(t *testing.T) {
	m := New(config.ChatConfig{ProjectPath: t.TempDir()}, "http://127.0.0.1:4317")
	m.sessionLoading = true
	next, cmd := m.Update(SessionDefaultsMsg{
		Provider:   "grok",
		Model:      "grok-4.5",
		CatalogErr: "context deadline exceeded",
	})
	am := next.(*AppModel)
	if am.sessionLoading {
		t.Fatal("defaults must clear sessionLoading")
	}
	blob := ""
	for _, msg := range am.messages {
		blob += msg.Content + "\n"
	}
	if !strings.Contains(blob, "slow/timeout") {
		t.Fatalf("catalog-slow must be framed as retryable, got:\n%s", blob)
	}
	if strings.Contains(blob, "Fix .env / runner") {
		t.Fatalf("catalog timeout must NOT be reported as runner dead:\n%s", blob)
	}
	if cmd == nil {
		t.Fatal("slow catalog must schedule work (retry + project refresh)")
	}
	// The retry lifecycle: a later ProjectsCatalogMsg binds the project so chat can start.
	am2, _ := am.Update(ProjectsCatalogMsg{
		Projects: []client.Project{{ID: "p1", Name: "proj", Path: am.cfg.ProjectPath}},
		Project:  &client.Project{ID: "p1", Name: "proj", Path: am.cfg.ProjectPath},
	})
	if am2.(*AppModel).project == nil || am2.(*AppModel).project.ID != "p1" {
		t.Fatalf("retry should bind project: %+v", am2.(*AppModel).project)
	}
}

// Near-miss: a genuine dial failure (runner process down) must still be reported
// as catalog unavailable / fix the runner — not silently retried forever.
func TestSessionDefaults_RunnerDead_ReportedAsUnavailable(t *testing.T) {
	m := New(config.ChatConfig{ProjectPath: t.TempDir()}, "http://127.0.0.1:4317")
	m.sessionLoading = true
	next, _ := m.Update(SessionDefaultsMsg{
		Provider:   "grok",
		CatalogErr: "dial tcp 127.0.0.1:4317: connect: connection refused",
	})
	am := next.(*AppModel)
	blob := ""
	for _, msg := range am.messages {
		blob += msg.Content + "\n"
	}
	if !strings.Contains(blob, "catalog unavailable") {
		t.Fatalf("runner-dead must be reported as unavailable:\n%s", blob)
	}
	if strings.Contains(blob, "slow/timeout") {
		t.Fatalf("dial refusal must NOT be framed as slow catalog:\n%s", blob)
	}
}

// Reported repro: defaults loaded but no project bound → Enter must refuse fast
// (record the draft line, no ConnRunning, no pendingPrompt, no start-run cmd).
func TestProcessInput_NoProjectRefusesFast(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex", ProjectPath: t.TempDir()}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.sessionDefaultsLoaded = true
	m.provider = "codex"
	m.project = nil
	m.projects = nil
	m2, cmd := m.processInput("hello")
	am := m2.(*AppModel)
	if cmd != nil {
		t.Fatal("no-project must not schedule a run")
	}
	if am.pendingPrompt != "" {
		t.Fatalf("pendingPrompt=%q", am.pendingPrompt)
	}
	if am.runHandle != nil {
		t.Fatal("no-project must not arm a run")
	}
	if am.connStatus == ConnRunning {
		t.Fatal("no-project must not enter ConnRunning")
	}
	if am.statusMsg != "chat disabled — no project_id" {
		t.Fatalf("refusal must surface the disabled state, status=%q", am.statusMsg)
	}
	blob := ""
	for _, msg := range am.messages {
		blob += msg.Content + "\n"
	}
	if !strings.Contains(blob, "project_id") {
		t.Fatalf("missing project help:\n%s", blob)
	}
	foundUser := false
	for _, msg := range am.messages {
		if msg.Role == "user" && msg.Content == "hello" {
			foundUser = true
		}
	}
	if !foundUser {
		t.Fatal("draft line must be recorded so the user can re-send after binding")
	}
}

// Near-miss: when the known catalog already has a matching project, Enter binds
// it and starts the run normally (no spurious refusal).
func TestProcessInput_BindsProjectFromKnownCatalog(t *testing.T) {
	path := t.TempDir()
	m := New(config.ChatConfig{Provider: "codex", ProjectPath: path}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.sessionDefaultsLoaded = true
	m.provider = "codex"
	m.project = nil
	m.projects = []client.Project{{ID: "p1", Name: "proj", Path: path}}
	m2, cmd := m.processInput("hello")
	am := m2.(*AppModel)
	if am.project == nil || am.project.ID != "p1" {
		t.Fatalf("should bind project from catalog: %+v", am.project)
	}
	if am.pendingPrompt != "hello" {
		t.Fatalf("pendingPrompt=%q", am.pendingPrompt)
	}
	if cmd == nil {
		t.Fatal("bound project must still start a run")
	}
}

// cmdStartRun's catalog fallback must be bounded: empty catalog + dead runner
// returns ErrMsg quickly instead of hanging (the original freeze path).
func TestCmdStartRun_EmptyCatalogFallbackBounded(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex", ProjectPath: t.TempDir()}, "http://127.0.0.1:1")
	m.sessionLoading = false
	m.sessionDefaultsLoaded = true
	m.provider = "codex"
	m.project = nil
	m.projects = nil
	cmd := m.cmdStartRun()
	if cmd == nil {
		t.Fatal("nil cmd")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if _, ok := msg.(ErrMsg); !ok {
			t.Fatalf("expected ErrMsg, got %T", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cmdStartRun with empty catalog must not hang")
	}
}

func TestRunnerDialDeadErr_ClassifiesTimeoutVsDead(t *testing.T) {
	cases := []struct {
		err  string
		want bool
	}{
		{"dial tcp 127.0.0.1:4317: connectex: No connection could be made", true},
		{"connection refused", true},
		{"i/o timeout", false},
		{"Get http://127.0.0.1:4317/client/projects: i/o timeout", false},
		{"context deadline exceeded", false},
		{"context deadline exceeded (Client.Timeout exceeded)", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := runnerDialDeadErr(tc.err); got != tc.want {
			t.Errorf("runnerDialDeadErr(%q)=%v want %v", tc.err, got, tc.want)
		}
	}
	// Broad poll classification still counts i/o timeout as a failed poll.
	if !runnerUnreachableErr("get steps: i/o timeout") {
		t.Error("runnerUnreachableErr must count i/o timeout toward the poll pause")
	}
	if !runnerUnreachableErr("dial tcp 127.0.0.1:4317: i/o timeout") {
		t.Error("runnerUnreachableErr must count dial tcp i/o timeout as unreachable")
	}
	if runnerUnreachableErr("") {
		t.Error("runnerUnreachableErr empty must be false")
	}
}

// CA-561: Windows reports an active TCP reset as WSAECONNRESET (10054) via
// wsarecv/wsasend — "An existing connection was forcibly closed by the remote
// host". That is the same dead-connection signal as Linux "connection reset
// by peer", so it must trip the dial-dead classifier (resume retry + total
// failure restore) while Windows i/o timeouts stay classified as slow.
func TestRunnerDialDeadErr_ClassifiesWindowsRST(t *testing.T) {
	cases := []struct {
		err  string
		want bool
	}{
		{`read tcp 127.0.0.1:64252->127.0.0.1:64251: wsarecv: An existing connection was forcibly closed by the remote host.`, true},
		{`write tcp 127.0.0.1:64252->127.0.0.1:64251: wsasend: An existing connection was forcibly closed by the remote host.`, true},
		{"wsarecv: An existing connection was forcibly closed by the remote host", true},
		// Windows connect timeouts must stay "slow", not dead (CA-514).
		{"A connection attempt failed because the connected party did not properly respond after a period of time", false},
		{"i/o timeout", false},
	}
	for _, tc := range cases {
		if got := runnerDialDeadErr(tc.err); got != tc.want {
			t.Errorf("runnerDialDeadErr(%q)=%v want %v", tc.err, got, tc.want)
		}
	}
	if !runnerUnreachableErr("wsarecv: An existing connection was forcibly closed by the remote host") {
		t.Error("Windows RST must count toward the poll pause too")
	}
}

// No false "ready": the status bar must reflect that chat is disabled when the
// catalog produced no project (slow retry, runner dead, or no path match).
func TestSessionDefaults_NoProjectStatusNotReady(t *testing.T) {
	cases := []struct {
		name       string
		catalogErr string
		projects   []client.Project
		want       string
	}{
		{"slow", "context deadline exceeded", nil, "loading catalog"},
		{"dead", "dial tcp 127.0.0.1:4317: connect: connection refused", nil, "catalog unavailable"},
		{"nomatch", "", []client.Project{{ID: "p1", Name: "proj", Path: t.TempDir()}}, "no project match"},
		{"empty", "", nil, "no project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(config.ChatConfig{ProjectPath: t.TempDir()}, "http://127.0.0.1:4317")
			m.sessionLoading = true
			next, _ := m.Update(SessionDefaultsMsg{
				Provider:   "grok",
				CatalogErr: tc.catalogErr,
				Projects:   tc.projects,
			})
			am := next.(*AppModel)
			if am.sessionLoading {
				t.Fatalf("%s: banner must clear once the catalog decision is made", tc.name)
			}
			if am.statusMsg == "ready" {
				t.Fatalf("%s: must not report ready with no project, status=%q", tc.name, am.statusMsg)
			}
			if !strings.Contains(am.statusMsg, tc.want) {
				t.Fatalf("%s: status=%q want contains %q", tc.name, am.statusMsg, tc.want)
			}
		})
	}
}

// /login must re-enter the same banner-until-catalog contract as cold start:
// sessionLoading stays true (and typing stays free, send stays blocked) until
// SessionDefaultsMsg resolves the catalog.
func TestLoginResultMsg_SchedulesSessionReload(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	next, cmd := m.Update(LoginResultMsg{Email: "a@b.com", UserID: "u1"})
	am := next.(*AppModel)
	if !am.sessionLoading {
		t.Fatal("login should re-enter session loading")
	}
	if cmd == nil {
		t.Fatal("login must schedule the session reload cmd")
	}
	am2, _ := am.Update(sessionKeysUnlockMsg{})
	if !am2.(*AppModel).sessionLoading {
		t.Fatal("stale keys-unlock must not clear the banner after login reload")
	}
	am3, _ := am2.Update(SessionDefaultsMsg{Provider: "grok", Model: "grok-4.5"})
	if am3.(*AppModel).sessionLoading {
		t.Fatal("SessionDefaultsMsg must clear the banner after login reload")
	}
}

// ConnectedMsg puts the banner up; only SessionDefaultsMsg (or the 45s safety
// net) may clear it — no 1.5s early unlock dismisses it mid-catalog.
func TestConnectedMsg_BannerPersistsUntilSessionDefaults(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	next, _ := m.Update(ConnectedMsg{RunnerURL: "http://127.0.0.1:4317"})
	am := next.(*AppModel)
	if !am.sessionLoading {
		t.Fatal("banner must be up after connect")
	}
	am2, _ := am.Update(sessionKeysUnlockMsg{})
	if !am2.(*AppModel).sessionLoading {
		t.Fatal("banner must not clear before the catalog decision")
	}
}

// The catalog fetch must start during the banner phase, overlapping the
// account/provider path — not wait for it (sequential would leave the banner
// up ~8s longer and push the project decision past the load).
func TestCmdLoadSessionDefaults_StartsCatalogInParallel(t *testing.T) {
	var mu sync.Mutex
	var projectsArrived time.Time
	var providersDone time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client/projects":
			mu.Lock()
			projectsArrived = time.Now()
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[]`)
		case "/client/provider-accounts":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[]`)
		case "/providers":
			time.Sleep(800 * time.Millisecond)
			mu.Lock()
			providersDone = time.Now()
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := New(config.ChatConfig{}, srv.URL)
	cmd := m.cmdLoadSessionDefaults()
	if cmd == nil {
		t.Fatal("nil cmd")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if _, ok := msg.(SessionDefaultsMsg); !ok {
			t.Fatalf("msg type %T", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cmdLoadSessionDefaults hung")
	}

	mu.Lock()
	defer mu.Unlock()
	if projectsArrived.IsZero() {
		t.Fatal("catalog never requested")
	}
	if providersDone.IsZero() {
		t.Fatal("providers never completed")
	}
	// Parallel: catalog arrives while providers still sleeping. Sequential would
	// only request projects after the 800ms providers call returns (~0 delta).
	if projectsArrived.Sub(providersDone) > -400*time.Millisecond {
		t.Fatalf("catalog must start during the banner load: projects@%v providersDone@%v", projectsArrived, providersDone)
	}
}
