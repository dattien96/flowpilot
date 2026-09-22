//go:build !windows

package runnerboot

import "syscall"

// pidAlive reports whether a process with the given pid exists. Signal 0
// performs error checking without delivering a signal.
func pidAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
