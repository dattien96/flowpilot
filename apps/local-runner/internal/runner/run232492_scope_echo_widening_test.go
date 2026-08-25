package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// run-232492 (CP-43 B14/C4 live): a user prompt declared `files: user.go`,
// but the model's final message echoed a [Change Contract] whose declared
// paths covered EVERY file it touched (calc.go, calc_test.go, user_test.go,
// ...). Because prepareChangeContract computed scope drift against the model's
// self-widened echo, actual_touched \ declared_scope was empty and r-scope
// never fired — the out-of-scope calc.go edit sailed through. This file locks
// the fix: a user-declared prompt scope is authoritative for r-scope, and a
// model echo must never widen it (single turn or across follow-ups).
//
// Additive file only — the legacy cp43_prompt_fallback_test.go suite is
// untouched. Provider-agnostic path (no ProviderKey branch in
// prepareChangeContract), parameterized over claude/codex/grok for parity.

func TestRun232492_EchoCannotWidenUserScope(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			prompt := "[Change Contract]\nfeature: calc-core\nintent: refactor parseUserID trong user.go dung helper moi\nfiles: user.go\nsymbols: parseUserID\n"
			// Model echo widens declared paths to everything it touched.
			final := "[Change Contract]\nfeature: calc-core\nintent: add IsValidInt and refactor parseUserID to use it\nfiles: calc.go, calc_test.go, user.go, user_test.go, change-audit/CA-914-*.md, requirements/08-Task/...\nsymbols: IsValidInt, parseUserID\n\nDone."
			diff := []flowgate.ChangedFile{
				{Path: "calc.go", Status: "M"},
				{Path: "calc_test.go", Status: "M"},
				{Path: "user.go", Status: "A"},
				{Path: "user_test.go", Status: "A"},
			}
			p := prepareChangeContract(context.Background(), dir, "run-232492", "chat-run-232492", prompt, final, diff, []string{"calc-core"})
			if !p.declared {
				t.Fatalf("%s: must be declared", provider)
			}
			// calc.go was NOT in the user's declared scope — the widened echo
			// must not hide it.
			if len(p.outOfScopePaths) == 0 {
				t.Fatalf("%s: out-of-scope paths must not be empty (echo widened scope)", provider)
			}
			for _, want := range []string{"calc.go", "calc_test.go"} {
				if !run232492Contains(p.outOfScopePaths, want) {
					t.Fatalf("%s: outOfScope=%v must include %q", provider, p.outOfScopePaths, want)
				}
			}
			// Stored contract must persist the USER scope, not the echo.
			store, err := changecontract.NewStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			stored, ok := store.GetLatestForRun("run-232492")
			if !ok {
				t.Fatalf("%s: stored contract missing", provider)
			}
			if len(stored.DeclaredPaths) != 1 || stored.DeclaredPaths[0] != "user.go" {
				t.Fatalf("%s: stored declared_paths=%v want [user.go]", provider, stored.DeclaredPaths)
			}
		})
	}
}

func TestRun232492_EchoCannotWidenScopeAcrossTurns(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			runID := "run-232492"
			stepID := "chat-run-232492"
			prompt := "[Change Contract]\nfeature: calc-core\nintent: refactor parseUserID\nfiles: user.go\nsymbols: parseUserID\n"
			echo := "[Change Contract]\nfeature: calc-core\nintent: add IsValidInt and refactor parseUserID\nfiles: calc.go, calc_test.go, user.go, user_test.go\nsymbols: IsValidInt, parseUserID\n\nDone."
			diff := []flowgate.ChangedFile{
				{Path: "calc.go", Status: "M"},
				{Path: "calc_test.go", Status: "M"},
				{Path: "user.go", Status: "A"},
				{Path: "user_test.go", Status: "A"},
			}
			// Turn 1: user declares user.go; model echo widens.
			p1 := prepareChangeContract(context.Background(), dir, runID, stepID, prompt, echo, diff, []string{"calc-core"})
			if len(p1.outOfScopePaths) == 0 {
				t.Fatalf("%s: turn 1 must flag widened echo", provider)
			}
			// Turn 2: user silent, model re-echoes the widened scope. The
			// persisted user scope must stay authoritative.
			p2 := prepareChangeContract(context.Background(), dir, runID, stepID, "", echo, diff, []string{"calc-core"})
			if !p2.declared {
				t.Fatalf("%s: turn 2 must still be declared", provider)
			}
			if len(p2.outOfScopePaths) == 0 {
				t.Fatalf("%s: turn 2 must keep flagging out-of-scope calc.go (stored scope reused)", provider)
			}
			if !run232492Contains(p2.outOfScopePaths, "calc.go") {
				t.Fatalf("%s: turn 2 outOfScope=%v must include calc.go", provider, p2.outOfScopePaths)
			}
		})
	}
}

func TestRun232492_InScopeEchoNoDrift(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			prompt := "[Change Contract]\nfeature: calc-core\nintent: add helper\nfiles: calc.go, calc_test.go\n"
			final := "[Change Contract]\nfeature: calc-core\nintent: add IsValidInt\nfiles: calc.go, calc_test.go\n\nDone."
			diff := []flowgate.ChangedFile{{Path: "calc.go", Status: "M"}, {Path: "calc_test.go", Status: "M"}}
			p := prepareChangeContract(context.Background(), dir, "run-x", "chat-run-x", prompt, final, diff, []string{"calc-core"})
			if len(p.outOfScopePaths) != 0 {
				t.Fatalf("%s: in-scope edit must have no drift, got %v", provider, p.outOfScopePaths)
			}
		})
	}
}

func TestRun232492_FinalOnlyScopeUsedWhenNoUserPrompt(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			// No prompt declaration — the model's echo is the only source, so
			// its scope must still apply (no over-flagging without user scope).
			final := "[Change Contract]\nfeature: calc-core\nintent: add IsValidInt\nfiles: calc.go\n\nDone."
			diff := []flowgate.ChangedFile{{Path: "calc.go", Status: "M"}}
			p := prepareChangeContract(context.Background(), dir, "run-y", "chat-run-y", "", final, diff, []string{"calc-core"})
			if !p.declared {
				t.Fatalf("%s: must be declared from echo", provider)
			}
			if len(p.outOfScopePaths) != 0 {
				t.Fatalf("%s: echo scope must apply when no user prompt declares, got %v", provider, p.outOfScopePaths)
			}
		})
	}
}

func run232492Contains(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}
