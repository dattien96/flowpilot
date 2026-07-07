package runner

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BUG-246: per-project, persisted "don't ask again" rules for SHELL-COMMAND
// approvals under YOLO=off.
//
// A rule is an executable+subcommand prefix (granularity B): approving
// `git status -sb` with remember stores the rule `git status`, which then
// auto-approves `git status`, `git status --short`, etc. — but never
// `git push` (different subcommand) nor any compound command (`&&`, `|`, ...).
//
// The list is consulted by turnBridge.RequestApproval BEFORE the ask path, so
// it is provider-neutral: Claude and Codex both route shell approvals through
// RequestApproval. It is persisted in .flowpilot/settings/approval-allowlist.json
// (the same workspace-rooted .flowpilot the ledger/catalog/gate-config already
// live in — CA-246 briefly relocated it to a FlowPilot-app-owned, project_id-keyed
// dir, then reverted per user direction: consistency with the rest of the
// context-engine state outweighed the cross-binding-path benefit) and synced to
// Drive via contextsync.SharedFiles (push) + restoreApprovalAllowlistFromDrive
// (pull), so the trust travels with the project across machines.
//
// NOTE: this deliberately does NOT touch Claude's own settings.json allow-rules
// (ensureClaudeConfigSettings still wipes those every turn). Trust lives in
// FlowPilot's layer, so a stale rule can never silently bypass gating (BUG-069).

const approvalAllowlistFileName = "approval-allowlist.json"

// approvalCommandOperators are shell metacharacters that make a command
// "compound". A command containing any of these is NEVER auto-approved and can
// NEVER be turned into a remembered rule — each piece must be judged on its own,
// so `git status && rm -rf /` always re-asks even when `git status` is
// remembered. Detection is intentionally fail-closed: a metacharacter inside a
// quoted argument also blocks remembering (worst case is asking when we could
// have auto-approved — the safe direction).
var approvalCommandOperators = []string{
	"&&", "||", "|", ";", "&", ">", "<", "`", "$(", "(", ")", "{", "}", "\n", "\r",
}

// approvalWrapperCommands are leading tokens that wrap another command (a token
// killer / privilege / timing shim). deriveApprovalRule skips them — but KEEPS
// them in the stored rule — so a wrapped command pins the REAL executable +
// subcommand instead of collapsing to `<wrapper> <tool>`. Without this, under a
// wrapper like `rtk`, `rtk git status` would store `rtk git` and green-light
// `rtk git push` too (BUG-246 follow-up for rtk sandboxes).
var approvalWrapperCommands = map[string]bool{
	"rtk":   true,
	"sudo":  true,
	"time":  true,
	"nice":  true,
	"npx":   true,
	"xargs": true,
}

// approvalAllowlistFile is the on-disk shape of approval-allowlist.json.
type approvalAllowlistFile struct {
	Allow []string `json:"allow"`
}

func approvalAllowlistPath(dotFP string) string {
	return filepath.Join(dotFP, "settings", approvalAllowlistFileName)
}

// isCompoundCommand reports whether a shell command chains/redirects/expands,
// which disqualifies it from auto-approval and from being remembered.
func isCompoundCommand(command string) bool {
	for _, op := range approvalCommandOperators {
		if strings.Contains(command, op) {
			return true
		}
	}
	return false
}

// looksLikeSubcommand reports whether a token is a bare verb (e.g. "status",
// "run", "install") rather than a flag, path, or assignment — so `git status`
// keeps the subcommand but `ls -la` and `cat file.txt` do not.
func looksLikeSubcommand(tok string) bool {
	if tok == "" {
		return false
	}
	for i, r := range tok {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		switch {
		case i == 0 && !isLetter:
			return false
		case !isLetter && !isDigit && r != '-' && r != '_':
			return false
		}
	}
	return true
}

// deriveApprovalRule extracts the executable+subcommand prefix (granularity B)
// for remembering, skipping any leading wrapper tokens (rtk/sudo/...) but keeping
// them in the rule. Returns ok=false for compound/empty commands. Examples:
//
//	"git status -sb"      -> "git status"
//	"npm run build"       -> "npm run"
//	"ls -la /foo"         -> "ls"
//	"cat file.txt"        -> "cat"
//	"go test ./..."       -> "go test"
//	"rtk git status -sb"  -> "rtk git status"   (wrapper skipped, real exec+subcmd pinned)
//	"rtk ls -la"          -> "rtk ls"
//	"sudo git status"     -> "sudo git status"
func deriveApprovalRule(command string) (string, bool) {
	command = strings.TrimSpace(command)
	if command == "" || isCompoundCommand(command) {
		return "", false
	}
	tokens := strings.Fields(command)
	if len(tokens) == 0 {
		return "", false
	}
	// Skip leading wrapper tokens so the rule pins the real executable+subcommand.
	start := 0
	for start < len(tokens) && approvalWrapperCommands[tokens[start]] {
		start++
	}
	if start >= len(tokens) {
		// Command was only wrapper tokens (e.g. bare "rtk"); remember it verbatim.
		return strings.Join(tokens, " "), true
	}
	end := start + 1 // real executable
	if start+1 < len(tokens) && looksLikeSubcommand(tokens[start+1]) {
		end = start + 2 // + real subcommand
	}
	return strings.Join(tokens[:end], " "), true
}

// matchesApprovalRule reports whether command is covered by any remembered rule.
// A rule matches when its tokens are an exact prefix of the command's tokens
// (token boundaries respected, so `git status` matches `git status -sb` but not
// `git statusfoo`). Compound commands never match.
func matchesApprovalRule(command string, rules []string) bool {
	if len(rules) == 0 {
		return false
	}
	command = strings.TrimSpace(command)
	if command == "" || isCompoundCommand(command) {
		return false
	}
	cmdTokens := strings.Fields(command)
	if len(cmdTokens) == 0 {
		return false
	}
	for _, rule := range rules {
		ruleTokens := strings.Fields(strings.TrimSpace(rule))
		if len(ruleTokens) == 0 || len(ruleTokens) > len(cmdTokens) {
			continue
		}
		match := true
		for i, rt := range ruleTokens {
			if cmdTokens[i] != rt {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// readApprovalAllowRules loads the persisted rules; absent/unreadable/malformed
// yields no rules (the safe default is to ask).
func readApprovalAllowRules(dotFP string) []string {
	data, err := os.ReadFile(approvalAllowlistPath(dotFP))
	if err != nil {
		return nil
	}
	var f approvalAllowlistFile
	if json.Unmarshal(data, &f) != nil {
		return nil
	}
	return f.Allow
}

// writeApprovalAllowRules trims, de-duplicates, and sorts rules before writing,
// so the file is stable across machines (Drive-sync friendly, additive merges).
func writeApprovalAllowRules(dotFP string, rules []string) error {
	settingsDir := filepath.Join(dotFP, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	sort.Strings(out)
	data, err := json.MarshalIndent(approvalAllowlistFile{Allow: out}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(approvalAllowlistPath(dotFP), data, 0o644)
}

// addApprovalAllowRule appends rule (idempotent — writeApprovalAllowRules dedupes).
func addApprovalAllowRule(dotFP, rule string) error {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return nil
	}
	return writeApprovalAllowRules(dotFP, append(readApprovalAllowRules(dotFP), rule))
}

// removeApprovalAllowRule drops an exact rule (for the settings management UI).
func removeApprovalAllowRule(dotFP, rule string) error {
	rule = strings.TrimSpace(rule)
	existing := readApprovalAllowRules(dotFP)
	out := make([]string, 0, len(existing))
	for _, r := range existing {
		if strings.TrimSpace(r) != rule {
			out = append(out, r)
		}
	}
	return writeApprovalAllowRules(dotFP, out)
}

// ---- HTTP handlers (settings management) ------------------------------------

type approvalAllowlistPayload struct {
	Allow []string `json:"allow"`
}

type removeApprovalAllowRuleRequest struct {
	WorkingDirectory string `json:"workingDirectory"`
	Rule             string `json:"rule"`
}

// handleGetApprovalAllowlist handles
// GET /client/projects/{projectId}/engine/approval-allowlist
func (s *InteractiveService) handleGetApprovalAllowlist(w http.ResponseWriter, r *http.Request) {
	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(r.URL.Query().Get("workingDirectory"))
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	dotFP := filepath.Join(workingDirectory, ".flowpilot")
	rules := readApprovalAllowRules(dotFP)
	if rules == nil {
		rules = []string{}
	}
	writeInteractiveJSON(w, http.StatusOK, approvalAllowlistPayload{Allow: rules})
}

// handleRemoveApprovalAllowRule handles
// POST /client/projects/{projectId}/engine/approval-allowlist/remove
func (s *InteractiveService) handleRemoveApprovalAllowRule(w http.ResponseWriter, r *http.Request) {
	var req removeApprovalAllowRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(req.WorkingDirectory)
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	dotFP := filepath.Join(workingDirectory, ".flowpilot")
	if err := removeApprovalAllowRule(dotFP, req.Rule); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "write_failed", err.Error()))
		return
	}
	rules := readApprovalAllowRules(dotFP)
	if rules == nil {
		rules = []string{}
	}
	writeInteractiveJSON(w, http.StatusOK, approvalAllowlistPayload{Allow: rules})
}
