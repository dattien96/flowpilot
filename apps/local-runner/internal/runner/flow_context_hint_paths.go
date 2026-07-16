package runner

import (
	"context"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"flowpilot-runner/internal/flowgate"
)

// extractPromptSourcePaths pulls path-like tokens from a user/plan prompt
// (Task-246). Pure string parse — no filesystem I/O.
// V10R4: quote-aware tokenization so `src/my file.go` stays one path.
func extractPromptSourcePaths(prompt string) []string {
	if strings.TrimSpace(prompt) == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, tok := range tokenizePromptTokens(prompt) {
		tok = strings.TrimFunc(tok, func(r rune) bool {
			return r == '`' || r == '\'' || r == '"' || r == '(' || r == ')' || r == ',' ||
				r == '[' || r == ']' || r == '{' || r == '}' || unicode.IsSpace(r)
		})
		// Task-246: strip trailing prose punctuation only (keep leading "./").
		// "Fix apps/foo.go." → apps/foo.go
		tok = strings.TrimRight(tok, ".,:;!?")
		if tok == "" {
			continue
		}
		low := strings.ToLower(tok)
		if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
			continue
		}
		if !strings.ContainsAny(tok, `/\`) {
			continue
		}
		if filepath.Ext(tok) == "" {
			continue
		}
		// Normalize Windows-style separators so prose paths work on all OSes.
		norm := filepath.ToSlash(strings.ReplaceAll(tok, `\`, `/`))
		if flowgate.IsDocOrAuditFile(norm) {
			continue
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		out = append(out, norm)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// tokenizePromptTokens splits prompt into tokens, treating content inside
// matching quotes (", ', `) as a single token so paths with spaces survive
// (V10R4 / Task-246). Unclosed quotes take the rest of the string as one token.
//
// V10R4 P2-04: only treat quote runes as openers at a token boundary. An
// apostrophe mid-token (don't, it's) is kept as a letter so it does not
// swallow the rest of the prompt as one quoted blob.
func tokenizePromptTokens(prompt string) []string {
	var out []string
	var b strings.Builder
	var quote rune // 0 when not in quotes
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	atBoundary := true
	for _, r := range prompt {
		if quote != 0 {
			if r == quote {
				quote = 0
				// keep delimiters out of token; flush after close
				flush()
				atBoundary = true
				continue
			}
			b.WriteRune(r)
			atBoundary = false
			continue
		}
		switch r {
		case '"', '`':
			// Double-quote and backtick always open at boundary (after flush).
			flush()
			quote = r
			atBoundary = false
		case '\'':
			// Apostrophe only opens a quote at a token boundary. Mid-token
			// apostrophes (contractions) stay in the current token.
			if atBoundary {
				flush()
				quote = r
				atBoundary = false
			} else {
				b.WriteRune(r)
			}
		default:
			if unicode.IsSpace(r) {
				flush()
				atBoundary = true
			} else {
				b.WriteRune(r)
				atBoundary = false
			}
		}
	}
	flush()
	return out
}

// uncommittedChangedPaths lists staged+unstaged+untracked paths vs HEAD
// (Task-246). Non-git workspace or any error → nil (soft degrade).
func uncommittedChangedPaths(workspace string) []string {
	if strings.TrimSpace(workspace) == "" {
		return nil
	}
	start := time.Now()
	// Task-246: each git command gets its own 3s budget (not a shared context).
	runGit := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "git", append([]string{"-C", workspace}, args...)...).Output()
	}
	// BUG-288 #29: -z NUL-delimited so paths with spaces/newlines parse safely.
	diffOut, err1 := runGit("diff", "--name-only", "-z", "HEAD")
	untrackedOut, err2 := runGit("ls-files", "-z", "--others", "--exclude-standard")
	// Task-246 / CP-50: any git error → soft-degrade to nil (not "both must fail").
	// E.g. fresh git init has no HEAD so diff HEAD fails while ls-files may succeed.
	if err1 != nil || err2 != nil {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	add := func(raw []byte) {
		// V10R4 / Task-246: Git -z preserves exact pathnames; only drop the empty
		// NUL terminator record. Do NOT TrimSpace — leading/trailing space or
		// newline can be valid path components.
		for _, line := range strings.Split(string(raw), "\x00") {
			if line == "" {
				continue
			}
			p := filepath.ToSlash(line)
			if seen[p] || flowgate.IsDocOrAuditFile(p) {
				continue
			}
			seen[p] = true
			paths = append(paths, p)
			if len(paths) >= 20 {
				return
			}
		}
	}
	if err1 == nil {
		add(diffOut)
	}
	if err2 == nil && len(paths) < 20 {
		add(untrackedOut)
	}
	if time.Since(start) > time.Second {
		log.Printf("[source.excerpt] uncommittedChangedPaths workspace=%q took %s paths=%d", workspace, time.Since(start), len(paths))
	}
	return paths
}
