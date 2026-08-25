package app

import (
	"io"
	"os"
	"sync"
	"time"
)

// Output queue for the renderer (CA-636).
//
// The Windows console blocks WriteConsole while the user has a text selection
// active (QuickEdit/select — copying text from the terminal freezes output
// until a keypress clears the selection). bubbletea's renderer flush() calls
// r.out.Write() while holding its internal mutex, so a single blocked write
// freezes the whole event loop: the paste keys never reach Update, the conhost
// 64-slot input queue overflows, and the TUI looks hung for minutes (log
// 13:06:31 → 13:54:51: 0 KeyMsg, 0 CPU, all threads waiting, recovery only
// when the user pressed a key).
//
// Every prior fix (CA-610/612/615/621/630/631/633) hardened the input side.
// This bounds the OUTPUT side: renderer writes are queued to a single drain
// goroutine with a small FIFO. Write() always returns immediately — when the
// console is wedged the queue fills and frames are dropped (the renderer
// re-renders every tick, so a dropped frame costs nothing), and the event loop
// keeps dispatching keys. When the console unblocks, the queued frames drain
// in order.
//
// queuedOutput also implements term.File (Read/Close/Fd delegate to the real
// stdout) so bubbletea's ttyOutput detection, checkResize and WindowSizeMsg
// delivery keep working exactly as before.

const (
	// outputQueueCap is the max in-flight frames while the console write is
	// blocked. 8 frames ≈ 128ms of backlog at the default renderer tick —
	// enough to coast a brief stall, small enough that a wedged console
	// drops frames instead of accumulating stale ones.
	outputQueueCap = 8
	// outputDropLogThrottle stops the "dropping frame" log from spamming
	// tui.log while a wedge lasts (same throttle idea as View slow, CA-621).
	outputDropLogThrottle = 5 * time.Second
)

// queuedOutput serializes writes through one drain goroutine so a blocked
// console write can never hold the renderer mutex and freeze the event loop.
type queuedOutput struct {
	f *os.File // real stdout: Fd/Read/Close delegation for term.File
	w io.Writer // drain target; equals f in production, injectable in tests

	ch chan []byte // FIFO of pending write payloads

	mu         sync.Mutex
	lastDropAt time.Time
}

// newQueuedOutput wraps f (usually os.Stdout) with the bounded drain queue.
func newQueuedOutput(f *os.File) *queuedOutput {
	return newQueuedOutputTarget(f, f)
}

// newQueuedOutputTarget is the testable constructor: the drain goroutine
// writes to w while Fd/Read/Close still delegate to f.
func newQueuedOutputTarget(f *os.File, w io.Writer) *queuedOutput {
	o := &queuedOutput{f: f, w: w, ch: make(chan []byte, outputQueueCap)}
	go o.drain()
	return o
}

// Write enqueues p for the drain goroutine and returns immediately. When the
// queue is full (console output blocked), the frame is dropped — the renderer
// repaints on the next tick, so the caller never waits on a stuck console.
func (o *queuedOutput) Write(p []byte) (int, error) {
	b := make([]byte, len(p))
	copy(b, p)
	select {
	case o.ch <- b:
		return len(p), nil
	default:
		o.noteDrop()
		return len(p), nil
	}
}

// drain writes queued payloads in order. A blocked console stalls this one
// goroutine only — the event loop and input reader stay live.
func (o *queuedOutput) drain() {
	for b := range o.ch {
		_, _ = o.w.Write(b)
	}
}

func (o *queuedOutput) noteDrop() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if time.Since(o.lastDropAt) >= outputDropLogThrottle {
		o.lastDropAt = time.Now()
		tuiLog("output queue full — dropping frame (console output blocked/selected?)")
	}
}

// Read delegates to the real file so queuedOutput satisfies io.ReadCloser
// (part of term.File). bubbletea never reads from the output handle.
func (o *queuedOutput) Read(p []byte) (int, error) { return o.f.Read(p) }

// Close delegates to the real file. The drain goroutine is intentionally not
// stopped on Close — writes after Close would fail harmlessly and the process
// is exiting anyway.
func (o *queuedOutput) Close() error { return o.f.Close() }

// Fd returns the real stdout descriptor so bubbletea keeps treating the
// program output as a terminal: checkResize / listenForResize / WindowSizeMsg
// all continue to work through the wrapped writer.
func (o *queuedOutput) Fd() uintptr { return o.f.Fd() }