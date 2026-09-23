package runner

import (
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// CP-64 (reproduce-first-gate): runner-side half of the reproduce contract.
//
// The rule engine (flowgate/reproduce_rule.go) is deliberately pure — it only
// reads the caller-computed TurnResult signals. Everything needing I/O or
// topology lives here: flag resolution, which turn is a reproduce turn, the
// compile/assertion classification feed, the read-only lock written into the
// coder step's frozen contract, and the bridge enforcement that makes the lock
// real for every provider.

// ReproduceGateEnv is RETIRED (CP-67 P-2, post-review B-9): the reproduce
// gate is always-on and this env var no longer has any effect. The constant
// survives only so old Setenv calls in tests keep compiling; setting it to
// any value changes nothing.
const ReproduceGateEnv = "FLOWPILOT_ENABLE_REPRODUCE_GATE"

// Reproduce node assets (CP-64 P-2). The legacy degrade pair
// (prompts/test-signatures.md + agents/tester.md) is retired with the flag
// (CP-67 B-9) — the pack files remain only as rollback artifacts.
const (
	reproducePromptPath = "prompts/reproduce-failing-test.md"
	reproduceAgentRef   = "agents/reproducer.md"
)

// ReproduceGateEnabled reports whether the reproduce-first gate is on. B-9
// retire: it ALWAYS is — the CP-64 flag path no longer exists as a runtime
// option (CP-67 P-2), so every workspace runs reproduce-first and
// contract-first unconditionally; the fallback for a bad rollout is a revert
// commit, not a flag flip.
func ReproduceGateEnabled() bool {
	return true
}

// IsReproduceBehavior reports whether a node's declared behavior resolves to
// the CP-64 reproduce behavior (aliases included).
func IsReproduceBehavior(behavior string) bool {
	canonical, ok := agentpack.NormalizeBehaviorID(behavior)
	return ok && canonical == "agent.reproduce"
}

// flowWriterNodeIDForRun resolves the flow's agent.code writer step (the coder
// node a reproduce test file is locked against). Empty when the run has no live
// topology or no writer.
func (s *InteractiveService) flowWriterNodeIDForRun(parentRunID string) string {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return ""
	}
	for _, n := range s.activeFlowNodesFor(parentRunID) {
		if canonical, ok := agentpack.NormalizeBehaviorID(n.Behavior); ok && canonical == "agent.code" {
			return n.ID
		}
	}
	return ""
}

// flowHasReproduceNode reports whether the run's live topology declares a
// reproduce node — the CP-64 P-1 "this IS a bug flow" test for the
// behavior-change activation rule (data-driven, never flow-id string matching).
func (s *InteractiveService) flowHasReproduceNode(parentRunID string) bool {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return false
	}
	for _, n := range s.activeFlowNodesFor(parentRunID) {
		if IsReproduceBehavior(n.Behavior) {
			return true
		}
	}
	return false
}

// reproduceTurnForRun reports whether the CURRENT turn must demonstrate the bug
// (CP-64 P-1 activation): the run's active node is a reproduce node, or the turn
// declared change_type behavior-change inside a flow that has one.
func (s *InteractiveService) reproduceTurnForRun(rs *interactiveRun) bool {
	if !ReproduceGateEnabled() || s == nil || rs == nil {
		return false
	}
	if node, ok := flowNodeForRun(s, rs); ok && IsReproduceBehavior(node.Behavior) {
		return true
	}
	s.mu.Lock()
	changeType := rs.changeType
	parentID := rs.parentRunID
	parent := s.runs[parentID]
	s.mu.Unlock()
	if parent != nil && strings.TrimSpace(parent.changeType) != "" {
		changeType = parent.changeType
	}
	if !strings.EqualFold(strings.TrimSpace(changeType), "behavior-change") {
		return false
	}
	if strings.TrimSpace(parentID) == "" {
		// A root/hub turn owns the topology itself.
		return s.flowHasReproduceNode(rs.id)
	}
	return s.flowHasReproduceNode(parentID)
}

// ReproduceTestFilesWritten filters a turn's written paths down to the test
// files that must be locked read-only for the coder. Test classification reuses
// the same helper the oracle uses, so a path treated as a test by the oracle is
// treated as a test by the lock (no second, drifting definition).
func ReproduceTestFilesWritten(written []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range written {
		p = strings.TrimSpace(p)
		if p == "" || !flowgate.IsTestFile(p) || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, filepath.ToSlash(p))
	}
	return out
}

// recordReproduceTestLock writes the reproduction test file(s) into the coder
// step's frozen contract as read-only, AFTER the r-reproduce gate passed
// (CP-64 P-3 / Task-366 T-2). Never fails a turn: a missing frozen contract or
// an I/O error is logged and the turn still completes — the lock is
// defense-in-depth on top of the gate, not a gate itself.
func (s *InteractiveService) recordReproduceTestLock(cwd, parentRunID string, written []string) {
	if !ReproduceGateEnabled() || strings.TrimSpace(cwd) == "" || strings.TrimSpace(parentRunID) == "" {
		return
	}
	locked := ReproduceTestFilesWritten(written)
	if len(locked) == 0 {
		return
	}
	writerID := s.flowWriterNodeIDForRun(parentRunID)
	if writerID == "" {
		log.Printf("[gate] reproduce-first: no agent.code writer node in run %q; skipping test-file lock", parentRunID)
		return
	}
	rec, err := changecontract.LockReproduceTestPaths(cwd, parentRunID, writerID, locked, time.Now().UTC())
	if err != nil {
		log.Printf("[gate] reproduce-first: lock test file(s) %v for step %q failed: %v", locked, writerID, err)
		return
	}
	log.Printf("[gate] reproduce-first: locked %v read-only for coder step %q (contract v%d)", locked, writerID, rec.Version)
}

// normalizeReproduceLockCandidate makes an approval's path operand comparable to
// a frozen record's workspace-relative ReadOnlyPaths: separators unified,
// absolute paths made workspace-relative, escapes rejected.
func normalizeReproduceLockCandidate(workspace, raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, `\`, `/`)
	if filepath.IsAbs(filepath.FromSlash(p)) {
		if strings.TrimSpace(workspace) == "" {
			return ""
		}
		rel, err := filepath.Rel(workspace, filepath.FromSlash(p))
		if err != nil {
			return ""
		}
		p = filepath.ToSlash(rel)
	}
	p = path.Clean(p)
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return ""
	}
	return p
}

// reproduceLockCommandTargetsLockedPath reports whether a shell command's text
// names one of the locked test files. Matches the full relative path or the bare
// basename, so a `rm -rf x_test.go`-style command is caught too.
func reproduceLockCommandTargetsLockedPath(cmd string, rec changecontract.FrozenContractRecord) bool {
	slashed := filepath.ToSlash(cmd)
	for _, locked := range rec.ReadOnlyPaths {
		locked = filepath.ToSlash(strings.TrimSpace(locked))
		if locked == "" {
			continue
		}
		if strings.Contains(slashed, locked) {
			return true
		}
		if base := path.Base(locked); base != "" && base != "." && strings.Contains(slashed, base) {
			return true
		}
	}
	return false
}

// decideReproduceTestLock is the bridge-level enforcement of the read-only lock
// (CP-64 P-3 / Task-366 T-2). handled=true only ever means DENY: a write or an
// unclassified mutation aimed at a locked reproduce test file is silent-denied
// for every provider, before YOLO can auto-approve it. Reads (Kind=file read
// tools, read-only shell commands) are deliberately left unhandled so they fall
// through to the legacy path unchanged.
func (s *InteractiveService) decideReproduceTestLock(rs *interactiveRun, details ApprovalDetails) (decision, reason string, handled bool) {
	if !ReproduceGateEnabled() || s == nil || rs == nil {
		return "", "", false
	}
	parentID := strings.TrimSpace(rs.parentRunID)
	if parentID == "" {
		return "", "", false
	}
	writerID := s.flowWriterNodeIDForRun(parentID)
	if writerID == "" {
		return "", "", false
	}
	// Only the coder child of this flow is lock-enforced — a reviewer or any
	// other node has no write path to the test file anyway.
	if label := strings.TrimSpace(rs.label); label != writerID {
		if stepID := strings.TrimSpace(rs.stepID); stepID != writerID {
			return "", "", false
		}
	}
	cwd := strings.TrimSpace(rs.workspaceCwd)
	if cwd == "" {
		return "", "", false
	}
	store, err := changecontract.NewFrozenStore(cwd)
	if err != nil {
		return "", "", false
	}
	rec, ok, err := store.GetFrozenForStep(parentID, writerID)
	if err != nil || !ok || len(rec.ReadOnlyPaths) == 0 {
		return "", "", false
	}

	switch details.Kind {
	case "exec":
		if isReadOnlyCommand(details.Command) {
			return "", "", false
		}
		if reproduceLockCommandTargetsLockedPath(details.Command, rec) {
			return "deny", "reproduce_test_locked", true
		}
	case "file":
		// A read tool on the locked file stays allowed (the coder must be able
		// to read the failing test it is fixing).
		if isReadOnlyToolName(details.Reason) {
			return "", "", false
		}
		candidate := normalizeReproduceLockCandidate(cwd, details.Command)
		if candidate == "" {
			return "", "", false
		}
		// BUG-388: records frozen before the abs->rel store fix may carry
		// absolute ReadOnlyPaths — relativize-under-workspace on read too.
		if changecontract.IsReadOnlyLockedPathUnder(rec, candidate, cwd) {
			return "deny", "reproduce_test_locked", true
		}
	}
	return "", "", false
}

// reproduceFailuresExerciseTarget implements the BUG-389 fabricated-RED check:
// a reproduce turn's RED is only a reproduction when at least one failing test
// exercises a symbol declared in the step's frozen scope. Live proof of the
// gap: `simulatedBuggy := 3 + 4; if simulatedBuggy != 12 { t.Fatalf(...) }`
// satisfied r-reproduce on runs 4501/5307/6008.
//
// Returns (checked, hit):
//   - checked=false — the runner cannot verify (no frozen contract, no Go
//     declared files, no declared symbols, or no failing test resolves to a
//     source file). Typed degradation: the named-failure verdict stands rather
//     than hard-blocking every reproduce turn on an unverifiable shape.
//   - checked=true, hit=false — failing tests resolved but none reference a
//     declared symbol: the failure is unrelated to the reported bug.
func (s *InteractiveService) reproduceFailuresExerciseTarget(workspace, contractRunID string, failedNames, writtenPaths []string) (checked, hit bool) {
	if len(failedNames) == 0 || strings.TrimSpace(workspace) == "" {
		return false, false
	}
	rec, ok := s.frozenContractForRun(workspace, contractRunID)
	if !ok {
		return false, false
	}
	symSet := map[string]bool{}
	declaredDirs := map[string]bool{}
	for _, p := range rec.DeclaredPaths {
		rel := strings.TrimSpace(p)
		if rel == "" || flowgate.IsTestFile(rel) || flowgate.LangForPath(rel) != "go" {
			continue
		}
		if d := path.Dir(rel); d != "" && d != "." {
			declaredDirs[d] = true
		} else {
			declaredDirs["."] = true
		}
		src, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		infos, err := flowgate.ExtractSymbolInfos("go", rel, src, nil)
		if err != nil {
			continue
		}
		for _, in := range infos {
			if in.Name != "" {
				symSet[in.Name] = true
			}
		}
	}
	if len(symSet) == 0 {
		return false, false
	}
	// Candidate test files: this turn's written *_test.go plus every *_test.go
	// under the declared dirs — the reproduction file may have been written on
	// an earlier reprompt turn of the same episode.
	candidates := map[string]bool{}
	for _, p := range writtenPaths {
		rel := filepath.ToSlash(strings.TrimSpace(p))
		if strings.HasSuffix(rel, "_test.go") {
			candidates[rel] = true
		}
	}
	for d := range declaredDirs {
		matches, _ := filepath.Glob(filepath.Join(workspace, filepath.FromSlash(d), "*_test.go"))
		for _, m := range matches {
			if rel, err := filepath.Rel(workspace, m); err == nil {
				candidates[filepath.ToSlash(rel)] = true
			}
		}
	}
	resolved := false
	for _, name := range failedNames {
		root := name
		if i := strings.Index(root, "/"); i >= 0 {
			root = root[:i]
		}
		if root == "" {
			continue
		}
		decl := "func " + root + "("
		for f := range candidates {
			b, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(f)))
			if err != nil || !strings.Contains(string(b), decl) {
				continue
			}
			resolved = true
			for sym := range symSet {
				if testSourceCallsSymbol(string(b), sym) {
					return true, true
				}
			}
			break
		}
	}
	return resolved, false
}

// testSourceCallsSymbol reports whether src contains a call-shaped use of sym
// (`sym(` at an identifier boundary) — the deterministic proxy for "this test
// exercises the reported production code".
func testSourceCallsSymbol(src, sym string) bool {
	for idx := 0; ; {
		i := strings.Index(src[idx:], sym+"(")
		if i < 0 {
			return false
		}
		pos := idx + i
		// Identifier-boundary check: the char before must not be
		// letter/digit/underscore (so "ReadAll(" does not match "All(").
		if pos == 0 || !isIdentByte(src[pos-1]) {
			return true
		}
		idx = pos + len(sym)
	}
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
