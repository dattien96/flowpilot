package runner

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/docscan"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })
	return &buf
}

// BUG-422: CP-48 requires `docscan_scan_completed files_scanned=N issues_found=M`
// after every scan — none is emitted today.
func TestBug422_ScanEmitsAuditLine(t *testing.T) {
	root := t.TempDir()
	writeSandboxFile(t, root, "requirements/05-System-Specs/SS-01-Auth.md", sampleSSDoc)
	writeSandboxFile(t, root, "requirements/06-System-Tech-Design/SD-01-Auth.md", sampleSSDoc)

	buf := captureLogs(t)
	report, err := scanDocFiles([]string{
		filepath.Join(root, "requirements/05-System-Specs/SS-01-Auth.md"),
		filepath.Join(root, "requirements/06-System-Tech-Design/SD-01-Auth.md"),
	})
	if err != nil {
		t.Fatalf("scanDocFiles: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "docscan_scan_completed") {
		t.Fatalf("missing docscan_scan_completed audit line; log=%q", out)
	}
	if !strings.Contains(out, "files_scanned=2") {
		t.Fatalf("audit line missing files_scanned=2; log=%q", out)
	}
	if !strings.Contains(out, "issues_found=") {
		t.Fatalf("audit line missing issues_found=; log=%q", out)
	}
	_ = report
}

// BUG-422: the ScanDirectory path (full-project conformance + SS-Lock
// post-publish scan) must emit the same audit line.
func TestBug422_ScanDirectoryEmitsAuditLine(t *testing.T) {
	root := t.TempDir()
	writeSandboxFile(t, root, "requirements/05-System-Specs/SS-01-Auth.md", sampleSSDoc)

	buf := captureLogs(t)
	if _, err := docscan.ScanDirectory(filepath.Join(root, "requirements")); err != nil {
		t.Fatalf("ScanDirectory: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "docscan_scan_completed") || !strings.Contains(out, "files_scanned=1") {
		t.Fatalf("missing docscan_scan_completed files_scanned=1; log=%q", out)
	}
}

// BUG-422: every repaired file must log
// `docscan_autofix_applied file=… changes=…`.
func TestBug422_AutofixEmitsAuditLinePerRepairedFile(t *testing.T) {
	root := t.TempDir()
	// A minimal draft missing metadata/AI-Quick-View — autofix will change it.
	draft := writeSandboxFile(t, root,
		"requirements/05-System-Specs/todo/SS-99-Demo.md",
		"# SS-99: Demo\n\nSome draft body without the required blocks.\n")

	buf := captureLogs(t)
	if err := autoFixDraftDocs([]string{draft}); err != nil {
		t.Fatalf("autoFixDraftDocs: %v", err)
	}

	after, err := os.ReadFile(draft)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "without the required blocks") && !strings.Contains(string(after), "AI Quick View") {
		t.Fatalf("precondition failed: draft was not modified by autofix")
	}

	out := buf.String()
	if !strings.Contains(out, "docscan_autofix_applied") {
		t.Fatalf("missing docscan_autofix_applied audit line; log=%q", out)
	}
	if !strings.Contains(out, "file=") || !strings.Contains(out, "changes=") {
		t.Fatalf("audit line missing file=/changes= keys; log=%q", out)
	}
}
