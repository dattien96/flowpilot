package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CP-70 additive tests for the Devin GetUserStatus account-usage path.

func writeDevinCredentials(t *testing.T, homePath, content string) string {
	t.Helper()
	dir := filepath.Join(homePath, ".local", "share", "devin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "credentials.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	return path
}

func stubDevinUserStatusHTTP(t *testing.T, handler func(*http.Request) (int, string)) *int {
	t.Helper()
	calls := 0
	prev := devinUserStatusHTTPDo
	devinUserStatusHTTPDo = func(request *http.Request) (*http.Response, error) {
		calls++
		code, body := handler(request)
		return &http.Response{
			StatusCode: code,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}
	t.Cleanup(func() { devinUserStatusHTTPDo = prev })
	return &calls
}

func resetDevinUserStatusCache(t *testing.T) {
	t.Helper()
	devinUserStatusCacheMu.Lock()
	devinUserStatusCache = map[string]devinUserStatusCacheEntry{}
	devinUserStatusCacheMu.Unlock()
	t.Cleanup(func() {
		devinUserStatusCacheMu.Lock()
		devinUserStatusCache = map[string]devinUserStatusCacheEntry{}
		devinUserStatusCacheMu.Unlock()
	})
}

const devinUserStatusFixture = `{
  "userStatus": {
    "email": "dev@example.com",
    "name": "Dev Example",
    "planStatus": {
      "planStart": "2026-09-20T09:16:50Z",
      "planEnd": "2026-10-20T09:16:50Z",
      "availablePromptCredits": -1,
      "dailyQuotaRemainingPercent": 100,
      "weeklyQuotaRemainingPercent": 94,
      "dailyQuotaResetAtUnix": "1790064000",
      "weeklyQuotaResetAtUnix": "1790496000",
      "planInfo": {
        "planName": "Pro",
        "devinInfo": { "accountDisplayName": "DN7696" }
      }
    }
  }
}`

func TestLoadDevinAccountMetadataMapsQuotaAndPlan(t *testing.T) {
	resetDevinUserStatusCache(t)
	home := t.TempDir()
	writeDevinCredentials(t, home, "windsurf_api_key = \"k\"\napi_server_url = \"https://api.example\"\n")

	var gotBody map[string]any
	calls := stubDevinUserStatusHTTP(t, func(req *http.Request) (int, string) {
		if !strings.HasSuffix(req.URL.Path, "SeatManagementService/GetUserStatus") {
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		if req.URL.Host != "api.example" {
			t.Fatalf("unexpected host %s", req.URL.Host)
		}
		raw, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		return 200, devinUserStatusFixture
	})

	meta, err := loadDevinAccountMetadata(home)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("expected 1 HTTP call, got %d", *calls)
	}
	if meta.accountEmail != "dev@example.com" {
		t.Fatalf("accountEmail=%q", meta.accountEmail)
	}
	if meta.accountName != "DN7696" {
		t.Fatalf("accountName=%q", meta.accountName)
	}
	if meta.usageSummary != "Pro until 2026-10-20" {
		t.Fatalf("usageSummary=%q", meta.usageSummary)
	}
	if meta.remaining5hPercent == nil || *meta.remaining5hPercent != 100 {
		t.Fatalf("remaining5hPercent=%v", meta.remaining5hPercent)
	}
	if meta.remaining7dPercent == nil || *meta.remaining7dPercent != 94 {
		t.Fatalf("remaining7dPercent=%v", meta.remaining7dPercent)
	}
	if meta.remaining5hResetAt != "2026-09-22T08:00:00Z" {
		t.Fatalf("remaining5hResetAt=%q", meta.remaining5hResetAt)
	}
	if meta.remaining7dResetAt != "2026-09-27T08:00:00Z" {
		t.Fatalf("remaining7dResetAt=%q", meta.remaining7dResetAt)
	}
	if len(meta.usageDetailLines) != 2 {
		t.Fatalf("usageDetailLines=%d", len(meta.usageDetailLines))
	}
	if meta.usageDetailLines[0].label != "Daily quota" || meta.usageDetailLines[0].remainingPercent != 100 {
		t.Fatalf("daily line=%+v", meta.usageDetailLines[0])
	}
	if meta.usageDetailLines[1].label != "Weekly quota" || meta.usageDetailLines[1].remainingPercent != 94 {
		t.Fatalf("weekly line=%+v", meta.usageDetailLines[1])
	}

	// api_key must travel inside metadata (live-verified Connect contract).
	metaMap, ok := gotBody["metadata"].(map[string]any)
	if !ok || metaMap["api_key"] != "k" {
		t.Fatalf("metadata.api_key missing: %v", gotBody)
	}
	if metaMap["ide_name"] == "" || metaMap["ide_version"] == "" ||
		metaMap["extension_name"] == "" || metaMap["extension_version"] == "" {
		t.Fatalf("metadata ide/extension fields missing: %v", metaMap)
	}
}

func TestLoadDevinAccountMetadataCachesSuccessAndFailure(t *testing.T) {
	resetDevinUserStatusCache(t)
	home := t.TempDir()
	writeDevinCredentials(t, home, "windsurf_api_key = \"k\"\napi_server_url = \"https://api.example\"\n")

	calls := stubDevinUserStatusHTTP(t, func(req *http.Request) (int, string) {
		return 200, devinUserStatusFixture
	})
	for i := 0; i < 3; i++ {
		meta, err := loadDevinAccountMetadata(home)
		if err != nil || meta.remaining7dPercent == nil {
			t.Fatalf("iter %d: err=%v meta=%+v", i, err, meta)
		}
	}
	if *calls != 1 {
		t.Fatalf("cache miss — expected 1 HTTP call, got %d", *calls)
	}

	// Failure path is cached too (cannot stall every account refresh).
	devinUserStatusCacheMu.Lock()
	devinUserStatusCache = map[string]devinUserStatusCacheEntry{}
	devinUserStatusCacheMu.Unlock()
	*calls = 0
	prev := devinUserStatusHTTPDo
	devinUserStatusHTTPDo = func(req *http.Request) (*http.Response, error) {
		*calls++
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	}
	for i := 0; i < 2; i++ {
		meta, err := loadDevinAccountMetadata(home)
		if err != nil {
			t.Fatalf("iter %d: err=%v", i, err)
		}
		if meta.remaining5hPercent != nil || len(meta.usageDetailLines) != 0 {
			t.Fatalf("iter %d: expected degraded metadata, got %+v", i, meta)
		}
	}
	devinUserStatusHTTPDo = prev
	if *calls != 1 {
		t.Fatalf("failure cache miss — expected 1 HTTP call, got %d", *calls)
	}
}

func TestLoadDevinAccountMetadataDegradesWithoutCredentials(t *testing.T) {
	resetDevinUserStatusCache(t)
	meta, err := loadDevinAccountMetadata(t.TempDir())
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if meta.usageSummary != "" || meta.remaining5hPercent != nil || len(meta.usageDetailLines) != 0 {
		t.Fatalf("expected bare metadata, got %+v", meta)
	}
}

func TestLoadDevinAccountMetadataDegradesOnMalformedResponse(t *testing.T) {
	resetDevinUserStatusCache(t)
	home := t.TempDir()
	writeDevinCredentials(t, home, "windsurf_api_key = \"k\"\napi_server_url = \"https://api.example\"\n")
	stubDevinUserStatusHTTP(t, func(req *http.Request) (int, string) {
		return 200, "not-json{{"
	})
	meta, err := loadDevinAccountMetadata(home)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if meta.remaining5hPercent != nil || len(meta.usageDetailLines) != 0 {
		t.Fatalf("expected degraded metadata, got %+v", meta)
	}
}

func TestDevinQuotaPercentHandlesNumberAndString(t *testing.T) {
	if got, ok := devinQuotaPercent(float64(94)); !ok || got != 94 {
		t.Fatalf("number: %v %v", got, ok)
	}
	if got, ok := devinQuotaPercent("61"); !ok || got != 61 {
		t.Fatalf("string: %v %v", got, ok)
	}
	if got, ok := devinQuotaPercent(float64(140)); !ok || got != 100 {
		t.Fatalf("clamp high: %v %v", got, ok)
	}
	if _, ok := devinQuotaPercent("n/a"); ok {
		t.Fatal("expected not-ok for non-numeric")
	}
	if _, ok := devinQuotaPercent(nil); ok {
		t.Fatal("expected not-ok for nil")
	}
}

func TestLoadDevinAccountMetadataHonorsDevinAPIURLFallback(t *testing.T) {
	resetDevinUserStatusCache(t)
	home := t.TempDir()
	// api_server_url absent -> devin_api_url is the documented fallback.
	writeDevinCredentials(t, home, "windsurf_api_key = \"k\"\ndevin_api_url = \"https://fallback.example\"\n")
	calls := stubDevinUserStatusHTTP(t, func(req *http.Request) (int, string) {
		return 200, devinUserStatusFixture
	})
	meta, err := loadDevinAccountMetadata(home)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if *calls != 1 || meta.usageSummary == "" {
		t.Fatalf("calls=%d usageSummary=%q", *calls, meta.usageSummary)
	}
}
