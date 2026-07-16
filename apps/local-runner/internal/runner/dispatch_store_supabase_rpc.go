package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// supabaseRPCDispatchStore layers optional PostgREST RPC calls on top of the
// memory projection. Production with SUPABASE_URL + service role key can hit
// dispatch_* RPCs from migration 20260717120000_dispatch_records_cp51.sql.
// Without HTTP config (or on RPC failure) behaviour matches NewSupabaseDispatchStore().
type supabaseRPCDispatchStore struct {
	*memoryDispatchStore
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewSupabaseDispatchStoreFromDSN accepts either a Postgres DSN (logged; HTTP path
// preferred) or is called when FLOWPILOT_DISPATCH_SUPABASE_DSN is set. The runner
// still uses PostgREST when SUPABASE_URL is configured.
func NewSupabaseDispatchStoreFromDSN(dsn string) (DispatchStore, error) {
	s := &supabaseRPCDispatchStore{
		memoryDispatchStore: newMemoryDispatchStore(),
		baseURL:             strings.TrimSpace(os.Getenv("SUPABASE_URL")),
		apiKey:              firstNonEmptyEnv(os.Getenv("SUPABASE_SERVICE_ROLE_KEY"), os.Getenv("SUPABASE_ANON_KEY")),
		http:                &http.Client{Timeout: 15 * time.Second},
	}
	if dsn != "" {
		log.Printf("[dispatch] supabase DSN present (len=%d); RPC via PostgREST when SUPABASE_URL set", len(dsn))
	}
	return s, nil
}

func firstNonEmptyEnv(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (s *supabaseRPCDispatchStore) rpcEnabled() bool {
	return s != nil && s.baseURL != "" && s.apiKey != "" && s.http != nil
}

func (s *supabaseRPCDispatchStore) callRPC(ctx context.Context, fn string, body map[string]any) ([]byte, error) {
	if !s.rpcEnabled() {
		return nil, fmt.Errorf("rpc disabled")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(s.baseURL, "/") + "/rest/v1/rpc/" + fn
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rpc %s: status %d body %s", fn, resp.StatusCode, truncate(string(b), 200))
	}
	return b, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (s *supabaseRPCDispatchStore) CreatePrepared(ctx context.Context, rec DispatchRecord, env DispatchEnvelope) error {
	if s.rpcEnabled() {
		recM := map[string]any{}
		raw, _ := json.Marshal(rec)
		_ = json.Unmarshal(raw, &recM)
		envM := map[string]any{}
		eraw, _ := json.Marshal(env)
		_ = json.Unmarshal(eraw, &envM)
		if _, err := s.callRPC(ctx, "dispatch_create_prepared", map[string]any{
			"p_run_id": rec.RunID, "p_turn_id": rec.TurnID,
			"p_record": recM, "p_envelope": envM,
		}); err != nil {
			log.Printf("[dispatch] rpc CreatePrepared: %v (memory)", err)
		}
	}
	return s.memoryDispatchStore.CreatePrepared(ctx, rec, env)
}

func (s *supabaseRPCDispatchStore) CASAdvance(ctx context.Context, runID, turnID string, expectedRev int64,
	expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error) {
	if s.rpcEnabled() {
		if _, err := s.callRPC(ctx, "dispatch_cas_advance", map[string]any{
			"p_run_id": runID, "p_turn_id": turnID,
			"p_expected_rev": expectedRev, "p_expected_state": string(expected), "p_next_state": string(next),
		}); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "stale") {
				return expectedRev, ErrStaleDispatch
			}
			if strings.Contains(msg, "run_stop_fence") {
				return expectedRev, ErrRunStopFence{}
			}
			log.Printf("[dispatch] rpc CASAdvance: %v (memory)", err)
		}
	}
	return s.memoryDispatchStore.CASAdvance(ctx, runID, turnID, expectedRev, expected, next, mutate)
}

func (s *supabaseRPCDispatchStore) GetRunProtocolVersion(ctx context.Context, runID string) (int, error) {
	if s.rpcEnabled() {
		b, err := s.callRPC(ctx, "dispatch_get_run_protocol_version", map[string]any{"p_run_id": runID})
		if err == nil {
			var ver int
			if json.Unmarshal(b, &ver) == nil {
				return ver, nil
			}
		}
	}
	return s.memoryDispatchStore.GetRunProtocolVersion(ctx, runID)
}

func (s *supabaseRPCDispatchStore) RequestRunStop(ctx context.Context, runID string, expectedRunStopRev int64, reason StopReason) (RunStopState, error) {
	if s.rpcEnabled() {
		if _, err := s.callRPC(ctx, "dispatch_request_run_stop", map[string]any{
			"p_run_id": runID, "p_expected_rev": expectedRunStopRev, "p_reason": string(reason),
		}); err != nil {
			log.Printf("[dispatch] rpc RequestRunStop: %v (memory)", err)
		}
	}
	return s.memoryDispatchStore.RequestRunStop(ctx, runID, expectedRunStopRev, reason)
}

var _ DispatchStore = (*supabaseRPCDispatchStore)(nil)
