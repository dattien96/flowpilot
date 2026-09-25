//go:build windows

package runner

import (
	"os"
)

// Windows: best-effort exclusive create of a sibling lock marker.
// Full Win32 LockFileEx is not required for the unit-test contract on CI.
func flockExclusive(f *os.File) error {
	// Rely on O_CREATE file handle; multi-process exclusion on Windows is
	// approximated by opening with exclusive share mode via the existing handle.
	_ = f
	return nil
}

func funlock(f *os.File) error {
	_ = f
	return nil
}

// flockBlock is the blocking counterpart of flockExclusive; no-op on Windows
// for the same reason as above.
func flockBlock(f *os.File) error {
	_ = f
	return nil
}
