package runner

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/contextsync"
)

func TestIsCompoundCommand(t *testing.T) {
	compound := []string{
		"git status && rm -rf /",
		"cat a | grep b",
		"echo hi > out.txt",
		"ls; pwd",
		"echo `whoami`",
		"echo $(date)",
		"a & b",
		"(cd x)",
	}
	for _, c := range compound {
		if !isCompoundCommand(c) {
			t.Errorf("isCompoundCommand(%q) = false, want true", c)
		}
	}
	simple := []string{"git status", "git status -sb", "ls -la /foo", "npm test", "go build ./..."}
	for _, c := range simple {
		if isCompoundCommand(c) {
			t.Errorf("isCompoundCommand(%q) = true, want false", c)
		}
	}
}

func TestDeriveApprovalRule(t *testing.T) {
	cases := []struct {
		cmd  string
		want string
		ok   bool
	}{
		{"git status -sb", "git status", true},
		{"npm run build", "npm run", true},
		{"ls -la /foo", "ls", true},
		{"cat file.txt", "cat", true},
		{"go test ./...", "go test", true},
		{"pwd", "pwd", true},
		{"rtk git status -sb", "rtk git status", true}, // wrapper skipped, real exec+subcmd pinned
		{"rtk ls -la", "rtk ls", true},
		{"sudo git status", "sudo git status", true},
		{"rtk npm run build", "rtk npm run", true},
		{"rtk", "rtk", true},                  // bare wrapper -> verbatim
		{"git status && rm -rf /", "", false}, // compound never derivable
		{"", "", false},
		{"   ", "", false},
	}
	for _, c := range cases {
		got, ok := deriveApprovalRule(c.cmd)
		if ok != c.ok || got != c.want {
			t.Errorf("deriveApprovalRule(%q) = (%q, %v), want (%q, %v)", c.cmd, got, ok, c.want, c.ok)
		}
	}
}

func TestMatchesApprovalRule(t *testing.T) {
	rules := []string{"git status", "npm test", "ls", "rtk git status"}
	match := []string{
		"git status", "git status -sb", "git status --short", "npm test -- --watch", "ls", "ls -la",
		"rtk git status", "rtk git status -sb", // wrapper rule matches wrapped command
	}
	for _, c := range match {
		if !matchesApprovalRule(c, rules) {
			t.Errorf("matchesApprovalRule(%q) = false, want true", c)
		}
	}
	noMatch := []string{
		"git push",             // different subcommand
		"git statusfoo",        // token boundary respected
		"lsof -i",              // not the same token
		"git status && rm -rf", // compound never matches even with prefix
		"npm run build",        // different subcommand
		"rtk git push",         // wrapper rule pins subcommand: git push still asks
	}
	for _, c := range noMatch {
		if matchesApprovalRule(c, rules) {
			t.Errorf("matchesApprovalRule(%q) = true, want false", c)
		}
	}
	if matchesApprovalRule("git status", nil) {
		t.Error("empty rules should never match")
	}
}

func TestApprovalAllowlistReadWriteRoundTrip(t *testing.T) {
	dotFP := t.TempDir()
	if got := readApprovalAllowRules(dotFP); len(got) != 0 {
		t.Fatalf("fresh dir should have no rules, got %v", got)
	}
	if err := addApprovalAllowRule(dotFP, "git status"); err != nil {
		t.Fatal(err)
	}
	if err := addApprovalAllowRule(dotFP, "npm test"); err != nil {
		t.Fatal(err)
	}
	// dedupe on re-add
	if err := addApprovalAllowRule(dotFP, "git status"); err != nil {
		t.Fatal(err)
	}
	got := readApprovalAllowRules(dotFP)
	if len(got) != 2 {
		t.Fatalf("want 2 rules after dedupe, got %v", got)
	}
	if err := removeApprovalAllowRule(dotFP, "git status"); err != nil {
		t.Fatal(err)
	}
	got = readApprovalAllowRules(dotFP)
	if len(got) != 1 || got[0] != "npm test" {
		t.Fatalf("want [npm test] after remove, got %v", got)
	}
}

func TestCodexApprovalKind(t *testing.T) {
	cases := map[string]string{
		"execCommandApproval":                   "exec",
		"item/commandExecution/requestApproval": "exec",
		"approval/request":                      "exec",
		"applyPatchApproval":                    "file",
		"item/fileChange/requestApproval":       "file",
		"mcpServer/elicitation/request":         "mcp",
		"item/permissions/requestApproval":      "other",
		"something/else":                        "other",
	}
	for method, want := range cases {
		if got := codexApprovalKind(method); got != want {
			t.Errorf("codexApprovalKind(%q) = %q, want %q", method, got, want)
		}
	}
}

func TestClaudeApprovalDetailsKind(t *testing.T) {
	bash := claudeApprovalDetails(map[string]any{
		"tool_name": "Bash",
		"input":     map[string]any{"command": "git status"},
	})
	if bash.Kind != "exec" {
		t.Errorf("Bash approval Kind = %q, want exec", bash.Kind)
	}
	write := claudeApprovalDetails(map[string]any{
		"tool_name": "Write",
		"input":     map[string]any{"file_path": "/tmp/x.txt"},
	})
	if write.Kind != "file" {
		t.Errorf("Write approval Kind = %q, want file", write.Kind)
	}
}

func TestSharedFilesIncludesApprovalAllowlist(t *testing.T) {
	store, err := contextsync.NewEngineStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range store.SharedFiles() {
		if filepath.Base(p) == approvalAllowlistFileName {
			found = true
		}
	}
	if !found {
		t.Errorf("SharedFiles() missing %s: %v", approvalAllowlistFileName, store.SharedFiles())
	}
}

// TestSubmitApprovalDecisionRemembersExecCommand covers the persist path: an
// approved shell command with remember=true writes a granularity-B rule; a
// compound command, a deny, and a non-exec approval do NOT persist.
func TestSubmitApprovalDecisionRemembersExecCommand(t *testing.T) {
	cases := []struct {
		name     string
		kind     string
		command  string
		decision string
		remember bool
		wantRule string // "" means nothing persisted
	}{
		{"approve exec remember", "exec", "git status -sb", "approve", true, "git status"},
		{"approve exec no remember", "exec", "git status", "approve", false, ""},
		{"deny exec remember", "exec", "rm -rf x", "deny", true, ""},
		{"approve compound remember", "exec", "git status && rm -rf /", "approve", true, ""},
		{"approve file remember", "file", "/tmp/x.txt", "approve", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := NewInteractiveService()
			cwd := t.TempDir()
			rs := &interactiveRun{id: "run-1", workspaceCwd: cwd}
			rec := &approvalRecord{
				id:      "appr-1",
				runID:   rs.id,
				status:  "pending",
				resolve: make(chan string, 1),
				details: ApprovalDetails{
					Command: c.command,
					Kind:    c.kind,
					Decisions: []ApprovalDecisionOption{
						{Value: "approve", Label: "Approve"},
						{Value: "deny", Label: "Deny"},
					},
				},
			}
			svc.runs[rs.id] = rs
			svc.approvals[rec.id] = rec

			if apiErr := svc.submitApprovalDecision(rec.id, c.decision, c.remember); apiErr != nil {
				t.Fatalf("submitApprovalDecision: %v", apiErr)
			}
			if got := <-rec.resolve; got != c.decision {
				t.Fatalf("resolve = %q, want %q", got, c.decision)
			}
			rules := readApprovalAllowRules(filepath.Join(cwd, ".flowpilot"))
			if c.wantRule == "" {
				if len(rules) != 0 {
					t.Fatalf("expected no persisted rule, got %v", rules)
				}
				return
			}
			if len(rules) != 1 || rules[0] != c.wantRule {
				t.Fatalf("persisted rules = %v, want [%q]", rules, c.wantRule)
			}
		})
	}
}

// TestRequestApprovalAutoApprovesRememberedCommand covers the read path: a
// remembered exec command auto-approves without emitting a card; an unremembered
// one still asks (blocks), and a compound command is never auto-approved.
func TestRequestApprovalAutoApprovesRememberedCommand(t *testing.T) {
	svc := NewInteractiveService()
	cwd := t.TempDir()
	if err := addApprovalAllowRule(filepath.Join(cwd, ".flowpilot"), "git status"); err != nil {
		t.Fatal(err)
	}
	rs := &interactiveRun{id: "run-1", workspaceCwd: cwd}
	svc.runs[rs.id] = rs
	b := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), yolo: false}

	decision, err := b.RequestApproval(ApprovalDetails{Kind: "exec", Command: "git status -sb"})
	if err != nil || decision != "approve" {
		t.Fatalf("remembered command: got (%q, %v), want (approve, nil)", decision, err)
	}

	// A compound command sharing the prefix must NOT auto-approve — it should
	// block on a pending card, which we detect via a short timeout.
	svc.approvalTTL = 50 * time.Millisecond
	done := make(chan string, 1)
	go func() {
		d, _ := b.RequestApproval(ApprovalDetails{Kind: "exec", Command: "git status && rm -rf /"})
		done <- d
	}()
	select {
	case d := <-done:
		if d == "approve" {
			t.Fatalf("compound command auto-approved, want a pending/expired card")
		}
	case <-context.Background().Done():
	}
}
