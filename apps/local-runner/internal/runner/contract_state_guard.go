package runner

import (
	"path/filepath"
	"strings"
)

// BUG-397 (live CP-64 run-5307): an agent that knows the contract file's
// location can rewind durable FlowPilot state out from under the runner —
// the live reproducer ran `git checkout -- .flowpilot/contracts/
// frozen_contracts.ndjson` (plus git restore / git clean -fd / git reset)
// repeatedly during the reproduce window; the next lock lookup then
// hard-blocked "coder has no frozen contract" with the reproduce step falsely
// marked DONE. Append-only writes cannot help against a rewind below the
// filesystem layer, so the shared approval bridge denies destructive commands
// while a frozen contract is active for the run — the same provider-neutral
// choke point as decideReproduceTestLock, likewise BEFORE YOLO.
//
// Two shapes are denied:
//   - rewind/delete/overwrite verbs whose target touches .flowpilot/**:
//     git checkout/restore/reset/clean/rm on a .flowpilot path, rm/mv/cp/
//     truncate/dd/sed -i/find -delete on one, and > / >> / tee redirection
//     into one (the store file is a plain file an agent can truncate).
//   - whole-tree rewinds that destroy .flowpilot state without naming it:
//     `git reset --hard`, unlimited `git clean`, `git stash`,
//     `git checkout`/`git switch` to a ref, `git checkout/restore .`,
//     and tree-rewriting plumbing (rebase/merge/cherry-pick/revert/am/
//     apply/pull).
//
// Reads and non-destructive git work (status/diff/log/show/add/commit/fetch/
// checkout -b) fall through to the normal approval path unchanged. When no
// frozen contract exists for the run — or the store cannot be read — the
// guard does not fire (typed degradation: the ordinary approval card still
// stands between the agent and the file).
func (s *InteractiveService) decideFrozenContractStateGuard(rs *interactiveRun, details ApprovalDetails) (decision, reason string, handled bool) {
	if s == nil || rs == nil || details.Kind != "exec" {
		return "", "", false
	}
	cwd := strings.TrimSpace(rs.workspaceCwd)
	if cwd == "" {
		return "", "", false
	}
	// The contract record is keyed on the flow run id — the parent's for a
	// child run, the run's own for a hub. Check both.
	for _, runID := range []string{strings.TrimSpace(rs.parentRunID), strings.TrimSpace(rs.id)} {
		if runID == "" {
			continue
		}
		if _, ok := s.frozenContractForRun(cwd, runID); ok {
			if commandRewindsFlowPilotState(details.Command) {
				return "deny", "frozen_contract_state_protected", true
			}
			return "", "", false
		}
	}
	return "", "", false
}

// commandRewindsFlowPilotState reports whether a shell command would rewind,
// delete, or overwrite .flowpilot durable state — directly (path named) or
// wholesale (tree-wide rewind).
func commandRewindsFlowPilotState(command string) bool {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false
	}
	segs := splitReadOnlySegments(cmd)
	if len(segs) == 0 {
		// Unparseable/backgrounded — fail closed on the whole command text.
		segs = []string{cmd}
	}
	for _, seg := range segs {
		if segmentRewindsFlowPilotState(seg) {
			return true
		}
	}
	return false
}

func segmentRewindsFlowPilotState(seg string) bool {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return false
	}
	mentionsFP := segmentNamesFlowPilot(seg)
	if mentionsFP && segmentRedirectsIntoFlowPilot(seg) {
		return true
	}
	fields := strings.Fields(seg)
	if len(fields) == 0 {
		return false
	}
	switch pathBaseName(fields[0]) {
	case "rm", "rmdir", "unlink", "shred", "truncate", "dd", "tee", "mv", "cp", "rsync":
		if mentionsFP {
			return true
		}
		for _, f := range fields[1:] {
			if f == "." || f == "./" || f == "/*" || f == "/" {
				return true
			}
		}
		return false
	case "sed", "perl":
		// Only in-place editing mutates; a plain `sed -n` read stays allowed.
		return mentionsFP && (hasFlagPrefix(fields[1:], "-i") || hasFlagPrefix(fields[1:], "--in-place"))
	case "find":
		return hasFlagToken(fields, "-delete") && (mentionsFP || findRootCoversWorkspace(fields[1:]))
	case "git":
		return gitSegmentRewindsFlowPilotState(fields[1:], mentionsFP)
	}
	return false
}

// segmentNamesFlowPilot reports whether any token references a .flowpilot
// path (relative or absolute).
func segmentNamesFlowPilot(seg string) bool {
	for _, f := range strings.Fields(seg) {
		if strings.Contains(filepath.ToSlash(strings.Trim(f, "\"'")), ".flowpilot") {
			return true
		}
	}
	return false
}

// segmentRedirectsIntoFlowPilot reports whether the segment writes stdout into
// a .flowpilot path via > / >>. Redirections to other targets (including
// stderr plumbing like 2>/dev/null) do not match.
func segmentRedirectsIntoFlowPilot(seg string) bool {
	slashed := filepath.ToSlash(seg)
	for i := 0; i < len(slashed); i++ {
		if slashed[i] != '>' {
			continue
		}
		rest := strings.TrimLeft(slashed[i+1:], " >")
		end := strings.IndexAny(rest, " \t&|;")
		target := rest
		if end >= 0 {
			target = rest[:end]
		}
		if strings.Contains(strings.Trim(target, "\"'"), ".flowpilot") {
			return true
		}
	}
	return false
}

// gitSegmentRewindsFlowPilotState classifies the git subcommand's blast
// radius: path-targeted rewinds must name .flowpilot; tree-wide verbs are
// denied unconditionally because they cannot exclude .flowpilot.
func gitSegmentRewindsFlowPilotState(args []string, mentionsFP bool) bool {
	// Skip global flags (git -C dir <sub>, git --git-dir=… <sub>).
	idx := 0
	for idx < len(args) && strings.HasPrefix(args[idx], "-") {
		f := args[idx]
		idx++
		if f == "-C" || f == "-c" || f == "--git-dir" || f == "--work-tree" || f == "--namespace" {
			idx++
		}
	}
	if idx >= len(args) {
		return false
	}
	sub := args[idx]
	rest := args[idx+1:]

	// pathspecHitsProtected: pathspecs after `--` (or a bare `.`/root spec)
	// that cover .flowpilot — `.`, `./`, `:/`, `:(top)` rewind the whole tree.
	pathspecHitsProtected := func() bool {
		for _, a := range rest {
			norm := filepath.ToSlash(strings.Trim(a, "\"'"))
			if strings.Contains(norm, ".flowpilot") || norm == "." || norm == "./" ||
				norm == ":/" || strings.HasPrefix(norm, ":(top") {
				return true
			}
		}
		return false
	}

	switch sub {
	case "restore":
		// restore only ever takes pathspecs — no ref ambiguity. Deny only
		// .flowpilot/root pathspecs; `git restore calc.go` is a legitimate
		// self-undo.
		return mentionsFP || pathspecHitsProtected()
	case "checkout", "switch":
		if mentionsFP || pathspecHitsProtected() {
			return true
		}
		// `git checkout <ref>` / `git switch <ref>` move the whole tree to
		// another commit — every tracked .flowpilot record rewinds with it.
		// With `--`, the tokens after it are pathspecs (file-scoped rewind of
		// non-.flowpilot files is the agent's own business) — allowed unless
		// the pathspec check above fired. Ref-create/detach flag forms
		// (-b/-B/--orphan/--detach) never touch worktree files → allowed.
		sawDashDash := false
		for i := 0; i < len(rest); i++ {
			a := rest[i]
			if a == "--" {
				sawDashDash = true
				continue
			}
			if sawDashDash {
				continue // pathspec — already judged above
			}
			switch a {
			case "-b", "-B", "-t", "--track", "-C", "--detach", "--orphan":
				if a == "-b" || a == "-B" || a == "-t" || a == "--track" || a == "--orphan" {
					i++ // consume the flag's value
				}
				continue
			}
			if strings.HasPrefix(a, "-") {
				continue
			}
			// A bare ref token without `--` moves HEAD/tree — deny.
			return true
		}
		return false
	case "reset":
		for _, a := range rest {
			if a == "--hard" || strings.Contains(filepath.ToSlash(a), ".flowpilot") {
				return true
			}
		}
		return false
	case "clean":
		// `git clean` deletes untracked files — .flowpilot bookkeeping is
		// routinely untracked. Deny when forced and not limited to a safe path.
		force, pathLimited := false, false
		for _, a := range rest {
			if a == "--" {
				continue
			}
			if strings.HasPrefix(a, "-") {
				if strings.Contains(a, "f") {
					force = true
				}
				continue
			}
			norm := filepath.ToSlash(strings.Trim(a, "\"'"))
			if strings.Contains(norm, ".flowpilot") || norm == "." || norm == "./" || norm == ":/" {
				return true
			}
			pathLimited = true
		}
		return force && !pathLimited
	case "stash":
		// stash shelves ALL tracked modifications — .flowpilot included.
		return true
	case "rm", "mv":
		return mentionsFP || pathspecHitsProtected()
	case "rebase", "merge", "cherry-pick", "revert", "am", "apply", "pull", "update-ref":
		// Tree/history rewrites always touch the whole worktree (or refs).
		return true
	}
	return false
}

func hasFlagToken(fields []string, flag string) bool {
	for _, f := range fields {
		if f == flag {
			return true
		}
	}
	return false
}

func hasFlagPrefix(fields []string, prefix string) bool {
	for _, f := range fields {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// findRootCoversWorkspace reports whether a find command's root operand is
// missing or is a whole-workspace root (`.`, `./`, `/`), which a `-delete`
// would sweep .flowpilot through.
func findRootCoversWorkspace(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			return false // first flag reached — no root operand found
		}
		norm := filepath.ToSlash(strings.Trim(a, "\"'"))
		return norm == "." || norm == "./" || norm == "/" || strings.Contains(norm, ".flowpilot")
	}
	return false
}

func pathBaseName(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	return p
}
