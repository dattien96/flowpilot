package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FlowSymbol is one code symbol participating in a GitNexus execution flow
// (Process node). The GitNexus symbol UID embeds kind, repo-relative path and
// name as "Kind:path:name" (e.g. "Method:internal/runner/foo.go:Run").
type FlowSymbol struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path"`
	Name string `json:"name"`
}

// FlowSummary is one GitNexus execution flow (Process node) with its member
// symbols, the macro-level unit CP-66 distills into knowledge sections.
type FlowSummary struct {
	ID          string       `json:"id"`
	Label       string       `json:"label"`
	ProcessType string       `json:"processType"`
	StepCount   int          `json:"stepCount"`
	SymbolCount int          `json:"symbolCount"`
	Symbols     []FlowSymbol `json:"symbols"`
}

// ModelInfo is one data-model-shaped symbol (struct/interface/class/...)
// ranked by how many execution flows it participates in.
type ModelInfo struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	FlowCount int    `json:"flowCount"`
}

// DefaultProcessLimit bounds the background distill scan (CP-66 R-1: the
// first scan must stay cheap; Q-1 sharding only matters past 500 flows).
const DefaultProcessLimit = 60

// DefaultModelLimit bounds the data-model candidate scan.
const DefaultModelLimit = 80

// modelKinds are the symbol kinds that count as data models for
// data-models.md. Kept in one place so distiller and tests agree.
var modelKinds = map[string]bool{
	"Struct": true, "Interface": true, "Class": true, "Record": true,
	"Enum": true, "Trait": true, "TypeAlias": true,
}

// Processes lists the top execution flows of the repo at repoDir with their
// member symbols, via `gitnexus cypher` against the local index. limit <= 0
// selects DefaultProcessLimit. Flows sort by step count descending (meatiest
// business flows first).
//
// Purely additive surface (CP-66 Task-373 T-1): gitnexus.go is untouched; this
// file owns the processes read path. A non-nil error means the CLI is missing,
// the repo is not indexed, or the graph query failed — callers must stay
// non-fatal (log + retry later, never fail a flow).
func Processes(ctx context.Context, repoDir string, limit int) ([]FlowSummary, error) {
	if limit <= 0 {
		limit = DefaultProcessLimit
	}
	// Over-fetch, then rank in Go: keeps the Cypher dialect surface to
	// MATCH/RETURN/WHERE/IN only (no ORDER BY portability risk).
	rows, err := runCypher(ctx, repoDir,
		"MATCH (p:Process) RETURN p.id, p.label, p.heuristicLabel, p.processType, p.stepCount LIMIT 1000")
	if err != nil {
		return nil, err
	}
	flows := make([]FlowSummary, 0, len(rows))
	for _, r := range rows {
		id := cell(r, 0)
		if id == "" {
			continue
		}
		label := cell(r, 1)
		if label == "" {
			label = cell(r, 2)
		}
		if label == "" {
			label = id
		}
		flows = append(flows, FlowSummary{
			ID:          id,
			Label:       label,
			ProcessType: cell(r, 3),
			StepCount:   atoi(cell(r, 4)),
		})
	}
	sort.SliceStable(flows, func(i, j int) bool {
		if flows[i].StepCount != flows[j].StepCount {
			return flows[i].StepCount > flows[j].StepCount
		}
		return flows[i].ID < flows[j].ID
	})
	if len(flows) > limit {
		flows = flows[:limit]
	}
	if err := attachFlowSymbols(ctx, repoDir, flows); err != nil {
		return nil, err
	}
	return flows, nil
}

// ModelCandidates returns data-model-shaped symbols ranked by execution-flow
// participation (most-shared entities first). limit <= 0 selects
// DefaultModelLimit. Ranking happens in Go for the same dialect-portability
// reason as Processes.
func ModelCandidates(ctx context.Context, repoDir string, limit int) ([]ModelInfo, error) {
	if limit <= 0 {
		limit = DefaultModelLimit
	}
	rows, err := runCypher(ctx, repoDir,
		"MATCH (s)-->(p:Process) RETURN s.id, count(*)")
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		if id := cell(r, 0); id != "" {
			counts[id] += atoi(cell(r, 1))
		}
	}
	models := make([]ModelInfo, 0, len(counts))
	for id, c := range counts {
		sym := parseFlowSymbol(id)
		if !modelKinds[sym.Kind] {
			continue
		}
		models = append(models, ModelInfo{
			ID: sym.ID, Kind: sym.Kind, Path: sym.Path, Name: sym.Name, FlowCount: c,
		})
	}
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].FlowCount != models[j].FlowCount {
			return models[i].FlowCount > models[j].FlowCount
		}
		return models[i].ID < models[j].ID
	})
	if len(models) > limit {
		models = models[:limit]
	}
	return models, nil
}

// attachFlowSymbols fills Symbols (+ derived SymbolCount) for each flow with
// one batched IN query. Chunked at 100 ids per query to bound query size.
func attachFlowSymbols(ctx context.Context, repoDir string, flows []FlowSummary) error {
	byID := make(map[string]*FlowSummary, len(flows))
	for i := range flows {
		byID[flows[i].ID] = &flows[i]
	}
	for start := 0; start < len(flows); start += 100 {
		end := start + 100
		if end > len(flows) {
			end = len(flows)
		}
		ids := make([]string, 0, end-start)
		for _, f := range flows[start:end] {
			ids = append(ids, cypherQuote(f.ID))
		}
		rows, err := runCypher(ctx, repoDir,
			"MATCH (s)-->(p:Process) WHERE p.id IN ["+strings.Join(ids, ",")+"] RETURN p.id, s.id")
		if err != nil {
			return err
		}
		for _, r := range rows {
			f, ok := byID[cell(r, 0)]
			if !ok {
				continue
			}
			if symID := cell(r, 1); symID != "" {
				f.Symbols = append(f.Symbols, parseFlowSymbol(symID))
			}
		}
	}
	for i := range flows {
		sort.SliceStable(flows[i].Symbols, func(a, b int) bool {
			return flows[i].Symbols[a].ID < flows[i].Symbols[b].ID
		})
		if flows[i].SymbolCount == 0 {
			flows[i].SymbolCount = len(flows[i].Symbols)
		}
	}
	return nil
}

// parseFlowSymbol splits a GitNexus symbol UID "Kind:path:name". Malformed
// UIDs degrade to Name-only symbols rather than being dropped — a distiller
// must never lose a flow member to a parse quirk.
func parseFlowSymbol(uid string) FlowSymbol {
	sym := FlowSymbol{ID: strings.TrimSpace(uid)}
	rest := sym.ID
	if i := strings.Index(rest, ":"); i >= 0 {
		sym.Kind = rest[:i]
		rest = rest[i+1:]
	}
	if i := strings.LastIndex(rest, ":"); i >= 0 {
		sym.Path = strings.TrimSpace(rest[:i])
		sym.Name = strings.TrimSpace(rest[i+1:])
	} else {
		sym.Name = strings.TrimSpace(rest)
	}
	if sym.Name == "" {
		sym.Name = sym.ID
	}
	return sym
}

// runCypher executes `gitnexus cypher <query> --repo <repo>` in repoDir and
// parses the JSON envelope. The CLI answers either {"error": "..."} or
// {"markdown": "<gfm table>", "row_count": n}.
func runCypher(ctx context.Context, repoDir, query string) ([][]string, error) {
	ctx, cancel := ensureDeadline(ctx, 60*time.Second)
	defer cancel()

	repo := repoNameFromDir(repoDir)
	cmd := exec.CommandContext(ctx, "gitnexus", "cypher", query, "--repo", repo)
	cmd.Dir = repoDir

	var buf bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := gitNexusResponseError(buf.String()); msg != "" {
			return nil, fmt.Errorf("gitnexus cypher: %s", msg)
		}
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("gitnexus cypher: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("gitnexus cypher: %w", err)
	}
	return parseCypherTable(buf.String())
}

// parseCypherTable decodes the CLI envelope and extracts data rows from the
// GFM table: first two pipe-lines are header + separator, the rest are data.
func parseCypherTable(output string) ([][]string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}
	var env struct {
		Markdown string `json:"markdown"`
		Error    string `json:"error"`
	}
	dec := json.NewDecoder(strings.NewReader(output))
	if err := dec.Decode(&env); err != nil {
		return nil, fmt.Errorf("gitnexus cypher: cannot decode response: %w", err)
	}
	if msg := strings.TrimSpace(env.Error); msg != "" {
		return nil, fmt.Errorf("gitnexus cypher: %s", msg)
	}
	var rows [][]string
	for _, line := range strings.Split(env.Markdown, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		parts := strings.Split(line, "|")
		cells := make([]string, 0, len(parts))
		for _, p := range parts[1 : len(parts)-1] {
			cells = append(cells, unquoteCell(strings.TrimSpace(p)))
		}
		if len(cells) == 0 || isSeparatorRow(cells) {
			continue
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	// First surviving row is the header (p.id, p.label, ...).
	return rows[1:], nil
}

// isSeparatorRow reports the "| --- | --- |" divider line.
func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		t := strings.Trim(c, ": ")
		if t == "" {
			continue
		}
		for _, r := range t {
			if r != '-' {
				return false
			}
		}
	}
	return true
}

// unquoteCell strips one layer of surrounding single/double quotes some
// engines add around string cells.
func unquoteCell(c string) string {
	if len(c) >= 2 {
		if (c[0] == '\'' && c[len(c)-1] == '\'') || (c[0] == '"' && c[len(c)-1] == '"') {
			return c[1 : len(c)-1]
		}
	}
	return c
}

// cypherQuote single-quotes a process id for an IN list, escaping quote and
// backslash so ids can never break out of the literal.
func cypherQuote(id string) string {
	r := strings.ReplaceAll(id, `\`, `\\`)
	r = strings.ReplaceAll(r, `'`, `\'`)
	return "'" + r + "'"
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
