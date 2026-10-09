package workingmode

import "testing"

// fakeAncestry models a cloned_from chain: ref -> (clonedFrom, packFlowID).
func fakeAncestry(rows map[string][2]string) FlowAncestryLookup {
	return func(ref string) (string, string, bool) {
		row, ok := rows[ref]
		if !ok {
			return "", "", false
		}
		return row[0], row[1], true
	}
}

func TestCloneInheritsVibeStart(t *testing.T) {
	lookup := fakeAncestry(map[string][2]string{
		// clone-1 is a user copy of the vibe-tasks mirror row.
		"clone-1":      {"mirror-tasks-uuid", ""},
		"mirror-tasks": {"", "vibe-tasks"},
		// clone-2 chains through another user copy (clone of a clone).
		"clone-2": {"clone-1", ""},
		// sprint-clone roots at a system-only flow: never user-startable.
		"sprint-clone":  {"mirror-sprint-uuid", ""},
		"mirror-sprint": {"", "vibe-sprint"},
		// plain-uuid is a user flow with no clone provenance at all.
		"plain-uuid": {"", ""},
		// orphan-clone points at a row that no longer exists.
		"orphan-clone": {"gone-uuid", ""},
		// cyc-a/cyc-b reference each other: must terminate, fail closed.
		"cyc-a": {"cyc-b", ""},
		"cyc-b": {"cyc-a", ""},
	})
	// NOTE: mirror rows resolve by pack ref in production, but the walker
	// only ever looks up UUIDs from cloned_from chains — the map above
	// uses short UUID stand-ins. Wire the clone entries to their mirrors.
	lookupWithMirrors := func(ref string) (string, string, bool) {
		if ref == "mirror-tasks-uuid" {
			return "", "vibe-tasks", true
		}
		if ref == "mirror-sprint-uuid" {
			return "", "vibe-sprint", true
		}
		return lookup(ref)
	}

	cases := []struct {
		ref  string
		want bool
	}{
		{"clone-1", true},
		{"clone-2", true},
		// A sprint clone keeps the system role: startable as a user-owned
		// copy (it is no longer the engine-mounted canonical id the main
		// gate pins as forbidden).
		{"sprint-clone", true},
		// Mirror rows themselves never inherit: the main gate owns their
		// refs (CA-1078 laundering pin).
		{"mirror-tasks-uuid", false},
		{"plain-uuid", false},
		{"orphan-clone", false},
		{"cyc-a", false},
		{"missing-uuid", false},
		{"", false},
		{"vibe-tasks", false}, // canonical ids are the main gate's job, not the walker's
	}
	for _, tc := range cases {
		if got := CloneInheritsVibeStart(tc.ref, lookupWithMirrors); got != tc.want {
			t.Errorf("CloneInheritsVibeStart(%q) = %v, want %v", tc.ref, got, tc.want)
		}
	}
	if CloneInheritsVibeStart("clone-1", nil) {
		t.Error("CloneInheritsVibeStart with nil lookup must fail closed")
	}
}
