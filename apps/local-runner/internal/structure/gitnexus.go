package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type gitNexusProvider struct {
	repoDir string
}

func (g *gitNexusProvider) Available() bool {
	return true
}

// Dependents runs "npx gitnexus impact <target> --repo <repo>" and parses CLI JSON.
// GitNexus resolves symbols, not file paths. CLI errors (missing target, bad flags)
// return a non-nil error so callers can distinguish failure from a legitimate zero
// blast radius; callers must remain non-fatal (AC-9).
func (g *gitNexusProvider) Dependents(ctx context.Context, target string) (DependentsSummary, error) {
	ctx, cancel := ensureDeadline(ctx, 30*time.Second)
	defer cancel()

	repo := repoNameFromDir(g.repoDir)
	args := []string{"gitnexus", "impact", target, "--repo", repo}
	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Dir = g.repoDir

	var buf bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := gitNexusResponseError(buf.String()); msg != "" {
			return DependentsSummary{}, fmt.Errorf("gitnexus impact: %s", msg)
		}
		if stderr.Len() > 0 {
			return DependentsSummary{}, fmt.Errorf("gitnexus impact: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return DependentsSummary{}, fmt.Errorf("gitnexus impact: %w", err)
	}

	output := buf.String()
	if msg := gitNexusResponseError(output); msg != "" {
		return DependentsSummary{}, fmt.Errorf("gitnexus impact: %s", msg)
	}

	summary := parseGitNexusOutput(output)
	return summary, nil
}

func repoNameFromDir(repoDir string) string {
	clean := filepath.Clean(strings.TrimSpace(repoDir))
	if clean == "" || clean == "." {
		return "."
	}
	return filepath.Base(clean)
}

// gitNexusResponseError extracts a top-level {"error":"..."} payload when present.
func gitNexusResponseError(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	var probe struct {
		Error string `json:"error"`
	}
	dec := json.NewDecoder(strings.NewReader(output))
	if err := dec.Decode(&probe); err != nil {
		return ""
	}
	return strings.TrimSpace(probe.Error)
}

// parseGitNexusOutput attempts the real GitNexus CLI schema first, then legacy JSON,
// then plain-text fallback.
func parseGitNexusOutput(output string) DependentsSummary {
	if summary, ok := tryParseGitNexusImpactV2(output); ok {
		return finalizeDependentsSummary(summary, output)
	}

	if parsed, ok := tryParseJSON(output); ok {
		summary := parsed
		return finalizeDependentsSummary(summary, output)
	}

	summary := tryParseText(output)
	return finalizeDependentsSummary(summary, output)
}

func finalizeDependentsSummary(summary DependentsSummary, rawOutput string) DependentsSummary {
	if len(summary.Nearest) > 10 {
		summary.Nearest = summary.Nearest[:10]
	}
	if summary.Count == 0 && len(summary.Nearest) > 0 {
		summary.Count = len(summary.Nearest)
	}
	summary.Complete = !strings.Contains(rawOutput, "dynamic") &&
		!strings.Contains(rawOutput, "interface{}")
	return summary
}

type gitNexusImpactEntry struct {
	Name string `json:"name"`
}

type gitNexusImpactV2 struct {
	ImpactedCount     *int                             `json:"impactedCount"`
	AffectedProcesses []gitNexusImpactEntry            `json:"affected_processes"`
	AffectedModules   []gitNexusImpactEntry            `json:"affected_modules"`
	ByDepth           map[string][]gitNexusImpactEntry `json:"byDepth"`
}

func tryParseGitNexusImpactV2(output string) (DependentsSummary, bool) {
	var raw gitNexusImpactV2
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(output)))
	if err := dec.Decode(&raw); err != nil {
		return DependentsSummary{}, false
	}

	isV2 := raw.ImpactedCount != nil ||
		len(raw.AffectedProcesses) > 0 ||
		len(raw.AffectedModules) > 0 ||
		len(raw.ByDepth) > 0
	if !isV2 {
		return DependentsSummary{}, false
	}

	flows := make([]string, 0, len(raw.AffectedProcesses))
	seenFlow := make(map[string]struct{}, len(raw.AffectedProcesses))
	for _, p := range raw.AffectedProcesses {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		if _, ok := seenFlow[name]; ok {
			continue
		}
		seenFlow[name] = struct{}{}
		flows = append(flows, name)
	}
	sort.Strings(flows)

	nearest := collectGitNexusImpactNames(raw)
	count := 0
	if raw.ImpactedCount != nil {
		count = *raw.ImpactedCount
	}
	if count == 0 {
		count = len(nearest)
	}

	return DependentsSummary{
		Count:    count,
		Nearest:  nearest,
		Flows:    flows,
		Complete: true,
	}, true
}

func collectGitNexusImpactNames(raw gitNexusImpactV2) []string {
	seen := make(map[string]struct{})
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
	}

	for _, p := range raw.AffectedProcesses {
		add(p.Name)
	}
	for _, m := range raw.AffectedModules {
		add(m.Name)
	}

	depthKeys := make([]string, 0, len(raw.ByDepth))
	for k := range raw.ByDepth {
		depthKeys = append(depthKeys, k)
	}
	sort.Strings(depthKeys)
	for _, k := range depthKeys {
		for _, entry := range raw.ByDepth[k] {
			add(entry.Name)
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// tryParseJSON attempts to decode legacy gitnexus JSON output.
// The schema uses "dependents", "nearest", and "flows" top-level fields.
func tryParseJSON(output string) (DependentsSummary, bool) {
	var raw struct {
		Dependents []string `json:"dependents"`
		Nearest    []string `json:"nearest"`
		Flows      []string `json:"flows"`
	}

	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(output)))
	if err := dec.Decode(&raw); err != nil {
		return DependentsSummary{}, false
	}

	nearest := raw.Nearest
	if len(nearest) == 0 {
		nearest = raw.Dependents
	}

	// Distinguish legacy shape from unrelated JSON objects (e.g. CLI error handled elsewhere).
	if len(nearest) == 0 && len(raw.Flows) == 0 && len(raw.Dependents) == 0 {
		return DependentsSummary{}, false
	}

	return DependentsSummary{
		Nearest: nearest,
		Flows:   raw.Flows,
	}, true
}

// tryParseText extracts symbol names from plain-text gitnexus output.
// Lines containing "→" or "depends on" are treated as dependency references.
func tryParseText(output string) DependentsSummary {
	var nearest []string
	seen := make(map[string]bool)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "→") || strings.Contains(line, "depends on") {
			parts := strings.FieldsFunc(line, func(r rune) bool {
				return r == '→' || r == ' ' || r == '\t'
			})
			for _, p := range parts {
				p = strings.Trim(p, `"',`)
				if p != "" && !seen[p] && p != "depends" && p != "on" {
					seen[p] = true
					nearest = append(nearest, p)
				}
			}
		}
	}

	return DependentsSummary{Nearest: nearest}
}

// ensureDeadline returns ctx and a cancel func if it already has a deadline,
// otherwise wraps it with the given timeout.
func ensureDeadline(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
