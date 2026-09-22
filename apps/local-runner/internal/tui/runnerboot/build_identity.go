package runnerboot

// CP-81 Task-416 T-3: build identity for the binary runnerboot would spawn.
// The runner reports the same value via /health buildId — both sides compute
// lifecycle.ExecutableBuildID() on the same file, so a stale `go run` temp
// binary never compares equal to a fresh one.

import (
	"runtime/debug"

	"flowpilot-runner/internal/lifecycle"
)

// defaultProtocolVersion mirrors runner.ProtocolVersion. Callers that import
// the runner package (internal/cli) pass the authoritative value via
// Config.ExpectedProtocolVersion; this constant is the runnerboot-side
// default when none is supplied (kept in sync with SD-28 §6.2).
const defaultProtocolVersion = 1

// BuildIdentity is the spawnable binary's identity triple (SD-28 §6.2).
type BuildIdentity struct {
	ProtocolVersion int
	BuildID         string
	Version         string
}

// CurrentBuildIdentity computes the identity of the current executable — the
// same file EnsureRunner would spawn, so it is the expected value to compare
// against a running runner's reported buildId.
func CurrentBuildIdentity() (BuildIdentity, error) {
	id := BuildIdentity{
		ProtocolVersion: defaultProtocolVersion,
		BuildID:         lifecycle.ExecutableBuildID(),
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		id.Version = bi.Main.Version
	}
	return id, nil
}
