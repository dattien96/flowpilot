//go:build !unix

package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// openRegularFileNoFollow is a best-effort non-unix fallback: Lstat rejects
// symlinks and non-regular files, then Open. Residual TOCTOU between Lstat and
// Open is accepted only on non-unix platforms where O_NOFOLLOW is unavailable.
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
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// Re-stat the open fd to reject TOCTOU type swaps after Lstat.
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

// errNotRegularFile mirrors the unix build's sentinel (BUG-288 P1-11) so
// flow_context_package.go can share one omission-reason mapping across
// platforms.
var errNotRegularFile = errors.New("not a regular file")

// openWorkspaceRegularFile is the non-unix twin of the unix openat(2)-based
// walk (BUG-288 P1-11). BUG-288 P1-19 (Vòng 12) tightens this further: it now
// walks via os.Root (Go >=1.24, stdlib os/root), which performs each
// component open using the platform's fd/handle-relative primitives
// internally — an openat-equivalent on every OS, including Windows, where
// there is no public openat API for us to call directly. That replaces the
// previous approach of re-Lstat-ing a component then re-opening the NEXT
// component by concatenated string path, which is exactly the gap an
// attacker could use to swap an intermediate directory for a symlink between
// validating component N and touching component N+1: os.Root anchors every
// step to the already-validated root/parent handle instead of a possibly
// swapped path string. This still cannot achieve full closure — Go's stdlib
// exposes no atomic "Lstat this exact handle to confirm non-symlink AND open
// it" primitive even via os.Root, so there remains a window between this
// function's Lstat validation pass and the final root.Open call — but it
// shrinks the window from "entire excerpt read via string paths" to
// "one os.Root-internal open per component", which is the most Go's standard
// library allows here without cgo/syscall-level Windows APIs. The leaf gets
// one last identity check: a fresh root.Lstat compared via os.SameFile
// against the opened handle's own Stat, to catch a same-name type swap that
// happened between validation and open.
func openWorkspaceRegularFile(workspaceRoot, targetPath string) (*os.File, error) {
	rel := strings.TrimPrefix(targetPath, workspaceRoot)
	rel = strings.Trim(rel, string(os.PathSeparator))
	if rel == "" {
		return nil, fmt.Errorf("empty relative path under workspace root")
	}
	components := strings.Split(rel, string(os.PathSeparator))
	for _, comp := range components {
		if comp == "" || comp == "." || comp == ".." {
			return nil, fmt.Errorf("invalid path component %q", comp)
		}
	}

	root, err := os.OpenRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	cur := ""
	for i, comp := range components {
		cur = filepath.Join(cur, comp)
		isLeaf := i == len(components)-1
		fi, err := root.Lstat(cur)
		if err != nil {
			return nil, err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			// Reject a symlink at ANY component, not just the leaf — this
			// package's policy (unlike os.Root's default escape-only guard)
			// is "no symlinks anywhere in the path", matching the unix build.
			return nil, fmt.Errorf("symlink refused: %s", filepath.Join(workspaceRoot, cur))
		}
		if !isLeaf && !fi.IsDir() {
			return nil, fmt.Errorf("not a directory: %s", filepath.Join(workspaceRoot, cur))
		}
		if isLeaf && !fi.Mode().IsRegular() {
			return nil, errNotRegularFile
		}
	}

	f, err := root.Open(rel)
	if err != nil {
		return nil, err
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
	// Final consistency check: a fresh Lstat of the leaf, compared against
	// the opened handle's own Stat via os.SameFile, catches a type swap that
	// happened between the validation loop above and this Open call.
	if fresh, ferr := root.Lstat(rel); ferr == nil {
		if !os.SameFile(fresh, st) {
			f.Close()
			return nil, errNotRegularFile
		}
	}
	return f, nil
}
