package cli

import (
	"os"
	"strings"
	"testing"
)

// CA-911: /system/shutdown exits the process silently (os.Exit(0)), so when the
// runner disappears mid-request the only evidence was cli-runner.log's
// "exited: <nil>" — with no record of WHO sent the POST. Both system control
// endpoints must log the caller's remote addr + user agent before tearing down.
func TestSystemShutdown_LogsRequesterIdentity(t *testing.T) {
	source, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatalf("read root.go: %v", err)
	}
	text := string(source)

	shutdownIdx := strings.Index(text, `mux.HandleFunc("/system/shutdown"`)
	if shutdownIdx < 0 {
		t.Fatal("missing /system/shutdown handler")
	}
	shutdownBody := text[shutdownIdx:]
	if end := strings.Index(shutdownBody, "mux.HandleFunc"); end > 0 {
		shutdownBody = shutdownBody[:end]
	}
	if !strings.Contains(shutdownBody, "RemoteAddr") || !strings.Contains(shutdownBody, "UserAgent") {
		t.Fatal("/system/shutdown must log r.RemoteAddr and r.UserAgent() before exiting")
	}

	restartIdx := strings.Index(text, `mux.HandleFunc("/system/restart"`)
	if restartIdx < 0 {
		t.Fatal("missing /system/restart handler")
	}
	restartBody := text[restartIdx:]
	if end := strings.Index(restartBody, "mux.HandleFunc"); end > 0 {
		restartBody = restartBody[:end]
	}
	if !strings.Contains(restartBody, "RemoteAddr") || !strings.Contains(restartBody, "UserAgent") {
		t.Fatal("/system/restart must log r.RemoteAddr and r.UserAgent() before restarting")
	}
}
