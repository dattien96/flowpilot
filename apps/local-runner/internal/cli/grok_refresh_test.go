package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGrokKeyExpired(t *testing.T) {
	if grokKeyExpired("") {
		t.Fatal("empty should not be expired")
	}
	future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339Nano)
	if grokKeyExpired(future) {
		t.Fatalf("future %q should not be expired", future)
	}
	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano)
	if !grokKeyExpired(past) {
		t.Fatalf("past %q should be expired", past)
	}
	// within 5m buffer is considered expired
	soon := time.Now().UTC().Add(2 * time.Minute).Format(time.RFC3339Nano)
	if !grokKeyExpired(soon) {
		t.Fatalf("soon %q should be expired (5m buffer)", soon)
	}
}

func TestRefreshGrokAccessToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type=%q", got)
		}
		if got := r.Form.Get("refresh_token"); got != "rt-old" {
			t.Fatalf("refresh_token=%q", got)
		}
		if got := r.Form.Get("client_id"); got != "cid" {
			t.Fatalf("client_id=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-new",
			"refresh_token": "rt-new",
			"expires_in":    21600,
		})
	}))
	defer srv.Close()

	at, rt, exp, err := refreshGrokAccessToken("rt-old", srv.URL, "cid")
	if err != nil {
		t.Fatalf("refresh err: %v", err)
	}
	if at != "at-new" || rt != "rt-new" {
		t.Fatalf("at=%q rt=%q", at, rt)
	}
	if time.Until(exp) < 5*time.Hour {
		t.Fatalf("exp too soon %v", exp)
	}
}

func TestLoadGrokQuotaWithRefresh_RetriesAfter401(t *testing.T) {
	// token refresh server
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-fresh",
			"refresh_token": "rt-fresh",
			"expires_in":    21600,
		})
	}))
	defer tokenSrv.Close()

	// billing server: first call 401, second call 200 with weekly
	call := 0
	billingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		auth := r.Header.Get("Authorization")
		if call == 1 {
			if auth != "Bearer at-expired" {
				t.Fatalf("call1 auth=%q", auth)
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if auth != "Bearer at-fresh" {
			t.Fatalf("call2 auth=%q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grokBillingResponse{
			Config: grokBillingConfig{
				CurrentPeriod:      &grokBillingPeriod{Type: "USAGE_PERIOD_TYPE_WEEKLY", End: "2026-08-26T23:46:37Z"},
				CreditUsagePercent: floatPtr(24),
				BillingPeriodEnd:   "2026-08-26T23:46:37Z",
			},
		})
	}))
	defer billingSrv.Close()

	origBase := grokCLIChatProxyBaseURL
	grokCLIChatProxyBaseURL = billingSrv.URL
	defer func() { grokCLIChatProxyBaseURL = origBase }()

	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	entry := grokAuthEntry{
		Email:        "a@b.com",
		Key:          "at-expired",
		RefreshToken: "rt-old",
		OidcIssuer:   tokenSrv.URL,
		OidcClientID: "cid",
		ExpiresAt:    time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano),
	}
	// Persist initial file for tryRefreshGrokAuth to update
	initial := map[string]grokAuthEntry{"https://auth.x.ai::cid": entry}
	b, _ := json.Marshal(initial)
	_ = os.WriteFile(authPath, b, 0600)

	// mutate copy
	mutable := entry
	line := loadGrokQuotaWithRefresh(&mutable, authPath, "https://auth.x.ai::cid")
	if line == nil {
		t.Fatal("expected line after 401->refresh retry")
	}
	if line.label != "Weekly limit" || line.remainingPercent != 76 {
		t.Fatalf("line=%+v", line)
	}
	if mutable.Key != "at-fresh" {
		t.Fatalf("mutable key not updated %q", mutable.Key)
	}
	// file should be updated
	raw, _ := os.ReadFile(authPath)
	var m2 map[string]map[string]any
	_ = json.Unmarshal(raw, &m2)
	if got := m2["https://auth.x.ai::cid"]["key"]; got != "at-fresh" {
		t.Fatalf("file key=%v", got)
	}
}

func floatPtr(v float64) *float64 { return &v }

func TestLoadGrokAccountMetadata_ExpiredPreRefresh(t *testing.T) {
	// billing server returns weekly on fresh token
	billingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at-fresh2" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grokBillingResponse{
			Config: grokBillingConfig{
				CurrentPeriod:      &grokBillingPeriod{Type: "USAGE_PERIOD_TYPE_WEEKLY", End: "2026-08-26T23:46:37Z"},
				CreditUsagePercent: floatPtr(10),
			},
		})
	}))
	defer billingSrv.Close()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at-fresh2", "refresh_token": "rt-fresh2", "expires_in": 21600})
	}))
	defer tokenSrv.Close()

	origBase := grokCLIChatProxyBaseURL
	grokCLIChatProxyBaseURL = billingSrv.URL
	defer func() { grokCLIChatProxyBaseURL = origBase }()

	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	initial := map[string]grokAuthEntry{
		"https://auth.x.ai::cid": {
			Email:        "a@b.com",
			Key:          "at-stale",
			RefreshToken: "rt-old2",
			OidcIssuer:   tokenSrv.URL,
			OidcClientID: "cid",
			ExpiresAt:    time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano),
		},
	}
	b, _ := json.Marshal(initial)
	_ = os.WriteFile(authPath, b, 0600)

	meta, err := loadGrokAccountMetadata(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if meta.remaining7dPercent == nil || *meta.remaining7dPercent != 90 {
		t.Fatalf("7d=%v want 90", meta.remaining7dPercent)
	}
}
