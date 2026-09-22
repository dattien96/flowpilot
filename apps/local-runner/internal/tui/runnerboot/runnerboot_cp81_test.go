// CP-81 Task-416: shared-runner boot tests — classification, idle-only stale
// replacement, boot serialization, detached spawn, fenced kill. Internal
// package so tests can stub the spawn/lock seams.
package runnerboot

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cp81Server fakes a lifecycle-aware runner: /health + /system/lifecycle +
// /system/shutdown with a flippable alive/identity state.
type cp81Server struct {
	*httptest.Server
	mu          sync.Mutex
	instanceID  string
	buildID     string
	protocol    int
	cwd         string
	phase       string
	clients     int
	work        int
	alive       bool
	shutdownReq atomic.Int32
	shutdownIDC chan string
}

func newCP81Server(t *testing.T, instanceID, buildID string) *cp81Server {
	t.Helper()
	s := &cp81Server{
		instanceID:  instanceID,
		buildID:     buildID,
		protocol:    1,
		phase:       "ready",
		alive:       true,
		shutdownIDC: make(chan string, 4),
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Server.Close)
	return s
}

func (s *cp81Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	alive, phase := s.alive, s.phase
	inst, build, proto, cwd := s.instanceID, s.buildID, s.protocol, s.cwd
	clients, work := s.clients, s.work
	s.mu.Unlock()

	switch r.URL.Path {
	case "/health":
		if !alive {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status": "online", "runnerVersion": "dev", "cwd": cwd, "os": runtime.GOOS,
			"startedAt":        "2026-09-22T00:00:00Z",
			"runnerInstanceId": inst, "generation": 1, "protocolVersion": proto,
			"buildId": build, "lifecycleMode": "client-managed", "phase": phase,
		})
	case "/system/lifecycle":
		if !alive {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		cl := make([]map[string]any, 0, clients)
		for i := 0; i < clients; i++ {
			cl = append(cl, map[string]any{"leaseId": fmt.Sprintf("lease_%d", i)})
		}
		items := make([]map[string]any, 0, work)
		for i := 0; i < work; i++ {
			items = append(items, map[string]any{"kind": "turn", "runId": fmt.Sprintf("run-%d", i)})
		}
		json.NewEncoder(w).Encode(map[string]any{
			"runnerInstanceId": inst, "generation": 1, "protocolVersion": proto,
			"buildId": build, "lifecycleMode": "client-managed", "phase": phase,
			"clients": cl, "workload": map[string]any{"items": items},
			"inventoryRevision": 1, "serverNow": time.Now().UTC(),
		})
	case "/system/shutdown":
		var body struct {
			ExpectedInstanceID string `json:"expectedInstanceId"`
			Reason             string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.shutdownReq.Add(1)
		select {
		case s.shutdownIDC <- body.ExpectedInstanceID:
		default:
		}
		if body.ExpectedInstanceID != "" && body.ExpectedInstanceID != inst {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":{"code":"stale_lifecycle_snapshot","message":"wrong instance"}}`)
			return
		}
		s.mu.Lock()
		s.alive = false // accepted shutdown takes the runner down
		s.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"accepted"}`)
	default:
		http.NotFound(w, r)
	}
}

func (s *cp81Server) set(fn func(*cp81Server)) {
	s.mu.Lock()
	fn(s)
	s.mu.Unlock()
}

// cfgFor points a Config at the test server's host:port.
func cfgFor(t *testing.T, srv *httptest.Server, mutate func(*Config)) Config {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	fmt.Sscanf(u.Port(), "%d", &port)
	cfg := Config{Host: "127.0.0.1", Port: port, ExpectedBuildID: "self-build", ExpectedProtocolVersion: 1}
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

// stubSpawn swaps spawnRunnerFn for a test double.
func stubSpawn(t *testing.T, fn func(Config) error) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	orig := spawnRunnerFn
	spawnRunnerFn = func(c Config) error {
		calls.Add(1)
		if fn != nil {
			return fn(c)
		}
		return nil
	}
	t.Cleanup(func() { spawnRunnerFn = orig })
	return &calls
}

func stubLock(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	orig := acquireBootLock
	acquireBootLock = func(ctx context.Context) (BootLock, error) {
		return acquireBootLockAt(ctx, filepath.Join(dir, "boot.lock"))
	}
	t.Cleanup(func() { acquireBootLock = orig })
}

func TestCurrentBuildIdentity_DeterministicForExecutable(t *testing.T) {
	a, err := CurrentBuildIdentity()
	if err != nil {
		t.Fatalf("CurrentBuildIdentity: %v", err)
	}
	b, err := CurrentBuildIdentity()
	if err != nil {
		t.Fatalf("CurrentBuildIdentity: %v", err)
	}
	if a.BuildID == "" || a.BuildID != b.BuildID {
		t.Fatalf("build identity not deterministic: %q vs %q", a.BuildID, b.BuildID)
	}
	if a.ProtocolVersion <= 0 {
		t.Fatalf("protocol version = %d, want >0", a.ProtocolVersion)
	}
}

func TestEnsureRunner_CompatibleReuses(t *testing.T) {
	srv := newCP81Server(t, "inst-1", "self-build")
	spawns := stubSpawn(t, nil)
	res, err := EnsureRunner(context.Background(), cfgFor(t, srv.Server, nil))
	if err != nil {
		t.Fatalf("EnsureRunner: %v", err)
	}
	if !res.Reused || res.Launched {
		t.Fatalf("got launched=%v reused=%v, want reuse", res.Launched, res.Reused)
	}
	if res.Classification != ClassCompatible {
		t.Fatalf("classification = %s, want compatible", res.Classification)
	}
	if spawns.Load() != 0 || srv.shutdownReq.Load() != 0 {
		t.Fatalf("compatible runner must not spawn/shutdown (spawns=%d shutdowns=%d)", spawns.Load(), srv.shutdownReq.Load())
	}
}

func TestEnsureRunner_IdleStaleAutoReplaces(t *testing.T) {
	srv := newCP81Server(t, "inst-old", "old-build")
	spawns := stubSpawn(t, func(Config) error {
		// Spawn brings the fresh instance online.
		srv.set(func(s *cp81Server) { s.alive, s.instanceID, s.buildID = true, "inst-new", "self-build" })
		return nil
	})
	stubLock(t)

	res, err := EnsureRunner(context.Background(), cfgFor(t, srv.Server, nil))
	if err != nil {
		t.Fatalf("EnsureRunner: %v", err)
	}
	if !res.Launched {
		t.Fatalf("stale-idle runner should be replaced (launched=false)")
	}
	if srv.shutdownReq.Load() != 1 {
		t.Fatalf("expected exactly 1 fenced shutdown, got %d", srv.shutdownReq.Load())
	}
	select {
	case id := <-srv.shutdownIDC:
		if id != "inst-old" {
			t.Fatalf("shutdown fence = %q, want inst-old", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown never carried expectedInstanceId")
	}
	if spawns.Load() != 1 {
		t.Fatalf("expected 1 spawn, got %d", spawns.Load())
	}
}

func TestEnsureRunner_BusyStaleDoesNotReplace(t *testing.T) {
	srv := newCP81Server(t, "inst-old", "old-build")
	srv.set(func(s *cp81Server) { s.clients = 1 })
	spawns := stubSpawn(t, nil)

	res, err := EnsureRunner(context.Background(), cfgFor(t, srv.Server, nil))
	if err != nil {
		t.Fatalf("EnsureRunner: %v", err)
	}
	if !res.Reused || res.Classification != ClassBusyStale || !res.UpdatePending {
		t.Fatalf("got %+v, want reused busy_stale updatePending", res)
	}
	if srv.shutdownReq.Load() != 0 || spawns.Load() != 0 {
		t.Fatal("busy stale runner must not be shut down or replaced")
	}
}

func TestEnsureRunner_ProtocolIncompatibleRejected(t *testing.T) {
	srv := newCP81Server(t, "inst-1", "self-build")
	srv.set(func(s *cp81Server) { s.protocol = 99 })
	spawns := stubSpawn(t, nil)
	_, err := EnsureRunner(context.Background(), cfgFor(t, srv.Server, nil))
	if err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("want protocol_incompatible error, got %v", err)
	}
	if spawns.Load() != 0 {
		t.Fatal("protocol-incompatible runner must not trigger spawn")
	}
}

func TestEnsureRunner_LegacyUnknownRequiresConfirmation(t *testing.T) {
	// Legacy runner: no lifecycle fields in /health, workspace mismatch.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{
				"status": "online", "runnerVersion": "dev", "cwd": "/elsewhere",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	spawns := stubSpawn(t, nil)

	_, err := EnsureRunner(context.Background(), cfgFor(t, srv, func(c *Config) {
		c.Workspace = "/my-ws"
	}))
	if err == nil {
		t.Fatal("legacy runner with workspace mismatch must not be silently reused or replaced")
	}
	if spawns.Load() != 0 {
		t.Fatal("legacy-unknown runner must never be auto-replaced")
	}
}

func TestEnsureRunner_WorkspaceMismatchRejected(t *testing.T) {
	srv := newCP81Server(t, "inst-1", "self-build")
	srv.set(func(s *cp81Server) { s.cwd = "/other-ws" })
	spawns := stubSpawn(t, nil)
	res, err := EnsureRunner(context.Background(), cfgFor(t, srv.Server, func(c *Config) {
		c.Workspace = "/my-ws"
	}))
	if err == nil || res.Classification != ClassWorkspaceMismatch {
		t.Fatalf("want workspace_mismatch error, got res=%+v err=%v", res, err)
	}
	if spawns.Load() != 0 || srv.shutdownReq.Load() != 0 {
		t.Fatal("foreign-workspace runner must not be replaced")
	}
}

func TestEnsureRunner_UnknownPortProcessNotKilled(t *testing.T) {
	// A foreign HTTP server (not a FlowPilot runner) owns the port.
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html>not flowpilot</html>")
	}))
	defer foreign.Close()
	spawns := stubSpawn(t, nil)

	res, err := EnsureRunner(context.Background(), cfgFor(t, foreign, nil))
	if err == nil || res.Classification != ClassPortConflict {
		t.Fatalf("want port_conflict error, got res=%+v err=%v", res, err)
	}
	if spawns.Load() != 0 {
		t.Fatal("foreign listener must never trigger spawn/kill")
	}
}

func TestEnsureRunner_ConcurrentStartsSingleWinner(t *testing.T) {
	// Reserve the port with a placeholder listener that accepts and
	// immediately closes: health checks fail fast, foreignListener reads it
	// as "not foreign" (no HTTP response), and the port cannot be stolen by
	// another test while the starters race. The spawn stub frees it and
	// rebinds the real health server in one goroutine — no TOCTOU window.
	rsv, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := rsv.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			c, aerr := rsv.Accept()
			if aerr != nil {
				return
			}
			c.Close()
		}
	}()
	t.Cleanup(func() { rsv.Close() })

	release := make(chan struct{})
	spawns := stubSpawn(t, func(Config) error {
		<-release // hold the spawn until both starters are inside EnsureRunner
		// The "spawned runner" comes online: release the placeholder and
		// serve health on the same port.
		rsv.Close()
		rln, lerr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if lerr != nil {
			return lerr
		}
		go http.Serve(rln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				json.NewEncoder(w).Encode(map[string]any{
					"status": "online", "runnerVersion": "dev", "cwd": "",
					"runnerInstanceId": "inst-win", "protocolVersion": 1, "buildId": "self-build",
				})
				return
			}
			http.NotFound(w, r)
		}))
		return nil
	})
	stubLock(t)
	cfg := Config{Host: "127.0.0.1", Port: port, ExpectedBuildID: "self-build", ExpectedProtocolVersion: 1}

	var wg sync.WaitGroup
	results := make([]Result, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = EnsureRunner(context.Background(), cfg)
		}(i)
	}
	time.Sleep(300 * time.Millisecond) // let both reach the boot lock
	close(release)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	if got := spawns.Load(); got != 1 {
		t.Fatalf("expected exactly 1 spawn across 2 concurrent starters, got %d", got)
	}
	launched := 0
	for _, r := range results {
		if r.Launched {
			launched++
		}
	}
	if launched != 1 {
		t.Fatalf("expected exactly 1 launched result, got %d (%+v)", launched, results)
	}
}

func TestRunnerBoot_SpawnPassesClientManagedMode(t *testing.T) {
	args := serveArgs(Config{Host: "127.0.0.1", Port: 4317}, "/ws")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--lifecycle-mode client-managed") {
		t.Fatalf("spawn args missing --lifecycle-mode client-managed: %v", args)
	}
	if !strings.Contains(joined, "--workspace /ws") {
		t.Fatalf("spawn args missing workspace: %v", args)
	}
}

func TestRunnerBoot_TUIDeathDoesNotKillSharedRunner(t *testing.T) {
	// The shared spawn must detach: on Unix Setsid, on Windows
	// DETACHED_PROCESS + no KILL_ON_JOB_CLOSE job assignment.
	cmd := exec.Command("true")
	setSysProcAttrShared(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("shared spawn must set SysProcAttr (detached)")
	}
	if runtime.GOOS != "windows" && !cmd.SysProcAttr.Setsid {
		t.Fatal("unix shared spawn must Setsid-detach from the TUI session")
	}
	// And the kill-on-close job must not be wired into the shared spawn path.
	src, err := os.ReadFile("runnerboot.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "assignRunnerJob(") {
		t.Fatal("spawnRunner must not assign the KILL_ON_JOB_CLOSE job in shared mode")
	}
}

func TestKillRunnerFallback_RequiresExpectedInstanceID(t *testing.T) {
	srv := newCP81Server(t, "inst-A", "self-build")
	if err := KillRunnerFenced(context.Background(), srv.URL, ""); err == nil {
		t.Fatal("empty expectedInstanceId must be rejected")
	}
	if err := KillRunnerFenced(context.Background(), srv.URL, "inst-B"); err == nil {
		t.Fatal("mismatched instanceId must refuse to kill")
	}
	// Matching instance: the httptest listener is owned by this test process
	// (self-excluded from the port lookup), so the fenced kill is a safe no-op.
	if err := KillRunnerFenced(context.Background(), srv.URL, "inst-A"); err != nil {
		t.Fatalf("matching instance fenced kill should succeed, got %v", err)
	}
}

func TestRunnerBoot_StaleLockRecoveredBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "boot.lock")
	// Stale lock: dead pid + old mtime.
	os.WriteFile(path, []byte("pid=999999999\ncreated=2020-01-01T00:00:00Z\n"), 0644)
	os.Chtimes(path, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	lock, err := acquireBootLockAt(ctx, path)
	if err != nil {
		t.Fatalf("stale lock should be reclaimed: %v", err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	// Live lock held by this process blocks a second acquirer (bounded by ctx).
	l1, err := acquireBootLockAt(context.Background(), path)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer l1.Unlock()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel2()
	if _, err := acquireBootLockAt(ctx2, path); err == nil {
		t.Fatal("second acquire while held should time out")
	}
}
