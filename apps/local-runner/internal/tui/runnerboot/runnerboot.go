// Package runnerboot auto-ensures a local runner process is online for the
// `flowpilot chat` command.  It never imports flowpilot-runner/internal/runner.
//
// Heuristic (CP-56):
//  1. If --runner-url is set, verify health only (no auto-start).
//  2. Else if --no-start-runner, verify the default candidate (host:port) health only.
//  3. Else health-check the candidate URL:
//     a. If healthy AND (status==online AND runnerVersion AND cwd match) → reuse.
//     b. Else spawn `<exe> runner serve --workspace <root> --host <host> --port <port>`
//        detached, log to <root>/.flowpilot/cli-runner.log, then poll until online.
//
// Skill: cli-tui (CP-56)
package runnerboot

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/tui/client"
)

const (
	pollInterval    = 300 * time.Millisecond
	pollTimeout     = 20 * time.Second
	logDir          = ".flowpilot"
	logFile         = "cli-runner.log"
	// Live Codex for TUI-spawned runners (parity with scripts/supervisor.js / just dev).
	// Without this, ProviderRegistryFor keeps the fake Codex adapter and chat returns
	// scripted "Sure — let me work through this step…" deltas.
	codexAppServerEnv = "FLOWPILOT_CODEX_APPSERVER"
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
}

// Result describes what EnsureRunner did.
type Result struct {
	RunnerURL string
	Launched  bool
	Reused    bool
}

// EnsureRunner ensures a runner is available and returns its base URL.
// It is safe to call from tests with a short ctx deadline to limit spawn attempts.
func EnsureRunner(ctx context.Context, cfg Config) (Result, error) {
	candidateURL := cfg.ExplicitURL
	if candidateURL == "" {
		candidateURL = fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	}

	cl := client.New(candidateURL)

	// Fast path: check health immediately.
	h, err := cl.Health(ctx)
	if err == nil && isHealthyAndCompatible(h, cfg) {
		return Result{RunnerURL: candidateURL, Reused: true}, nil
	}

	// If an explicit URL was given or no-start is set, don't spawn.
	if cfg.ExplicitURL != "" || cfg.NoStart {
		if err != nil {
			return Result{}, fmt.Errorf("runner not reachable at %s: %w", candidateURL, err)
		}
		return Result{RunnerURL: candidateURL, Reused: true}, nil
	}

	// Spawn a new runner.
	if err := spawnRunner(cfg); err != nil {
		return Result{}, fmt.Errorf("failed to spawn runner: %w", err)
	}

	// Poll until healthy.
	if err := pollUntilHealthy(ctx, cl, cfg); err != nil {
		return Result{}, fmt.Errorf("runner did not come online: %w", err)
	}

	return Result{RunnerURL: candidateURL, Launched: true}, nil
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

	args := []string{
		"runner", "serve",
		"--workspace", workspace,
		"--host", cfg.Host,
		"--port", fmt.Sprintf("%d", cfg.Port),
	}

	cmd := exec.Command(exe, args...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.Env = ApplyLiveRunnerDefaults(os.Environ())
	// Stay in the TUI process/console group so closing the terminal (not only
	// /exit) tears the runner down. CA-445 detached this so a reused Desktop
	// runner could outlive TUI; that leaked listeners on --port.
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		logF.Close()
		return err
	}
	// KILL_ON_JOB_CLOSE: when this TUI process dies (force-kill, crash, or
	// terminal close) Windows terminates the runner too — a wedged TUI can
	// never orphan the --port listener (BUG-328).
	if err := assignRunnerJob(uint32(cmd.Process.Pid)); err != nil {
		fmt.Fprintf(logF, "[runnerboot] job-assign skipped (runner may outlive TUI): %v\n", err)
	}
	// Don't wait — runner runs in background. Close the log file handle in this process.
	go func() {
		cmd.Wait() //nolint:errcheck
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
