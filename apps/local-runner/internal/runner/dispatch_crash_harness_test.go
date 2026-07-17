package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Task-255 Phase 1: real subprocess-kill crash harness.
//
// Key Decision T-2 (Task-255) explicitly rejects same-process RAM-drop/store.Close()
// rebuilds as a crash simulation — that pattern cannot prove deferred cleanup,
// held locks, or in-flight goroutines actually died. This file provides the
// real mechanism instead:
//
//   - fakeProviderServer: an HTTP server OWNED BY THE PARENT test process, with
//     a durable send log. Only the worker dies; the parent and its log survive,
//     so the log is ground truth for INV-2 (no duplicate) independent of what
//     the killed process's own memory believed happened.
//   - TestHelperDispatchWorker: the current test binary re-exec'd
//     (`-test.run=TestHelperDispatchWorker`, gated by an env var so normal
//     `go test` runs skip it) as a worker that drives the exact same
//     DispatchStore CAS calls production code uses, up to a named barrier,
//     writes a marker file, then blocks forever so the parent can hard-kill it
//     with a real, unrecoverable os.Process.Kill() (cross-platform — no
//     kill -9/SIGKILL assumption; this repo develops on Windows first).
//   - Two real matrix cells (B2, B4) proving INV-1 (three-outcome, no silent
//     loss) and INV-2 (no duplicate send) hold across a genuine process death.
//
// Remaining follow-up (NOT done here — see Task-255 §8): the full B0..B8e +
// settle-sub-barrier matrix, the Supabase tier (moot — Task-258 retired
// Supabase dispatch), go test -race wiring, and the randomized model-based
// suite (dispatch_model_test.go).

type crashBarrier string

const (
	// barrierAfterSendClaimed: killed after the send_claimed CAS, before the
	// send_started CAS — nothing was ever sent. Safely-retryable (Rr1).
	barrierAfterSendClaimed crashBarrier = "B2"
	// barrierAfterSendStarted: killed after a real send reached the provider,
	// before any receipt/terminal commit — the classic "did it get there?"
	// ambiguous cell. No adapter reconcile capability (Task-250 T-4 waiver) =>
	// must become uncertain, never re-terminalized, never re-sent (Rr2/Rr3).
	barrierAfterSendStarted crashBarrier = "B4"
)

// fakeProviderServer is the parent-owned durable request log (Task-255 Step 2).
// It is never killed — only the worker subprocess is — so its log is the
// ground truth for how many times a turn actually reached the provider.
type fakeProviderServer struct {
	mu      sync.Mutex
	log     []string // turnIDs, in send order
	srv     *httptest.Server
	logFile *os.File
}

func newFakeProviderServer(t *testing.T) *fakeProviderServer {
	t.Helper()
	f := &fakeProviderServer{}
	logPath := filepath.Join(t.TempDir(), "provider-sends.log")
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open provider log: %v", err)
	}
	f.logFile = lf
	mux := http.NewServeMux()
	mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			TurnID string `json:"turn_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.log = append(f.log, body.TurnID)
		_, _ = f.logFile.WriteString(body.TurnID + "\n")
		_ = f.logFile.Sync()
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"receipt_id":"fake-receipt"}`))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	t.Cleanup(func() { _ = f.logFile.Close() })
	return f
}

// sendsFor is the parent-side assertion API (Task-255 Step 2).
func (f *fakeProviderServer) sendsFor(turnID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, id := range f.log {
		if id == turnID {
			n++
		}
	}
	return n
}

func (f *fakeProviderServer) url() string { return f.srv.URL }

// spawnCrashWorker re-execs the current test binary as a worker that drives
// one dispatch turn toward barrier b, writes a marker file on arrival, then
// blocks forever. The caller must poll for the marker then Kill() it.
func spawnCrashWorker(t *testing.T, storeDir, providerURL, runID, turnID string, b crashBarrier) (cmd *exec.Cmd, markerPath string) {
	t.Helper()
	markerPath = filepath.Join(t.TempDir(), "barrier.marker")
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd = exec.Command(self, "-test.run=TestHelperDispatchWorker")
	cmd.Env = append(os.Environ(),
		"FLOWPILOT_CRASH_WORKER=1",
		"FLOWPILOT_CRASH_STORE_DIR="+storeDir,
		"FLOWPILOT_CRASH_PROVIDER_URL="+providerURL,
		"FLOWPILOT_CRASH_RUN_ID="+runID,
		"FLOWPILOT_CRASH_TURN_ID="+turnID,
		"FLOWPILOT_CRASH_BARRIER="+string(b),
		"FLOWPILOT_CRASH_MARKER="+markerPath,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	return cmd, markerPath
}

// waitForMarkerThenKill polls for the barrier marker file, then hard-kills the
// worker. Cross-platform (Key Decision T-2, Windows-first): plain
// os.Process.Kill(), no kill -9/SIGKILL assumption.
func waitForMarkerThenKill(t *testing.T, cmd *exec.Cmd, markerPath string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(markerPath); err == nil {
			killHard(t, cmd)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	killHard(t, cmd)
	t.Fatalf("worker never reached barrier (marker %s never appeared)", markerPath)
}

// killHard hard-kills the worker, retrying briefly. os.Process.Kill() on
// Windows occasionally returns "Access is denied" for a still-alive child
// (observed in CI) — likely a transient handle/permission race, not a real
// failure to terminate — so retry before falling back to `taskkill /F` and
// finally to a plain reap in case the process is already gone by the time we
// get here.
func killHard(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	var lastErr error
	for i := 0; i < 5; i++ {
		if err := cmd.Process.Kill(); err == nil {
			_, _ = cmd.Process.Wait()
			return
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", cmd.Process.Pid)).Run()
		_, _ = cmd.Process.Wait()
		return
	}
	_, _ = cmd.Process.Wait()
	t.Fatalf("kill worker pid=%d: %v", cmd.Process.Pid, lastErr)
}

// reopenStoreAfterKill retries briefly: the OS releases the worker's flock on
// process death, but that release is not guaranteed instantaneous from the
// parent's point of view.
func reopenStoreAfterKill(t *testing.T, dir string) *localDispatchStore {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		st, err := NewLocalDispatchStore(dir)
		if err == nil {
			return st
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("reopen store after real kill: %v", lastErr)
	return nil
}

// TestHelperDispatchWorker is not a real test case: it is re-exec'd as a
// worker process by spawnCrashWorker. Run directly (FLOWPILOT_CRASH_WORKER
// unset) it just skips, so normal `go test ./...` runs are unaffected.
func TestHelperDispatchWorker(t *testing.T) {
	if os.Getenv("FLOWPILOT_CRASH_WORKER") != "1" {
		t.Skip("re-exec worker helper; not a real test")
	}
	storeDir := os.Getenv("FLOWPILOT_CRASH_STORE_DIR")
	providerURL := os.Getenv("FLOWPILOT_CRASH_PROVIDER_URL")
	runID := os.Getenv("FLOWPILOT_CRASH_RUN_ID")
	turnID := os.Getenv("FLOWPILOT_CRASH_TURN_ID")
	barrier := crashBarrier(os.Getenv("FLOWPILOT_CRASH_BARRIER"))
	markerPath := os.Getenv("FLOWPILOT_CRASH_MARKER")

	store, err := NewLocalDispatchStore(storeDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "worker: open store:", err)
		os.Exit(2)
	}
	ctx := context.Background()
	env := testEnvelope(runID, turnID)
	rec := testPrepared(runID, turnID)
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		fmt.Fprintln(os.Stderr, "worker: CreatePrepared:", err)
		os.Exit(3)
	}
	rev, err := store.CASAdvance(ctx, runID, turnID, 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "worker: CAS->send_claimed:", err)
		os.Exit(4)
	}

	blockForever := func() {
		if err := os.WriteFile(markerPath, []byte("ok"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "worker: write marker:", err)
			os.Exit(5)
		}
		select {} // wait to be hard-killed — never returns, no deferred cleanup runs
	}

	if barrier == barrierAfterSendClaimed {
		blockForever()
	}

	rev, err = store.CASAdvance(ctx, runID, turnID, rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "worker: CAS->send_started:", err)
		os.Exit(6)
	}

	// Real external send: a genuine HTTP call to the parent-owned fake
	// provider, so the durable parent-side log is the ground truth for
	// INV-2 — not an in-process counter that would vanish with the worker.
	body, _ := json.Marshal(map[string]string{"turn_id": turnID, "run_id": runID})
	resp, err := http.Post(providerURL+"/send", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, "worker: send to provider:", err)
		os.Exit(7)
	}
	_ = resp.Body.Close()

	if barrier == barrierAfterSendStarted {
		blockForever()
	}

	// Not reached by either matrix cell in this phase; a future barrier past
	// B4 would continue to CommitReceiptAndClearIntent /
	// CommitTerminalAndSettleIntent here.
	os.Exit(0)
}

func TestDispatchCrashMatrix_RealKill_B2_SendClaimedSafelyRetryable(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess")
	}
	storeDir := t.TempDir()
	provider := newFakeProviderServer(t)
	runID, turnID := "r-real-b2", "t-real-b2"

	cmd, marker := spawnCrashWorker(t, storeDir, provider.url(), runID, turnID, barrierAfterSendClaimed)
	waitForMarkerThenKill(t, cmd, marker)

	store := reopenStoreAfterKill(t, storeDir)
	defer store.Close()
	ctx := context.Background()

	got, _, err := store.Get(ctx, runID, turnID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != DispatchSendClaimed {
		t.Fatalf("want send_claimed after real kill at B2, got %s", got.State)
	}
	if n := provider.sendsFor(turnID); n != 0 {
		t.Fatalf("INV-2: worker was killed before any external send, but provider log shows %d sends", n)
	}

	sc := &RecoveryScanner{Store: store, Owner: "crash-matrix", Lease: time.Minute}
	if err := sc.ScanRun(ctx, runID); err != nil {
		t.Fatalf("ScanRun: %v", err)
	}
	got, _, _ = store.Get(ctx, runID, turnID)
	if got.State != DispatchSendClaimed {
		t.Fatalf("recovery must leave a real B2 kill safely-retryable, got %s", got.State)
	}
	if n := provider.sendsFor(turnID); n != 0 {
		t.Fatalf("INV-2: a bare scan (no EnsureLiveAndRedispatch wired) must not itself send, got %d", n)
	}
}

func TestDispatchCrashMatrix_RealKill_B4_SendStartedNoProofBecomesUncertain(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess")
	}
	storeDir := t.TempDir()
	provider := newFakeProviderServer(t)
	runID, turnID := "r-real-b4", "t-real-b4"

	cmd, marker := spawnCrashWorker(t, storeDir, provider.url(), runID, turnID, barrierAfterSendStarted)
	waitForMarkerThenKill(t, cmd, marker)

	store := reopenStoreAfterKill(t, storeDir)
	defer store.Close()
	ctx := context.Background()

	got, _, err := store.Get(ctx, runID, turnID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != DispatchSendStarted {
		t.Fatalf("want send_started after real kill at B4, got %s", got.State)
	}
	if n := provider.sendsFor(turnID); n != 1 {
		t.Fatalf("ground truth: exactly one real send must have reached the provider before kill, got %d", n)
	}

	sc := &RecoveryScanner{Store: store, Owner: "crash-matrix", Lease: time.Minute}
	if err := sc.ScanRun(ctx, runID); err != nil {
		t.Fatalf("ScanRun: %v", err)
	}
	got, _, _ = store.Get(ctx, runID, turnID)
	if got.State != DispatchUncertain {
		t.Fatalf("INV-1: no proof after a real crash at B4 must classify uncertain (never dangling, never re-terminalized), got %s", got.State)
	}
	if n := provider.sendsFor(turnID); n != 1 {
		t.Fatalf("INV-2: recovery must never re-send after a genuine post-send crash, got %d sends", n)
	}
}
