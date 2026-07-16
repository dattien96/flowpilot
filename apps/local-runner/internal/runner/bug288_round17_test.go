package runner

import (
	"encoding/json"
	"fmt"
	"testing"
)

// ---- R17-P0: durable idempotency snapshot keeps high-gen keys ------------

func TestDurableIdempotencySnapshotKeepsLastKeys(t *testing.T) {
	m := map[string]string{}
	for i := 0; i < 60; i++ {
		m[fmt.Sprintf("durable-c-restart-%02d", i)] = fmt.Sprintf("turn-%d", i)
	}
	m["client-key"] = "x"
	snap := durableIdempotencySnapshot(m)
	if len(snap) != 48 {
		t.Fatalf("len=%d want 48", len(snap))
	}
	if _, ok := snap["client-key"]; ok {
		t.Fatal("non-durable must drop")
	}
	// Highest gens (lex last) must remain.
	if snap["durable-c-restart-59"] != "turn-59" {
		t.Fatalf("missing high gen: %#v", snap)
	}
	if _, ok := snap["durable-c-restart-00"]; ok {
		t.Fatal("lowest gen should be pruned when over cap")
	}
}

// ---- R17-P0: Supabase never marshals null idempotency_keys ---------------

func TestIdempotencyKeysOrEmptyNeverNil(t *testing.T) {
	if idempotencyKeysOrEmpty(nil) == nil {
		t.Fatal("nil in must not stay nil")
	}
	b, err := json.Marshal(idempotencyKeysOrEmpty(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Fatalf("got %s want {}", b)
	}
}

// ---- R17-P1: marker re-activate switches mint secret ----------------------

func TestMarkerInitActivatesDirSecretForMint(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	InitRunMarkerSecretFromDir(d1)
	a1 := runMarkerMAC("fcp", "x")
	InitRunMarkerSecretFromDir(d2)
	a2 := runMarkerMAC("fcp", "x")
	// Different dirs get different secrets with high probability.
	// Re-init d1 restores a1.
	InitRunMarkerSecretFromDir(d1)
	if runMarkerMAC("fcp", "x") != a1 {
		t.Fatal("activate d1 must restore d1 mint")
	}
	_ = a2
}
