package runner

import (
	"context"
	"fmt"
	"strings"
)

// Task-231 (CP-05-04 P-3/P-4): firebase.crashlytics mirrors jira.issue's
// shape (context_source_jira.go) — opt-in, excluded from the live AI-turn
// collect path in favor of a runtime-target prompt note.
//
// Unlike jira.issue/jira.sprint, this source's production adapter is
// deliberately left UNWIRED (no SetFirebaseCrashlyticsAdapter call in
// AttachRunner). Jira already had a working REST integration to reuse
// (CP-05-01/02); Firebase has none, and CP-05-04 P-1/Q-1 already resolved
// the sole intended access path as the official firebase-tools MCP
// (`crashlytics_get_issue`/`crashlytics_list_events`) called by the AI
// itself during its turn — hand-rolling an unverified Crashlytics
// Management API v1alpha REST client (with its own OAuth/JWT exchange from
// the service-account credential) would be fabricated, untested surface
// area this task does not need: the registry/validation/runtime-question/
// prompt-note plumbing below is the real deliverable, and it degrades
// gracefully (FlowContextSection error → Collect warning, never a hard
// failure) via Fetch's existing "no adapter configured" branch, exactly the
// same contract mcpDriverSource/jiraIssueSource already use.
const ContextSourceFirebaseCrashlytics ContextSourceID = "firebase.crashlytics"

// FirebaseCrashlyticsAdapter fetches a single Crashlytics issue's bounded
// content by crash issue id. Mirrors JiraIssueAdapter's shape
// (context_source_jira.go). No production implementation is wired in this
// task (see package doc above) — a future task may add one once a
// deterministic, testable Crashlytics access path exists outside the
// AI-turn's own MCP tool calls.
type FirebaseCrashlyticsAdapter interface {
	Fetch(ctx context.Context, crashRef string) (content string, err error)
}

// firebaseCrashlyticsSource reads hints.FirebaseCrashRef. Opt-in, mirrors
// jiraIssueSource exactly.
type firebaseCrashlyticsSource struct {
	priority int
	adapter  FirebaseCrashlyticsAdapter
}

func (s *firebaseCrashlyticsSource) ID() string          { return string(ContextSourceFirebaseCrashlytics) }
func (s *firebaseCrashlyticsSource) Priority() int       { return s.priority }
func (s *firebaseCrashlyticsSource) Deterministic() bool { return true }

func (s *firebaseCrashlyticsSource) Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	crashRef := strings.TrimSpace(hints.FirebaseCrashRef)
	if crashRef == "" {
		return section, nil
	}
	if s.adapter == nil {
		return section, fmt.Errorf("firebase.crashlytics: no adapter configured")
	}
	section.SourceRef = "firebase:" + crashRef
	content, err := mcpBoundedFetch(ctx, 0, mcpDriverTimeout, mcpDriverContentCap, s.ID(), crashRef, s.adapter.Fetch)
	if err != nil {
		return section, err
	}
	section.Body = content
	return section, nil
}

// SetFirebaseCrashlyticsAdapter wires a production FirebaseCrashlyticsAdapter
// onto the already-registered firebase.crashlytics source, mirroring
// SetJiraIssueAdapter. Not currently called from AttachRunner (see package
// doc above) — available for a future task to wire once a deterministic
// backing exists.
func (r *ContextSourceRegistry) SetFirebaseCrashlyticsAdapter(adapter FirebaseCrashlyticsAdapter) {
	src, err := r.Resolve(string(ContextSourceFirebaseCrashlytics))
	if err != nil {
		return
	}
	if s, ok := src.(*firebaseCrashlyticsSource); ok {
		s.adapter = adapter
	}
}
