package runner

import (
	"log"
	"os"
	"strings"
)

// OpenDispatchStoreForServe picks the production dispatch store:
//   - FLOWPILOT_TEST_SUPABASE_DSN / FLOWPILOT_DISPATCH_SUPABASE_DSN → Supabase RPC client
//     (falls back to memory projection if RPC unreachable at open — still registers schema)
//   - else local NDJSON at dataDir/dispatch.ndjson
//
// dataDir is typically <cwd>/.flowpilot/chats (same root as session store).
func OpenDispatchStoreForServe(dataDir string) (DispatchStore, error) {
	dsn := strings.TrimSpace(os.Getenv("FLOWPILOT_DISPATCH_SUPABASE_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("FLOWPILOT_TEST_SUPABASE_DSN"))
	}
	if dsn != "" {
		store, err := NewSupabaseDispatchStoreFromDSN(dsn)
		if err != nil {
			log.Printf("[dispatch] supabase DSN set but open failed: %v; falling back to local", err)
		} else {
			return store, nil
		}
	}
	if dataDir == "" {
		return NewMemoryDispatchStore(), nil
	}
	return NewLocalDispatchStore(dataDir)
}
