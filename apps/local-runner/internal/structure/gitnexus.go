package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"
	"os/exec"
)

type gitNexusProvider struct {
	repoDir string
}

func (g *gitNexusProvider) Available() bool {
	return true
}

// Dependents runs "npx gitnexus impact <target> --json" and parses the output.
// All errors are non-fatal: on failure it returns an incomplete summary with Complete=false.
func (g *gitNexusProvider) Dependents(ctx context.Context, target string) (DependentsSummary, error) {
	ctx = ensureDeadline(ctx, 30*time.Second)

	cmd := exec.CommandContext(ctx, "npx", "gitnexus", "impact", target, "--json")
	cmd.Dir = g.repoDir

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &bytes.Buffer{}

	if err := cmd.Run(); err != nil {
		return DependentsSummary{Complete: false}, nil
	}

	output := buf.String()
	summary := parseGitNexusOutput(output)
	return summary, nil
}

// parseGitNexusOutput attempts JSON first, then falls back to text parsing.
func parseGitNexusOutput(output string) DependentsSummary {
	var summary DependentsSummary

	if parsed, ok := tryParseJSON(output); ok {
		summary = parsed
	} else {
		summary = tryParseText(output)
	}

	if len(summary.Nearest) > 10 {
		summary.Nearest = summary.Nearest[:10]
	}

	summary.Complete = !strings.Contains(output, "dynamic") &&
		!strings.Contains(output, "interface{}")
	summary.Count = len(summary.Nearest)

	return summary
}

// tryParseJSON attempts to decode gitnexus JSON output.
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

// ensureDeadline returns ctx if it already has a deadline, otherwise wraps it
// with the given timeout.
func ensureDeadline(ctx context.Context, timeout time.Duration) context.Context {
	if _, ok := ctx.Deadline(); ok {
		return ctx
	}
	newCtx, _ := context.WithTimeout(ctx, timeout) //nolint:govet
	return newCtx
}
