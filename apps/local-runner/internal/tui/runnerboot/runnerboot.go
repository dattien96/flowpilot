// Package runnerboot auto-ensures a local runner process is online for the
// `flowpilot chat` command.  It never imports flowpilot-runner/internal/runner.
//
// Heuristic (CP-56, extended by CP-81 Task-416):
//  1. If --runner-url is set, verify health only (no auto-start); a lifecycle
//     runner still reports its classification to UX.
//  2. Else if --no-start-runner, verify the default candidate (host:port) health only.
//  3. Else health-check the candidate URL and classify:
//     a. compatible        → reuse.
//     b. idle_stale        → fenced /system/shutdown (reason=stale_build) then
//     spawn the current binary under the boot lock.
//     c. busy_stale        → reuse; report updatePending to UX.
//     d. workspace_mismatch / protocol_incompatible / legacy_unknown mismatch →
//     error; a shared or foreign runner is never killed.
//     e. port_conflict     → error; the listener is not ours, never killed.
//     f. unreachable       → spawn `<exe> runner serve --lifecycle-mode
//     client-managed ...` detached under the boot lock,
//     log to <root>/.flowpilot/cli-runner.log, poll.
//
// Skill: cli-tui (CP-56)
package runnerboot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/tui/client"
)

const (
	pollInterval = 300 * time.Millisecond
	pollTimeout  = 20 * time.Second
	logDir       = ".flowpilot"
	logFile      = "cli-runner.log"
	// Live Codex for TUI-spawned runners (parity with scripts/supervisor.js / just dev).
	// Without this, ProviderRegistryFor keeps the fake Codex adapter and chat returns
	// scripted "Sure — let me work through this step…" deltas.
	codexAppServerEnv = "FLOWPILOT_CODEX_APPSERVER"
	// staleReplaceDeadline bounds waiting for an idle-stale runner to exit
	// after it accepted a fenced shutdown (T-4/T-5).
	staleReplaceDeadline = 15 * time.Second
)

// RunnerClassification is the reuse decision for an existing listener
// (Task-416 T-3). Only compatible runners are silently reused; every other
// class is either surfaced to UX or drives the idle-only replacement path.
type RunnerClassification string

const (
	ClassCompatible           RunnerClassification = "compatible"
	ClassIdleStale            RunnerClassification = "idle_stale"
	ClassBusyStale            RunnerClassification = "busy_stale"
	ClassProtocolIncompatible RunnerClassification = "protocol_incompatible"
	ClassLegacyUnknown        RunnerClassification = "legacy_unknown"
	ClassWorkspaceMismatch    RunnerClassification = "workspace_mismatch"
	ClassPortConflict         RunnerClassification = "port_conflict"
)

// Config holds the inputs to EnsureRunner.
type Config struct {
	// ExplicitURL overrides all auto-start logic (--runner-url).
	ExplicitURL string
	// NoStart disables spawning (--no-start-runner).
	NoStart bool
	// Workspace is the resolved runner workspace root.
	Workspace string
	// Host and Port are passed to `runner serve`.
	Host string
	Port int
	// ExpectedVersion, if non-empty, is matched against the health response.
	// Empty = accept any version.
	ExpectedVersion string
	// ExpectedBuildID / ExpectedProtocolVersion fence lifecycle-aware reuse
	// (CP-81). Empty/zero skips that check — pass CurrentBuildIdentity values.
	ExpectedBuildID         string
	ExpectedProtocolVersion int
}

// Result describes what EnsureRunner did.
type Result struct {
	RunnerURL string
	Launched  bool
	Reused    bool
	// CP-81 additive fields (RunnerURL/Launched/Reused keep their CP-56 names
	// — existing callers and tests pin them).
	OwnsRunner     bool // true only when this process spawned the runner
	Classification RunnerClassification
	Snapshot       client.LifecycleSnapshot // best-effort attach-time snapshot
	UpdatePending  bool                     // busy_stale: an update waits for idle
}

// EnsureResult is the SD-28-facing name for Result.
type EnsureResult = Result

// spawnRunnerFn / acquireBootLockFn are the package seams tests use to fake
// process spawn and boot-lock acquisition without forking real subprocesses.
var (
	spawnRunnerFn    = spawnRunner
	acquireBootLock  = AcquireRunnerBootLock
	httpHealthClient = &http.Client{Timeout: 1500 * time.Millisecond}
)

// EnsureRunner ensures a runner is available and returns its base URL.
// It is safe to call from tests with a short ctx deadline to limit spawn attempts.
func EnsureRunner(ctx context.Context, cfg Config) (Result, error) {
	candidateURL := cfg.ExplicitURL
	if candidateURL == "" {
		candidateURL = fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	}
	cl := client.New(candidateURL)

	h, err := cl.Health(ctx)
	if err == nil {
		return ensureHealthyRunner(ctx, cfg, cl, candidateURL, h)
	}

	// Unreachable (or unparseable). Distinguish "nothing listening" from a
	// foreign process squatting on the port — the latter is never killed.
	if cfg.ExplicitURL != "" || cfg.NoStart {
		return Result{}, fmt.Errorf("runner not reachable at %s: %w", candidateURL, err)
	}
	if foreignListener(candidateURL) {
		return Result{RunnerURL: candidateURL, Classification: ClassPortConflict},
			fmt.Errorf("port %s is held by a non-FlowPilot process; refusing to kill or replace it", candidateURL)
	}

	return spawnUnderLock(ctx, cfg, cl, candidateURL)
}

// ensureHealthyRunner handles the /health-OK path: classify, then reuse,
// replace-if-idle-stale, or fail with a typed classification.
func ensureHealthyRunner(ctx context.Context, cfg Config, cl *client.Client, candidateURL string, h client.HealthResponse) (Result, error) {
	// Legacy runner (pre-lifecycle build reports no runnerInstanceId):
	// preserve the CP-56 contract byte-for-byte — reuse when version/cwd
	// match, otherwise surface an error. Never auto-replace a runner whose
	// identity we cannot fence.
	if h.RunnerInstanceID == "" {
		if isHealthyAndCompatible(h, cfg) {
			return Result{RunnerURL: candidateURL, Reused: true, Classification: ClassLegacyUnknown}, nil
		}
		if cfg.ExplicitURL != "" {
			return Result{RunnerURL: candidateURL, Reused: true, Classification: ClassLegacyUnknown}, nil
		}
		return Result{RunnerURL: candidateURL, Classification: ClassLegacyUnknown},
			fmt.Errorf("runner at %s is a legacy build that does not match this workspace/version; stop it manually and retry", candidateURL)
	}

	cls := classifyRunner(h, cfg)
	switch cls {
	case ClassCompatible:
		return Result{RunnerURL: candidateURL, Reused: true, Classification: cls,
			Snapshot: fetchSnapshot(cl, ctx)}, nil
	case ClassWorkspaceMismatch:
		// ExplicitURL is an explicit attach: still reuse but report the
		// mismatch so UX can warn (T-4).
		if cfg.ExplicitURL != "" {
			return Result{RunnerURL: candidateURL, Reused: true, Classification: cls,
				Snapshot: fetchSnapshot(cl, ctx)}, nil
		}
		return Result{RunnerURL: candidateURL, Classification: cls},
			fmt.Errorf("runner at %s serves workspace %q, not %q; refusing to attach", candidateURL, h.Cwd, cfg.Workspace)
	case ClassProtocolIncompatible:
		return Result{RunnerURL: candidateURL, Classification: cls},
			fmt.Errorf("runner at %s speaks lifecycle protocol %d, need %d; upgrade or stop it manually",
				candidateURL, h.ProtocolVersion, cfg.ExpectedProtocolVersion)
	}

	// Stale build: replace only when the lifecycle snapshot proves the runner
	// is idle (zero leases, zero protected work, not draining).
	snap, snapErr := cl.GetLifecycleSnapshot(ctx)
	idle := snapErr == nil && len(snap.Clients) == 0 && snap.Workload.ActiveCount() == 0 && !snap.IsDraining()
	if !idle {
		return Result{RunnerURL: candidateURL, Reused: true, Classification: ClassBusyStale,
			UpdatePending: true, Snapshot: snap}, nil
	}
	return replaceIdleStaleRunner(ctx, cfg, cl, candidateURL, h, snap)
}

// classifyRunner buckets a lifecycle-aware runner's health response.
// Workspace and protocol mismatches are hard rejections; a buildId mismatch
// is stale (idle vs busy is decided by the live snapshot in the caller).
func classifyRunner(h client.HealthResponse, cfg Config) RunnerClassification {
	if cfg.ExplicitURL == "" && cfg.Workspace != "" &&
		!strings.EqualFold(filepath.Clean(h.Cwd), filepath.Clean(cfg.Workspace)) {
		return ClassWorkspaceMismatch
	}
	if cfg.ExpectedProtocolVersion != 0 && h.ProtocolVersion != 0 &&
		h.ProtocolVersion != cfg.ExpectedProtocolVersion {
		return ClassProtocolIncompatible
	}
	if cfg.ExpectedBuildID != "" && h.BuildID != "" && h.BuildID != cfg.ExpectedBuildID {
		return ClassIdleStale // provisional; busy check happens against snapshot
	}
	return ClassCompatible
}

// replaceIdleStaleRunner performs the fenced swap (T-4/T-5): the runner
// accepts shutdown only for its own runnerInstanceId, we wait for that exact
// instance to disappear, then spawn the current binary and poll for a
// compatible health.
func replaceIdleStaleRunner(ctx context.Context, cfg Config, cl *client.Client, candidateURL string, h client.HealthResponse, snap client.LifecycleSnapshot) (Result, error) {
	_, err := cl.RequestRunnerShutdown(ctx, client.SystemActionRequest{
		ExpectedInstanceID: h.RunnerInstanceID,
		Reason:             "stale_build",
	})
	if err != nil {
		var le *client.LifecycleError
		if errors.As(err, &le) && le.Code == "lifecycle_confirmation_required" {
			// A client attached between our snapshot and the shutdown — treat
			// as busy_stale rather than racing the confirmation flow.
			return Result{RunnerURL: candidateURL, Reused: true, Classification: ClassBusyStale,
				UpdatePending: true, Snapshot: snap}, nil
		}
		return Result{RunnerURL: candidateURL, Classification: ClassIdleStale},
			fmt.Errorf("stale runner refused fenced shutdown: %w", err)
	}

	lock, lerr := acquireBootLock(ctx)
	if lerr != nil {
		return Result{}, fmt.Errorf("acquire boot lock: %w", lerr)
	}
	defer lock.Unlock()

	deadline := time.Now().Add(staleReplaceDeadline)
	for time.Now().Before(deadline) {
		h2, herr := cl.Health(ctx)
		if herr != nil || (h2.RunnerInstanceID != "" && h2.RunnerInstanceID != h.RunnerInstanceID) {
			break // old instance gone (or port already rebound by a winner)
		}
		time.Sleep(pollInterval)
	}
	// If the old instance is STILL serving after the deadline it wedged —
	// fenced kill is allowed only while health still proves the same
	// runnerInstanceId (T-7). Anything else is left alone.
	if h3, herr := cl.Health(ctx); herr == nil && h3.RunnerInstanceID == h.RunnerInstanceID {
		if kerr := KillRunnerFenced(ctx, candidateURL, h.RunnerInstanceID); kerr != nil {
			return Result{RunnerURL: candidateURL, Classification: ClassIdleStale},
				fmt.Errorf("stale runner accepted shutdown but did not exit, and fenced kill failed: %w", kerr)
		}
	}
	// A concurrent replacer may have already spawned the new runner while we
	// waited — reuse it instead of double-spawning onto the port.
	if h4, herr := cl.Health(ctx); herr == nil && isHealthyAndCompatible(h4, cfg) &&
		h4.RunnerInstanceID != "" && h4.RunnerInstanceID != h.RunnerInstanceID {
		return Result{RunnerURL: candidateURL, Reused: true,
			Classification: classifyRunner(h4, cfg), Snapshot: fetchSnapshot(cl, ctx)}, nil
	}
	return spawnLocked(ctx, cfg, cl, candidateURL)
}

// spawnUnderLock serializes concurrent starters so a TUI+Desktop boot storm
// produces exactly one runner (T-6). After acquiring, health is re-checked —
// the winner's runner is reused instead of double-spawning.
func spawnUnderLock(ctx context.Context, cfg Config, cl *client.Client, candidateURL string) (Result, error) {
	lock, err := acquireBootLock(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("acquire boot lock: %w", err)
	}
	defer lock.Unlock()
	if h, herr := cl.Health(ctx); herr == nil && isHealthyAndCompatible(h, cfg) {
		return Result{RunnerURL: candidateURL, Reused: true,
			Classification: classifyRunner(h, cfg), Snapshot: fetchSnapshot(cl, ctx)}, nil
	}
	return spawnLocked(ctx, cfg, cl, candidateURL)
}

// spawnLocked performs the actual spawn + poll. Caller must hold the boot
// lock. The runner always launches shared: detached from this process's
// lifetime and running the client-managed lifecycle authority.
func spawnLocked(ctx context.Context, cfg Config, cl *client.Client, candidateURL string) (Result, error) {
	if err := spawnRunnerFn(cfg); err != nil {
		return Result{}, fmt.Errorf("failed to spawn runner: %w", err)
	}
	if err := pollUntilHealthy(ctx, cl, cfg); err != nil {
		return Result{}, fmt.Errorf("runner did not come online: %w", err)
	}
	return Result{RunnerURL: candidateURL, Launched: true, OwnsRunner: true,
		Classification: ClassCompatible, Snapshot: fetchSnapshot(cl, ctx)}, nil
}

// KillRunnerFenced is the only direct-kill path runnerboot exposes (T-7): it
// refuses to kill unless live health reports exactly expectedInstanceID —
// foreign listeners and already-swapped runners are never touched.
func KillRunnerFenced(ctx context.Context, runnerURL, expectedInstanceID string) error {
	if strings.TrimSpace(expectedInstanceID) == "" {
		return fmt.Errorf("kill requires an expectedInstanceId fence")
	}
	h, err := client.New(runnerURL).Health(ctx)
	if err != nil {
		return nil // already gone — nothing to kill
	}
	if h.RunnerInstanceID != expectedInstanceID {
		return fmt.Errorf("refusing to kill: live instance %q != expected %q", h.RunnerInstanceID, expectedInstanceID)
	}
	return killRunnerOnPort(runnerURL)
}

// foreignListener reports whether an HTTP server that is NOT shaped like a
// FlowPilot runner is answering on the candidate URL — the port_conflict
// signal that must never trigger a kill.
func foreignListener(candidateURL string) bool {
	resp, err := httpHealthClient.Get(candidateURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return true // responded but unreadable → something is listening
	}
	var probe struct {
		Status string `json:"status"`
	}
	if resp.StatusCode == http.StatusOK && json.Unmarshal(body, &probe) == nil && probe.Status != "" {
		return false // looks like a FlowPilot runner mid-boot
	}
	return true
}

func fetchSnapshot(cl *client.Client, ctx context.Context) client.LifecycleSnapshot {
	snap, err := cl.GetLifecycleSnapshot(ctx)
	if err != nil {
		return client.LifecycleSnapshot{}
	}
	return snap
}

// IsHealthyAndCompatible checks whether the health response is acceptable
// for reuse given the requested Config (status, version, cwd).
func IsHealthyAndCompatible(h client.HealthResponse, cfg Config) bool {
	return isHealthyAndCompatible(h, cfg)
}

func isHealthyAndCompatible(h client.HealthResponse, cfg Config) bool {
	if h.Status != "online" {
		return false
	}
	if cfg.ExpectedVersion != "" && h.RunnerVersion != cfg.ExpectedVersion {
		return false
	}
	if cfg.Workspace != "" && !strings.EqualFold(filepath.Clean(h.Cwd), filepath.Clean(cfg.Workspace)) {
		return false
	}
	return true
}

// FindWorkspaceRoot walks up from startDir looking for a FlowPilot root.
// A root is a directory that contains "apps/local-runner" OR ".agents".
// Falls back to startDir when nothing is found.
func FindWorkspaceRoot(startDir string) string {
	dir := startDir
	for {
		if looksLikeFlowPilotRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return startDir
		}
		dir = parent
	}
}

func looksLikeFlowPilotRoot(dir string) bool {
	_, err1 := os.Stat(filepath.Join(dir, "apps", "local-runner"))
	_, err2 := os.Stat(filepath.Join(dir, ".agents"))
	return err1 == nil || err2 == nil
}

// ApplyLiveRunnerDefaults mirrors scripts/supervisor.js: default
// FLOWPILOT_CODEX_APPSERVER=1 so Codex uses the real app-server, unless the
// caller already set the variable (including explicit disable via 0/false).
func ApplyLiveRunnerDefaults(env []string) []string {
	out := append([]string(nil), env...)
	prefix := codexAppServerEnv + "="
	for _, e := range out {
		if strings.HasPrefix(e, prefix) {
			return out
		}
	}
	return append(out, prefix+"1")
}

// serveArgs is the exact argv the spawned runner runs (Task-416 T-2):
// client-managed lifecycle mode makes the runner own its own lease/TTL/idle
// state instead of any client's process lifetime.
func serveArgs(cfg Config, workspace string) []string {
	return []string{
		"runner", "serve",
		"--workspace", workspace,
		"--host", cfg.Host,
		"--port", fmt.Sprintf("%d", cfg.Port),
		"--lifecycle-mode", "client-managed",
	}
}

func spawnRunner(cfg Config) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	workspace := cfg.Workspace
	if workspace == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine working directory: %w", err)
		}
		workspace = cwd
	}

	logPath := filepath.Join(workspace, logDir, logFile)
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return fmt.Errorf("cannot create log dir: %w", err)
	}
	logF, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file: %w", err)
	}

	cmd := exec.Command(exe, serveArgs(cfg, workspace)...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.Env = ApplyLiveRunnerDefaults(os.Environ())
	// CP-81 T-1: the shared runner is detached from this client's lifetime —
	// Setsid on Unix, DETACHED_PROCESS + no KILL_ON_JOB_CLOSE on Windows.
	// Orphan prevention moved to lease TTL expiry + idle grace shutdown.
	setSysProcAttrShared(cmd)

	if err := cmd.Start(); err != nil {
		logF.Close()
		return err
	}
	fmt.Fprintf(logF, "[runnerboot] runner pid=%d started\n", cmd.Process.Pid)
	// Don't wait — runner runs in background. Close the log file handle in this process.
	go func() {
		waitErr := cmd.Wait()
		fmt.Fprintf(logF, "[runnerboot] runner pid=%d exited: %v\n", cmd.Process.Pid, waitErr)
		logF.Close()
	}()
	return nil
}

func pollUntilHealthy(ctx context.Context, cl *client.Client, cfg Config) error {
	deadline := time.Now().Add(pollTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for runner", pollTimeout)
		}
		h, err := cl.Health(ctx)
		if err == nil && h.Status == "online" {
			return nil
		}
		time.Sleep(pollInterval)
	}
}
