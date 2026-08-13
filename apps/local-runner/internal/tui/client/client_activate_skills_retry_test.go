package client_test

import (
	"context"
	"encoding/json"
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
