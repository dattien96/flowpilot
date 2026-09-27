package runner

import (
	"testing"

	"flowpilot-runner/internal/flowgate"
)

// BUG-532 (live run-4429): a normal_chat turn that wrote notes.txt triggered
// the r-tests reprompt ("you changed production code... ADD a new test
// file") — a plain text file counted as production code because
// IsDocOrAuditFile only exempted md/requirements/change-audit/.flowpilot.
// The agent was forced to invent nonsense coverage (a test asserting the
// haiku file exists).
func TestBug532_NonCodeDocFilesDoNotCountAsCodeChanges(t *testing.T) {
	for _, p := range []string{
		"notes.txt",
		"docs/readme.txt",
		"data/export.csv",
		"data/table.tsv",
		"logs/session.log",
		"guide.rst",
		"guide.adoc",
		"manual.pdf",
	} {
		if !flowgate.IsDocOrAuditFile(p) {
			t.Fatalf("%s is a doc/data file, not production code", p)
		}
	}

	// Guard the boundary: real code and code-adjacent config still count.
	for _, p := range []string{
		"main.go",
		"handler_test.go",
		"config.yaml",
		"settings.json",
		"script.sh",
		"Makefile",
	} {
		if flowgate.IsDocOrAuditFile(p) {
			t.Fatalf("%s must still count as code-adjacent (not doc)", p)
		}
	}

	// The r-tests trigger shape: a diff of ONLY the notes file must not
	// report code changes.
	diff := []flowgate.ChangedFile{{Path: "notes.txt", Status: "A"}}
	if flowgate.HasCodeChanges(diff) {
		t.Fatal("a notes.txt-only diff must not report code changes (BUG-532)")
	}
}
