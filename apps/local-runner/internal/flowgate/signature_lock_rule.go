package flowgate

import (
	"fmt"
	"strings"
)

// SignatureLockRuleID is the CP-67 P-2 (Task-379) signature-lock gate. Like
// r-reproduce it is deliberately NOT part of DefaultRules() — the runner
// appends it for a coder turn on a scaffold-TDD flow only (see
// EnabledSignatureLockRules), so every pre-CP-67 workspace keeps its exact
// rule set byte-identical.
const SignatureLockRuleID = "r-signature-lock"

// SignatureLockRule forces a coder turn to leave the scaffold architect's
// API contract untouched: the canonical signature hash computed before the
// turn must equal the one computed after it. Bypass is legal ONLY through a
// batch renegotiation (CoderRenegotiating=true, the buffered
// submit_coder_outcome batch) — the Main Agent mediates those; a silent
// in-file signature edit is never legal (B-8.1: no "additive change"
// exemption — added or removed helper functions shift the hash too).
func SignatureLockRule() Rule {
	return Rule{
		ID:             SignatureLockRuleID,
		Scope:          "step",
		Trigger:        "signature_modified",
		RequiredOutput: "signatures_unchanged",
		Action:         "reprompt",
		Enabled:        true,
	}
}

// EnabledSignatureLockRules returns base with r-signature-lock appended when
// the caller decided this coder turn is signature-locked (a scaffold-TDD flow
// whose frozen contract carries a SignatureHash). Idempotent.
func EnabledSignatureLockRules(base []Rule, expected bool) []Rule {
	if !expected {
		return base
	}
	for _, r := range base {
		if r.ID == SignatureLockRuleID {
			return base
		}
	}
	return append(append([]Rule(nil), base...), SignatureLockRule())
}

// checkSignatureLockRule is the r-signature-lock evaluator.
//
//   - no snapshot (pre-CP-67 contract, or a non-coder turn) → nil
//   - hashes match (body-only edits)                            → nil
//   - hashes differ + CoderRenegotiating                        → nil (hub mediates)
//   - hashes differ, silent                                     → violation
func checkSignatureLockRule(rule Rule, tr TurnResult) *Violation {
	if tr.SignatureHashBefore == "" || tr.SignatureHashAfter == "" {
		return nil
	}
	if tr.SignatureHashBefore == tr.SignatureHashAfter {
		return nil
	}
	if tr.CoderRenegotiating {
		return nil
	}
	detail := fmt.Sprintf("signature lock violated: canonical signature hash changed %s..%s -> %s..%s — fill bodies only; never add, remove, or modify a declaration. Any signature change must ride ONE batched renegotiation (submit_coder_outcome status=renegotiate_signatures)",
		tr.SignatureHashBefore[:8], tr.SignatureHashBefore[len(tr.SignatureHashBefore)-8:],
		tr.SignatureHashAfter[:8], tr.SignatureHashAfter[len(tr.SignatureHashAfter)-8:])
	if len(tr.SignatureDrift) > 0 {
		detail += "; drifted declarations: " + strings.Join(tr.SignatureDrift, ", ")
	}
	return &Violation{Rule: rule, Detail: detail}
}
