package desktopboot_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/desktopboot"
)

func TestPortOpen_TrueWhenListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	if !desktopboot.PortOpen("127.0.0.1", port) {
		t.Fatal("expected port open")
	}
}

func TestPortOpen_FalseWhenClosed(t *testing.T) {
	if desktopboot.PortOpen("127.0.0.1", 1) {
		t.Fatal("port 1 should be closed")
	}
}

func TestEnsureDesktop_ReusesWhenAlreadyUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	u := strings.TrimPrefix(srv.URL, "http://")
	host, portStr, _ := net.SplitHostPort(u)
	port, _ := strconv.Atoi(portStr)

	res, err := desktopboot.EnsureDesktop(desktopboot.Config{
		Host:      host,
		Port:      port,
		Workspace: t.TempDir(), // unused on reuse path
		RunnerURL: "http://127.0.0.1:4317",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reused || res.Launched {
		t.Fatalf("res=%+v want reused", res)
	}
}

func TestEnsureDesktop_MissingDesktopDir(t *testing.T) {
	dir := t.TempDir()
	_, err := desktopboot.EnsureDesktop(desktopboot.Config{
		Host:      "127.0.0.1",
		Port:      59999, // assume closed
		Workspace: dir,
		RunnerURL: "http://127.0.0.1:4317",
	})
	if err == nil || !strings.Contains(err.Error(), "desktop app not found") {
		t.Fatalf("err=%v", err)
	}
}
