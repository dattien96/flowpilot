package runnerboot

// CP-81 Task-416 T-6: machine-scoped boot serialization. Concurrent
// EnsureRunner callers (TUI, Desktop, stacked `flowpilot chat` invocations)
// take this file lock so the classify → shutdown-stale → spawn sequence has a
// single winner; losers re-check health after acquiring and reuse the winner's
// runner. Stale locks (dead holder pid or age) are reclaimed, bounded.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// bootLockStaleAfter bounds a dead holder's lock. Generous so a slow spawn
// never loses the lock mid-boot, short enough that a crashed starter's lock
// is recovered within one boot attempt.
var bootLockStaleAfter = 90 * time.Second

// bootLockPoll is the retry cadence while another starter holds the lock.
const bootLockPoll = 150 * time.Millisecond

// bootLockWriteGrace bounds the torn-write window on the O_EXCL fallback:
// a lock file younger than this with unparseable content is treated as a
// holder mid-write, not as stale.
const bootLockWriteGrace = 5 * time.Second

// BootLock releases the machine-scoped boot serialization lock.
type BootLock interface {
	Unlock() error
}

type fileBootLock struct {
	path string
}

func (l *fileBootLock) Unlock() error {
	if l == nil || l.path == "" {
		return nil
	}
	err := os.Remove(l.path)
	l.path = ""
	return err
}

// AcquireRunnerBootLock serializes runner boot on this machine. The lock file
// lives in the OS temp dir (host-wide, not workspace-scoped) because Desktop
// and TUI may boot the same runner from different workspaces. Returns a
// released-by-default lock on ctx cancellation.
func AcquireRunnerBootLock(ctx context.Context) (BootLock, error) {
	return acquireBootLockAt(ctx, filepath.Join(os.TempDir(), "flowpilot-runner.boot.lock"))
}

// acquireBootLockAt is the testable core: atomic create, stale reclaim,
// bounded retry on ctx.
//
// The claim must be atomic: a contender that observes a half-written lock
// file (created but pid not yet flushed) would misjudge it as stale and
// reclaim it, letting two starters hold the lock simultaneously. We write
// the fully-populated content to a unique temp file and hard-link it into
// place; on filesystems without hard-link support we fall back to O_EXCL
// create and rely on the write-grace in bootLockStale.
func acquireBootLockAt(ctx context.Context, path string) (BootLock, error) {
	content := []byte(fmt.Sprintf("pid=%d\ncreated=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano)))
	for {
		tmp := fmt.Sprintf("%s.tmp.%d.%d", path, os.Getpid(), time.Now().UnixNano())
		if werr := os.WriteFile(tmp, content, 0644); werr != nil {
			return nil, fmt.Errorf("boot lock: %w", werr)
		}
		lerr := os.Link(tmp, path)
		_ = os.Remove(tmp)
		if lerr == nil {
			return &fileBootLock{path: path}, nil
		}
		if !errors.Is(lerr, os.ErrExist) {
			f, ferr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
			if ferr == nil {
				_, _ = f.Write(content)
				f.Close()
				return &fileBootLock{path: path}, nil
			}
			if !errors.Is(ferr, os.ErrExist) {
				return nil, fmt.Errorf("boot lock: %w", ferr)
			}
		}
		if bootLockStale(path) {
			_ = os.Remove(path) // best-effort reclaim; next create may still race
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(bootLockPoll):
		}
	}
}

// bootLockStale reports whether the lock file's holder is gone or the lock is
// older than bootLockStaleAfter. Missing/unparseable content is treated as
// stale (fail-recoverable: a malformed lock must not serialize forever).
func bootLockStale(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false // cannot stat → let the create retry decide
	}
	if time.Since(info.ModTime()) > bootLockStaleAfter {
		return true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	pid := parseLockPID(string(raw))
	if pid <= 0 {
		// A holder may still be flushing its pid (O_EXCL fallback path):
		// only a malformed lock that has had ample write time is stale.
		return time.Since(info.ModTime()) > bootLockWriteGrace
	}
	return !pidAlive(pid)
}

func parseLockPID(content string) int {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "pid=") {
			pid, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "pid=")))
			return pid
		}
	}
	return 0
}
