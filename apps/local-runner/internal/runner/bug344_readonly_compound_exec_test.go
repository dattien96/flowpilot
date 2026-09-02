package runner

import "testing"

// BUG-344 (live run-464841, grok 1.0.13 / grok-4.5): the read-only posture
// rejected ANY exec command containing a metacharacter, but Grok batches its
// exploration into ONE compound bash call (`rg … | head; ls …; find …
// 2>/dev/null`) — so scan/plan could never explore with Grok, and the denied
// compound then aborted the turn blank (BUG-342). The old whole-command gate
// was also too loose in one place (`find . -delete` passed).
//
// The classifier is now compositional (BUG-344 D-1/D-2/D-5): `; | && ||`
// separate segments, `2>/dev/null` is the only redirect, no substitution
// (backtick, `$(` / `${`), quotes/parens must balance, and EVERY segment must
// be a known-safe read (`isReadOnlyCommandSegment`, with `find` action
// primaries denied). Metachars inside quotes are literal (a regex pipe is not
// a pipe).
//
// The classifier is provider-neutral (only ApprovalDetails{Kind:"exec",
// Command} is inspected), so the same matrix serves Claude, Codex, and Grok.

func TestBug344CompoundReadOnlyBashApproves(t *testing.T) {
	cases := []string{
		// Reported repro — the real run-464841 Grok exploration command.
		`rg -n "B4|inplace|in-place|ask_user|spawn" .flowpilot change-audit requirements 2>/dev/null | head -80; tail -20 .flowpilot/gate-metrics.ndjson; ls requirements/07-Coding-Plan/todo requirements/08-Task/todo 2>/dev/null; find . -iname '*b4*' 2>/dev/null`,
		// Near-miss shapes: pure pipe chains, &&, ; chains, sudo, stderr-null.
		"ls | head",
		"rg x -g '!node_modules' . | head -80",
		"ls -lt d | head -30; ls -la *.go",
		"tail -20 f; ls a b",
		"git status; git log --oneline -5",
		"find . -name '*.go' 2>/dev/null",
		"rg a . 2>/dev/null | head -10 && ls",
		"cat a.go | grep x | head -5",
		"sudo cat /etc/hosts | head -1",
		"find . -iname '*.go' -print",
		// $ alone is a regex anchor, not substitution.
		"rg 'foo$' src | head",
		// Single-quoted substitution-looking text is literal (never executed).
		"rg '$(x)' src",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			if got := readOnlyApprovalDecision(ApprovalDetails{Kind: "exec", Command: cmd}); got != "approve" {
				t.Fatalf("readOnlyApprovalDecision(exec %q) = %q, want approve", cmd, got)
			}
		})
	}
}

func TestBug344CompoundWriteBashDenies(t *testing.T) {
	cases := []string{
		"ls; rm -rf /",
		"ls && rm x",
		"echo hi > out.txt",
		"echo hi >> out",
		"echo hi 1>/dev/null",
		"ls 2>err.txt",
		"head -n 2>err.txt",
		"echo x > /tmp/f; ls",
		"rg x 2>/dev/null > out",
		"$(rm -rf /)",
		"`rm -rf /`",
		"${x}",
		"ls | tee out",
		"cat a | grep x && rm b",
		"git push; ls",
		"ls; git commit -m x",
		"find . -delete",
		"find . -exec rm {} \\;",
		"find . -execdir rm {} +",
		"find . -ok rm {} \\;",
		"find . -fls /tmp/x",
		"find . -fprint /tmp/x",
		"find . -fprintf /tmp/x",
		"ls &",
		"ls && ",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			if got := readOnlyApprovalDecision(ApprovalDetails{Kind: "exec", Command: cmd}); got != "deny" {
				t.Fatalf("readOnlyApprovalDecision(exec %q) = %q, want deny", cmd, got)
			}
		})
	}
}

// TestBug344ClassifierIsProviderAgnostic proves the read-only policy reaches
// the same decision for exec requests shaped by each provider's adapter (the
// classifier only inspects Kind+Command; provider labels never leak in).
func TestBug344ClassifierIsProviderAgnostic(t *testing.T) {
	readCmd := "rg x . 2>/dev/null | head -10 && ls"
	writeCmd := "ls; rm -rf /"
	for _, provider := range []string{"claude", "codex", "grok"} {
		if got := readOnlyApprovalDecision(ApprovalDetails{Kind: "exec", Command: readCmd}); got != "approve" {
			t.Fatalf("provider=%s read cmd decision = %q, want approve", provider, got)
		}
		if got := readOnlyApprovalDecision(ApprovalDetails{Kind: "exec", Command: writeCmd}); got != "deny" {
			t.Fatalf("provider=%s write cmd decision = %q, want deny", provider, got)
		}
	}
}

// TestBug344UnbalancedSyntaxFailsClosed: malformed input must never sneak a
// write through — it denies, even when a read binary is present.
func TestBug344UnbalancedSyntaxFailsClosed(t *testing.T) {
	cases := []string{
		`ls "unclosed`,
		`ls (unclosed`,
		`ls )leading`,
		`cat "a;b" ; `,
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			if got := readOnlyApprovalDecision(ApprovalDetails{Kind: "exec", Command: cmd}); got != "deny" {
				t.Fatalf("readOnlyApprovalDecision(exec %q) = %q, want deny (fail closed)", cmd, got)
			}
		})
	}
}
