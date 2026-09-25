package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Task-445 (CP-87 P-1): typed provider-limit events. One normalized taxonomy
// (quota_exhausted | rate_limited | credits_exhausted | billing_required) from
// adapter to UI — structured payloads/stop reasons classify exact, the shared
// string fallback stays only for unstructured CLI/RPC text (heuristic).
// Provider parity: fixture-contract for Claude/Codex (no live accounts), fake
// ACP process for Grok/OpenCode/Devin. Additive-only; no existing test edits.

func task445LimitEvents(evs []ProviderEvent) []ProviderEvent {
	var out []ProviderEvent
	for _, ev := range evs {
		if ev.Type == EventProviderLimitReached {
			out = append(out, ev)
		}
	}
	return out
}

func task445FailedEvents(evs []ProviderEvent) []ProviderEvent {
	var out []ProviderEvent
	for _, ev := range evs {
		if ev.Type == EventTurnFailed {
			out = append(out, ev)
		}
	}
	return out
}

func TestTask445_ClassifierKindsAndSources(t *testing.T) {
	cases := []struct {
		name       string
		provider   ProviderKey
		payload    any
		err        error
		wantKind   ProviderLimitKind
		wantSource string
		wantConf   string
		wantRetry  int64
	}{
		{
			name:     "stop reason rate_limited",
			provider: ProviderKeyOpencode,
			payload:  map[string]any{"stopReason": "rate_limited"},
			wantKind: ProviderLimitRateLimited, wantSource: "stop_reason", wantConf: "exact",
		},
		{
			name:     "stop reason quota",
			provider: ProviderKeyDevin,
			payload:  map[string]any{"stopReason": "quota_exceeded"},
			wantKind: ProviderLimitQuotaExhausted, wantSource: "stop_reason", wantConf: "exact",
		},
		{
			name:     "stop reason billing",
			provider: ProviderKeyDevin,
			payload:  map[string]any{"stopReason": "billing_required"},
			wantKind: ProviderLimitBillingRequired, wantSource: "stop_reason", wantConf: "exact",
		},
		{
			name:     "structured http_status 402",
			provider: ProviderKeyGrok,
			payload:  map[string]any{"error": map[string]any{"data": map[string]any{"http_status": float64(402), "message": "balance exhausted"}}},
			wantKind: ProviderLimitCreditsExhausted, wantSource: "structured_payload", wantConf: "exact",
		},
		{
			name:     "structured http_status 429 with retry_after",
			provider: ProviderKeyCodex,
			payload:  map[string]any{"error": map[string]any{"data": map[string]any{"http_status": float64(429), "retry_after": float64(7)}}},
			wantKind: ProviderLimitRateLimited, wantSource: "structured_payload", wantConf: "exact", wantRetry: 7,
		},
		{
			name:     "stderr fallback quota",
			provider: ProviderKeyClaude,
			err:      errors.New("Claude usage limit reached: quota reset at 5pm. Switch Claude account"),
			wantKind: ProviderLimitQuotaExhausted, wantSource: "stderr_fallback", wantConf: "heuristic",
		},
		{
			name:     "stderr fallback mixed copy prefers the operative cause",
			provider: ProviderKeyClaude,
			err:      errors.New("Claude usage limit reached: extra usage unavailable (out of credits)"),
			wantKind: ProviderLimitCreditsExhausted, wantSource: "stderr_fallback", wantConf: "heuristic",
		},
		{
			name:     "stderr fallback rate limit with retry after",
			provider: ProviderKeyGrok,
			err:      errors.New("session/prompt failed: rate_limited: retry after 9s"),
			wantKind: ProviderLimitRateLimited, wantSource: "stderr_fallback", wantConf: "heuristic", wantRetry: 9,
		},
		{
			name:     "stderr fallback credits",
			provider: ProviderKeyClaude,
			err:      errors.New("out_of_credits"),
			wantKind: ProviderLimitCreditsExhausted, wantSource: "stderr_fallback", wantConf: "heuristic",
		},
		{
			name:     "stderr fallback billing",
			provider: ProviderKeyOpencode,
			err:      errors.New("Internal error: No payment method. Add a payment method here: https://opencode.ai/workspace/x/billing"),
			wantKind: ProviderLimitBillingRequired, wantSource: "stderr_fallback", wantConf: "heuristic",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limit, ok := classifyProviderLimit(tc.provider, tc.payload, tc.err)
			if !ok {
				t.Fatalf("classifyProviderLimit returned no limit")
			}
			if limit.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", limit.Kind, tc.wantKind)
			}
			if limit.ProviderKey != tc.provider {
				t.Errorf("providerKey = %q, want %q", limit.ProviderKey, tc.provider)
			}
			if limit.DetectionSource != tc.wantSource {
				t.Errorf("detectionSource = %q, want %q", limit.DetectionSource, tc.wantSource)
			}
			if limit.Confidence != tc.wantConf {
				t.Errorf("confidence = %q, want %q", limit.Confidence, tc.wantConf)
			}
			if tc.wantRetry > 0 && limit.RetryAfterSeconds != tc.wantRetry {
				t.Errorf("retryAfterSeconds = %d, want %d", limit.RetryAfterSeconds, tc.wantRetry)
			}
			if limit.SanitizedMessage == "" {
				t.Errorf("sanitizedMessage must not be empty")
			}
		})
	}
}

func TestTask445_RateLimitDistinctFromQuota(t *testing.T) {
	// rate_limited with a bounded Retry-After is safely retryable; every other
	// kind (and a rate limit with no/oversized hint) is terminal.
	retryable, ok := classifyProviderLimit(ProviderKeyGrok, nil, errors.New("rate_limited: retry after 2s"))
	if !ok || retryable.Kind != ProviderLimitRateLimited {
		t.Fatalf("want rate_limited classification, got %+v ok=%v", retryable, ok)
	}
	if !providerLimitRecoverable(*retryable) {
		t.Fatalf("rate_limited with retryAfter=2 must be bounded-retryable")
	}
	if !isRecoverableSendError(&providerLimitError{limit: retryable, err: errors.New("rate_limited")}) {
		t.Fatalf("typed rate_limited+retryAfter error must be recoverable")
	}

	noHint, ok := classifyProviderLimit(ProviderKeyGrok, nil, errors.New("429 rate_limited"))
	if !ok || noHint.Kind != ProviderLimitRateLimited {
		t.Fatalf("want rate_limited, got %+v ok=%v", noHint, ok)
	}
	if providerLimitRecoverable(*noHint) {
		t.Fatalf("rate_limited without retry-after must NOT be retried")
	}

	quota, ok := classifyProviderLimit(ProviderKeyOpencode, nil, errors.New("quota_exceeded: model over quota"))
	if !ok || quota.Kind != ProviderLimitQuotaExhausted {
		t.Fatalf("want quota_exhausted, got %+v ok=%v", quota, ok)
	}
	if providerLimitRecoverable(*quota) {
		t.Fatalf("quota_exhausted must never be retried")
	}
	if isRecoverableSendError(&providerLimitError{limit: quota, err: errors.New("quota_exceeded")}) {
		t.Fatalf("typed quota_exhausted error must be non-recoverable")
	}
}

func TestTask445_RetryWaitHonorsBoundedRetryAfter(t *testing.T) {
	var waited []time.Duration
	orig := providerLimitRetryDelayFn
	providerLimitRetryDelayFn = func(ctx context.Context, d time.Duration) error {
		waited = append(waited, d)
		return nil
	}
	defer func() { providerLimitRetryDelayFn = orig }()

	adapter := &task445FlakyAdapter{
		errs: []error{
			&providerLimitError{limit: &ProviderLimit{Kind: ProviderLimitRateLimited, RetryAfterSeconds: 3}, err: errors.New("rate_limited: retry after 3s")},
		},
	}
	svc := &InteractiveService{maxTurnAttempts: 2}
	bridge := &task445Bridge{}
	err := svc.sendTurnWithRetry(context.Background(), adapter, TurnRequest{RunID: "r1"}, bridge)
	if err != nil {
		t.Fatalf("retryable rate limit should succeed on retry, got %v", err)
	}
	if len(waited) != 1 || waited[0] != 3*time.Second {
		t.Fatalf("retry wait = %v, want one bounded 3s wait", waited)
	}
	if adapter.calls != 2 {
		t.Fatalf("SendTurn calls = %d, want 2", adapter.calls)
	}
}

func TestTask445_UnknownShapeAudited(t *testing.T) {
	var audited []string
	orig := unclassifiedProviderFailureFn
	unclassifiedProviderFailureFn = func(provider ProviderKey, err error) {
		audited = append(audited, string(provider)+"|"+err.Error())
	}
	defer func() { unclassifiedProviderFailureFn = orig }()

	// Unknown shapes never classify, never become quota, and the failure is
	// audited as unclassified through providerLimitAwareError.
	err := providerLimitAwareError(ProviderKeyOpencode, errors.New("weird future failure shape"))
	if _, isLimit := err.(*providerLimitError); isLimit {
		t.Fatalf("unknown shape must not classify as a provider limit")
	}
	if len(audited) != 1 || !strings.HasPrefix(audited[0], "opencode|") {
		t.Fatalf("unclassified failure must be audited, got %v", audited)
	}

	if _, ok := classifyProviderLimit(ProviderKeyClaude, nil, errors.New("weird future failure shape")); ok {
		t.Fatalf("unknown shape must not classify")
	}
	if isProviderUsageLimitError(errors.New("weird future failure shape")) {
		t.Fatalf("unknown shape must not trip the legacy classifier")
	}
}

// ---- provider fixtures / fake-ACP end-to-end -------------------------------

func TestTask445_ClaudeFixtures(t *testing.T) {
	// Result frame carrying a usage-limit payload (live-observed copy shape):
	// typed limit event precedes the terminal failure; stable copy preserved.
	evs := mapClaudeResult(map[string]any{
		"type":     "result",
		"subtype":  "error_during_execution",
		"is_error": true,
		"result":   "Your usage limit has been reached — quota reset at 5pm.",
	})
	limits := task445LimitEvents(evs)
	if len(limits) != 1 {
		t.Fatalf("want 1 provider_limit_reached event, got %+v", evs)
	}
	if limits[0].ProviderLimit == nil {
		t.Fatalf("provider_limit_reached must carry ProviderLimit payload")
	}
	l := limits[0].ProviderLimit
	if l.Kind != ProviderLimitQuotaExhausted || l.ProviderKey != ProviderKeyClaude {
		t.Fatalf("limit = %+v, want quota_exhausted/claude", l)
	}
	failed := task445FailedEvents(evs)
	if len(failed) != 1 {
		t.Fatalf("want exactly 1 turn_failed, got %+v", evs)
	}
	if failed[0].Recoverable {
		t.Fatalf("limit turn_failed must not be recoverable")
	}
	// ordering: limit event precedes the terminal failure
	if evs[len(evs)-1].Type != EventTurnFailed || evs[len(evs)-2].Type != EventProviderLimitReached {
		t.Fatalf("expected [.., provider_limit_reached, turn_failed], got %+v", evs)
	}

	// rate-limit + credits + billing variants classify distinct kinds.
	for text, want := range map[string]ProviderLimitKind{
		"Error: rate limit reached, retry after 5s": ProviderLimitRateLimited,
		"out_of_credits":                            ProviderLimitCreditsExhausted,
		"payment required for this workspace":       ProviderLimitBillingRequired,
	} {
		evs := mapClaudeResult(map[string]any{"type": "result", "is_error": true, "result": text})
		limits := task445LimitEvents(evs)
		if len(limits) != 1 || limits[0].ProviderLimit.Kind != want {
			t.Fatalf("claude %q: want %v limit event, got %+v", text, want, evs)
		}
	}
}

func TestTask445_CodexFixtures(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		switch m["method"] {
		case "thread/start":
			fc.reply(m["id"], map[string]any{"threadId": "th-limit"})
		case "turn/start":
			fc.send(map[string]any{"jsonrpc": "2.0", "id": m["id"],
				"error": map[string]any{"code": float64(-32000), "message": "quota_exceeded: account over weekly quota"}})
		}
	})
	a := newCodexAdapter(d, t.TempDir())
	bridge := &captureBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := a.SendTurn(ctx, TurnRequest{RunID: "run-445", Prompt: "hi", Cwd: t.TempDir()}, bridge)
	if err == nil {
		t.Fatalf("quota turn/start error must surface, got nil")
	}
	limit, ok := providerLimitFromError(err)
	if !ok {
		t.Fatalf("codex quota error must carry typed ProviderLimit, got %T %v", err, err)
	}
	if limit.Kind != ProviderLimitQuotaExhausted || limit.ProviderKey != ProviderKeyCodex {
		t.Fatalf("limit = %+v, want quota_exhausted/codex", limit)
	}
	if isRecoverableSendError(err) {
		t.Fatalf("quota_exhausted must be non-recoverable")
	}
}

func TestTask445_GrokFixtures(t *testing.T) {
	d, fg := startFakeGrok(t, nil)
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": "ses_445"})
		case "session/prompt":
			// BUG-374 live wire shape: 402 detail in error.data.
			fg.send(map[string]any{"jsonrpc": "2.0", "id": m["id"],
				"error": map[string]any{"code": float64(-32000), "message": "Internal error",
					"data": map[string]any{"http_status": float64(402), "message": "API error (status 402 Payment Required): Grok Build usage balance exhausted"}}})
		}
	})
	a := newGrokAdapter(d, t.TempDir())
	bridge := &fakeGrokBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := a.SendTurn(ctx, TurnRequest{RunID: "run-445", Prompt: "hi", Cwd: t.TempDir()}, bridge)
	if err == nil {
		t.Fatalf("grok 402 must surface, got nil")
	}
	limit, ok := providerLimitFromError(err)
	if !ok {
		t.Fatalf("grok 402 must carry typed ProviderLimit, got %T %v", err, err)
	}
	if limit.Kind != ProviderLimitCreditsExhausted || limit.ProviderKey != ProviderKeyGrok {
		t.Fatalf("limit = %+v, want credits_exhausted/grok", limit)
	}
	if limit.DetectionSource != "structured_payload" {
		t.Fatalf("grok http_status detection must be structured_payload, got %q", limit.DetectionSource)
	}
}

func TestTask445_OpenCodeFixtures(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": "ses_445"})
		case "session/prompt":
			go func() {
				fg.notify("session/update", map[string]any{"sessionId": "ses_445", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "partial"}}})
				time.Sleep(10 * time.Millisecond)
				fg.reply(m["id"], map[string]any{"stopReason": "billing_required"})
			}()
		}
	})
	a := newOpencodeAdapter(d, t.TempDir())
	a.sessionStore = &recordingSessionStore{}
	bridge := &fakeOpencodeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.SendTurn(ctx, TurnRequest{RunID: "run-445", Prompt: "hi", Cwd: t.TempDir()}, bridge); err != nil {
		t.Fatalf("quota stopReason must be consumed into terminal events, got %v", err)
	}
	limits := task445LimitEvents(bridge.events)
	if len(limits) != 1 {
		t.Fatalf("want 1 provider_limit_reached, got %+v", bridge.events)
	}
	l := limits[0].ProviderLimit
	if l.Kind != ProviderLimitBillingRequired || l.ProviderKey != ProviderKeyOpencode {
		t.Fatalf("limit = %+v, want billing_required/opencode", l)
	}
	if l.DetectionSource != "stop_reason" || l.Confidence != "exact" {
		t.Fatalf("stopReason detection = %q/%q, want stop_reason/exact", l.DetectionSource, l.Confidence)
	}
	failed := task445FailedEvents(bridge.events)
	if len(failed) != 1 || failed[0].Recoverable {
		t.Fatalf("want exactly 1 non-recoverable turn_failed, got %+v", bridge.events)
	}
}

func TestTask445_DevinFixtures(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fd.reply(m["id"], map[string]any{"sessionId": "ses_445"})
		case "session/set_config_option":
			fd.reply(m["id"], map[string]any{})
		case "session/prompt":
			go func() {
				fd.notify("session/update", map[string]any{"sessionId": "ses_445", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "partial"}}})
				time.Sleep(10 * time.Millisecond)
				fd.reply(m["id"], map[string]any{"stopReason": "insufficient_credits"})
			}()
		}
	})
	a := newDevinAdapter(d, t.TempDir())
	a.sessionStore = &recordingSessionStore{}
	bridge := &fakeDevinBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.SendTurn(ctx, TurnRequest{RunID: "run-445", Prompt: "hi", Cwd: t.TempDir()}, bridge); err != nil {
		t.Fatalf("quota stopReason must be consumed into terminal events, got %v", err)
	}
	limits := task445LimitEvents(bridge.events)
	if len(limits) != 1 {
		t.Fatalf("want 1 provider_limit_reached, got %+v", bridge.events)
	}
	l := limits[0].ProviderLimit
	if l.Kind != ProviderLimitCreditsExhausted || l.ProviderKey != ProviderKeyDevin {
		t.Fatalf("limit = %+v, want credits_exhausted/devin", l)
	}
	if l.DetectionSource != "stop_reason" || l.Confidence != "exact" {
		t.Fatalf("stopReason detection = %q/%q, want stop_reason/exact", l.DetectionSource, l.Confidence)
	}
}

// task445FlakyAdapter returns scripted errors then succeeds.
type task445FlakyAdapter struct {
	errs  []error
	calls int
}

func (a *task445FlakyAdapter) Key() ProviderKey               { return ProviderKeyGrok }
func (a *task445FlakyAdapter) Capabilities() ProviderCapabilities { return ProviderCapabilities{} }

func (a *task445FlakyAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	a.calls++
	if a.calls <= len(a.errs) {
		return a.errs[a.calls-1]
	}
	return nil
}

type task445Bridge struct {
	events []ProviderEvent
}

func (b *task445Bridge) Emit(ev ProviderEvent)            { b.events = append(b.events, ev) }
func (b *task445Bridge) Accepted(ReceiptEvidence)         {}
func (b *task445Bridge) Terminal(TerminalEvidence)        {}
func (b *task445Bridge) RequestApproval(ApprovalDetails) (string, error) {
	return "deny", nil
}
func (b *task445Bridge) AskQuestion(string, []QuestionOption, bool) ([]string, error) {
	return nil, errors.New("no")
}
func (b *task445Bridge) SpawnAgent(SpawnAgentInput) (SpawnAgentResult, error) {
	return SpawnAgentResult{}, nil
}
func (b *task445Bridge) SubmitFlowControl(FlowControlInput) (FlowControlResult, error) {
	return FlowControlResult{}, nil
}
