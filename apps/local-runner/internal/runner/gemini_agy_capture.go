package runner

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ansiSequenceRe matches VT/ANSI escape sequences that may appear in captured
// terminal output: CSI sequences (cursor moves, screen clears, SGR), OSC
// sequences (window-title set), and other Fe escapes.
var ansiSequenceRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

// stripTerminalSequences removes ANSI/VT escape sequences and stray C0 control
// bytes from captured terminal output, leaving the plain-text response. CR/LF is
// normalised to LF; tabs and newlines are preserved.
func stripTerminalSequences(s string) string {
	s = ansiSequenceRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// captureBufferedCommand runs cmd capturing stdout/stderr into buffers. This is
// the ordinary path used on macOS/Linux and for test stubs on every platform.
func captureBufferedCommand(cmd *exec.Cmd) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// isAgyCommand reports whether cmd will execute the real agy binary rather than
// a test shell stub (which runs `sh`). Tests use this to keep shell stubs on the
// hermetic buffered path.
func isAgyCommand(cmd *exec.Cmd) bool {
	if cmd == nil || len(cmd.Args) == 0 {
		return false
	}
	want := agyExecBaseName(geminiBinaryName())
	got := agyExecBaseName(cmd.Args[0])
	return want != "" && got == want
}

func agyExecBaseName(p string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(p)))
	return strings.TrimSuffix(base, ".exe")
}
