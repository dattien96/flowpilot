package knowledge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Layout filenames inside .flowpilot/knowledge/.
const (
	fileOverview = "system-overview.md"
	fileFlows    = "execution-flows.md"
	fileModels   = "data-models.md"
	fileIndex    = "index.json"
	dirShards    = "flows"
	fileManifest = "flows/manifest.md"
)

// KnowledgeDir returns the living-knowledge root for a workspace.
func KnowledgeDir(repoDir string) string {
	return filepath.Join(repoDir, ".flowpilot", "knowledge")
}

// Missing reports whether the knowledge base was never bootstrapped
// (index.json absent). P-2 Fetch and the P-3 audit hook treat missing as a
// normal "not indexed yet" state and degrade silently.
func Missing(repoDir string) bool {
	if strings.TrimSpace(repoDir) == "" {
		return true
	}
	_, err := os.Stat(filepath.Join(KnowledgeDir(repoDir), fileIndex))
	return err != nil
}

// LoadIndex reads the lookup plane. A corrupt/mismatched index is an error —
// IncrementalUpdate treats it as "rebuild from scratch", never as fatal.
func LoadIndex(repoDir string) (*KnowledgeIndex, error) {
	data, err := os.ReadFile(filepath.Join(KnowledgeDir(repoDir), fileIndex))
	if err != nil {
		return nil, err
	}
	var idx KnowledgeIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// WriteFull persists the whole base atomically (temp file + rename per file)
// and cleans the stale layout: previously-sharded leftovers when collapsing
// to single-file, or the single file when sharding.
func WriteFull(repoDir string, kb *KnowledgeBase) error {
	dir := KnowledgeDir(repoDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	sharded := isSharded(kb)
	if sharded {
		if err := os.MkdirAll(filepath.Join(dir, dirShards), 0o755); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(dir, fileFlows))
	} else {
		_ = os.RemoveAll(filepath.Join(dir, dirShards))
	}
	files := map[string]string{
		fileOverview: kb.Overview,
		fileModels:   kb.Models,
		fileIndex:    mustMarshalIndex(kb.Index),
	}
	if sharded {
		byFile := make(map[string][]RenderedFlow)
		for _, r := range kb.Flows {
			byFile[r.File] = append(byFile[r.File], r)
		}
		names := make([]string, 0, len(byFile))
		for name := range byFile {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			files[name] = joinSections(byFile[name])
		}
		files[fileManifest] = renderManifest(byFile)
	} else {
		files[fileFlows] = joinSections(kb.Flows)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := writeAtomic(filepath.Join(dir, name), files[name]); err != nil {
			return err
		}
	}
	return nil
}

// IncrementalUpdate re-distills only the flows affected by changedPaths and
// merges them back, leaving every other section byte-identical. Files whose
// bytes do not change are not rewritten (plus index.json when affected).
//
// changedPaths are repo-relative slash paths (anything else is normalized).
// redistill supplies the fresh base; it is NOT called when no indexed flow
// matches (fast path) or when the base is missing/corrupt — that case falls
// back to a full WriteFull rebuild. Errors from redistill propagate; callers
// (background workers) must log, never fail a flow.
func IncrementalUpdate(repoDir string, changedPaths []string, redistill func(ctx context.Context) (*KnowledgeBase, error)) error {
	idx, err := LoadIndex(repoDir)
	if err != nil || idx.SchemaVersion != SchemaVersion {
		return fullRebuild(context.Background(), repoDir, redistill)
	}
	affected := affectedFlows(idx, normalizePaths(changedPaths))
	if len(affected) == 0 {
		return nil
	}
	fresh, err := redistill(context.Background())
	if err != nil {
		return err
	}
	freshByID := make(map[string]RenderedFlow, len(fresh.Flows))
	for _, r := range fresh.Flows {
		freshByID[r.ID] = r
	}
	dir := KnowledgeDir(repoDir)
	// Group affected flows by their on-disk file (per the CURRENT index).
	byFile := make(map[string][]string)
	for id := range affected {
		entry, ok := idx.Flows[id]
		if !ok {
			continue
		}
		byFile[entry.File] = append(byFile[entry.File], id)
	}
	changedFiles := make(map[string]bool)
	for name, ids := range byFile {
		cur, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue // shard vanished mid-flight; full rebuild next round
		}
		want := make(map[string]string, len(ids))
		for _, id := range ids {
			if r, ok := freshByID[id]; ok && r.Body != "" {
				// Section bodies exclude the heading line (the swapper
				// owns headings); strip it for an exact body-for-body swap.
				want[id] = stripSectionHeading(r.Body)
			}
		}
		if len(want) == 0 {
			continue
		}
		merged, changed := replaceFlowSections(string(cur), want)
		if !changed {
			continue
		}
		if err := writeAtomic(filepath.Join(dir, name), merged); err != nil {
			return err
		}
		changedFiles[name] = true
	}
	// New flows (in fresh, absent from the current index) append to their
	// target file; dropped flows keep their old sections (conservative: the
	// distiller is additive-biased, never deletes distilled knowledge here).
	for _, r := range fresh.Flows {
		if _, ok := idx.Flows[r.ID]; ok || r.Body == "" {
			continue
		}
		path := filepath.Join(dir, r.File)
		cur, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		merged := strings.TrimRight(string(cur), "\n") + "\n\n" + strings.TrimRight(r.Body, "\n") + "\n"
		if err := writeAtomic(path, merged); err != nil {
			return err
		}
		changedFiles[r.File] = true
	}
	if len(changedFiles) == 0 {
		return nil
	}
	return writeAtomic(filepath.Join(dir, fileIndex), mustMarshalIndex(fresh.Index))
}

// affectedFlows maps changed paths to indexed flow ids via the reverse map.
// Symbol-only changes always ride along a file change, so paths suffice.
func affectedFlows(idx *KnowledgeIndex, paths []string) map[string]bool {
	out := make(map[string]bool)
	for _, p := range paths {
		for _, id := range idx.Paths[p] {
			out[id] = true
		}
	}
	return out
}

// replaceFlowSections swaps the `## Flow: <id>` sections named in want,
// returning the merged doc and whether any bytes changed. Unknown ids in
// want are ignored (their file placement is the writer's job, not the
// section swapper's).
func replaceFlowSections(content string, want map[string]string) (string, bool) {
	parts := splitFlowSections(content)
	changed := false
	for i, p := range parts {
		if p.flowID == "" {
			continue
		}
		if body, ok := want[p.flowID]; ok && p.body != body {
			parts[i].body = body
			changed = true
		}
	}
	if !changed {
		return content, false
	}
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p.head + p.body)
	}
	return b.String(), true
}

// flowSection is one `## Flow: <id>` block (head = preamble before the first
// section, flowID empty) split so untouched blocks round-trip byte-identical.
type flowSection struct {
	head   string
	flowID string
	body   string
}

func splitFlowSections(content string) []flowSection {
	var out []flowSection
	rest := content
	// Preamble: everything before the first "## Flow: ".
	if i := strings.Index(rest, "## Flow: "); i >= 0 {
		if i > 0 {
			out = append(out, flowSection{head: rest[:i]})
		}
		rest = rest[i:]
	} else {
		return []flowSection{{head: content}}
	}
	for rest != "" {
		// rest starts with "## Flow: <id>\n..."
		lineEnd := strings.Index(rest, "\n")
		var headLine, after string
		if lineEnd < 0 {
			headLine, after = rest, ""
		} else {
			headLine, after = rest[:lineEnd+1], rest[lineEnd+1:]
		}
		id := strings.TrimSpace(strings.TrimPrefix(headLine[:len(headLine)-1], "## Flow: "))
		next := strings.Index(after, "\n## Flow: ")
		var body string
		if next < 0 {
			body = after
			rest = ""
		} else {
			body = after[:next+1]
			rest = after[next+1:]
		}
		out = append(out, flowSection{head: headLine, flowID: id, body: body})
	}
	return out
}

// ExtractFlowSection returns the `## Flow: <id>` section starting at the
// heading line through (not including) the next flow heading, or "" when the
// heading is absent. Exported for the P-2 knowledge.flow source, which reads
// sections straight from disk (single implementation — no runner duplicate).
func ExtractFlowSection(doc, heading string) string {
	start := strings.Index(doc, heading)
	if start < 0 {
		return ""
	}
	rest := doc[start:]
	next := strings.Index(rest[len(heading):], "\n## Flow: ")
	if next < 0 {
		return rest
	}
	return rest[:len(heading)+next]
}

// stripSectionHeading drops the "## Flow: <id>" first line so a fresh
// section swaps body-for-body against the split parts.
func stripSectionHeading(section string) string {
	if i := strings.Index(section, "\n"); i >= 0 {
		return section[i+1:]
	}
	return ""
}

func fullRebuild(ctx context.Context, repoDir string, redistill func(ctx context.Context) (*KnowledgeBase, error)) error {
	fresh, err := redistill(ctx)
	if err != nil {
		return err
	}
	return WriteFull(repoDir, fresh)
}

func isSharded(kb *KnowledgeBase) bool {
	for _, r := range kb.Flows {
		if strings.HasPrefix(r.File, dirShards+"/") {
			return true
		}
	}
	return false
}

func joinSections(rs []RenderedFlow) string {
	var b strings.Builder
	for i, r := range rs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimRight(r.Body, "\n") + "\n")
	}
	return b.String()
}

func renderManifest(byFile map[string][]RenderedFlow) string {
	names := make([]string, 0, len(byFile))
	for name := range byFile {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("# Flow Shards (Q-1)\n\n")
	b.WriteString("_execution-flows.md exceeded 500 flows; sections live in per-domain shards. index.json remains the only lookup plane._\n\n")
	for _, name := range names {
		ids := make([]string, 0, len(byFile[name]))
		for _, r := range byFile[name] {
			ids = append(ids, r.ID)
		}
		sort.Strings(ids)
		b.WriteString("## " + name + " (" + itoa(len(ids)) + " flows)\n\n")
		for _, id := range ids {
			b.WriteString("- " + id + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func mustMarshalIndex(idx KnowledgeIndex) string {
	if idx.Flows == nil {
		idx.Flows = make(map[string]FlowIndexEntry)
	}
	if idx.Paths == nil {
		idx.Paths = make(map[string][]string)
	}
	if idx.Symbols == nil {
		idx.Symbols = make(map[string][]string)
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return `{"schemaVersion":` + itoa(SchemaVersion) + `,"flows":{},"paths":{},"symbols":{}}`
	}
	return string(data) + "\n"
}

// writeAtomic writes via temp-file + rename so readers never see a torn
// index.json pointing at sections that do not exist yet (T-3).
func writeAtomic(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func normalizePaths(paths []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, p := range paths {
		p = filepath.ToSlash(strings.TrimSpace(p))
		p = strings.TrimPrefix(p, "./")
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
