package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// BUG-456 (live run-6893): a frozen-scope coder that runs bare `go build`
// during verification emits an extension-less binary at repo root (e.g.
// `livebed`). The frozen-scope drift gate counts it as a written path and
// parks WAITING_USER_APPROVAL — yet flowgate.IsBinaryOrBuildArtifact already
// classifies exactly this shape as a non-code build artifact (CP-43 D-3:
// "bare root binaries like `gatesandbox`"), and the planner-side exclusion
// (isFlowPlannerExcludedPath) already exempts it via IsDocOrAuditFile.
// The exemption was lost when CA-427 Finding 2 replaced the broad
// IsDocOrAuditFile filter with exact-path checks — the binary clause must be
// restored without reopening the .flowpilot/** security hole.
//
// Additive file only — legacy suites untouched.

func TestGateDriftIgnoresBareRootBuildBinary(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"strutil.go"})
	rs := newP4ChildRun(svc, "child-bin", parentID, dir, head)

	// Declared source write + a `go build` artifact at repo root.
	p4WriteFile(t, dir, "strutil.go", "package main\n\nfunc Reverse(s string) string { return s }\n")
	bin := filepath.Join(dir, "livebed")
	if err := os.WriteFile(bin, []byte{0x7f, 'E', 'L', 'F'}, 0o755); err != nil {
		t.Fatal(err)
	}

	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{"strutil.go", "livebed"}}, 0) {
		t.Fatal("a bare-root go-build artifact must NOT count as scope drift")
	}
}

func TestGateDriftIgnoresBinaryExtensionsAndBuildDirs(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"strutil.go"})
	rs := newP4ChildRun(svc, "child-bins", parentID, dir, head)

	p4WriteFile(t, dir, "strutil.go", "package main\n\nfunc Reverse(s string) string { return s }\n")
	p4WriteFile(t, dir, "bin/tool.test", "\x00")
	p4WriteFile(t, dir, "coverage.out", "\x00")

	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{"strutil.go", "bin/tool.test", "coverage.out"}}, 0) {
		t.Fatal("binary extensions + build output dirs must NOT count as scope drift")
	}
}

// The exemption must not reopen CA-427 Finding 2: a writer-smuggled file that
// merely LOOKS non-code but is real source must still drift — e.g. an
// extension-less file under a subdir is not a bare-root binary.
func TestGateDriftStillBlocksExtensionlessSubdirFile(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"strutil.go"})
	rs := newP4ChildRun(svc, "child-subdir", parentID, dir, head)

	p4WriteFile(t, dir, "strutil.go", "package main\n\nfunc Reverse(s string) string { return s }\n")
	p4WriteFile(t, dir, "internal/payload", "package internal\n")

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{"strutil.go", "internal/payload"}}, 0) {
		t.Fatal("an undeclared extension-less file under a subdir must still count as drift")
	}
}
