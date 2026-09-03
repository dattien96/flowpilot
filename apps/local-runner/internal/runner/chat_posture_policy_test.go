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
		{"pipe read", ApprovalDetails{Kind: "exec", Command: "ls | grep foo"}}, // BUG-344: both segments read-only
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
		{"pipe to tee", ApprovalDetails{Kind: "exec", Command: "ls | tee out"}}, // BUG-344: tee is not an allowlisted read binary
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

func TestReadOnlyApprovalDecision_AskUserApproved(t *testing.T) {
	// BUG-xxx: the FlowPilot ask_user question tool must pass the scan/plan
	// posture gate — approving the gate only renders the AskQuestion card, it
	// never auto-answers. Covers the live Grok shape (run-477800 / run-464841)
	// plus the shared MCP/tool-name shapes for provider parity.
	cases := []struct {
		name    string
		details ApprovalDetails
	}{
		{"grok use_tool ask_user", ApprovalDetails{Kind: "other", Command: "flowpilot__ask_user", Reason: "use_tool"}},
		{"grok use_tool ask_user bare", ApprovalDetails{Kind: "other", Command: "ask_user", Reason: "use_tool"}},
		{"mcp ask_user", ApprovalDetails{Kind: "mcp", Command: "mcp__flowpilot__ask_user"}},
		{"tool-name ask_user", ApprovalDetails{Kind: "file", Reason: "ask_user"}},
		{"command ask_user reason use_tool", ApprovalDetails{Kind: "other", Command: "mcp__flowpilot__ask_user", Reason: "use_tool"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readOnlyApprovalDecision(tc.details); got != "approve" {
				t.Fatalf("readOnlyApprovalDecision(%+v) = %q, want approve", tc.details, got)
			}
		})
	}
}

func TestReadOnlyApprovalDecision_AskUserNeverAutoAnswersOtherTools(t *testing.T) {
	// Approving ask_user must not leak into other tools: spawn_agent (can
	// write), the native Grok ask_user_question (steered away), and generic
	// MCP/other still fail closed.
	cases := []struct {
		name    string
		details ApprovalDetails
	}{
		{"grok use_tool spawn_agent", ApprovalDetails{Kind: "other", Command: "flowpilot__spawn_agent", Reason: "use_tool"}},
		{"native ask_user_question", ApprovalDetails{Kind: "other", Command: "ask_user_question", Reason: "use_tool"}},
		{"generic mcp", ApprovalDetails{Kind: "mcp", Command: "mcp_thing"}},
		{"generic other", ApprovalDetails{Kind: "other", Command: "anything"}},
		{"ask_user in a write exec", ApprovalDetails{Kind: "exec", Command: "echo x > ask_user.txt"}},
		{"reason use_tool unknown command", ApprovalDetails{Kind: "other", Command: "some_random_mcp", Reason: "use_tool"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readOnlyApprovalDecision(tc.details); got != "deny" {
				t.Fatalf("readOnlyApprovalDecision(%+v) = %q, want deny", tc.details, got)
			}
		})
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