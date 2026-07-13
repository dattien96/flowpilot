package changecontract

import "time"

// Retire action values (SD-21 §5 retire transitions).
const (
	RetireActionRenamed    = "renamed"
	RetireActionMerged     = "merged"
	RetireActionDeprecated = "deprecated"
)

// RetireHead retires h (SD-21 §5, Task-187 T-3): sets status/superseded_by/
// retired_at on h, and — for renamed/merged only, never deprecated — copies
// h's Decisions into each target Head so the negative knowledge a retiring
// feature accumulated is never lost. Callers are responsible for loading
// each target via LoadHead and saving both h and the returned targets via
// SaveHead; RetireHead only computes the new struct values (no I/O), mirroring
// UpdateHead/RebaselineWithSpec's pure-function shape. Retired Heads are
// never deleted — they stay on disk for provenance, just excluded from
// default packing (Task-188).
func RetireHead(h CanonicalHead, action string, targets []CanonicalHead) (CanonicalHead, []CanonicalHead) {
	now := time.Now().UTC()
	h.Status = action
	h.SupersededBy = append([]string(nil), headKeys(targets)...)
	h.RetiredAt = &now
	h.UpdatedAt = now

	if action != RetireActionRenamed && action != RetireActionMerged {
		return h, targets
	}

	updatedTargets := make([]CanonicalHead, len(targets))
	for i, target := range targets {
		target.Decisions = append(append([]Decision(nil), target.Decisions...), h.Decisions...)
		target.UpdatedAt = now
		target.IntentSignature = ComputeSignature(target)
		updatedTargets[i] = target
	}
	return h, updatedTargets
}

func headKeys(heads []CanonicalHead) []string {
	keys := make([]string, len(heads))
	for i, h := range heads {
		keys[i] = h.FeatureKey
	}
	return keys
}
