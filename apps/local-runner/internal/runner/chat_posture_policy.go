package runner

import "strings"

// Read-only chat-posture policy (Task-xxx / CA-xxx).
//
// Scan/Plan postures are "gated + silent": every tool reaches the runner's
// approval bridge (adapters are wired to gated permission modes so nothing is
// bypassed), and the bridge then auto-decides WITHOUT asking the human:
//   - reads  -> approve
//   - writes -> deny
//   - unknown -> deny (fail closed)
//
// The classifier is provider-neutral: it inspects ApprovalDetails (Kind,
// Command, and Reason which Claude/Grok carry the tool name in) and only
// approves operations that are unambiguously read-only. Everything else is a
// silent deny — a read-only posture must never let a write through.

// readOnlyApprovalDecision returns "approve" or "deny" for an ApprovalDetails
// under a read-only posture. Conservative: only confidently-read operations are
// approved; exec commands and unknown tools default to deny.
func readOnlyApprovalDecision(details ApprovalDetails) string {
	switch details.Kind {
	case "exec":
		if isReadOnlyCommand(details.Command) {
			return "approve"
		}
		return "deny"
	case "file":
		// Claude classifies BOTH Read and Write as Kind "file" (file_path in
		// input) and puts the tool name in Reason. Grok classifies read/edit/
		// write/create/delete kinds as "file" too. Codex file changes
		// (applyPatch / item/fileChange) are always writes. Classify by the
		// tool name/kind carried in Reason; anything unrecognized is denied.
		if isReadOnlyToolName(details.Reason) {
			return "approve"
		}
		return "deny"
	case "mcp", "other", "":
		// MCP elicitation and anything unclassifiable can mutate — never
		// auto-approved in a read-only posture.
		return "deny"
	default:
		return "deny"
	}
}

// isReadOnlyToolName reports whether a tool name/kind marker is a read-only
// operation. Matches Claude tool names (Read/Glob/Grep/LS/WebFetch/WebSearch),
// Grok kind markers ("read"), and read verbs. Matching is token/whole-word based
// so a write tool whose name merely CONTAINS a read verb (ReadWrite, open_pr,
// Restart, Breadcrumb) never false-positives to "approve" — a read-only posture
// must not let a write through.
func isReadOnlyToolName(reason string) bool {
	r := strings.ToLower(strings.TrimSpace(reason))
	if r == "" {
		return false
	}
	// Fail closed: if the tool name embeds ANY write intent (create/update/
	// edit/write/delete/remove/modify/add/commit/pr/mr/merge/push/move/copy/
	// rename), deny outright — a read-only posture must never let a write
	// through, even when the name also contains a read verb (open_pr, modify,
	// ReadWrite, createfile).
	for _, writeToken := range []string{
		"write", "create", "update", "edit", "delete", "remove", "modify",
		"add", "commit", "merge", "push", "move", "copy", "rename",
		"paste", "upload", "insert", "replace", "patch", "_pr", "_mr",
	} {
		if strings.Contains(r, writeToken) {
			return false
		}
	}
	// Whole-word read verbs. Tokenize on non-alphanumerics so
	// "Read", "read", "Glob", "WebSearch" all match while "ReadWrite" does not.
	matchesReadVerb := func(word string) bool {
		switch word {
		case "read", "glob", "grep", "list", "ls", "view", "search",
			"fetch", "stat", "cat", "open", "find", "head", "tail",
			"wc", "get", "lookup", "show", "peek", "inspect", "query":
			return true
		}
		return false
	}
	// Exact full-string tool names (many provider tool names are compound and
	// must not be broken into verbs, e.g. "get_file_contents" is read-only but
	// we only allow it whole).
	switch r {
	case "read", "glob", "grep", "ls", "list", "view", "search", "fetch",
		"stat", "cat", "head", "tail", "wc", "get_file_contents",
		"read_file", "search_files", "list_files", "webfetch", "websearch",
		"web_fetch", "web_search", "readfile", "grep_search",
		"find_in_files", "explore", "get_workspace_context":
		return true
	}
	for _, tok := range strings.FieldsFunc(r, func(c rune) bool {
		return !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9')
	}) {
		if matchesReadVerb(tok) {
			return true
		}
	}
	return false
}

// isReadOnlyCommand reports whether a shell command is read-only. Approved only
// when the command is a known-safe read (optionally with read-only flags) and
// contains no shell metacharacters that could redirect/chain a write.
func isReadOnlyCommand(command string) bool {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false
	}
	// Reject any command that contains a write/chain/redirect metacharacter —
	// even a read binary is unsafe when piped/redirected.
	if strings.ContainsAny(cmd, ">|;&`$") {
		return false
	}
	first := strings.Fields(cmd)
	if len(first) == 0 {
		return false
	}
	// `sudo <read>` is still read-only; skip the sudo token to classify the
	// actual binary. (Rejected earlier if the command also carried a
	// redirect/chain metachar.)
	tokens := first
	if strings.EqualFold(tokens[0], "sudo") {
		tokens = tokens[1:]
		if len(tokens) == 0 {
			return false
		}
	}
	bin := strings.ToLower(strings.TrimSpace(tokens[0]))
	bin = strings.TrimPrefix(bin, "./")
	bin = strings.TrimPrefix(bin, "/")
	// Strip any remaining path components so /usr/bin/ls, ls, ./ls all match.
	if i := strings.LastIndex(bin, "/"); i >= 0 {
		bin = bin[i+1:]
	}
	switch bin {
	case "ls", "cat", "find", "grep", "rg", "head", "tail", "wc",
		"pwd", "echo", "printf", "whoami", "hostname", "uname", "env",
		"printenv", "date", "stat", "file", "which", "type", "tree",
		"git":
		// git is only read-only for its non-mutating subcommands.
		return bin != "git" || isReadOnlyGit(tokens[1:])
	default:
		return false
	}
}

// isReadOnlyGit reports whether a git invocation is unambiguously read-only.
// Conservative: subcommands that have BOTH read and write forms (branch, remote,
// config, tag, stash, fetch, mergetool, ...) are only approved when the actual
// args make the operation read-only; otherwise they are denied so a read-only
// posture never leaks a write.
func isReadOnlyGit(args []string) bool {
	if len(args) == 0 {
		return true // bare `git` prints help — read-only
	}
	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	switch cmd {
	case "status", "log", "diff", "show", "rev-parse", "ls-files",
		"help", "blame", "describe", "shortlog", "reflog", "grep":
		return true
	case "branch":
		// `git branch` / `git branch --list` list; anything else (a name) creates.
		for _, a := range rest {
			if a == "--list" || a == "-a" || a == "--all" || a == "-r" || a == "--remotes" || a == "--no-color" {
				continue
			}
			if strings.HasPrefix(a, "-") {
				continue // read-only flags are fine
			}
			return false // a branch name would create it
		}
		return true
	case "remote":
		// `git remote` / `git remote -v` list; `add/set-url/remove/rename` write.
		if len(rest) == 0 {
			return true
		}
		if rest[0] == "-v" || rest[0] == "--verbose" {
			return true
		}
		return false
	case "config":
		// Read flags make the whole invocation read-only:
		//   git config --list / -l / --get <key> / --get-regexp <p> / --get-all
		//   / --show-origin / --show-scope
		for _, a := range rest {
			switch a {
			case "--list", "-l", "--get", "--get-regexp", "--get-all",
				"--show-origin", "--show-scope":
				return true
			}
		}
		// Otherwise `git config` with only scope flags (--global/--local/
		// --system) and no positional args lists that scope (read); any
		// positional arg is a key/value WRITE.
		for _, a := range rest {
			if strings.HasPrefix(a, "-") {
				continue
			}
			return false // a key/value pair would write
		}
		return true
	case "tag":
		// `git tag` / `git tag -l` list; `git tag <name>` creates.
		if len(rest) == 0 {
			return true
		}
		for _, a := range rest {
			if a == "-l" || a == "--list" || a == "-n" || strings.HasPrefix(a, "-") {
				continue
			}
			return false // a tag name would create it
		}
		return true
	case "fetch", "stash", "mergetool", "push", "commit", "add", "reset",
		"checkout", "clean", "mv", "rm", "rebase", "merge", "cherry-pick",
		"revert", "apply", "am", "switch", "restore", "init", "clone",
		"update-ref", "write-tree", "gc", "prune", "pack", "repack":
		// Network writes / worktree writes / index mutations — never allowed.
		return false
	default:
		return false
	}
}