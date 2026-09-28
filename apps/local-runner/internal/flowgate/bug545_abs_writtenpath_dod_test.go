package flowgate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BUG-545 (live run-34322 / tournament run-34296): provider file-change
// events (devin ACP) carry ABSOLUTE paths. When such a path lands in
// WrittenPaths / pendingGateCodePaths, MissingDodDocs flagged it "missing"
// via the filepath.IsAbs escape guard even though the doc sits inside the
// workspace with a valid DoD checklist — r-dod-present could never pass and
// the reprompt budget burned to an escalate wedge. The escape guard must
// keep out-of-workspace absolutes missing but resolve in-workspace ones.

func TestBug545_AbsolutePathInsideWorkspaceResolves(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/08-Task/done/Task-06.md"
	writeDodWorkspaceDoc(t, dir, rel, validDodDocTask100)
	abs, err := filepath.Abs(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if resolved, rerr := filepath.EvalSymlinks(dir); rerr == nil {
		abs = filepath.Join(resolved, filepath.FromSlash(rel))
	}
	got := MissingDodDocs(dir, []string{abs})
	if len(got) != 0 {
		t.Fatalf("MissingDodDocs(%q) = %v, want empty — doc exists inside workspace with a DoD checklist", abs, got)
	}
}

func TestBug545_AbsolutePathOutsideWorkspaceStillMissing(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	rel := "Task-999.md"
	abs := filepath.Join(outside, rel)
	if err := os.WriteFile(abs, []byte(validDodDocTask100), 0o644); err != nil {
		t.Fatal(err)
	}
	got := MissingDodDocs(dir, []string{abs})
	if len(got) != 1 {
		t.Fatalf("MissingDodDocs(%q) = %v, want flagged — escape guard must keep out-of-workspace absolutes missing", abs, got)
	}
}

func TestBug545_AbsolutePathInsideWorkspaceWithoutDodStillMissing(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/08-Task/done/Task-07.md"
	writeDodWorkspaceDoc(t, dir, rel, noDodDocTask100)
	abs, err := filepath.Abs(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if resolved, rerr := filepath.EvalSymlinks(dir); rerr == nil {
		abs = filepath.Join(resolved, filepath.FromSlash(rel))
	}
	got := MissingDodDocs(dir, []string{abs})
	if len(got) != 1 || !strings.Contains(got[0], "Task-07.md") {
		t.Fatalf("MissingDodDocs(%q) = %v, want flagged — doc lacks a DoD checklist", abs, got)
	}
}
