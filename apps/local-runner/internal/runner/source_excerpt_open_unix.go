//go:build unix

package runner

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// openRegularFileNoFollow opens path without following a final-component symlink
// and requires Mode().IsRegular() so FIFO/device/dir cannot hang excerpt reads
// (V10R4 P1 TOCTOU / special-file).
//
// Order matters: Lstat + IsRegular runs BEFORE open so a FIFO never blocks the
// process (open(FIFO) waits for a writer even with O_NOFOLLOW). O_NONBLOCK is a
// second defense against a TOCTOU type-swap to FIFO between Lstat and Open.
// O_NOFOLLOW rejects a final-component symlink swap.
//
// Deprecated for workspace-scoped reads: only the FINAL path component is
// protected here. Callers that need the full-path TOCTOU guarantee (an
// intermediate directory swapped for a symlink between validation and open —
// BUG-288 P1-11) must use openWorkspaceRegularFile instead, which walks every
// component with O_NOFOLLOW via openat(2).
func openRegularFileNoFollow(path string) (*os.File, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlink refused: %s", path)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	// Clear O_NONBLOCK for subsequent reads (regular files ignore it, but be tidy).
	if fl, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0); err == nil {
		_, _ = unix.FcntlInt(uintptr(fd), unix.F_SETFL, fl&^unix.O_NONBLOCK)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	return f, nil
}

// errNotRegularFile is returned by openWorkspaceRegularFile when the resolved
// leaf exists but is not a regular file (FIFO, device, directory, socket).
// Callers map this to a distinct "not_regular" omission reason rather than
// conflating it with a generic path-resolution/symlink error (BUG-288 P1-11 /
// TestReadSourceExcerptsRejectsNonRegular).
var errNotRegularFile = errors.New("not a regular file")

// openWorkspaceRegularFile opens targetPath — which MUST already have been
// verified to lexically fall under workspaceRoot — one path component at a
// time via openat(2), starting from a directory FD opened on workspaceRoot
// (BUG-288 P1-11). Every intermediate component is opened with
// O_DIRECTORY|O_NOFOLLOW, so a symlink swapped into any intermediate
// directory between validation and this call is rejected instead of
// silently escaping the workspace (the classic TOCTOU gap: EvalSymlinks-then
// reopen-by-string-path only protects the FINAL component). The leaf is
// opened with O_NOFOLLOW|O_NONBLOCK and fstat-verified regular before any
// read is attempted, so a FIFO cannot hang the process and a device/socket
// cannot be read as source text.
func openWorkspaceRegularFile(workspaceRoot, targetPath string) (*os.File, error) {
	rootFD, err := unix.Open(workspaceRoot, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open workspace root: %w", err)
	}
	defer unix.Close(rootFD)

	rel := strings.TrimPrefix(targetPath, workspaceRoot)
	rel = strings.Trim(rel, string(os.PathSeparator))
	if rel == "" {
		return nil, fmt.Errorf("empty relative path under workspace root")
	}
	components := strings.Split(rel, string(os.PathSeparator))

	curFD := rootFD
	closeCur := func() {
		if curFD != rootFD {
			unix.Close(curFD)
		}
	}
	defer closeCur()

	for i, comp := range components {
		if comp == "" || comp == "." || comp == ".." {
			return nil, fmt.Errorf("invalid path component %q", comp)
		}
		isLeaf := i == len(components)-1
		if !isLeaf {
			nextFD, err := unix.Openat(curFD, comp, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return nil, fmt.Errorf("openat intermediate %q: %w", comp, err)
			}
			closeCur()
			curFD = nextFD
			continue
		}
		leafFD, err := unix.Openat(curFD, comp, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			if errors.Is(err, unix.ELOOP) {
				return nil, errNotRegularFile
			}
			return nil, fmt.Errorf("openat leaf %q: %w", comp, err)
		}
		f := os.NewFile(uintptr(leafFD), targetPath)
		if fl, ferr := unix.FcntlInt(uintptr(leafFD), unix.F_GETFL, 0); ferr == nil {
			_, _ = unix.FcntlInt(uintptr(leafFD), unix.F_SETFL, fl&^unix.O_NONBLOCK)
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		if !st.Mode().IsRegular() {
			f.Close()
			return nil, errNotRegularFile
		}
		return f, nil
	}
	return nil, fmt.Errorf("unreachable: empty component walk")
}
