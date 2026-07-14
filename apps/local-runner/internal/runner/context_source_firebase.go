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
// Its production adapter (firebaseToolsMcpAdapter, firebase_tools_mcp_client.go,
// wired in AttachRunner) is a real MCP CLIENT speaking to a spawned
// `firebase-tools mcp --only crashlytics` process — the official access path
// CP-05-04 P-1/Q-1 resolved on, not a hand-rolled Crashlytics REST client
// (whose exact wire contract this codebase has no way to verify without a
// live Firebase project). Fetch still degrades gracefully (FlowContextSection
// error → Collect warning, never a hard failure) on any adapter error, same
// contract mcpDriverSource/jiraIssueSource already use.
const ContextSourceFirebaseCrashlytics ContextSourceID = "firebase.crashlytics"

// FirebaseCrashlyticsAdapter fetches a single Crashlytics issue's bounded
// content by crash issue id. Mirrors JiraIssueAdapter's shape
// (context_source_jira.go).
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
// SetJiraIssueAdapter. Wired in AttachRunner with firebaseToolsMcpAdapter.
func (r *ContextSourceRegistry) SetFirebaseCrashlyticsAdapter(adapter FirebaseCrashlyticsAdapter) {
	src, err := r.Resolve(string(ContextSourceFirebaseCrashlytics))
	if err != nil {
		return
	}
	if s, ok := src.(*firebaseCrashlyticsSource); ok {
		s.adapter = adapter
	}
}
