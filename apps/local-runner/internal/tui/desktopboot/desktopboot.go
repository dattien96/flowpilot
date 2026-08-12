// Package desktopboot ensures the Desktop app (Vite/Electron) is reachable
// for `flowpilot chat` slash commands like /settings. It never imports
// flowpilot-runner/internal/runner.
//
// Skill: cli-tui (CP-56)
package desktopboot

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHost = "127.0.0.1"
	defaultPort = 5173
	dialTimeout = 400 * time.Millisecond
	logDir      = ".flowpilot"
	logFile     = "cli-desktop.log"
)

// Config holds inputs for EnsureDesktop.
type Config struct {
	// Workspace is the FlowPilot repo root (contains apps/desktop-flowpilot).
	Workspace string
	// Host/Port are the Desktop Vite bind address (FLOWPILOT_DESKTOP_PORT).
	Host string
	Port int
	// RunnerURL is passed as VITE_RUNNER_URL so Desktop talks to the same runner.
	RunnerURL string
	// DesktopDir overrides apps/desktop-flowpilot under Workspace when set.
	DesktopDir string
}

// Result describes what EnsureDesktop did.
type Result struct {
	URL      string
	Launched bool
	Reused   bool
}

// EnsureDesktop returns immediately if Desktop already accepts TCP on host:port.
// Otherwise it spawns `npm run dev` detached (best-effort) and returns.
func EnsureDesktop(cfg Config) (Result, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = defaultHost
	}
	port := cfg.Port
	if port <= 0 {
		port = defaultPort
	}
	url := fmt.Sprintf("http://%s:%d", host, port)
	out := Result{URL: url}

	if portOpen(host, port) {
		out.Reused = true
		return out, nil
	}

	desktopDir, err := resolveDesktopDir(cfg)
	if err != nil {
		return out, err
	}
	if err := spawnDesktop(cfg, desktopDir, host, port); err != nil {
		return out, err
	}
	out.Launched = true
	return out, nil
}

// PortOpen reports whether host:port accepts a TCP connection.
func PortOpen(host string, port int) bool {
	if strings.TrimSpace(host) == "" {
		host = defaultHost
	}
	if port <= 0 {
		port = defaultPort
	}
	return portOpen(host, port)
}

func portOpen(host string, port int) bool {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func resolveDesktopDir(cfg Config) (string, error) {
	if d := strings.TrimSpace(cfg.DesktopDir); d != "" {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d, nil
		}
		return "", fmt.Errorf("desktop dir not found: %s", d)
	}
	ws := strings.TrimSpace(cfg.Workspace)
	if ws == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		ws = cwd
	}
	candidate := filepath.Join(ws, "apps", "desktop-flowpilot")
	if st, err := os.Stat(candidate); err == nil && st.IsDir() {
		return candidate, nil
	}
	return "", fmt.Errorf("desktop app not found at %s — set workspace to the FlowPilot checkout", candidate)
}

func spawnDesktop(cfg Config, desktopDir, host string, port int) error {
	npm := "npm"
	if runtime.GOOS == "windows" {
		npm = "npm.cmd"
	}
	args := []string{
		"run", "dev", "--",
		"--port", strconv.Itoa(port),
		"--strictPort",
		"--host", host,
	}

	logRoot := strings.TrimSpace(cfg.Workspace)
	if logRoot == "" {
		logRoot = desktopDir
	}
	logPath := filepath.Join(logRoot, logDir, logFile)
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return fmt.Errorf("cannot create desktop log dir: %w", err)
	}
	logF, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open desktop log: %w", err)
	}

	cmd := exec.Command(npm, args...)
	cmd.Dir = desktopDir
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.Env = append(os.Environ(),
		"VITE_RUNNER_URL="+strings.TrimSpace(cfg.RunnerURL),
	)
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		logF.Close()
		return fmt.Errorf("spawn desktop: %w", err)
	}
	go func() {
		_ = cmd.Wait()
		logF.Close()
	}()
	return nil
}

// DefaultPort returns FLOWPILOT_DESKTOP_PORT or 5173.
func DefaultPort() int {
	v := strings.TrimSpace(os.Getenv("FLOWPILOT_DESKTOP_PORT"))
	if v == "" {
		return defaultPort
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultPort
	}
	return n
}
