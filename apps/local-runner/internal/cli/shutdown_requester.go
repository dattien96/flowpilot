package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// CA-913: the runner vanished mid-scaffold twice; CA-911 proved a POST
// /system/shutdown arrived from ua="Go-http-client/1.1" but could not say
// WHICH process sent it (TUI stayed alive, no QuitMsg). Remote port alone is
// useless post-hoc — resolve the owning local PID at request time via the
// system connection table, plus the X-Client marker callers self-report.
//
// describeRequester renders the identity block for the system-control log
// line: remote addr, user agent, X-Client, and the local process owning the
// client socket when resolvable (best-effort, bounded — never blocks teardown).
func describeRequester(r *http.Request) string {
	proc := ""
	if p := lookupRequesterProcess(r.RemoteAddr); p != "" {
		proc = " proc=" + p
	}
	return fmt.Sprintf("remote=%s ua=%q xclient=%q%s",
		r.RemoteAddr, r.UserAgent(), r.Header.Get("X-Client"), proc)
}

// lookupRequesterProcess maps the client's local TCP port (from remoteAddr)
// to the owning process: e.g. "pid=17264 name=flowpilot.exe". Injectable for
// tests. Returns "" when the mapping can't be resolved quickly.
var lookupRequesterProcess = lookupRequesterProcessDefault

func lookupRequesterProcessDefault(remoteAddr string) string {
	pid := remoteOwnerPID(remoteAddr)
	if pid <= 0 {
		return ""
	}
	return fmt.Sprintf("pid=%d name=%s", pid, processName(pid))
}

// remoteOwnerPID finds the local process holding remoteAddr's port open.
// Windows: `netstat -ano` — the row whose LOCAL endpoint equals remoteAddr is
// the client socket (server row has our listen port instead). Unix: lsof.
func remoteOwnerPID(remoteAddr string) int {
	host, port, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil || host == "" || port == "" {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	var out []byte
	if runtime.GOOS == "windows" {
		out, err = exec.CommandContext(ctx, "netstat", "-ano", "-p", "tcp").Output()
	} else {
		out, err = exec.CommandContext(ctx, "lsof", "-nP", "-iTCP:"+port, "-sTCP:ESTABLISHED", "-Fp").Output()
	}
	if err != nil {
		return 0
	}
	return parseOwnerPID(string(out), remoteAddr)
}

// parseOwnerPID extracts the PID owning remoteAddr's local endpoint from
// `netstat -ano` (windows) or `lsof -Fp` (unix) output.
func parseOwnerPID(output, remoteAddr string) int {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return 0
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// lsof -Fp: bare "p<pid>" records.
		if strings.HasPrefix(line, "p") && !strings.Contains(line, " ") {
			if pid, err := strconv.Atoi(line[1:]); err == nil && pid > 0 {
				return pid
			}
			continue
		}
		fields := strings.Fields(line)
		// netstat -ano -p tcp:  Proto  Local  Foreign  State  PID
		if len(fields) >= 5 && strings.EqualFold(fields[0], "TCP") && fields[1] == remoteAddr {
			if pid, err := strconv.Atoi(fields[len(fields)-1]); err == nil && pid > 0 {
				return pid
			}
		}
	}
	return 0
}

// processName resolves a PID to its executable name (Windows tasklist /
// unix ps). Empty on failure — callers tolerate it.
func processName(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var out []byte
	var err error
	if runtime.GOOS == "windows" {
		out, err = exec.CommandContext(ctx, "tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	} else {
		out, err = exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	}
	if err != nil {
		return "?"
	}
	text := strings.TrimSpace(string(out))
	if runtime.GOOS == "windows" {
		// CSV /NH: "name.exe","1234","Console","1","56,789 K"
		text = strings.TrimPrefix(text, "\"")
		if idx := strings.Index(text, "\""); idx > 0 {
			text = text[:idx]
		}
		if strings.HasPrefix(text, "INFO:") || text == "" {
			return "?"
		}
	}
	return text
}
