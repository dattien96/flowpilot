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
// Remaining follow-up (NOT done here — see Task-255 §8): settle sub-barriers
// B8a..B8e (need Task-251's settle driver wired to production first — it
// isn't), the Supabase tier (moot — Task-258 retired Supabase dispatch),
// go test -race wiring, and the randomized model-based suite
// (dispatch_model_test.go).

type crashBarrier string

const (
	// barrierPreCreatePrepared: killed before CreatePrepared ever runs — no
	// record exists at all. Nothing to recover; the retry decision lives
	// above the dispatch store (out of this store-level matrix's scope).
	barrierPreCreatePrepared crashBarrier = "B0"
	// barrierAfterPrepared: killed after CreatePrepared, before the
	// send_claimed CAS. Safely-retryable (Rr1).
	barrierAfterPrepared crashBarrier = "B1"
	// barrierAfterSendClaimed: killed after the send_claimed CAS, before the
	// send_started CAS — nothing was ever sent. Safely-retryable (Rr1).
	barrierAfterSendClaimed crashBarrier = "B2"
	// barrierPreExternalSend: killed after the send_started CAS but BEFORE
	// the external send actually happens. From the store's perspective this
	// looks identical to barrierAfterSendStarted (state=send_started) — which
	// is exactly the point: recovery cannot tell "CAS landed, external call
	// never happened" apart from "CAS landed, external call happened and we
	// don't know the result", so it must treat both as uncertain (never
	// silently assume zero-sends just because the recorded state alone can't
	// prove it either way).
	barrierPreExternalSend crashBarrier = "B3"
	// barrierAfterSendStarted: killed after a real send reached the provider,
	// before any receipt/terminal commit — the classic "did it get there?"
	// ambiguous cell. No adapter reconcile capability (Task-250 T-4 waiver) =>
	// must become uncertain, never re-terminalized, never re-sent (Rr2/Rr3).
	barrierAfterSendStarted crashBarrier = "B4"
	// barrierReceiptInRAMPreCommit: the worker holds a (simulated) receipt in
	// RAM but crashes before CommitReceiptAndClearIntent durably records it —
	// the "v1-draft leak cell": proves an in-RAM-only receipt is fully lost on
	// crash, not partially applied.
	barrierReceiptInRAMPreCommit crashBarrier = "B5"
	// barrierAfterReceiptCommit: killed after the receipt is durably
	// committed (state -> provider_accepted) but before any terminal is
	// observed.
	barrierAfterReceiptCommit crashBarrier = "B6"
	// barrierTerminalObservedPreCommit: the worker has (simulated) observed
	// the provider's final result in RAM but crashes before
	// CommitTerminalAndSettleIntent durably records it — proves an
	// in-RAM-only terminal observation is exactly as unrecoverable as never
	// having observed it at all (recovery still has zero durable proof).
	barrierTerminalObservedPreCommit crashBarrier = "B7"
	// barrierAfterTerminalCommitSettlePending: killed after the terminal
	// commit durably lands (SettlePhase=SettlePending, since testPrepared
	// sets SettleOwed=true) but before any settle-phase driver work. Proves
	// the terminal record + its settle-pending obligation survive a crash
	// intact; the settle driver itself isn't wired to the recovery scanner in
	// production yet (Task-251), which this cell also makes visible rather
	// than papering over.
	barrierAfterTerminalCommitSettlePending crashBarrier = "B8"
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

	blockForever := func() {
		if err := os.WriteFile(markerPath, []byte("ok"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "worker: write marker:", err)
			os.Exit(5)
		}
		select {} // wait to be hard-killed — never returns, no deferred cleanup runs
	}

	if barrier == barrierPreCreatePrepared {
		blockForever()
	}

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

	if barrier == barrierAfterPrepared {
		blockForever()
	}

	rev, err := store.CASAdvance(ctx, runID, turnID, 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "worker: CAS->send_claimed:", err)
		os.Exit(4)
	}

	if barrier == barrierAfterSendClaimed {
		blockForever()
	}

	rev, err = store.CASAdvance(ctx, runID, turnID, rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "worker: CAS->send_started:", err)
		os.Exit(6)
	}

	if barrier == barrierPreExternalSend {
		blockForever()
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

	// Simulate receiving the provider's acceptance receipt into RAM only —
	// nothing durable yet.
	receipt := ReceiptEvidence{
		ProviderKey:   "fake",
		ReceiptID:     "receipt-" + turnID,
		EvidenceKind:  "fake-receipt",
		PayloadSHA256: HashBytes([]byte("receipt-" + turnID)),
	}

	if barrier == barrierReceiptInRAMPreCommit {
		blockForever()
	}

	rev, err = store.CommitReceiptAndClearIntent(ctx, runID, turnID, rev, receipt, rec.IntentOwnerRunID, rec.OuterIntentKey, rec.OuterIntentGen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "worker: commit receipt:", err)
		os.Exit(8)
	}

	if barrier == barrierAfterReceiptCommit {
		blockForever()
	}

	// Simulate observing the provider's final result in RAM only — nothing
	// durable yet.
	proof := TerminalEvidence{
		ProviderKey:   "fake",
		EvidenceKind:  "fake-terminal",
		Outcome:       "completed",
		PayloadSHA256: HashBytes([]byte("terminal-" + turnID)),
	}

	if barrier == barrierTerminalObservedPreCommit {
		blockForever()
	}

	if _, err := store.CommitTerminalAndSettleIntent(ctx, runID, turnID, rev, proof, rec.IntentOwnerRunID, rec.OuterIntentKey, rec.OuterIntentGen); err != nil {
		fmt.Fprintln(os.Stderr, "worker: commit terminal:", err)
		os.Exit(9)
	}

	if barrier == barrierAfterTerminalCommitSettlePending {
		blockForever()
	}

	// Full happy path reached with no barrier requested — clean exit.
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

// crashCellResult captures the durable+provider-log state immediately after
// a real kill, and again after a recovery scan, for the remaining B0/B1/B3/
// B5/B6/B7/B8 cells (B2/B4 above stay as their own explicit tests — already
// committed and verified; new cells share this helper to avoid repeating the
// spawn/kill/reopen/scan boilerplate seven more times).
type crashCellResult struct {
	before         DispatchRecord
	beforeErr      error
	sendsBefore    int
	afterScan      DispatchRecord
	afterScanErr   error
	sendsAfterScan int
}

func runCrashCell(t *testing.T, b crashBarrier) crashCellResult {
	t.Helper()
	storeDir := t.TempDir()
	provider := newFakeProviderServer(t)
	runID, turnID := "r-real-"+string(b), "t-real-"+string(b)

	cmd, marker := spawnCrashWorker(t, storeDir, provider.url(), runID, turnID, b)
	waitForMarkerThenKill(t, cmd, marker)

	store := reopenStoreAfterKill(t, storeDir)
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	before, _, beforeErr := store.Get(ctx, runID, turnID)
	sendsBefore := provider.sendsFor(turnID)

	sc := &RecoveryScanner{Store: store, Owner: "crash-matrix", Lease: time.Minute}
	if err := sc.ScanRun(ctx, runID); err != nil {
		t.Fatalf("ScanRun: %v", err)
	}

	afterScan, _, afterScanErr := store.Get(ctx, runID, turnID)
	sendsAfterScan := provider.sendsFor(turnID)

	return crashCellResult{
		before: before, beforeErr: beforeErr, sendsBefore: sendsBefore,
		afterScan: afterScan, afterScanErr: afterScanErr, sendsAfterScan: sendsAfterScan,
	}
}

func TestDispatchCrashMatrix_RealKill_B0_NoRecordEverCreated(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess")
	}
	res := runCrashCell(t, barrierPreCreatePrepared)
	if res.beforeErr == nil {
		t.Fatalf("B0: no record should exist — CreatePrepared never ran before the kill, got %+v", res.before)
	}
	if res.sendsBefore != 0 {
		t.Fatalf("B0: no send should ever happen before a record even exists, got %d", res.sendsBefore)
	}
}

func TestDispatchCrashMatrix_RealKill_B1_PreparedSafelyRetryable(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess")
	}
	res := runCrashCell(t, barrierAfterPrepared)
	if res.beforeErr != nil {
		t.Fatalf("B1: Get after real kill: %v", res.beforeErr)
	}
	if res.before.State != DispatchPrepared {
		t.Fatalf("B1: want prepared after real kill, got %s", res.before.State)
	}
	if res.afterScan.State != DispatchPrepared {
		t.Fatalf("B1: recovery must leave a real kill at prepared safely-retryable, got %s", res.afterScan.State)
	}
	if res.sendsAfterScan != 0 {
		t.Fatalf("B1: INV-2: a bare scan (no EnsureLiveAndRedispatch wired) must not itself send, got %d", res.sendsAfterScan)
	}
}

func TestDispatchCrashMatrix_RealKill_B3_SendStartedBeforeExternalSend_StillUncertain(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess")
	}
	res := runCrashCell(t, barrierPreExternalSend)
	if res.before.State != DispatchSendStarted {
		t.Fatalf("B3: want send_started after real kill, got %s", res.before.State)
	}
	if res.sendsBefore != 0 {
		t.Fatalf("B3: the external send never happened before this kill, provider log must show 0, got %d", res.sendsBefore)
	}
	if res.afterScan.State != DispatchUncertain {
		t.Fatalf("B3: recovery cannot distinguish an un-sent send_started (B3) from a sent one (B4) from durable state alone — it must classify uncertain either way, got %s", res.afterScan.State)
	}
}

func TestDispatchCrashMatrix_RealKill_B5_ReceiptInRAMOnly_NeverPartiallyApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess")
	}
	res := runCrashCell(t, barrierReceiptInRAMPreCommit)
	if res.before.State != DispatchSendStarted {
		t.Fatalf("B5: receipt commit never landed before this kill, state must still be send_started, got %s", res.before.State)
	}
	if res.before.ReceiptEvidence != nil {
		t.Fatalf("B5: an in-RAM-only receipt must never leak into the durable record — the v1-draft-leak cell")
	}
	if res.sendsBefore != 1 {
		t.Fatalf("B5: exactly one real send must have reached the provider before this kill, got %d", res.sendsBefore)
	}
	if res.afterScan.State != DispatchUncertain {
		t.Fatalf("B5: no durable receipt proof after a real crash must classify uncertain, got %s", res.afterScan.State)
	}
	if res.sendsAfterScan != 1 {
		t.Fatalf("B5: INV-2: recovery must never re-send, got %d", res.sendsAfterScan)
	}
}

func TestDispatchCrashMatrix_RealKill_B6_ProviderAcceptedNoTerminalProof_StillUncertain(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess")
	}
	res := runCrashCell(t, barrierAfterReceiptCommit)
	if res.before.State != DispatchProviderAccepted {
		t.Fatalf("B6: receipt commit landed before this kill, want provider_accepted, got %s", res.before.State)
	}
	if res.before.ReceiptEvidence == nil {
		t.Fatalf("B6: a committed receipt must be durable — ReceiptEvidence is nil")
	}
	if res.afterScan.State != DispatchUncertain {
		t.Fatalf("B6: provider_accepted with no terminal proof after a real crash must classify uncertain, got %s", res.afterScan.State)
	}
	if res.sendsAfterScan != 1 {
		t.Fatalf("B6: INV-2: recovery must never re-send, got %d", res.sendsAfterScan)
	}
}

func TestDispatchCrashMatrix_RealKill_B7_TerminalObservedInRAMOnly_StillUncertain(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess")
	}
	res := runCrashCell(t, barrierTerminalObservedPreCommit)
	// Nothing new was persisted between B6 and B7 — the "observation" lived
	// only in the worker's RAM, so the durable state is identical to B6.
	// This is the point: even though the model had genuinely finished and
	// the worker "knew" the result, if it never committed, recovery has zero
	// durable proof and must fall back exactly as if nothing was observed.
	if res.before.State != DispatchProviderAccepted {
		t.Fatalf("B7: terminal commit never landed before this kill, durable state must equal B6 (provider_accepted), got %s", res.before.State)
	}
	if res.afterScan.State != DispatchUncertain {
		t.Fatalf("B7: an in-RAM-only terminal observation must not affect recovery's classification — want uncertain, got %s", res.afterScan.State)
	}
	if res.sendsAfterScan != 1 {
		t.Fatalf("B7: INV-2: recovery must never re-send, got %d", res.sendsAfterScan)
	}
}

func TestDispatchCrashMatrix_RealKill_B8_TerminalCommittedSettlePending_SurvivesIntact(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess")
	}
	res := runCrashCell(t, barrierAfterTerminalCommitSettlePending)
	if !res.before.State.IsTerminal() {
		t.Fatalf("B8: terminal commit landed before this kill, want a terminal state, got %s", res.before.State)
	}
	if res.before.SettlePhase != SettlePending {
		t.Fatalf("B8: testPrepared sets SettleOwed=true, so a real terminal commit must leave SettlePhase=pending, got %s", res.before.SettlePhase)
	}
	// Known gap, not a Task-255 defect: Task-251's settle-phase driver is not
	// wired to the recovery scanner in production yet, so reconcileOne
	// intentionally no-ops on a terminal record (see dispatch_recovery.go).
	// This assertion documents that gap explicitly rather than silently
	// passing regardless of what the scan did.
	if res.afterScan.SettlePhase != SettlePending {
		t.Fatalf("B8: a bare recovery scan must not itself finalize settle (that is the still-unwired Task-251 driver's job) — got phase=%s", res.afterScan.SettlePhase)
	}
}
