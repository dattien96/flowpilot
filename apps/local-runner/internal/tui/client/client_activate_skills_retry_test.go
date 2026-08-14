package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func TestActivateProviderAccount_UnwrapsAccountEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider-accounts/activate" {
			t.Errorf("path=%s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"account": map[string]any{
				"id":           "acc-grok-2",
				"provider_key": "grok",
				"display_name": "Grok Home 2",
				"home_path":    "/Users/me/.grokHome2",
				"is_active":    true,
				"auth_status":  "connected",
			},
		})
	}))
	defer srv.Close()

	acc, err := client.New(srv.URL).ActivateProviderAccount(context.Background(), "acc-grok-2")
	if err != nil {
		t.Fatalf("ActivateProviderAccount: %v", err)
	}
	if acc.ID != "acc-grok-2" || acc.ProviderKey != "grok" {
		t.Fatalf("account=%+v", acc)
	}
	if acc.DisplayLabel != "Grok Home 2" {
		t.Fatalf("DisplayLabel=%q want display_name fallback", acc.DisplayLabel)
	}
	if acc.HomePath != "/Users/me/.grokHome2" {
		t.Fatalf("HomePath=%q", acc.HomePath)
	}
}

func TestListProviderAccounts_ParsesHomePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "a1", "provider_key": "grok", "display_label": "Default", "home_path": "/Users/me/.grokHome1", "is_active": true, "auth_status": "connected"},
		})
	}))
	defer srv.Close()
	accs, err := client.New(srv.URL).ListProviderAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListProviderAccounts: %v", err)
	}
	if len(accs) != 1 || accs[0].HomePath != "/Users/me/.grokHome1" {
		t.Fatalf("HomePath not parsed: %+v", accs)
	}
}

func TestListSkills_UsesProviderSkillsEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode([]map[string]string{{"name": "coding", "source": "provider"}})
	}))
	defer srv.Close()

	skills, err := client.New(srv.URL).ListSkills(context.Background(), "grok", "/ws")
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if gotPath != "/client/provider-skills" {
		t.Fatalf("path=%q want /client/provider-skills", gotPath)
	}
	if len(skills) != 1 || skills[0].Name != "coding" {
		t.Fatalf("skills=%+v", skills)
	}
}

func TestIsRetryableAPIError_ConflictAndCodes(t *testing.T) {
	if !client.IsRetryableAPIError(&client.APIError{Status: 409, Code: "gate_in_progress", Message: "busy"}) {
		t.Fatal("409 gate_in_progress should retry")
	}
	if !client.IsRetryableAPIError(&client.APIError{Status: 409, Code: "other", Message: "post-turn gate still running"}) {
		t.Fatal("HTTP 409 should retry even if code is unfamiliar")
	}
	if client.IsRetryableAPIError(&client.APIError{Status: 400, Code: "bad_request", Message: "nope"}) {
		t.Fatal("400 must not retry")
	}
	if client.IsRetryableAPIError(errors.New("dial tcp 127.0.0.1:4090: connection refused")) {
		t.Fatal("port 4090 must not match as HTTP 409")
	}
	if client.RetryableCode("409") {
		t.Fatal("RetryableCode must not treat HTTP status 409 as an error code")
	}
}
