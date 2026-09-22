//go:build windows

package runnerboot

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

// killRunnerOnPort terminates the process LISTENing on the runner's port via
// netstat + taskkill /T (tree kill covers go.exe wrapper → compiled child,
// preserving BUG-240). Only invoked by KillRunnerFenced AFTER the runner
// accepted shutdown and live health still reports the expected
// runnerInstanceId — never on an unidentified listener.
func killRunnerOnPort(runnerURL string) error {
	u, err := url.Parse(runnerURL)
	if err != nil {
		return fmt.Errorf("parse runner URL: %w", err)
	}
	port := u.Port()
	if port == "" {
		return fmt.Errorf("no port in runner URL %q", runnerURL)
	}
	pid, err := findListenerPID(port)
	if err != nil || pid <= 0 {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

// findListenerPID maps a TCP LISTENING port to a pid via netstat, excluding
// this process.
func findListenerPID(port string) (int, error) {
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
		if !localAddrHasPort(fields[1], port) {
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

// localAddrHasPort reports whether a netstat local-address column ends with
// the wanted port (handles IPv4 0.0.0.0:PORT and IPv6 [::]:PORT rows).
func localAddrHasPort(localAddr, port string) bool {
	idx := strings.LastIndex(localAddr, ":")
	if idx < 0 {
		return false
	}
	return localAddr[idx+1:] == port
}
