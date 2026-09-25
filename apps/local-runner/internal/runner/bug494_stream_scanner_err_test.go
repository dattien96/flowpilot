package runner

// BUG-494: the Claude print-mode capture goroutines scanned stdout/stderr
// with bufio.Scanner but never checked scanner.Err() after the loop. A
// mid-stream read error — or a single line exceeding the 10 MB token cap —
// terminated the loop silently: captured output was truncated while the
// command exit looked fine, producing a misleading "response did not
// include a result event" (or worse, partial stderr in error reports).
// The contract: stream readers surface scanner errors, never treat
// truncation as EOF.

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

// errAfterReader yields the payload once, then fails — a mid-stream read
// error that bufio.Scanner surfaces via Err() after Scan() returns false.
type errAfterReader struct {
	payload []byte
	done    bool
	err     error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.payload), nil
}

func TestBUG494_ScanStreamLinesSurfacesMidStreamError(t *testing.T) {
	readErr := errors.New("read: connection reset")
	r := &errAfterReader{payload: []byte("line one\nline two\n"), err: readErr}
	scanner := bufio.NewScanner(r)
	var got []string
	err := scanStreamLines(scanner, func(line []byte) {
		got = append(got, string(line))
	})
	if !errors.Is(err, readErr) {
		t.Fatalf("scanStreamLines err = %v, want %v", err, readErr)
	}
	// Lines before the fault are still delivered — partial capture is
	// allowed, silent truncation is not.
	if len(got) != 2 {
		t.Fatalf("got %d lines before fault, want 2", len(got))
	}
}

func TestBUG494_ScanStreamLinesSurfacesOversizeLine(t *testing.T) {
	// A single line larger than the 10 MB cap makes Scan() return false
	// with Err() = bufio.ErrTooLong — previously swallowed as clean EOF.
	huge := strings.Repeat("x", 11*1024*1024)
	scanner := bufio.NewScanner(strings.NewReader(huge))
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	var got int
	err := scanStreamLines(scanner, func([]byte) { got++ })
	if err == nil {
		t.Fatal("oversize line must surface ErrTooLong, not silent EOF")
	}
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("err = %v, want bufio.ErrTooLong", err)
	}
	if got != 0 {
		t.Fatalf("got %d lines, want 0 (oversize token not delivered)", got)
	}
}

func TestBUG494_ScanStreamLinesHealthyEOF(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("a\nb\nc\n"))
	var got []string
	err := scanStreamLines(scanner, func(line []byte) {
		got = append(got, string(line))
	})
	if err != nil {
		t.Fatalf("clean EOF must return nil, got %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
}

func TestBUG494_ScanStreamLinesEmptyReader(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(""))
	var got int
	err := scanStreamLines(scanner, func([]byte) { got++ })
	if err != nil {
		t.Fatalf("empty reader: err = %v, want nil", err)
	}
	if got != 0 {
		t.Fatalf("got %d lines, want 0", got)
	}
}

// Guard: io.EOF from the underlying reader is not an error to surface.
func TestBUG494_ScanStreamLinesPureEOFFault(t *testing.T) {
	r := &errAfterReader{payload: []byte("x\n"), err: io.EOF}
	scanner := bufio.NewScanner(r)
	var got int
	err := scanStreamLines(scanner, func([]byte) { got++ })
	if err != nil {
		t.Fatalf("EOF termination: err = %v, want nil", err)
	}
	if got != 1 {
		t.Fatalf("got %d lines, want 1", got)
	}
}
