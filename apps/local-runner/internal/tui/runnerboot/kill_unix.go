//go:build !windows

package runnerboot

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

// killRunnerOnPort SIGTERMs the process LISTENing on the runner's port.
// Only invoked by KillRunnerFenced AFTER the runner accepted shutdown and the
// live health check still reports the expected runnerInstanceId — never on an
// unidentified listener.
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
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)
	return nil
}

// findListenerPID maps a TCP LISTEN port to a pid via lsof, excluding this
// process.
func findListenerPID(port string) (int, error) {
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
