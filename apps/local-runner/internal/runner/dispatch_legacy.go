package runner

import (
	"strings"
)

// dispatchFromLegacyIdem maps pre-CP-51 idempotency encodings into DispatchRecord
// states (SD-24 §5.6 — ONE rule everywhere):
//
//	prep:<turnID>  → prepared  (caller may promote to uncertain if prompt log missing)
//	bare + terminal/lastTurnID corroboration → terminal_completed
//	bare without corroboration → uncertain
//
// Legacy records carry ProtocolVersion=1 and are resolve-only.
func dispatchFromLegacyIdem(runID, key, raw string, lastTurnID string, hasTerminal func(turnID string) bool) DispatchRecord {
	turnID, launched := parseDurableIdemValue(raw)
	r := DispatchRecord{
		ProtocolVersion: 1,
		TurnID:          turnID,
		RunID:           runID,
		OuterIntentKey:  key,
		Revision:        1,
	}
	switch {
	case !launched:
		// "prep:<turnID>" — envelope synthesized from the persisted
		// turnLogKindPrompt line by the caller; if that log line is missing,
		// the caller must set State=uncertain (we cannot prove WHAT would be re-sent).
		r.State = DispatchPrepared
	case lastTurnID == turnID || (hasTerminal != nil && hasTerminal(turnID)):
		r.State, r.Outcome = DispatchTerminalCompleted, "completed"
	default:
		// bare, no corroboration -> never assume accepted
		r.State = DispatchUncertain
	}
	return r
}

// dispatchFromLegacyIdemOnRun is a convenience over interactiveRun event state.
func dispatchFromLegacyIdemOnRun(rs *interactiveRun, key, raw string) DispatchRecord {
	if rs == nil {
		return dispatchFromLegacyIdem("", key, raw, "", nil)
	}
	return dispatchFromLegacyIdem(rs.id, key, raw, rs.lastTurnID, func(turnID string) bool {
		for _, ev := range rs.events {
			if ev.ProviderTurnID != turnID {
				continue
			}
			switch ev.Type {
			case EventTurnCompleted, EventTurnFailed:
				return true
			}
		}
		return false
	})
}

// promotePreparedIfMissingPrompt upgrades a legacy prepared record to uncertain
// when the prompt log line needed for envelope synthesis is absent.
func promotePreparedIfMissingPrompt(r *DispatchRecord, promptPresent bool) {
	if r == nil {
		return
	}
	if r.State == DispatchPrepared && !promptPresent {
		r.State = DispatchUncertain
	}
}

// legacyIdemKeys returns durable-* keys from a session snapshot.
func legacyIdemKeys(keys map[string]string) map[string]string {
	if keys == nil {
		return nil
	}
	out := make(map[string]string, len(keys))
	for k, v := range keys {
		if strings.HasPrefix(k, "durable-") {
			out[k] = v
		}
	}
	return out
}
