package lsp

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerStatusInstalled(t *testing.T) {
	binDir := t.TempDir()
	lspWithBinariesOnPath(t, binDir, "gopls")
	ws := lspFixtureDir(t, map[string]string{"go.mod": "module x\n"})

	set := NewServerSet(nil)
	st := set.Status(ws)
	if st.Platform != "golang" || st.Binary != "gopls" {
		t.Fatalf("status = %+v", st)
	}
	if !st.Installed || st.Warn {
		t.Fatalf("status = %+v, want installed without warning", st)
	}
	if st.InstallHint == "" {
		t.Fatal("InstallHint must always be populated for registered platforms")
	}
}

func TestServerStatusMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no servers anywhere
	ws := lspFixtureDir(t, map[string]string{"go.mod": "module x\n"})

	set := NewServerSet(nil)
	st := set.Status(ws)
	if st.Platform != "golang" || st.Binary != "gopls" {
		t.Fatalf("status = %+v", st)
	}
	if st.Installed || !st.Warn {
		t.Fatalf("status = %+v, want missing with warning", st)
	}
	if !strings.Contains(st.Notice, "gopls") || !strings.Contains(st.Notice, st.InstallHint) {
		t.Fatalf("notice = %q, must name the binary and hint", st.Notice)
	}
	// Pure query: nothing spawned, stable across calls.
	st2 := set.Status(ws)
	if st2 != st {
		t.Fatalf("status not stable: %+v vs %+v", st, st2)
	}
	if n := set.ServerCount(); n != 0 {
		t.Fatalf("servers = %d, Status must never spawn", n)
	}
}

func TestServerStatusUnknownPlatform(t *testing.T) {
	set := NewServerSet(nil)
	ws := t.TempDir() // no markers -> "general"
	st := set.Status(ws)
	if st.Platform != "general" || st.Binary != "" || st.Warn || st.Installed {
		t.Fatalf("status = %+v, want quiet unregistered", st)
	}
	if got := set.Status(""); got.Warn || got.Platform != "" {
		t.Fatalf("empty root status = %+v", got)
	}
}

func TestServerSetMissingBinaryWarnsOnce(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ws := lspFixtureDir(t, map[string]string{"go.mod": "module x\n"})
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })

	set := NewServerSet(nil)
	for i := 0; i < 3; i++ {
		if got := set.CheckFiles(nil, ws, []string{"a.go"}); got != "" {
			t.Fatalf("check = %q, want empty", got)
		}
	}
	if n := strings.Count(buf.String(), "not found in PATH"); n != 1 {
		t.Fatalf("missing-binary warning logged %d times, want exactly once:\n%s", n, buf.String())
	}
}

func TestRegistryInstallHintsPresent(t *testing.T) {
	for platform, cfg := range DefaultRegistry() {
		if strings.TrimSpace(cfg.InstallHint) == "" {
			t.Fatalf("platform %q (%s) has no install hint", platform, cfg.Binary)
		}
	}
}
