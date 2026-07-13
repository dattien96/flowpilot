package featurecatalog

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"flowpilot-runner/internal/changeledger"
)

var reSupersededBy = regexp.MustCompile(`(?i)\bsuperseded\s+by\s+([a-z][a-z0-9-]+)\b`)

type Feature struct {
	Key       string   `json:"feature_key"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	Keywords  []string `json:"keywords"`
	FileGlobs []string `json:"file_globs"`
	DocRefs   []string `json:"doc_refs"`
}

type Catalog struct {
	features map[string]Feature
	mu       sync.RWMutex
}

func New() *Catalog { return &Catalog{features: make(map[string]Feature)} }

func (c *Catalog) Add(f Feature) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.features[f.Key] = f
}

func (c *Catalog) Get(key string) (Feature, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	f, ok := c.features[key]
	return f, ok
}

func (c *Catalog) All() []Feature {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Feature, 0, len(c.features))
	for _, f := range c.features {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

var stopwords = map[string]struct{}{
	"the": {}, "a": {}, "and": {}, "or": {}, "in": {}, "of": {},
	"for": {}, "to": {}, "is": {}, "are": {}, "with": {}, "that": {},
	"this": {}, "it": {}, "as": {}, "by": {},
}

func tokenize(text string) []string {
	lower := strings.ToLower(text)
	parts := strings.FieldsFunc(lower, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		if len(p) < 3 {
			continue
		}
		if _, stop := stopwords[p]; stop {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func mergeKeywords(existing, extra []string) []string {
	seen := make(map[string]struct{})
	for _, k := range existing {
		seen[k] = struct{}{}
	}
	out := append([]string(nil), existing...)
	for _, k := range extra {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}

// Build constructs a catalog from FEATURE-KEYS.md, SS/CP docs, and ledger keys,
// then persists it to dotFlowpilotDir/catalog/features.ndjson.
func Build(repoDir string, ledger interface{ ListFeatures() []string }, dotFlowpilotDir string) (*Catalog, error) {
	c := New()

	// (a) Parse FEATURE-KEYS.md
	if err := loadFeatureKeys(c, filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md")); err != nil {
		// non-fatal: file may not exist yet
		_ = err
	}

	// (b) Parse SS and CP docs
	if err := loadDocRefs(c, repoDir); err != nil {
		_ = err
	}

	// (c) Ledger feature keys
	for _, key := range ledger.ListFeatures() {
		if _, exists := c.Get(key); !exists {
			c.Add(Feature{Key: key})
		}
	}

	// (d) Derive historical file-glob hints from the committed paths for each
	// feature key. This powers path-anchored resolution and suggestion scoring.
	if entriesLister, ok := ledger.(interface{ AllEntries() []changeledger.Entry }); ok {
		inferFileGlobs(c, repoDir, entriesLister.AllEntries())
	}

	// (e) Persist
	if err := writeCatalog(c, dotFlowpilotDir); err != nil {
		return c, err
	}
	return c, nil
}

func inferFileGlobs(c *Catalog, repoDir string, entries []changeledger.Entry) {
	if repoDir == "" || len(entries) == 0 {
		return
	}

	byFeature := make(map[string]map[string]struct{})
	for _, entry := range entries {
		if entry.FeatureKey == "" || entry.CommitHash == "" {
			continue
		}
		files := changedFilesForCommit(repoDir, entry.CommitHash)
		if len(files) == 0 {
			continue
		}
		featureFiles := byFeature[entry.FeatureKey]
		if featureFiles == nil {
			featureFiles = make(map[string]struct{})
			byFeature[entry.FeatureKey] = featureFiles
		}
		for _, file := range files {
			glob := filePathToGlob(file)
			if glob != "" {
				featureFiles[glob] = struct{}{}
			}
		}
	}

	for key, globs := range byFeature {
		feat, ok := c.Get(key)
		if !ok {
			feat = Feature{Key: key}
		}
		for glob := range globs {
			feat.FileGlobs = append(feat.FileGlobs, glob)
		}
		sort.Strings(feat.FileGlobs)
		c.Add(feat)
	}
}

func changedFilesForCommit(repoDir, commitHash string) []string {
	out, err := exec.Command(
		"git", "-C", repoDir,
		"show", "--name-only", "--no-commit-id", "--format=", commitHash,
	).Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	var files []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		file := strings.TrimSpace(sc.Text())
		if file != "" {
			files = append(files, filepath.ToSlash(file))
		}
	}
	return files
}

func filePathToGlob(file string) string {
	file = filepath.ToSlash(strings.TrimSpace(file))
	if file == "" {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(file))
	if dir == "." {
		return file
	}
	return dir + "/**"
}

func supersededSuccessor(line string) string {
	if m := reSupersededBy.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

func loadFeatureKeys(c *Catalog, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		// expect: "- key — description" or "- key - description"
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		line = line[2:]
		var key, description string
		// try em-dash separator first, then plain dash
		if idx := strings.Index(line, " — "); idx >= 0 {
			key = strings.TrimSpace(line[:idx])
			description = strings.TrimSpace(line[idx+len(" — "):])
		} else if idx := strings.Index(line, " - "); idx >= 0 {
			key = strings.TrimSpace(line[:idx])
			description = strings.TrimSpace(line[idx+3:])
		} else {
			key = strings.TrimSpace(line)
		}
		if key == "" {
			continue
		}
		if successor := supersededSuccessor(line); successor != "" {
			key = successor
			description = strings.TrimSpace(description + " canonical successor")
		}
		feat := Feature{
			Key:      key,
			Summary:  description,
			Keywords: tokenize(key + " " + description),
		}
		c.Add(feat)
	}
	return sc.Err()
}

func loadDocRefs(c *Catalog, repoDir string) error {
	patterns := []string{
		filepath.Join(repoDir, "requirements", "05-System-Specs", "SS-*.md"),
		filepath.Join(repoDir, "requirements", "06-System-Tech-Design", "SD-*.md"),
		filepath.Join(repoDir, "requirements", "07-Coding-Plan", "**", "CP-*.md"),
	}

	var files []string
	for _, pat := range patterns {
		matches, err := filepath.Glob(pat)
		if err != nil {
			continue
		}
		files = append(files, matches...)
	}

	// filepath.Glob does not support **, so walk for docs under nested folders.
	// SD docs (BUG-280) and CP docs can live in todo/inprogress/done subfolders.
	_ = walkGlob(filepath.Join(repoDir, "requirements", "06-System-Tech-Design"), "SD-*.md", &files)
	cpDir := filepath.Join(repoDir, "requirements", "07-Coding-Plan")
	_ = walkGlob(cpDir, "CP-*.md", &files)

	// deduplicate
	seen := make(map[string]struct{})
	var unique []string
	for _, f := range files {
		if _, ok := seen[f]; !ok {
			seen[f] = struct{}{}
			unique = append(unique, f)
		}
	}

	for _, path := range unique {
		title, summary, keywords, featureKeys := parseDocFile(path)
		stem := docStem(path)
		if stem == "" {
			continue
		}

		// BUG-280: the primary linkage — a governing doc declares which real
		// feature_key(s) it governs via a "Feature Keys:" metadata line, and
		// its stem is attached to each of those features' DocRefs. Real feature
		// keys were loaded from FEATURE-KEYS.md in Build step (a), before this
		// runs, so they resolve here. This is what makes a feature spec-backed
		// (before this, DocRefs only ever matched doc-stem-named phantom
		// features and real features were left spec_less forever).
		for _, fk := range featureKeys {
			if feat, ok := c.Get(fk); ok {
				if !containsStr(feat.DocRefs, stem) {
					feat.DocRefs = append(feat.DocRefs, stem)
				}
				feat.Keywords = mergeKeywords(feat.Keywords, keywords)
				c.Add(feat)
			}
		}

		// Legacy fallback (Task-097): attach to a feature whose key equals the
		// doc stem, else create a doc-stem-named entry. Kept for backward compat
		// with resolver behavior; harmless alongside the feature-key linkage.
		existing, exists := c.Get(stem)
		if exists {
			existing.Title = title
			if existing.Summary == "" {
				existing.Summary = summary
			}
			existing.Keywords = mergeKeywords(existing.Keywords, keywords)
			if !containsStr(existing.DocRefs, stem) {
				existing.DocRefs = append(existing.DocRefs, stem)
			}
			c.Add(existing)
		} else {
			feat := Feature{
				Key:      stem,
				Title:    title,
				Summary:  summary,
				Keywords: keywords,
				DocRefs:  []string{stem},
			}
			c.Add(feat)
		}
	}
	return nil
}

func walkGlob(dir, pattern string, out *[]string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil {
			return nil
		}
		if matched {
			*out = append(*out, path)
		}
		return nil
	})
}

// featureKeysLineRe matches a governing doc's metadata line declaring which
// real feature_key(s) it governs, e.g. "- Feature Keys: change-contract" or
// "Feature Keys: `change-contract`, `other`" (BUG-280). Case-insensitive on
// the label; singular "Feature Key" also accepted.
var featureKeysLineRe = regexp.MustCompile("(?i)^-?\\s*feature\\s*keys?\\s*:\\s*(.+)$")

// parseFeatureKeysLine returns the declared feature keys on a metadata line, or
// nil when the line is not a Feature Keys declaration. Backticks are stripped.
func parseFeatureKeysLine(trimmed string) []string {
	m := featureKeysLineRe.FindStringSubmatch(trimmed)
	if m == nil {
		return nil
	}
	raw := strings.ReplaceAll(m[1], "`", "")
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if key := strings.TrimSpace(part); key != "" {
			out = append(out, key)
		}
	}
	return out
}

func parseDocFile(path string) (title, summary string, keywords, featureKeys []string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	inSummary := false
	summaryLines := 0

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		if title == "" && strings.HasPrefix(trimmed, "# ") {
			title = strings.TrimSpace(trimmed[2:])
			keywords = append(keywords, tokenize(title)...)
			continue
		}

		// BUG-280: a governing doc may declare which real feature_key(s) it
		// governs, so its stem can be attached to that feature's DocRefs.
		if fk := parseFeatureKeysLine(trimmed); fk != nil {
			featureKeys = append(featureKeys, fk...)
			continue
		}

		if trimmed == "### Summary" {
			inSummary = true
			summaryLines = 0
			continue
		}

		if inSummary {
			if summaryLines >= 3 {
				inSummary = false
				continue
			}
			if strings.HasPrefix(trimmed, "- ") {
				if summary != "" {
					summary += " "
				}
				summary += strings.TrimSpace(trimmed[2:])
				keywords = append(keywords, tokenize(trimmed[2:])...)
				summaryLines++
			} else if trimmed == "" && summaryLines > 0 {
				inSummary = false
			}
		}
	}
	return
}

func docStem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func writeCatalog(c *Catalog, dotFlowpilotDir string) error {
	dir := filepath.Join(dotFlowpilotDir, "catalog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "features.ndjson")
	fh, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()

	for _, feat := range c.All() {
		line, err := json.Marshal(feat)
		if err != nil {
			continue
		}
		if _, err := fh.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// LoadCatalog reads dotFlowpilotDir/catalog/features.ndjson and returns a Catalog.
func LoadCatalog(dotFlowpilotDir string) (*Catalog, error) {
	path := filepath.Join(dotFlowpilotDir, "catalog", "features.ndjson")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := New()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var feat Feature
		if json.Unmarshal(line, &feat) == nil && feat.Key != "" {
			c.Add(feat)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return c, nil
}
