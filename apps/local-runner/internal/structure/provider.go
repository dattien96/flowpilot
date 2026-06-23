package structure

import (
	"context"
)

// Provider can answer "what depends on <target>?" questions.
// Implementations differ in fidelity: gitNexusProvider uses the call graph index;
// fallbackProvider uses git history and file imports as a coarse substitute.
type Provider interface {
	Available() bool
	Dependents(ctx context.Context, target string) (DependentsSummary, error)
}

// DependentsSummary is the result of a dependents query.
// Complete is false when dynamic dispatch was detected (interface{} / dynamic calls),
// meaning the list of Nearest may be incomplete.
type DependentsSummary struct {
	Count    int      `json:"count"`
	Nearest  []string `json:"nearest"`
	Flows    []string `json:"flows"`
	Complete bool     `json:"complete"`
}

// New returns the best available Provider for the given repository directory.
// Pass isGitNexusOK=true only when the caller has already verified that
// "npx gitnexus" is runnable (e.g. via the tooling package check).
func New(repoDir string, isGitNexusOK bool) Provider {
	if isGitNexusOK {
		return &gitNexusProvider{repoDir: repoDir}
	}
	return &fallbackProvider{repoDir: repoDir}
}
