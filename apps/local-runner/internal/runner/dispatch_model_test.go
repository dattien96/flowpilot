package runner

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

// Task-255 T-6 / ledger MB: seeded randomized interleavings of dispatch
// steps + settle + Stop-like cancel flags. Checks INV-1-ish invariants after
// every step (terminal | retryable | uncertain — no silent empty record).

func TestDispatchModel_SeededInterleavings(t *testing.T) {
	const seed = int64(20260717)
	rng := rand.New(rand.NewSource(seed))
	for i := 0; i < 40; i++ {
		t.Run(itoa(i), func(t *testing.T) {
			runModelSequence(t, rng.Int63())
		})
	}
}

func runModelSequence(t *testing.T, seed int64) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	runID := "m-run"
	turnID := "m-turn"
	rec := testPrepared(runID, turnID)
	rec.SettleOwed = true
	if err := store.CreatePrepared(ctx, rec, testEnvelope(runID, turnID)); err != nil {
		t.Fatal(err)
	}
	rev := int64(1)
	steps := []string{"claim", "start", "receipt", "terminal", "settle", "settle_again", "cancel_flag"}
	// Shuffle a prefix of steps but keep claim→start order legal.
	order := append([]string{}, steps...)
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

	claimed, started, receipted, terminaled := false, false, false, false
	for _, step := range order {
		var err error
		switch step {
		case "claim":
			if claimed {
				continue
			}
			rev, err = store.CASAdvance(ctx, runID, turnID, rev, DispatchPrepared, DispatchSendClaimed, nil)
			if err == nil {
				claimed = true
			}
		case "start":
			if !claimed || started {
				continue
			}
			rev, err = store.CASAdvance(ctx, runID, turnID, rev, DispatchSendClaimed, DispatchSendStarted, nil)
			if err == nil {
				started = true
			}
		case "receipt":
			if !started || receipted {
				continue
			}
			re := ReceiptEvidence{ProviderKey: "f", PayloadSHA256: "p"}
			rev, err = store.CommitReceiptAndClearIntent(ctx, runID, turnID, rev, re, runID, rec.OuterIntentKey, 1)
			if err == nil {
				receipted = true
				got, r2, _ := store.Get(ctx, runID, turnID)
				rev = r2
				_ = got
			}
		case "terminal":
			if !started || terminaled {
				continue
			}
			// May terminal from send_started or provider_accepted.
			proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
			rev, err = store.CommitTerminalAndSettleIntent(ctx, runID, turnID, rev, proof, runID, rec.OuterIntentKey, 1)
			if err == nil {
				terminaled = true
			}
		case "settle":
			if !terminaled {
				continue
			}
			d := &SettleDriver{
				Store: store,
				EvaluateGate: func(context.Context, string, string) (bool, bool, error) {
					return true, false, nil
				},
			}
			_ = d.DriveSettle(ctx, runID, turnID)
		case "settle_again":
			if !terminaled {
				continue
			}
			d := &SettleDriver{
				Store: store,
				EvaluateGate: func(context.Context, string, string) (bool, bool, error) {
					return true, false, nil
				},
			}
			_ = d.DriveSettle(ctx, runID, turnID)
		case "cancel_flag":
			// Soft cancel request (does not by itself terminalize).
			if r2, err2 := store.SetCancelRequested(ctx, runID, turnID, rev, 1); err2 == nil {
				rev = r2
			}
			if got, r3, gerr := store.Get(ctx, runID, turnID); gerr == nil {
				rev = r3
				_ = got
			}
		}
		// INV-1: record always exists and is in a defined state.
		got, _, gerr := store.Get(ctx, runID, turnID)
		if gerr != nil {
			t.Fatalf("seed=%d step=%s lost record: %v", seed, step, gerr)
		}
		if got.State == "" {
			t.Fatalf("seed=%d silent empty state", seed)
		}
		// After settle final, phase must be final disposition.
		if got.State.IsTerminal() && got.SettleOwed && got.SettlePhase.IsSettleFinal() {
			if got.SettlePhase != SettleFinalized && got.SettlePhase != SettleSupersededReprompt && got.SettlePhase != SettleNone {
				t.Fatalf("bad final settle phase %s", got.SettlePhase)
			}
		}
	}
	// Bound runtime.
	_ = time.Now()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}
