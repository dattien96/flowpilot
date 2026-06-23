package changeledger

import (
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// gitLogFormat records: fields separated by 0x1f (unit sep), records by 0x1e (record sep).
// Fields: hash, committed-ISO8601, subject, body
const gitLogFormat = "--pretty=format:%H\x1f%cI\x1f%s\x1f%b\x1e"

var (
	reChangeType  = regexp.MustCompile(`(?i)^\[(\w+)\]`)
	reSourceDocID = regexp.MustCompile(`(Task-\d+|BUG-\d+|CP-\d+[\w-]*)`)
)

// ParseRepo runs `git log` on repoDir, reads commits since the last cursor (incremental),
// parses each into an Entry, sorts ascending by CommittedAt, assigns OrderIndex,
// and writes the new cursor. Returns nil, nil when there are no new commits.
func ParseRepo(repoDir, dotFlowpilotDir string) ([]Entry, error) {
	cursorFile := CursorPath(dotFlowpilotDir)
	cursor := readCursor(cursorFile)

	args := []string{"-C", repoDir, "log", "--no-merges", gitLogFormat}
	if cursor != "" {
		args = append(args, cursor+"..HEAD")
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		// Non-fatal: empty repo, cursor ahead of HEAD, or git not on PATH.
		return nil, nil
	}

	rawRecords := strings.Split(string(out), "\x1e")
	var entries []Entry
	for _, raw := range rawRecords {
		e, ok := parseRecord(raw)
		if ok {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		return nil, nil
	}

	// Sort ascending by CommittedAt so newest = max OrderIndex.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CommittedAt < entries[j].CommittedAt
	})
	for i := range entries {
		entries[i].OrderIndex = i
	}

	// Persist cursor = hash of the newest commit.
	newest := entries[len(entries)-1].CommitHash
	_ = writeCursor(cursorFile, newest)

	return entries, nil
}

// parseRecord converts one raw git log record (unit-sep fields) into an Entry.
func parseRecord(raw string) (Entry, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Entry{}, false
	}
	parts := strings.SplitN(raw, "\x1f", 4)
	if len(parts) < 3 {
		return Entry{}, false
	}
	hash := strings.TrimSpace(parts[0])
	if hash == "" {
		return Entry{}, false
	}
	committedAt := normalizeRFC3339(strings.TrimSpace(parts[1]))
	subject := strings.TrimSpace(parts[2])
	body := ""
	if len(parts) == 4 {
		body = strings.TrimSpace(parts[3])
	}

	return Entry{
		CommitHash:  hash,
		ChangeType:  extractChangeType(subject),
		SourceDocID: extractSourceDocID(subject, body),
		Summary:     cleanSummary(subject),
		CommittedAt: committedAt,
		Confidence:  ConfidenceLow, // enrich.go upgrades to high when CA block matches
	}, true
}

func extractChangeType(subject string) string {
	m := reChangeType.FindStringSubmatch(subject)
	if m == nil {
		return "other"
	}
	switch strings.ToLower(m[1]) {
	case "feature":
		return "feature"
	case "bugfix", "bug":
		return "bugfix"
	case "refactor":
		return "refactor"
	case "docs":
		return "docs"
	case "hotfix":
		return "hotfix"
	default:
		return "other"
	}
}

func extractSourceDocID(subject, body string) string {
	if m := reSourceDocID.FindString(subject); m != "" {
		return m
	}
	return reSourceDocID.FindString(body)
}

// cleanSummary strips the [Type]: Task-NNN prefix so the human-readable part remains.
func cleanSummary(subject string) string {
	s := subject

	// Strip leading [Type] tag
	if loc := reChangeType.FindStringIndex(s); loc != nil && loc[0] == 0 {
		s = s[loc[1]:]
	}
	s = strings.TrimSpace(s)

	// Strip leading colon separator
	s = strings.TrimPrefix(s, ":")
	s = strings.TrimSpace(s)

	// Strip leading SourceDocID (Task-NNN, BUG-NNN, CP-NN)
	if loc := reSourceDocID.FindStringIndex(s); loc != nil && loc[0] == 0 {
		s = s[loc[1]:]
	}
	s = strings.TrimSpace(s)

	if s == "" {
		return subject // fallback: never return an empty summary
	}
	return s
}

func normalizeRFC3339(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	// Some git versions emit with offset — try RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return ts // return as-is if unparseable; sorting will still work lexicographically
}

func readCursor(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeCursor(path, hash string) error {
	return os.WriteFile(path, []byte(hash+"\n"), 0o644)
}
