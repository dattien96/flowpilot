package runner

import (
	"errors"
	"strings"
	"testing"
)

// BUG-374: live Grok ACP wire shape (run-1770057, 2026-09-23):
//
//	{"error":{"code":-32603,
//	          "data":{"http_status":402,
//	                  "message":"API error (status 402 Payment Required): Grok Build usage balance exhausted"},
//	          "message":"Internal error"},"id":7}
//
// jsonRpcErrorMessage kept only `error.message` ("Internal error"), dropping
// `error.data.message` — the surfaced turn_failed lost every quota token, so
// isProviderUsageLimitError classified it as a generic failure and no
// account-switch/quota flow fired. The helper is shared by the grok, codex,
// devin, opencode and gemini dispatchers; the fix must be additive (append
// detail, never replace message) so existing single-string shapes are
// unchanged.
func TestBug374JSONRPCErrorKeepsDataDetail(t *testing.T) {
	live := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    -32603,
			"message": "Internal error",
			"data": map[string]interface{}{
				"http_status": 402,
				"message":     "API error (status 402 Payment Required): Grok Build usage balance exhausted",
			},
		},
	}
	got := jsonRpcErrorMessage(live)
	if !strings.Contains(got, "Internal error") {
		t.Fatalf("message dropped: got %q", got)
	}
	if !strings.Contains(got, "Payment Required") {
		t.Fatalf("data.message detail dropped: got %q", got)
	}
	if !isProviderUsageLimitError(errors.New(got)) {
		t.Fatalf("surfaced error %q must classify as usage-limit/quota", got)
	}
}

func TestBug374JSONRPCErrorStringData(t *testing.T) {
	got := jsonRpcErrorMessage(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    -32603,
			"message": "Internal error",
			"data":    "quota_exceeded: model over quota",
		},
	})
	if !strings.Contains(got, "quota_exceeded") {
		t.Fatalf("string data dropped: got %q", got)
	}
}

func TestBug374JSONRPCErrorNoDuplicateWhenDataRepeatsMessage(t *testing.T) {
	got := jsonRpcErrorMessage(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    -32000,
			"message": "rate limit reached",
			"data": map[string]interface{}{
				"message": "rate limit reached",
			},
		},
	})
	if strings.Count(got, "rate limit reached") != 1 {
		t.Fatalf("detail duplicated: got %q", got)
	}
}

func TestBug374JSONRPCErrorHealthyDataStaysHealthy(t *testing.T) {
	got := jsonRpcErrorMessage(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    -32603,
			"message": "Internal error",
			"data":    map[string]interface{}{"http_status": 500},
		},
	})
	if got == "" {
		t.Fatal("message dropped entirely")
	}
	if isProviderUsageLimitError(errors.New(got)) {
		t.Fatalf("healthy error %q must not classify as quota", got)
	}
}
