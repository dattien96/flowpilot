//go:build !windows

package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// killRunnerByURL extracts the port from the runner URL and sends SIGTERM to
// the process LISTENing on that port.  Excludes current TUI process ID (myPID)
// so the TUI never sends SIGTERM to itself.
//
// Skill: cli-tui (CP-56)
func killRunnerByURL(runnerURL string) error {
	u, err := url.Parse(runnerURL)
	if err != nil {
		return fmt.Errorf("parse runner URL: %w", err)
	}
	port := u.Port()
	if port == "" {
		return fmt.Errorf("no port in runner URL %q", runnerURL)
	}

	pid, err := findPIDOnPort(port)
	if err != nil || pid <= 0 {
		return err
	}

	// SIGTERM the process group first (runnerboot Setsid), then the listen PID.
	// Killing only the listen PID orphans grok agent / MCP children.
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)
	return nil
}

// findPIDOnPort uses `lsof` to find the PID listening on the given TCP port.
// Restricts search to LISTEN sockets (-sTCP:LISTEN) and excludes current process ID (myPID).
func findPIDOnPort(port string) (int, error) {
	myPID := os.Getpid()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	out, err := exec.CommandContext(ctx, "lsof", "-n", "-P", "-i", "tcp:"+port, "-sTCP:LISTEN", "-t").Output()
	if err != nil {
		return 0, nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, perr := strconv.Atoi(line)
		if perr != nil || pid == myPID {
			continue
		}
		return pid, nil
	}
	return 0, nil
}
