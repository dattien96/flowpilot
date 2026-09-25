package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Task-445 (CP-87 P-1): typed provider-limit normalization. One classifier from
// adapter to UI replaces the per-layer string token lists. Detection priority:
// structured payload fields → stop reasons → centralized string fallback
// (stderr_fallback / heuristic) for unstructured CLI/RPC text only.

// providerLimitMaxRetryWaitSeconds bounds the Retry-After the send loop will
// actually honor (Task-445 T-3): a rate limit with a wait beyond this is
// treated as terminal so the quota router can take over.
const providerLimitMaxRetryWaitSeconds int64 = 30

// providerLimitRetryDelayFn sleeps between bounded rate-limit retries; tests
// override it to assert the honored delay without sleeping.
var providerLimitRetryDelayFn = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// unclassifiedProviderFailureFn records failures that reached an adapter
// boundary but did not classify — the unclassified audit evidence required by
// Task-445 (unknown shapes stay generic failures, never silently quota).
var unclassifiedProviderFailureFn = func(provider ProviderKey, err error) {
	log.Printf("[provider-limit] unclassified provider failure provider=%q err=%q", provider, err.Error())
}

// rpcError preserves the JSON-RPC error envelope (code + data) that the ACP /
// app-server dispatchers used to flatten into a bare string, so classification
// can use structured fields (http_status, retry_after) before falling back to
// message text. Error() stays byte-identical to the old flattened message.
type rpcError struct {
	code     int64
	codeText string
	message  string
	data     map[string]any
}

func (e *rpcError) Error() string { return e.message }

// newRPCError builds a typed error from a JSON-RPC `error` member object,
// keeping the exact user-facing message jsonRpcErrorMessage produced.
func newRPCError(errObj any) error {
	re := &rpcError{message: jsonRpcErrorMessage(map[string]any{"error": errObj})}
	if m, ok := errObj.(map[string]any); ok {
		if c, ok := toInt64(m["code"]); ok {
			re.code = c
		} else if s, ok := m["code"].(string); ok {
			re.codeText = s
		}
		if data, ok := m["data"].(map[string]any); ok {
			re.data = data
		}
	}
	if strings.TrimSpace(re.message) == "" {
		re.message = "JSON-RPC error"
	}
	return re
}

// providerLimitError carries the typed classification alongside the original
// failure so the retry loop and finishTurn can act on the kind without
// re-parsing the message.
type providerLimitError struct {
	limit *ProviderLimit
	err   error
}

func (e *providerLimitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	if e.limit != nil {
		return e.limit.SanitizedMessage
	}
	return "provider limit reached"
}

func (e *providerLimitError) Unwrap() error { return e.err }

// providerLimitFromError unwraps a typed limit if the error carries one.
func providerLimitFromError(err error) (*ProviderLimit, bool) {
	var le *providerLimitError
	if errors.As(err, &le) && le.limit != nil {
		return le.limit, true
	}
	return nil, false
}

// providerLimitRecoverable is the Task-445 T-3 retry policy: only a rate limit
// carrying a bounded Retry-After is safely retryable; every other kind is a
// terminal quota signal for the CP-87 router.
func providerLimitRecoverable(limit ProviderLimit) bool {
	return limit.Kind == ProviderLimitRateLimited &&
		limit.RetryAfterSeconds > 0 &&
		limit.RetryAfterSeconds <= providerLimitMaxRetryWaitSeconds
}

// providerLimitAwareError wraps a provider-boundary error with its typed
// classification. Non-limit failures pass through untouched and are audited as
// unclassified (the log line is the evidence trail for unknown shapes).
func providerLimitAwareError(provider ProviderKey, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := providerLimitFromError(err); ok {
		return err
	}
	if limit, ok := classifyProviderLimit(provider, nil, err); ok {
		return &providerLimitError{limit: limit, err: err}
	}
	unclassifiedProviderFailureFn(provider, err)
	return err
}

// claudeLimitError wraps a legacy Claude sessions-path failure with its typed
// classification. stderr text is unstructured → the err slot (stderr_fallback);
// the parsed result payload is structured → the payload slot.
func claudeLimitError(payload any, err error) error {
	if err == nil {
		return nil
	}
	if limit, ok := classifyProviderLimit(ProviderKeyClaude, payload, err); ok {
		return &providerLimitError{limit: limit, err: err}
	}
	return err
}

// ---- classification -------------------------------------------------------

// classifyProviderLimit normalizes a limit failure. Detection priority
// (Task-445 T-2): structured payload fields → stop reasons → the shared string
// fallback over raw error text. Unknown shapes return false.
func classifyProviderLimit(provider ProviderKey, payload any, err error) (*ProviderLimit, bool) {
	if m, ok := payload.(map[string]any); ok && m != nil {
		if limit, ok := classifyPayloadLimit(provider, m); ok {
			return limit, true
		}
	}
	if err == nil {
		return nil, false
	}
	if limit, ok := providerLimitFromError(err); ok {
		return limit, true
	}
	var re *rpcError
	if errors.As(err, &re) {
		if limit, ok := classifyRPCLimit(provider, re); ok {
			return limit, true
		}
	}
	return classifyLimitText(provider, err.Error(), ProviderLimitDetectionStderrFallback)
}

// classifyPayloadLimit classifies a provider JSON frame (result map, session
// update payload, RPC params). Typed fields classify exact; a free-text token
// hit inside a structured carrier reports heuristic confidence.
func classifyPayloadLimit(provider ProviderKey, m map[string]any) (*ProviderLimit, bool) {
	if reason := findStopReasonField(m); reason != "" {
		if limit, ok := classifyStopReasonLimit(provider, reason); ok {
			return limit, true
		}
	}
	if limit, ok := classifyStructuredFields(provider, m); ok {
		return limit, true
	}
	if limit, ok := classifyLimitText(provider, flattenClaudeStrings(m), ProviderLimitDetectionStructuredPayload); ok {
		return limit, true
	}
	// Claude's own payload gate keeps parity with claudeUsageLimitMessage even
	// when the only signal is a bare "quota"/"credit" token the shared table
	// deliberately does not match on raw text.
	if provider == ProviderKeyClaude {
		if msg := claudeUsageLimitMessage(m); msg != "" {
			return &ProviderLimit{
				Kind:             ProviderLimitQuotaExhausted,
				ProviderKey:      provider,
				SanitizedMessage: sanitizeLimitMessage(msg),
				DetectionSource:  ProviderLimitDetectionStructuredPayload,
				Confidence:       ProviderLimitConfidenceHeuristic,
			}, true
		}
	}
	return nil, false
}

// classifyRPCLimit classifies a structured JSON-RPC error envelope: typed
// http_status/code fields classify exact; the message text falls to the token
// scan under the structured carrier.
func classifyRPCLimit(provider ProviderKey, re *rpcError) (*ProviderLimit, bool) {
	payload := map[string]any{"error": map[string]any{}}
	errMap := payload["error"].(map[string]any)
	if re.code != 0 {
		errMap["code"] = re.code
	}
	if re.codeText != "" {
		errMap["code"] = re.codeText
	}
	if re.data != nil {
		errMap["data"] = re.data
	}
	if limit, ok := classifyStructuredFields(provider, payload); ok {
		return limit, true
	}
	if limit, ok := classifyLimitText(provider, re.message, ProviderLimitDetectionStructuredPayload); ok {
		return limit, true
	}
	return nil, false
}

// classifyStopReasonLimit maps a terminal stopReason to a limit kind. Only
// unambiguously-billing tokens match — unknown reasons stay unclassified so a
// provider's "weird_future_reason" never masquerades as quota.
func classifyStopReasonLimit(provider ProviderKey, reason string) (*ProviderLimit, bool) {
	kind, ok := providerLimitKindForText(reason, providerLimitStopReasonRules)
	if !ok {
		return nil, false
	}
	return &ProviderLimit{
		Kind:             kind,
		ProviderKey:      provider,
		RawCode:          strings.TrimSpace(reason),
		SanitizedMessage: sanitizeLimitMessage(reason),
		DetectionSource:  ProviderLimitDetectionStopReason,
		Confidence:       ProviderLimitConfidenceExact,
	}, true
}

// providerLimitStopReasonRules maps normalized stopReason tokens to kinds.
// Order matters: first match wins.
var providerLimitStopReasonRules = []providerLimitTokenRule{
	{"rate_limit", ProviderLimitRateLimited},
	{"rate-limit", ProviderLimitRateLimited},
	{"rate limit", ProviderLimitRateLimited},
	{"insufficient_credit", ProviderLimitCreditsExhausted},
	{"insufficient credit", ProviderLimitCreditsExhausted},
	{"billing", ProviderLimitBillingRequired},
	{"payment", ProviderLimitBillingRequired},
	{"quota", ProviderLimitQuotaExhausted},
	{"usage_limit", ProviderLimitQuotaExhausted},
	{"usage-limit", ProviderLimitQuotaExhausted},
	{"usage limit", ProviderLimitQuotaExhausted},
}

// providerLimitTokenRules is the single free-text token table — the only place
// message strings map to limit kinds (was: isProviderUsageLimitError +
// claudeUsageLimitMessage + the desktop's isUsageLimitMessage). Ordered: first
// match wins; rate-limit tokens precede quota so a transient 429 is never
// mistaken for an exhausted account.
var providerLimitTokenRules = []providerLimitTokenRule{
	// transient rate limiting
	{"rate_limited", ProviderLimitRateLimited},
	{"rate-limit", ProviderLimitRateLimited},
	{"rate_limit", ProviderLimitRateLimited},
	{"rate limited", ProviderLimitRateLimited},
	{"rate limit", ProviderLimitRateLimited},
	{"too many requests", ProviderLimitRateLimited},
	{"throttled", ProviderLimitRateLimited},
	// credits exhausted
	{"out_of_credits", ProviderLimitCreditsExhausted},
	{"out of credits", ProviderLimitCreditsExhausted},
	{"insufficient_credit", ProviderLimitCreditsExhausted},
	{"insufficient credit", ProviderLimitCreditsExhausted},
	{"credits exhausted", ProviderLimitCreditsExhausted},
	{"credit balance", ProviderLimitCreditsExhausted},
	{"balance exhausted", ProviderLimitCreditsExhausted},
	{"personal-team-blocked", ProviderLimitCreditsExhausted},
	{"spending-limit", ProviderLimitCreditsExhausted},
	{"spending_limit", ProviderLimitCreditsExhausted},
	{"spending limit", ProviderLimitCreditsExhausted},
	// billing required
	{"payment_required", ProviderLimitBillingRequired},
	{"payment required", ProviderLimitBillingRequired},
	{"no payment method", ProviderLimitBillingRequired},
	{"add a payment method", ProviderLimitBillingRequired},
	{"billing_required", ProviderLimitBillingRequired},
	{"billing required", ProviderLimitBillingRequired},
	// quota exhausted (bare "quota" intentionally excluded — too broad for
	// unstructured text; payload scans add it via classifyStopReasonLimit /
	// the claude payload gate)
	{"usage_limit", ProviderLimitQuotaExhausted},
	{"usage-limit", ProviderLimitQuotaExhausted},
	{"usage limit", ProviderLimitQuotaExhausted},
	{"extra usage unavailable", ProviderLimitQuotaExhausted},
	{"quota_exceeded", ProviderLimitQuotaExhausted},
	{"quota exceeded", ProviderLimitQuotaExhausted},
	{"quota reset", ProviderLimitQuotaExhausted},
}

type providerLimitTokenRule struct {
	token string
	kind  ProviderLimitKind
}

// providerLimitKindForText returns the first matching token's kind.
func providerLimitKindForText(text string, rules []providerLimitTokenRule) (ProviderLimitKind, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return "", false
	}
	for _, rule := range rules {
		if strings.Contains(lower, rule.token) {
			return rule.kind, true
		}
	}
	return "", false
}

// classifyLimitText is the centralized string fallback — source distinguishes
// where the text lived (structured payload vs unstructured stderr), but a
// token match on free text is always heuristic evidence.
func classifyLimitText(provider ProviderKey, text string, source string) (*ProviderLimit, bool) {
	kind, ok := providerLimitKindForText(text, providerLimitTokenRules)
	if !ok {
		return nil, false
	}
	return &ProviderLimit{
		Kind:              kind,
		ProviderKey:       provider,
		RetryAfterSeconds: retryAfterFromText(text),
		ResetAt:           resetAtFromText(text),
		SanitizedMessage:  sanitizeLimitMessage(text),
		DetectionSource:   source,
		Confidence:        ProviderLimitConfidenceHeuristic,
	}, true
}

// classifyStructuredFields walks the payload for typed fields: numeric
// http_status (429 → rate_limited, 402 → credits/billing by accompanying
// detail), typed code/type strings, and retry_after fields. Matches here are
// exact evidence — a structured provider field named the failure.
func classifyStructuredFields(provider ProviderKey, m map[string]any) (*ProviderLimit, bool) {
	var (
		httpStatus int64
		rawCode    string
		retryAfter int64
		resetAt    string
		typedHit   ProviderLimitKind
		typedOK    bool
	)
	walkPayloadValues(m, 0, func(key string, v any) {
		switch strings.ToLower(key) {
		case "http_status", "httpstatus", "status_code", "statuscode":
			if n, ok := toInt64(v); ok && (n == 402 || n == 429) {
				httpStatus = n
			}
		case "retry_after", "retryafter", "retryafterseconds", "retry_after_seconds", "retryaftersecondsms":
			if n, ok := toInt64(v); ok && n > 0 {
				retryAfter = n
			}
		case "retryafterms", "retry_after_ms":
			if n, ok := toInt64(v); ok && n > 0 {
				retryAfter = (n + 999) / 1000
			}
		case "resetat", "reset_at", "resetsat", "resets_at":
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				resetAt = strings.TrimSpace(s)
			}
		case "type", "code", "errortype", "errorcode", "error_code", "error_type":
			if s, ok := v.(string); ok {
				if kind, hit := providerLimitKindForText(s, providerLimitTokenRules); hit && !typedOK {
					typedHit, typedOK = kind, true
					rawCode = s
				}
			}
		}
	})
	if typedOK {
		return &ProviderLimit{
			Kind:              typedHit,
			ProviderKey:       provider,
			RetryAfterSeconds: retryAfter,
			ResetAt:           resetAt,
			RawCode:           rawCode,
			SanitizedMessage:  sanitizeLimitMessage(flattenClaudeStrings(m)),
			DetectionSource:   ProviderLimitDetectionStructuredPayload,
			Confidence:        ProviderLimitConfidenceExact,
		}, true
	}
	if httpStatus == 429 {
		return &ProviderLimit{
			Kind:              ProviderLimitRateLimited,
			ProviderKey:       provider,
			RetryAfterSeconds: retryAfter,
			ResetAt:           resetAt,
			RawCode:           "http_status=429",
			SanitizedMessage:  sanitizeLimitMessage(flattenClaudeStrings(m)),
			DetectionSource:   ProviderLimitDetectionStructuredPayload,
			Confidence:        ProviderLimitConfidenceExact,
		}, true
	}
	if httpStatus == 402 {
		// 402 needs the accompanying detail to split credits from billing;
		// default to billing_required — the HTTP meaning of 402.
		kind := ProviderLimitBillingRequired
		if k, ok := providerLimitKindForText(flattenClaudeStrings(m), providerLimitTokenRules); ok &&
			k == ProviderLimitCreditsExhausted {
			kind = ProviderLimitCreditsExhausted
		}
		return &ProviderLimit{
			Kind:              kind,
			ProviderKey:       provider,
			RetryAfterSeconds: retryAfter,
			ResetAt:           resetAt,
			RawCode:           "http_status=402",
			SanitizedMessage:  sanitizeLimitMessage(flattenClaudeStrings(m)),
			DetectionSource:   ProviderLimitDetectionStructuredPayload,
			Confidence:        ProviderLimitConfidenceExact,
		}, true
	}
	return nil, false
}

// walkPayloadValues visits every (key, value) pair in nested maps/slices of a
// provider frame, depth-bounded so a pathological payload cannot recurse deep.
func walkPayloadValues(v any, depth int, visit func(key string, v any)) {
	if depth > 6 {
		return
	}
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			visit(k, val)
			walkPayloadValues(val, depth+1, visit)
		}
	case []any:
		for _, val := range t {
			walkPayloadValues(val, depth+1, visit)
		}
	}
}

// findStopReasonField finds a stopReason-shaped string in the payload
// (top-level or one level inside result/data/_meta/update wrappers).
func findStopReasonField(m map[string]any) string {
	var found string
	walkPayloadValues(m, 0, func(key string, v any) {
		if found != "" {
			return
		}
		k := strings.ToLower(strings.TrimSpace(key))
		if k == "stopreason" || k == "stop_reason" {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				found = s
			}
		}
	})
	return found
}

var providerLimitRetryAfterTextRe = regexp.MustCompile(`(?i)retry[- ]?after[: ]*(\d+)|retry in (\d+)|try again in (\d+)`)

func retryAfterFromText(text string) int64 {
	m := providerLimitRetryAfterTextRe.FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	for _, g := range m[1:] {
		if g == "" {
			continue
		}
		if n, err := strconv.ParseInt(g, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

var providerLimitResetTextRe = regexp.MustCompile(`(?i)(?:resets?|reset(?:s)? at|quota reset(?:s)? at)[: ]*([0-9]{2}:[0-9]{2}(?::[0-9]{2})?\s*(?:am|pm|utc|z)?)`)

func resetAtFromText(text string) string {
	m := providerLimitResetTextRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// sanitizeLimitMessage collapses whitespace and caps length — the same bound
// the adapters already applied to quota copy — so a provider payload never
// sprays raw dumps into the persisted event.
func sanitizeLimitMessage(msg string) string {
	msg = strings.Join(strings.Fields(strings.TrimSpace(msg)), " ")
	if runes := []rune(msg); len(runes) > 300 {
		msg = string(runes[:300]) + "…"
	}
	if msg == "" {
		msg = fmt.Sprintf("provider limit reached")
	}
	return msg
}
