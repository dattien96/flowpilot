package runner

import "testing"

import "flowpilot-runner/internal/flowgate"

// Provider-agnostic: binary ignore and plain-chat gate arm do not branch on provider.
// Parameterized over claude/codex/grok for parity (Case 1).

func TestBinaryScopeIgnoredForChatGate(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			// Gate's ScopeDiff must ignore gatesandbox binary even when declared
			// scope is calc.go/calc_test.go. This is the user-reported r-scope
			// noise from `go build` that should not trigger.
			if !flowgate.IsDocOrAuditFile("gatesandbox") {
				t.Fatalf("%s: gatesandbox must be ignored", provider)
			}
			if !flowgate.IsDocOrAuditFile("bin/app") {
				t.Fatalf("%s: bin/app must be ignored", provider)
			}
			if flowgate.IsDocOrAuditFile("calc.go") {
				t.Fatalf("%s: calc.go must not be ignored", provider)
			}
		})
	}
}

func TestPlainChatShouldDeferGateWhenCodeChanged(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			events := []ProviderEvent{
				{Type: EventFileChanged, Path: "calc.go"},
			}
			hasCode := false
			for _, e := range events {
				if e.Type == EventFileChanged && e.Path != "" && !flowgate.IsDocOrAuditFile(e.Path) {
					hasCode = true
					break
				}
			}
			if !hasCode {
				t.Fatalf("%s: calc.go must be considered code", provider)
			}
		})
	}
}

func TestPlainChatShouldNotDeferGateWhenOnlyDocs(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			events := []ProviderEvent{
				{Type: EventFileChanged, Path: "change-audit/CA-914-calc-core-subtract-with-guard.md"},
				{Type: EventFileChanged, Path: "requirements/08-Task/done/Task-910-calc-core-subtract-with-guard.md"},
			}
			hasCode := false
			for _, e := range events {
				if e.Type == EventFileChanged && e.Path != "" && !flowgate.IsDocOrAuditFile(e.Path) {
					hasCode = true
					break
				}
			}
			if hasCode {
				t.Fatalf("%s: docs only must not be considered code", provider)
			}
		})
	}
}

func TestPlainChatBinaryNotConsideredCode(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			events := []ProviderEvent{
				{Type: EventFileChanged, Path: "gatesandbox"},
			}
			hasCode := false
			for _, e := range events {
				if e.Type == EventFileChanged && e.Path != "" && !flowgate.IsDocOrAuditFile(e.Path) {
					hasCode = true
					break
				}
			}
			if hasCode {
				t.Fatalf("%s: gatesandbox must not be considered code", provider)
			}
		})
	}
}
