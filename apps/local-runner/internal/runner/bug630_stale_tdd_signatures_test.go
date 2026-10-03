package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-630 (live run-174243): an abandoned leg's Task-033 block in
// tdd-signatures.md survived into a NEW run for the same task — the file
// existence check counted it as this run's TDD evidence, and the
// whole-file last-wins waived parse let other tasks' RED expectations
// leak across sections. Signature evidence must be (a) scoped to the
// current task's section and (b) attested by THIS run's scaffold leg
// (the leg actually wrote the file under its gate turn).

const bug630Signatures = `# TDD signatures — crypto-ndk (Task-032)

## Production surface
### vault_container.cpp
` + "```cpp\nclass VaultContainer {};\n```" + `

## RED gate expectation
- red_tests: ` + "`[]`" + `
- failure_type: ` + "`none`" + `

# TDD signatures — crypto-ndk guard (Task-033)

## Production surface
### VaultContainer.h
` + "```cpp\nclass ProtectedKeyGuard {};\n```" + `

## RED gate expectation
- red_tests: ` + "`[\"ProtectedKeyGuardTest_x\"]`" + `
- failure_type: ` + "`not_implemented`" + `
`

func bug630WriteSigs(t *testing.T, cwd, body string) {
	t.Helper()
	p := filepath.Join(cwd, filepath.FromSlash(vibeTddSignaturesRel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBUG630_TaskSectionParsing(t *testing.T) {
	cwd := t.TempDir()
	bug630WriteSigs(t, cwd, bug630Signatures)
	if _, ok := vibeTddSignaturesTaskSection(cwd, "Task-032"); !ok {
		t.Fatal("Task-032 section must parse")
	}
	sec, ok := vibeTddSignaturesTaskSection(cwd, "Task-033")
	if !ok {
		t.Fatal("Task-033 section must parse")
	}
	if os.Getenv("X") == "" && len(sec) == 0 {
		t.Fatal("section must be non-empty")
	}
	if _, ok := vibeTddSignaturesTaskSection(cwd, "Task-034"); ok {
		t.Fatal("absent task must not match")
	}
}

// The waiver file carries Task-032's declared zero-red AND Task-033's real
// red list. Whole-file last-wins reads Task-033's values for everyone —
// section-scoped must isolate each task's own expectation.
func TestBUG630_ScaffoldRedWaivedScopedToTask(t *testing.T) {
	cwd := t.TempDir()
	bug630WriteSigs(t, cwd, bug630Signatures)
	if !vibeScaffoldRedWaivedForTask(cwd, "Task-032") {
		t.Fatal("Task-032 declares zero-red — must waive")
	}
	if vibeScaffoldRedWaivedForTask(cwd, "Task-033") {
		t.Fatal("Task-033 declares real red tests — must not inherit Task-032's waiver")
	}
	if vibeScaffoldRedWaivedForTask(cwd, "Task-034") {
		t.Fatal("task with no section must not be waived")
	}
}

// Live failure: stale Task-033 block written by an abandoned leg. The new
// run's scaffold has not attested it — file evidence must not count.
func TestBUG630_StaleBlockNotEvidenceWithoutAttestation(t *testing.T) {
	cwd := t.TempDir()
	bug630WriteSigs(t, cwd, bug630Signatures)
	rs := &interactiveRun{vibeTddSigAttestedTask: ""}
	if vibeTddFileEvidencePresent(rs, cwd, "Task-033") {
		t.Fatal("un-attested signatures file must not count as TDD evidence (BUG-630)")
	}
}

func TestBUG630_AttestedSectionCounts(t *testing.T) {
	cwd := t.TempDir()
	bug630WriteSigs(t, cwd, bug630Signatures)
	rs := &interactiveRun{vibeTddSigAttestedTask: "Task-033"}
	if !vibeTddFileEvidencePresent(rs, cwd, "Task-033") {
		t.Fatal("section attested by this run's scaffold must count")
	}
}

func TestBUG630_AttestationForOtherTaskDoesNotCount(t *testing.T) {
	cwd := t.TempDir()
	bug630WriteSigs(t, cwd, bug630Signatures)
	rs := &interactiveRun{vibeTddSigAttestedTask: "Task-032"}
	if vibeTddFileEvidencePresent(rs, cwd, "Task-033") {
		t.Fatal("attestation for a different task must not satisfy this task's evidence")
	}
}

// Legacy/unscoped posture preserved: when the run carries no task id the
// whole-file check applies (pre-fix runs, non-task-labelled fixtures).
func TestBUG630_UnknownTaskKeepsLegacyCheck(t *testing.T) {
	cwd := t.TempDir()
	bug630WriteSigs(t, cwd, bug630Signatures)
	rs := &interactiveRun{}
	if !vibeTddFileEvidencePresent(rs, cwd, "") {
		t.Fatal("empty task id must fall back to whole-file evidence")
	}
}
