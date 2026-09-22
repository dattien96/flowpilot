package app

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

// CA-636 regression tests: a wedged console output (Windows text selection
// blocks WriteConsole until a keypress) must never freeze the TUI event loop.
// The queuedOutput writer enqueues frames and returns immediately; the drain
// goroutine is the only thing that can stall on a blocked console.

// gateWriter blocks every Write until unblock() is called — simulates a
// console with an active selection.
type gateWriter struct {
	mu       sync.Mutex
	blocked  bool
	unblockC chan struct{}
	written  bytes.Buffer
	writes   int
}

func newGateWriter(blocked bool) *gateWriter {
	g := &gateWriter{blocked: blocked}
	if blocked {
		g.unblockC = make(chan struct{})
	} else {
		g.unblockC = closedCh()
	}
	return g
}

func closedCh() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (g *gateWriter) Write(p []byte) (int, error) {
	g.mu.Lock()
	blocked := g.blocked
	unblockC := g.unblockC
	g.mu.Unlock()
	if blocked {
		<-unblockC // release only on unblock()
	}
	g.mu.Lock()
	g.writes++
	g.written.Write(p)
	g.mu.Unlock()
	return len(p), nil
}

func (g *gateWriter) unblock() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.blocked {
		g.blocked = false
		close(g.unblockC)
	}
}

func (g *gateWriter) content() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.written.String()
}

func (g *gateWriter) writeCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.writes
}

// TestQueuedOutput_BlockedConsoleWriteReturnsImmediately is the core CA-636
// contract: while the underlying console write is blocked (selection active),
// Write must return promptly so the renderer mutex is released and the event
// loop keeps dispatching keys.
func TestQueuedOutput_BlockedConsoleWriteReturnsImmediately(t *testing.T) {
	gw := newGateWriter(true)
	o := newQueuedOutputTarget(nil, gw)

	start := time.Now()
	for i := 0; i < outputQueueCap*2; i++ {
		if _, err := o.Write([]byte("frame")); err != nil {
			t.Fatalf("Write must not error on blocked console: %v", err)
		}
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("Write blocked on wedged console for %v — event loop would freeze", d)
	}

	// Unblock the console: queued frames drain, later writes flow through.
	gw.unblock()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if gw.writeCount() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if gw.writeCount() == 0 {
		t.Fatal("drain goroutine never wrote after console unblocked")
	}
}

// TestQueuedOutput_HealthyConsoleWritesInOrder ensures the queue is a no-op
// pass-through when the console is healthy: frames arrive in order and the
// payload is preserved.
func TestQueuedOutput_HealthyConsoleWritesInOrder(t *testing.T) {
	gw := newGateWriter(false)
	o := newQueuedOutputTarget(nil, gw)

	frames := []string{"a", "b", "c"}
	for _, f := range frames {
		if _, err := o.Write([]byte(f)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if gw.writeCount() >= len(frames) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := gw.content(); got != "abc" {
		t.Fatalf("frames must drain in order, got %q", got)
	}
}

// TestQueuedOutput_ImplementsTermFile verifies the writer still satisfies the
// term.File contract (Fd/Read/Close) so bubbletea keeps ttyOutput detection,
// checkResize and WindowSizeMsg delivery working through the wrapper.
func TestQueuedOutput_ImplementsTermFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "tui-out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	o := newQueuedOutput(f)

	if o.Fd() != f.Fd() {
		t.Fatalf("Fd must delegate to the real file, got %d want %d", o.Fd(), f.Fd())
	}
	// term.File = io.ReadWriteCloser + Fd(); Read must delegate.
	if _, err := o.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read on empty temp file must return EOF")
	}
	if err := o.Close(); err != nil {
		t.Fatalf("Close must delegate: %v", err)
	}
}

// TestQueuedOutput_DropLogIsThrottled ensures the "dropping frame" evidence
// line does not spam tui.log while a wedge lasts.
func TestQueuedOutput_DropLogIsThrottled(t *testing.T) {
	o := newQueuedOutputTarget(nil, io.Discard)
	o.noteDrop()
	first := o.lastDropAt
	o.noteDrop()
	if !o.lastDropAt.Equal(first) {
		t.Fatal("second drop within throttle window must not re-log")
	}
	// Push the timestamp back beyond the throttle window: the next drop must
	// log again (timestamp advances).
	o.lastDropAt = time.Now().Add(-outputDropLogThrottle - time.Second)
	o.noteDrop()
	if o.lastDropAt.Before(time.Now().Add(-time.Second)) {
		t.Fatal("drop after throttle window must log again")
	}
}
