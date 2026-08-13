//go:build windows

package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// killRunnerByURL on Windows uses `netstat` + `taskkill` to find and terminate
// the runner process listening on the given port. Excludes current process ID (myPID).
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

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(pid), "/F").Run()
}

// findPIDOnPort uses netstat to find the PID listening on the given TCP port on Windows.
func findPIDOnPort(port string) (int, error) {
	myPID := os.Getpid()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	out, err := exec.CommandContext(ctx, "netstat", "-ano").Output()
	if err != nil {
		return 0, nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "LISTENING") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if !tcpLocalAddrHasPort(fields[1], port) {
			continue
		}
		pid, perr := strconv.Atoi(fields[4])
		if perr != nil || pid == myPID {
			continue
		}
		return pid, nil
	}
	return 0, nil
}
