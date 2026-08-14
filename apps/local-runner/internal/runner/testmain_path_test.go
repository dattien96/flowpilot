package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestMain prepends Git's sh.exe to PATH on Windows so live-provider and
// shell-script probe tests can run without a manual env setup.
func TestMain(m *testing.M) {
	ensureUnixShellOnPathForTests()
	os.Exit(m.Run())
}

func ensureUnixShellOnPathForTests() {
	if _, err := exec.LookPath("sh"); err == nil {
		return
	}
	if runtime.GOOS != "windows" {
		return
	}
	var candidates []string
	if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
		// User-scoped Git for Windows install.
		candidates = append(candidates,
			filepath.Join(localApp, "Programs", "Git", "bin"),
			filepath.Join(localApp, "Programs", "Git", "usr", "bin"),
		)
	}
	candidates = append(candidates,
		`C:\Program Files\Git\bin`,
		`C:\Program Files\Git\usr\bin`,
		`C:\Program Files (x86)\Git\bin`,
	)
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		candidates = append(candidates, filepath.Join(pf, "Git", "bin"), filepath.Join(pf, "Git", "usr", "bin"))
	}
	for _, dir := range candidates {
		sh := filepath.Join(dir, "sh.exe")
		if st, err := os.Stat(sh); err == nil && !st.IsDir() {
			os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			return
		}
	}
}

// skipIfNoUnixShell skips when sh is still unavailable after TestMain PATH fix.
func skipIfNoUnixShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("requires sh on PATH for live provider / shell probe tests: %v", err)
	}
}
