package skillpack

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-415: every SKILL.md installed from the embedded flow-pack must carry the
// current pack version marker so fileMatchesVersion reports current=true and a
// repeated bind-init is skipped instead of rewriting the whole pack.
func TestBug415_InstalledSkillsCarryPackVersion(t *testing.T) {
	target := t.TempDir()
	result, err := Install(target, "golang")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("install errors: %v", result.Errors)
	}
	if len(result.Installed) == 0 {
		t.Fatalf("expected skills to be installed")
	}
	var unversioned []string
	for _, dest := range result.Installed {
		if filepath.Base(dest) != "SKILL.md" {
			continue
		}
		if !fileMatchesVersion(dest, PackVersion) {
			unversioned = append(unversioned, dest)
		}
	}
	if len(unversioned) > 0 {
		t.Fatalf("%d installed SKILL.md files do not report version %d (first: %s)",
			len(unversioned), PackVersion, unversioned[0])
	}
}

// BUG-415: a second Install on an unchanged pack must skip every file —
// today it rewrites everything because the sources lack version markers.
func TestBug415_ReinstallIsIdempotent(t *testing.T) {
	target := t.TempDir()
	if _, err := Install(target, "golang"); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	second, err := Install(target, "golang")
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if len(second.Installed) != 0 {
		t.Fatalf("second Install rewrote %d files; want 0 (pack unchanged)", len(second.Installed))
	}
	if len(second.Skipped) == 0 {
		t.Fatalf("second Install skipped nothing; expected all files skipped")
	}
}

// BUG-415: Status must report the freshly installed pack as current.
func TestBug415_StatusReportsCurrentAfterInstall(t *testing.T) {
	target := t.TempDir()
	if _, err := Install(target, "golang"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	status, err := Status(target, "golang")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Installed || !status.Current {
		t.Fatalf("Status = installed:%v current:%v; want installed+current", status.Installed, status.Current)
	}
	for _, sk := range status.Skills {
		for _, p := range sk.Providers {
			if !p.Current {
				t.Errorf("skill %q provider %q not current at %s", sk.Name, p.Provider, p.Path)
			}
		}
	}
}

// BUG-415: a file stamped with an OLDER version must still be detected as
// stale — the stamp fix must not blanket-approve every file.
func TestBug415_OlderVersionStillDetectedStale(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "SKILL.md")
	content := "---\nname: demo\nversion: 5\n---\nbody\n"
	if err := os.WriteFile(stale, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if fileMatchesVersion(stale, PackVersion) {
		t.Fatalf("version-5 file reported current for pack version %d", PackVersion)
	}
}
