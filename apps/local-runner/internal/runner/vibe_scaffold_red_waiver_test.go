package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTddSignatures(t *testing.T, cwd, body string) {
	t.Helper()
	path := filepath.Join(cwd, filepath.FromSlash(vibeTddSignaturesRel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Live run-17384 shape: owner-debate remediation rewrote the RED gate
// expectation to declare the pre-existing implementation as the accepted
// artifact — empty red_tests + failure_type none.
const waivedTddSignatures = `# TDD Signature Lock — debate-verify

## RED gate expectation

- red_tests: ` + "`[]`" + ` — RED requirement waived by owner-debate remediation (see below); ` + "`TestToolValue`" + ` is green at baseline
- failure_type: ` + "`none`" + ` (pre-remediation expectation was ` + "`not_implemented`" + ` via ` + "`panic(\"not implemented\")`" + `)
- verify command: ` + "`go test ./src/...`" + `

Status: ` + "`scaffold_ready`" + ` — remediation resolved; locked signatures and
the existing executable test satisfy the node's contract.
`

func TestVibeScaffoldRedWaived_DeclaredZeroRed(t *testing.T) {
	cwd := t.TempDir()
	writeTddSignatures(t, cwd, waivedTddSignatures)
	if !vibeScaffoldRedWaived(cwd) {
		t.Fatal("declared red_tests:[] + failure_type:none must waive the RED requirement")
	}
}

func TestVibeScaffoldRedWaived_NormalScaffoldNotWaived(t *testing.T) {
	cwd := t.TempDir()
	writeTddSignatures(t, cwd, `# TDD Signature Lock — demo

## RED gate expectation

- red_tests: `+"`[\"TestToolValue\"]`"+` — suite must be red before the coder fills bodies
- failure_type: `+"`not_implemented`"+`
- verify command: `+"`go test ./src/...`"+`
`)
	if vibeScaffoldRedWaived(cwd) {
		t.Fatal("non-empty red_tests must not waive the RED requirement")
	}
}

func TestVibeScaffoldRedWaived_MissingOrPartialNotWaived(t *testing.T) {
	cwd := t.TempDir()
	if vibeScaffoldRedWaived(cwd) {
		t.Fatal("missing tdd-signatures.md must not waive")
	}
	// Only one of the two markers — a failure_type:none alone (or an empty
	// red_tests alone) is not the adjudicated zero-red shape.
	writeTddSignatures(t, cwd, `- red_tests: `+"`[]`"+`
- failure_type: `+"`not_implemented`"+`
`)
	if vibeScaffoldRedWaived(cwd) {
		t.Fatal("empty red_tests with a real failure_type must not waive")
	}
	writeTddSignatures(t, cwd, `- red_tests: `+"`[\"TestX\"]`"+`
- failure_type: `+"`none`"+`
`)
	if vibeScaffoldRedWaived(cwd) {
		t.Fatal("failure_type:none with non-empty red_tests must not waive")
	}
}
