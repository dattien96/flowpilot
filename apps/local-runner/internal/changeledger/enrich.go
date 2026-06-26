package changeledger

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"flowpilot-runner/internal/promptblock"
)

// reFeatureKeyLine matches lines like: `- chat-ui — description`
var reFeatureKeyLine = regexp.MustCompile(`^-\s+([a-z][a-z0-9-]+)\s`)
var reSupersededBy = regexp.MustCompile(`(?i)\bsuperseded\s+by\s+([a-z][a-z0-9-]+)\b`)

// reSafeKey normalizes an arbitrary string to a kebab-case key.
var reSafeKey = regexp.MustCompile(`[^a-z0-9]+`)

// EnrichAll resolves feature_key for each entry using the priority order from CP-35 §4.1:
//  0. feature declared in the commit tags `[Type][feature][layer?]` (Confidence=high, set by parser)
//  1. flowpilot:change-ledger §13 block in change-audit/CA-*.md (Confidence=high)
//  2. FEATURE-KEYS.md keyword match on subject (Confidence=low)
//  3. Dominant top-level changed path from `git show --name-only` (Confidence=low)
//  4. SourceDocID itself lowercased (Confidence=low)
//
// The CA index is built once; path resolution is lazy per-entry.
func EnrichAll(entries []Entry, repoDir string) []Entry {
	caIndex := buildCAIndex(repoDir)
	knownKeys := LoadKnownKeys(filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"))

	enriched := make([]Entry, len(entries))
	for i, e := range entries {
		enriched[i] = enrichEntry(e, repoDir, caIndex, knownKeys)
	}
	return enriched
}

type caRecord struct {
	FeatureKey string
	Excerpt    string
}

func enrichEntry(e Entry, repoDir string, caIndex map[string]caRecord, knownKeys map[string]struct{}) Entry {
	if e.FeatureKey != "" && isKnownFeatureKey(e.FeatureKey, knownKeys) {
		e.Confidence = ConfidenceHigh
	} else if e.FeatureKey != "" {
		e.Confidence = ConfidenceLow
	}

	// Priority 0: an explicitly declared and registry-verified feature key.
	if e.Confidence == ConfidenceHigh && e.FeatureKey != "" {
		if e.CAExcerpt == "" && e.SourceDocID != "" {
			if rec, ok := caIndex[e.SourceDocID]; ok && rec.Excerpt != "" {
				e.CAExcerpt = rec.Excerpt
			}
		}
		return e
	}

	// Priority 1: CA §13 block exact source_doc_id match
	if e.SourceDocID != "" {
		if rec, ok := caIndex[e.SourceDocID]; ok {
			if rec.FeatureKey != "" {
				e.FeatureKey = rec.FeatureKey
			}
			if rec.Excerpt != "" {
				e.CAExcerpt = rec.Excerpt
			}
			e.Confidence = ConfidenceHigh
			return e
		}
	}

	// Priority 2: FEATURE-KEYS.md keyword match on summary + source doc id
	combined := strings.ToLower(e.Summary + " " + e.SourceDocID)
	for key := range knownKeys {
		slug := strings.ReplaceAll(key, "-", " ")
		if strings.Contains(combined, slug) || strings.Contains(combined, key) {
			e.FeatureKey = key
			e.Confidence = ConfidenceLow
			return e
		}
	}

	// Priority 3: dominant top-level path from git show --name-only
	if key := pathFeatureKey(e.CommitHash, repoDir); key != "" {
		e.FeatureKey = key
		e.Confidence = ConfidenceLow
		return e
	}

	// Priority 4: SourceDocID itself (last resort)
	if e.SourceDocID != "" {
		e.FeatureKey = strings.ToLower(e.SourceDocID)
		e.Confidence = ConfidenceLow
		return e
	}

	e.FeatureKey = "unknown"
	e.Confidence = ConfidenceLow
	return e
}

// buildCAIndex scans change-audit/CA-*.md in the repo and builds a map of
// source_doc_id → feature_key extracted from flowpilot:change-ledger §13 blocks.
func buildCAIndex(repoDir string) map[string]caRecord {
	index := make(map[string]caRecord)
	pattern := filepath.Join(repoDir, "change-audit", "CA-*.md")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return index
	}
	for _, file := range files {
		key, docID, excerpt := parseCABlock(file)
		if key != "" && docID != "" {
			index[docID] = caRecord{FeatureKey: key, Excerpt: excerpt}
		}
	}
	return index
}

// parseCABlock reads one CA note and extracts (feature_key, source_doc_id) from the
// flowpilot:change-ledger block defined in SS-13 §13.1. Returns empty strings when
// the block is absent.
func parseCABlock(path string) (featureKey, sourceDocID, excerpt string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	inBlock := false
	var section string
	var scopeLine string
	var residualLine string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		stripped := strings.TrimSpace(line)
		if stripped == "# ---8<--- flowpilot:change-ledger" {
			inBlock = true
			continue
		}
		if stripped == "# --->8---" {
			inBlock = false
			continue
		}
		switch strings.ToLower(stripped) {
		case "## scope":
			section = "scope"
			continue
		case "## residual notes":
			section = "residual"
			continue
		case "## completed":
			section = ""
			continue
		}
		if section != "" && stripped != "" && !strings.HasPrefix(stripped, "feature_key:") && !strings.HasPrefix(stripped, "source_doc_id:") {
			if section == "scope" && scopeLine == "" {
				scopeLine = stripped
			}
			if section == "residual" && residualLine == "" {
				residualLine = stripped
			}
		}
		if !inBlock {
			continue
		}
		if strings.HasPrefix(stripped, "feature_key:") {
			featureKey = strings.TrimSpace(strings.TrimPrefix(stripped, "feature_key:"))
		}
		if strings.HasPrefix(stripped, "source_doc_id:") {
			sourceDocID = strings.TrimSpace(strings.TrimPrefix(stripped, "source_doc_id:"))
		}
	}
	parts := make([]string, 0, 2)
	if scopeLine != "" {
		parts = append(parts, "Scope: "+scopeLine)
	}
	if residualLine != "" {
		parts = append(parts, "Residual Notes: "+residualLine)
	}
	if len(parts) > 0 {
		excerpt, _ = promptblock.TruncateUTF8(strings.Join(parts, " | "), 400)
	}
	return
}

// LoadKnownKeys reads FEATURE-KEYS.md and returns all kebab-case keys.
// Lines follow the format: `- key — description`
func LoadKnownKeys(path string) map[string]struct{} {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	keys := make(map[string]struct{})
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if m := reFeatureKeyLine.FindStringSubmatch(line); m != nil {
			if successor := supersededSuccessor(line); successor != "" {
				keys[successor] = struct{}{}
				continue
			}
			keys[m[1]] = struct{}{}
		}
	}
	return keys
}

func supersededSuccessor(line string) string {
	if m := reSupersededBy.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

func isKnownFeatureKey(key string, knownKeys map[string]struct{}) bool {
	if key == "" {
		return false
	}
	_, ok := knownKeys[key]
	return ok
}

// pathFeatureKey runs `git show --name-only --no-commit-id --format= <hash>` and
// returns a kebab-case key derived from the most-changed top-level module directory.
func pathFeatureKey(commitHash, repoDir string) string {
	out, err := exec.Command(
		"git", "-C", repoDir,
		"show", "--name-only", "--no-commit-id", "--format=", commitHash,
	).Output()
	if err != nil || len(out) == 0 {
		return ""
	}

	counts := make(map[string]int)
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		file := strings.TrimSpace(sc.Text())
		if file == "" {
			continue
		}
		key := topLevelKey(file)
		if key != "" {
			counts[key]++
		}
	}

	var dominant string
	var max int
	for k, c := range counts {
		if c > max {
			max = c
			dominant = k
		}
	}
	return dominant
}

// topLevelKey extracts a kebab-case key from a file path using its first two segments.
// e.g. "apps/admin-web/src/chat/foo.tsx" → "admin-web"
//
//	"src/components/bar.go"             → "components"
func topLevelKey(filePath string) string {
	parts := strings.SplitN(filePath, "/", 4)
	var segment string
	switch len(parts) {
	case 1:
		segment = strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
	case 2:
		// Single dir: "src/foo.go" → "src"
		segment = parts[0]
	default:
		// Use the second segment which is usually the module name (skip "apps/", "src/", etc.)
		top := parts[0]
		if top == "apps" || top == "packages" || top == "services" || top == "libs" {
			segment = parts[1]
		} else {
			segment = top
		}
	}
	key := reSafeKey.ReplaceAllString(strings.ToLower(segment), "-")
	key = strings.Trim(key, "-")
	if key == "" || key == "-" {
		return ""
	}
	return key
}
