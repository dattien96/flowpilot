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
//
// BUG-344 composition invariant: an exec command is read-only iff every
// top-level segment (`;`, `|`, `&&`, `||`) is a known-safe read, no segment
// can redirect to a real file (only `2>/dev/null` and `2>&1` are allowed,
// stripped before classification), and nothing can substitute/execute hidden
// code (no backtick, `$(` or `${`). Fail closed on any parse doubt. `find` is
// allowlisted only for its pure-search forms; `go` only for its report
// subcommands (test/list/env/doc/version/help); `curl` only for stdout fetches
// (no -o/-O/-d/-F/-T/-X).

// readOnlyApprovalDecision returns "approve" or "deny" for an ApprovalDetails
// under a read-only posture. Conservative: only confidently-read operations are
// approved; exec commands and unknown tools default to deny.
//
// ask_user is a deliberate exception: it is a QUESTION, not a write. Approving
// its tool-call gate only lets the FlowPilot MCP AskQuestion card render — the
// bridge still blocks for the user's answer, so approving never auto-answers.
// Live run-477800 / run-464841: Grok routes the MCP `ask_user` as a gated
// `use_tool` request (Kind "other", Command "flowpilot__ask_user") and the
// fail-close deny aborted B4-inplace with a blank/question-less turn.
func readOnlyApprovalDecision(details ApprovalDetails) string {
	if isAskUserTool(details) {
		return "approve"
	}
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

// isAskUserTool reports whether the gated tool is FlowPilot's structured
// question tool `ask_user`. Grok wraps it as `use_tool` with Command carrying
// the MCP name (`flowpilot__ask_user`); Claude/Codex surface it as an MCP /
// tool-name call (`mcp__flowpilot__ask_user`, bare `ask_user`). The native
// Grok `ask_user_question` is deliberately NOT matched — the reinforcement
// steers onto the FlowPilot MCP tool, and the native one must keep being
// denied in scan.
func isAskUserTool(details ApprovalDetails) bool {
	for _, s := range []string{details.Command, details.Reason} {
		r := strings.ToLower(strings.TrimSpace(s))
		if r == "ask_user" || strings.HasSuffix(r, "__ask_user") || strings.Contains(r, "mcp__flowpilot__ask_user") {
			return true
		}
	}
	return false
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

// isReadOnlyCommand reports whether a shell command is read-only (BUG-344).
// Compositional: a compound command (`; | && ||`) is read-only iff EVERY
// segment is a known-safe read, no segment can redirect to a real file, and
// nothing can substitute/execute hidden code. Fail-closed on any doubt.
//
// The scanner is quote-aware so shell metacharacters inside quotes are literal
// (e.g. `rg -n "B4|inplace"` keeps its pipe — it is a pattern, not a pipe).
func isReadOnlyCommand(command string) bool {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false
	}
	cleaned, ok := scanShellCommandSyntax(cmd)
	if !ok {
		return false
	}
	segments := splitReadOnlySegments(cleaned)
	if len(segments) == 0 {
		return false
	}
	for _, seg := range segments {
		if !isReadOnlyCommandSegment(seg) {
			return false
		}
	}
	return true
}

// scanShellCommandSyntax enforces the BUG-344 composition guards (D-2/D-5)
// quote-aware:
//   - backtick, `$(`, `${` substitution denied (active inside double quotes,
//     matching shell semantics; literal inside single quotes);
//   - single/double quotes must balance; parentheses must balance;
//   - the ONLY allowed redirects are stderr-to-null `2>/dev/null` (also
//     `2> /dev/null`) and stderr-to-stdout `2>&1` (no file is written by
//     either), both stripped from the returned string; every other `>` shape
//     (>, >>, 2>file, 1>, &>, <>) denies.
//
// ok=false fails the whole command closed.
func scanShellCommandSyntax(s string) (string, bool) {
	var buf []byte
	var q byte // 0, '\'', '"'
	depth := 0
	for i := 0; i < len(s); {
		c := s[i]
		if q == '\'' {
			buf = append(buf, c)
			if c == '\'' {
				q = 0
			}
			i++
			continue
		}
		if q == '"' {
			if c == '"' {
				q = 0
				buf = append(buf, c)
				i++
				continue
			}
			// substitution still active inside double quotes
			if c == '`' {
				return "", false
			}
			if c == '$' && i+1 < len(s) && (s[i+1] == '(' || s[i+1] == '{') {
				return "", false
			}
			buf = append(buf, c)
			i++
			continue
		}
		switch c {
		case '\'', '"':
			q = c
			buf = append(buf, c)
			i++
		case '`':
			return "", false
		case '$':
			if i+1 < len(s) && (s[i+1] == '(' || s[i+1] == '{') {
				return "", false
			}
			buf = append(buf, c)
			i++
		case '(':
			depth++
			buf = append(buf, c)
			i++
		case ')':
			depth--
			if depth < 0 {
				return "", false
			}
			buf = append(buf, c)
			i++
		case '>':
			// Allowed only as `2>/dev/null`, `2> /dev/null`, or `2>&1` at top
			// level (single/double-quoted redirects are literal already).
			if i == 0 || s[i-1] != '2' {
				return "", false
			}
			j := i + 1
			var after int
			switch {
			case j < len(s) && s[j] == '>':
				return "", false // 2>>
			case j < len(s) && s[j] == '&':
				// stderr-to-stdout dup: exactly `2>&1`.
				if j+1 >= len(s) || s[j+1] != '1' {
					return "", false
				}
				after = j + 2
			default:
				k := j
				for k < len(s) && s[k] == ' ' {
					k++
				}
				if !strings.HasPrefix(s[k:], "/dev/null") {
					return "", false
				}
				after = k + len("/dev/null")
			}
			if after < len(s) {
				n := s[after]
				if n != ' ' && n != ';' && n != '|' && n != '&' && n != ')' {
					return "", false
				}
			}
			// strip the whole redirect (drop the '2' already buffered)
			if len(buf) > 0 && buf[len(buf)-1] == '2' {
				buf = buf[:len(buf)-1]
			}
			i = after
		default:
			buf = append(buf, c)
			i++
		}
	}
	if q != 0 || depth != 0 {
		return "", false
	}
	return string(buf), true
}

// splitReadOnlySegments splits a scanned command on the approved top-level
// separators `;`, `|`, `&&`, `||` (quote-aware: separators inside quotes are
// literal). Returns nil when a top-level single `&` (backgrounding) appears —
// not an approved separator, fail closed. Empty segments are kept and denied
// by isReadOnlyCommandSegment.
func splitReadOnlySegments(s string) []string {
	var segs []string
	var cur strings.Builder
	var q byte
	for i := 0; i < len(s); {
		c := s[i]
		if q != 0 {
			cur.WriteByte(c)
			if c == q {
				q = 0
			}
			i++
			continue
		}
		switch c {
		case '\'', '"':
			q = c
			cur.WriteByte(c)
			i++
		case ';':
			segs = append(segs, strings.TrimSpace(cur.String()))
			cur.Reset()
			i++
		case '|':
			if i+1 < len(s) && s[i+1] == '|' {
				i++
			}
			segs = append(segs, strings.TrimSpace(cur.String()))
			cur.Reset()
			i++
		case '&':
			if i+1 < len(s) && s[i+1] == '&' {
				i++
				segs = append(segs, strings.TrimSpace(cur.String()))
				cur.Reset()
				i++
				continue
			}
			return nil // single & backgrounding — not approved
		default:
			cur.WriteByte(c)
			i++
		}
	}
	segs = append(segs, strings.TrimSpace(cur.String()))
	return segs
}

// isReadOnlyCommandSegment classifies one top-level segment: it must start
// with an allowlisted read binary (after optional `sudo`) and its args must be
// read-only (`isReadOnlyGit`, `isReadOnlyFind`). Metacharacter safety is
// handled by the caller (scan + split), so no per-segment gate is needed.
func isReadOnlyCommandSegment(seg string) bool {
	tokens := strings.Fields(strings.TrimSpace(seg))
	if len(tokens) == 0 {
		return false
	}
	// `sudo <read>` is still read-only; skip the sudo token to classify the
	// actual binary.
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
	case "ls", "cat", "grep", "rg", "head", "tail", "wc",
		"pwd", "echo", "printf", "whoami", "hostname", "uname", "env",
		"printenv", "date", "stat", "file", "which", "type", "tree",
		"git":
		// git is only read-only for its non-mutating subcommands.
		return bin != "git" || isReadOnlyGit(tokens[1:])
	case "find":
		// BUG-344 D-4: find has write/exec action primaries; only pure search
		// forms pass.
		return isReadOnlyFind(tokens[1:])
	case "go":
		// BUG-344 follow-up: only go's read/report subcommands are safe —
		// build/run/get/install/generate/mod/vet write, download, or execute.
		return isReadOnlyGo(tokens[1:])
	case "curl":
		// Network fetch to stdout only: any output-file or data-sending flag
		// denies (curl -o/-O/-d/-F/-T/-X …).
		return isReadOnlyCurl(tokens[1:])
	default:
		return false
	}
}

// isReadOnlyFind reports whether a `find` invocation cannot mutate (BUG-344
// D-4). It scans the argument list for find's delete/exec/file-writing action
// primaries; any match denies the whole command. Pure search/print predicates
// (`-name`, `-iname`, `-type`, `-mtime`, `-print`, `-quit`, …) pass.
// Conservative false-negative: a filename pattern that is literally `-delete`
// (`find . -name '-delete'`) is denied too — acceptable for a read-only gate.
func isReadOnlyFind(args []string) bool {
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "-delete", "-exec", "-execdir", "-ok", "-okdir",
			"-fls", "-fprint", "-fprint0", "-fprintf":
			return false
		}
	}
	return true
}

// isReadOnlyGo reports whether a `go` invocation is read-only (BUG-344
// follow-up): test/list/env/doc/version/help report; a bare `go` prints help.
// Anything that builds, installs, downloads, or executes source
// (build/run/get/install/generate/mod/vet/…) denies. `go test` runs package
// tests (test code may write, accepted for scan) but must never compile a
// binary to disk: `-c`/`-o` deny.
func isReadOnlyGo(args []string) bool {
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "list", "env", "doc", "version", "help":
		return true
	case "test":
		for _, a := range args[1:] {
			if a == "-c" || a == "-o" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isReadOnlyCurl reports whether a `curl` invocation only fetches to stdout
// (BUG-344 follow-up): output-file flags (-o/-O/--output/--output-document)
// and data-sending flags (-d/-F/-T/--upload-file/-X/--request …) deny — a
// read-only posture must not download to disk or mutate a remote.
func isReadOnlyCurl(args []string) bool {
	for _, a := range args {
		switch a {
		case "-o", "-O", "--output", "--output-document",
			"-d", "--data", "--data-binary", "--data-urlencode",
			"-F", "--form", "-T", "--upload-file", "-X", "--request":
			return false
		}
	}
	return true
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
