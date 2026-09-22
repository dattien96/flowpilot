package lifecycle

// CP-81 Task-416 T-2/T-3: executable build identity. The runner reports this
// as snapshot.buildId and runnerboot computes the expected value the same way
// — both sides run the same binary, so identical inputs produce identical
// identities and a stale `go run` temp binary is always distinguishable.

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"os"
	"runtime/debug"
)

// ExecutableBuildID returns a stable identity for the current executable:
// vcs.revision (+dirty) when the binary carries VCS build info, else a
// truncated SHA-256 of the executable file. A rebuilt binary from the same
// clean commit keeps its identity; any source change produces a new one.
func ExecutableBuildID() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		var rev, modified string
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				modified = s.Value
			}
		}
		if rev != "" {
			if modified == "true" {
				return rev + "+dirty"
			}
			return rev
		}
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "sha256:" + exeSHA256()
}

func exeSHA256() string {
	exe, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	if resolved, rerr := os.Readlink(exe); rerr == nil && resolved != "" {
		exe = resolved
	}
	f, err := os.Open(exe)
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	sum := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			sum.Write(buf[:n])
		}
		if rerr != nil {
			break
		}
	}
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// FileBuildID reads build info for an arbitrary executable path — used by
// runnerboot when comparing a candidate binary without executing it.
func FileBuildID(path string) string {
	bi, err := buildinfo.ReadFile(path)
	if err == nil {
		var rev string
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				rev = s.Value
			}
		}
		if rev != "" {
			return rev
		}
	}
	return ""
}
