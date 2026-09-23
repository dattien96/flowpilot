package runner

import (
	"strings"
	"testing"
)

// BUG-381: ACP JSON-RPC errors carry the real reason in error.data
// ({"http_status":402,"message":"API error (status 402 Payment Required): …"})
// while top-level error.message is a generic "Internal error". The extractor
// must unwrap error.data so quota/auth failures surface the real reason.
func TestBug381_JSONRPCErrorMessageUnwrapsDataMessage(t *testing.T) {
	msg := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    -32603,
			"message": "Internal error",
			"data": map[string]interface{}{
				"http_status": 402,
				"message":     "API error (status 402 Payment Required): Grok Build usage balance exhausted",
			},
		},
	}
	got := jsonRpcErrorMessage(msg)
	if !strings.Contains(got, "Payment Required") || !strings.Contains(got, "balance exhausted") {
		t.Fatalf("jsonRpcErrorMessage dropped error.data reason: %q", got)
	}
}

// BUG-381: a data.message-free error still surfaces the top-level message.
func TestBug381_JSONRPCErrorMessageKeepsTopLevelWhenNoData(t *testing.T) {
	msg := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    -32603,
			"message": "session expired",
		},
	}
	if got := jsonRpcErrorMessage(msg); got != "session expired" {
		t.Fatalf("top-level message lost: %q", got)
	}
}

// BUG-381: http_status alone still yields a useful string when data.message
// is absent — never collapse to a bare "Internal error".
func TestBug381_JSONRPCErrorMessageUsesHTTPStatus(t *testing.T) {
	msg := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    -32603,
			"message": "Internal error",
			"data": map[string]interface{}{
				"http_status": 401,
			},
		},
	}
	got := jsonRpcErrorMessage(msg)
	if !strings.Contains(got, "401") {
		t.Fatalf("http_status not surfaced: %q", got)
	}
}
