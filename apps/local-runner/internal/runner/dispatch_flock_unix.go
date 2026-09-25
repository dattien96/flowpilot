//go:build !windows

package runner

import (
	"os"
	"syscall"
)

func flockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func funlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// flockBlock takes a blocking exclusive lock — for per-operation durable-file
// writers (sessions.ndjson.lock et al., BUG-502) where waiting out a sibling
// process's rewrite is correct and a failed-fast LOCK_NB would only inject
// spurious errors. Released by funlock or process exit.
func flockBlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}
