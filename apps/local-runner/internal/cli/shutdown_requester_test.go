package cli

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// CA-913: CA-911's remote+UA log proved the mystery shutdown came from a Go
// client but couldn't say WHICH process (several flowpilot.exe share the same
// default UA). At request time the runner must resolve the client's local
// port back to the owning PID via the system connection table, and honor the
// X-Client marker that cooperative callers send.

func TestParseOwnerPID_NetstatWindows(t *testing.T) {
	// `netstat -ano -p tcp` shape on Windows. The client socket row has the
	// remote addr in the LOCAL column; the server row has our listen port.
	fixture := `
  Proto  Local Address          Foreign Address        State           PID
  TCP    127.0.0.1:4317         127.0.0.1:59519        ESTABLISHED     4804
  TCP    127.0.0.1:59519        127.0.0.1:4317         ESTABLISHED     17264
  TCP    127.0.0.1:59519        127.0.0.1:4317         TIME_WAIT       0
`
	if pid := parseOwnerPID(fixture, "127.0.0.1:59519"); pid != 17264 {
		t.Fatalf("expected client pid 17264, got %d", pid)
	}
}

func TestParseOwnerPID_IgnoresServerRowAndMissing(t *testing.T) {
	fixture := `
  TCP    127.0.0.1:4317         127.0.0.1:59519        ESTABLISHED     4804
  TCP    127.0.0.1:59519        127.0.0.1:4317         ESTABLISHED     99999
`
	// Querying the server-side addr must not return the client's row.
	if pid := parseOwnerPID(fixture, "127.0.0.1:4317"); pid != 4804 {
		t.Fatalf("expected server pid 4804 for its own local endpoint, got %d", pid)
	}
	if pid := parseOwnerPID(fixture, ""); pid != 0 {
		t.Fatalf("empty remoteAddr must yield 0, got %d", pid)
	}
	if pid := parseOwnerPID("no rows", "127.0.0.1:1"); pid != 0 {
		t.Fatalf("unmatched port must yield 0, got %d", pid)
	}
}

func TestParseOwnerPID_LsofUnix(t *testing.T) {
	// lsof -Fp emits bare p<pid> records.
	if pid := parseOwnerPID("p22108\nf3\n", "127.0.0.1:59519"); pid != 22108 {
		t.Fatalf("expected 22108, got %d", pid)
	}
}

func TestDescribeRequester_IncludesClientMarkerAndProc(t *testing.T) {
	defer func() { lookupRequesterProcess = lookupRequesterProcessDefault }()
	lookupRequesterProcess = func(remoteAddr string) string {
		if remoteAddr != "127.0.0.1:59519" {
			return ""
		}
		return "pid=17264 name=flowpilot.exe"
	}

	r := httptest.NewRequest("POST", "/system/shutdown", nil)
	r.RemoteAddr = "127.0.0.1:59519"
	r.Header.Set("User-Agent", "Go-http-client/1.1")
	r.Header.Set("X-Client", "tui")

	got := describeRequester(r)
	for _, want := range []string{
		`remote=127.0.0.1:59519`,
		`ua="Go-http-client/1.1"`,
		`xclient="tui"`,
		`proc=pid=17264 name=flowpilot.exe`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("describeRequester missing %q in %q", want, got)
		}
	}
}
