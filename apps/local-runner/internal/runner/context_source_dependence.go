package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/structure"
	"flowpilot-runner/internal/tooling"
)

// ContextSourceDependence — Task-259 (CP-43 P-6). Blast-radius of declared
// change scope from GitNexus impact. Default set, priority 3 (after
// change.contract, before source.excerpt via SourceType tiebreak).
const ContextSourceDependence ContextSourceID = "source.dependence"

const (
	dependenceMaxTargets    = 10
	dependenceMaxDependents = 15
	dependenceTotalBudget   = 25 * time.Second
)

type dependenceSource struct {
	priority    int
	newProvider func(cwd string, hasGitNexus bool) structure.Provider
}

func (s *dependenceSource) ID() string          { return string(ContextSourceDependence) }
func (s *dependenceSource) Priority() int       { return s.priority }
func (s *dependenceSource) Deterministic() bool { return true }

func (s *dependenceSource) Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	if strings.TrimSpace(hints.WorkflowRunID) == "" || hints.Workspace == "" {
		return section, nil
	}

	c, ok := latestContractForRun(hints.Workspace, hints.WorkflowRunID)
	if !ok {
		return section, nil
	}

	targets := dependenceTargets(c)
	if len(targets) == 0 {
		return section, nil
	}

	dotFP := filepath.Join(hints.Workspace, ".flowpilot")
	statuses, _ := tooling.LoadToolingStatus(dotFP)
	hasGitNexus := tooling.StatusOf(statuses, "gitnexus").Status == "ok"

	provider := s.provider(hints.Workspace, hasGitNexus)
	if !provider.Available() {
		section.SourceRef = "gitnexus:impact"
		section.Body = "GitNexus not indexed. Run: npx gitnexus analyze"
		return section, nil
	}

	bctx, cancel := context.WithTimeout(ctx, dependenceTotalBudget)
	defer cancel()

	body := renderDependenceBody(bctx, provider, targets)
	if strings.TrimSpace(body) == "" {
		return section, nil
	}
	section.SourceRef = filepath.ToSlash(filepath.Join(dotFP, "contracts", "contracts.ndjson"))
	section.Body = body
	return section, nil
}

func (s *dependenceSource) provider(cwd string, hasGitNexus bool) structure.Provider {
	if s.newProvider != nil {
		return s.newProvider(cwd, hasGitNexus)
	}
	return structure.New(cwd, hasGitNexus)
}

func dependenceTargets(c changecontract.Contract) []string {
	out := changecontract.GitNexusImpactTargets(c)
	if len(out) > dependenceMaxTargets {
		out = out[:dependenceMaxTargets]
	}
	return out
}

func renderDependenceBody(ctx context.Context, provider structure.Provider, targets []string) string {
	var b strings.Builder
	any, incomplete := false, false
	for _, t := range targets {
		summary, err := provider.Dependents(ctx, t)
		if err != nil {
			continue
		}
		nearest := append([]string(nil), summary.Nearest...)
		sort.Strings(nearest)
		if len(nearest) > dependenceMaxDependents {
			nearest = nearest[:dependenceMaxDependents]
		}
		if summary.Count == 0 && len(nearest) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(&b, "- `%s` → %d:\n", t, summary.Count)
		for _, d := range nearest {
			fmt.Fprintf(&b, "  - %s\n", d)
		}
		if len(summary.Flows) > 0 {
			flows := append([]string(nil), summary.Flows...)
			sort.Strings(flows)
			fmt.Fprintf(&b, "  - flows: %s\n", strings.Join(flows, ", "))
		}
		if !summary.Complete {
			incomplete = true
		}
	}
	if !any {
		return ""
	}
	out := "## Dependence\n\n" + b.String()
	if incomplete {
		out += "\n_Note: partial caller list (dynamic dispatch)._"
	}
	return strings.TrimSpace(out)
}
