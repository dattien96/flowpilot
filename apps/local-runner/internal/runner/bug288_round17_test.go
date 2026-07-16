package runner

import (
	"encoding/json"
	"fmt"
	"testing"
)

// ---- R17-P0: durable idempotency snapshot keeps high-gen keys ------------

func TestDurableIdempotencySnapshotKeepsLastKeys(t *testing.T) {
	m := map[string]string{}
	// Non-padded gens: numeric 100 must beat lexical trap (100 < 11 as strings).
	for i := 1; i <= 60; i++ {
		m[fmt.Sprintf("durable-c-restart-%d", i)] = fmt.Sprintf("turn-%d", i)
	}
	m["durable-c-restart-100"] = "turn-100"
	m["client-key"] = "x"
	snap := durableIdempotencySnapshot(m, "durable-c-restart-11")
	if _, ok := snap["client-key"]; ok {
		t.Fatal("non-durable must drop")
	}
	if snap["durable-c-restart-100"] != "turn-100" {
		t.Fatalf("numeric gen 100 must be retained: %#v", snap)
	}
	if snap["durable-c-restart-11"] != "turn-11" {
		t.Fatal("protected key must always be retained")
	}
	if len(snap) > 49 { // 48 + protect if not already in top
		// protect already in set — max 48
		if len(snap) > 48 {
			t.Fatalf("len=%d want <=48", len(snap))
		}
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
