package runner

import "testing"

// Read-only chat-posture policy (Task-xxx/CA-xxx): scan/plan auto-approve reads,
// auto-deny writes, never ask. Conservative — unknown/ambiguous ops fail closed.

func TestReadOnlyApprovalDecision_ReadsApprove(t *testing.T) {
	cases := []struct {
		name    string
		details ApprovalDetails
	}{
		{"claude Read file", ApprovalDetails{Kind: "file", Reason: "Read"}},
		{"claude Glob", ApprovalDetails{Kind: "file", Reason: "Glob"}},
		{"claude Grep", ApprovalDetails{Kind: "file", Reason: "Grep"}},
		{"claude LS", ApprovalDetails{Kind: "file", Reason: "LS"}},
		{"claude WebFetch", ApprovalDetails{Kind: "file", Reason: "WebFetch"}},
		{"grok read kind", ApprovalDetails{Kind: "file", Reason: "read"}},
		{"ls exec", ApprovalDetails{Kind: "exec", Command: "ls -la"}},
		{"rg exec", ApprovalDetails{Kind: "exec", Command: "rg pattern src"}},
		{"git status", ApprovalDetails{Kind: "exec", Command: "git status"}},
		{"git diff", ApprovalDetails{Kind: "exec", Command: "git diff HEAD"}},
		{"git log", ApprovalDetails{Kind: "exec", Command: "git log --oneline -5"}},
		{"cat exec", ApprovalDetails{Kind: "exec", Command: "cat go.mod"}},
		{"sudo read", ApprovalDetails{Kind: "exec", Command: "sudo cat /etc/hosts"}},
		{"find exec", ApprovalDetails{Kind: "exec", Command: "find . -name '*.go'"}},
		{"echo read", ApprovalDetails{Kind: "exec", Command: "echo hi"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readOnlyApprovalDecision(tc.details); got != "approve" {
				t.Fatalf("readOnlyApprovalDecision(%+v) = %q, want approve", tc.details, got)
			}
		})
	}
}

func TestReadOnlyApprovalDecision_WritesDeny(t *testing.T) {
	cases := []struct {
		name    string
		details ApprovalDetails
	}{
		{"claude Write file", ApprovalDetails{Kind: "file", Reason: "Write"}},
		{"claude Edit", ApprovalDetails{Kind: "file", Reason: "Edit"}},
		{"codex applyPatch write", ApprovalDetails{Kind: "file", Reason: "applyPatch"}},
		{"codex file change", ApprovalDetails{Kind: "file", Reason: "fileChange"}},
		{"grok write kind", ApprovalDetails{Kind: "file", Reason: "write"}},
		{"grok create kind", ApprovalDetails{Kind: "file", Reason: "create"}},
		{"grok delete kind", ApprovalDetails{Kind: "file", Reason: "delete"}},
		{"rm exec", ApprovalDetails{Kind: "exec", Command: "rm foo.txt"}},
		{"redirect write", ApprovalDetails{Kind: "exec", Command: "echo hi > out.txt"}},
		{"chained write", ApprovalDetails{Kind: "exec", Command: "ls; rm foo"}},
		{"pipe to write", ApprovalDetails{Kind: "exec", Command: "ls | grep foo"}},
		{"git commit", ApprovalDetails{Kind: "exec", Command: "git commit -m x"}},
		{"git push", ApprovalDetails{Kind: "exec", Command: "git push"}},
		{"git branch create", ApprovalDetails{Kind: "exec", Command: "git branch feature/x"}},
		{"git remote add", ApprovalDetails{Kind: "exec", Command: "git remote add origin git@x"}},
		{"git tag create", ApprovalDetails{Kind: "exec", Command: "git tag v1.0"}},
		{"git config write", ApprovalDetails{Kind: "exec", Command: "git config user.name Foo"}},
		{"git fetch", ApprovalDetails{Kind: "exec", Command: "git fetch origin"}},
		{"git stash", ApprovalDetails{Kind: "exec", Command: "git stash"}},
		{"mcp tool", ApprovalDetails{Kind: "mcp", Command: "mcp_thing"}},
		{"other kind", ApprovalDetails{Kind: "other", Command: "anything"}},
		{"empty kind", ApprovalDetails{Command: "anything"}},
		{"empty reason file", ApprovalDetails{Kind: "file"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readOnlyApprovalDecision(tc.details); got != "deny" {
				t.Fatalf("readOnlyApprovalDecision(%+v) = %q, want deny", tc.details, got)
			}
		})
	}
}

func TestReadOnlyApprovalDecision_GitDualFormSubcommandsOnlyWhenRead(t *testing.T) {
	// Reads of dual-form git subcommands are approved when args are read-only.
	approves := []string{
		"git branch",
		"git branch --list",
		"git remote",
		"git remote -v",
		"git config --list",
		"git config --get remote.origin.url",
		"git tag",
		"git tag -l",
	}
	for _, cmd := range approves {
		if got := readOnlyApprovalDecision(ApprovalDetails{Kind: "exec", Command: cmd}); got != "approve" {
			t.Fatalf("isReadOnlyCommand(%q) = %q, want approve", cmd, got)
		}
	}
	// Writes of the same subcommands are denied.
	denies := []string{
		"git branch feature/x",
		"git remote add origin git@example.com:r/x.git",
		"git config user.name Foo",
		"git tag v1.0",
		"git stash",
		"git stash push",
		"git fetch origin",
	}
	for _, cmd := range denies {
		if got := readOnlyApprovalDecision(ApprovalDetails{Kind: "exec", Command: cmd}); got != "deny" {
			t.Fatalf("isReadOnlyCommand(%q) = %q, want deny", cmd, got)
		}
	}
}

func TestIsReadOnlyToolName_EmptyOrUnknownDenies(t *testing.T) {
	if isReadOnlyToolName("") {
		t.Fatal("empty tool name must not be read-only")
	}
	if isReadOnlyToolName("Write") {
		t.Fatal("Write must not classify read-only (substring safety)")
	}
	if isReadOnlyToolName("edit") {
		t.Fatal("edit must not classify read-only")
	}
	if !isReadOnlyToolName("Read") {
		t.Fatal("Read must classify read-only")
	}
	if !isReadOnlyToolName("WebSearch") {
		t.Fatal("WebSearch must classify read-only")
	}
}

func TestIsReadOnlyToolName_FalsePositivesDenied(t *testing.T) {
	// A write tool whose name merely CONTAINS a read verb must never classify
	// read-only (whole-word matching, not substring).
	cases := []string{
		"ReadWrite",
		"FileReadWrite",
		"open_pr",   // contains "open" + write intent
		"open_mr",   // GitLab MR creation
		"Restart",   // contains "stat"
		"Breadcrumb", // contains "read"
		"research",  // contains "search" but is a write-style MCP tool
		"createdocument",
		"updaterecord",
		"writestream",
		"modifyreadme", // contains "readme" → substring "read"
	}
	for _, tc := range cases {
		if isReadOnlyToolName(tc) {
			t.Fatalf("isReadOnlyToolName(%q) = true, want deny (false positive)", tc)
		}
	}
}

func TestIsReadOnlyToolName_ReadToolsApproved(t *testing.T) {
	cases := []string{
		"Read", "read", "Glob", "grep", "LS", "WebFetch", "WebSearch",
		"get_file_contents", "read_file", "search_files", "list_files",
		"get_workspace_context",
	}
	for _, tc := range cases {
		if !isReadOnlyToolName(tc) {
			t.Fatalf("isReadOnlyToolName(%q) = false, want approve", tc)
		}
	}
}