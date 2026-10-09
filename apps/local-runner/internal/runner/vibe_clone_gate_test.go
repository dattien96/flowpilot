package runner

import (
	"context"
	"testing"
)

// TestAllowVibeCloneStart pins the clone-keeps-its-role contract: a
// user-owned flow cloned from a vibe built-in (user-startable or system)
// may start with user kind in vibe mode. Everything else stays rejected.
func TestAllowVibeCloneStart(t *testing.T) {
	ctx := context.Background()
	store := newFakeFlowDefinitionStore()
	tasksMirror := FlowDefinitionRecord{
		FlowRef:    "flowpilot-core-flow-pack/vibe-tasks",
		Source:     "supabase_builtin_mirror",
		PackID:     "flowpilot-core-flow-pack",
		PackFlowID: "vibe-tasks",
	}
	sprintMirror := FlowDefinitionRecord{
		FlowRef:    "flowpilot-core-flow-pack/vibe-sprint",
		Source:     "supabase_builtin_mirror",
		PackID:     "flowpilot-core-flow-pack",
		PackFlowID: "vibe-sprint",
	}
	// Mirror rows are addressable by row UUID too: cloned_from points at
	// the source row id, and GetByRef resolves UUIDs by id.
	store.byRef[tasksMirror.FlowRef] = tasksMirror
	store.byRef["mirror-uuid-tasks"] = tasksMirror
	store.byRef[sprintMirror.FlowRef] = sprintMirror
	store.byRef["mirror-uuid-sprint"] = sprintMirror
	store.byRef["clone-uuid-tasks"] = FlowDefinitionRecord{
		FlowRef:    "clone-uuid-tasks",
		Source:     "supabase_user_definition",
		Editable:   true,
		Cloneable:  true,
		ClonedFrom: "mirror-uuid-tasks",
	}
	store.byRef["clone-uuid-tasks-copy"] = FlowDefinitionRecord{
		FlowRef:    "clone-uuid-tasks-copy",
		Source:     "supabase_user_definition",
		Editable:   true,
		Cloneable:  true,
		ClonedFrom: "clone-uuid-tasks",
	}
	store.byRef["clone-uuid-sprint"] = FlowDefinitionRecord{
		FlowRef:    "clone-uuid-sprint",
		Source:     "supabase_user_definition",
		Editable:   true,
		Cloneable:  true,
		ClonedFrom: "mirror-uuid-sprint",
	}
	store.byRef["plain-uuid"] = FlowDefinitionRecord{
		FlowRef:   "plain-uuid",
		Source:    "supabase_user_definition",
		Editable:  true,
		Cloneable: true,
	}

	cases := []struct {
		mode  string
		ref   string
		store FlowDefinitionStore
		want  bool
	}{
		{"vibe", "clone-uuid-tasks", store, true},
		{"vibe", "clone-uuid-tasks-copy", store, true},
		{"vibe", "clone-uuid-sprint", store, true},
		// Mirror rows themselves never inherit (CA-1078 laundering pin).
		{"vibe", "mirror-uuid-tasks", store, false},
		{"vibe", "mirror-uuid-sprint", store, false},
		{"vibe", "plain-uuid", store, false},
		{"vibe", "missing-uuid", store, false},
		{"vibe", "clone-uuid-tasks", nil, false},
		{"dev", "clone-uuid-tasks", store, false},
	}
	for _, tc := range cases {
		if got := allowVibeCloneStart(ctx, tc.store, tc.mode, tc.ref); got != tc.want {
			t.Errorf("allowVibeCloneStart(%q, %q) = %v, want %v", tc.mode, tc.ref, got, tc.want)
		}
	}
}

// TestVibeCloneRootPackID pins the ancestry root resolver backing the
// chat-orchestration deferral: any mirror-rooted clone chain resolves to
// its root pack id; mirrors themselves never inherit (CA-1078).
func TestVibeCloneRootPackID(t *testing.T) {
	ctx := context.Background()
	store := newFakeFlowDefinitionStore()
	store.byRef["flowpilot-core-flow-pack/vibe-tasks"] = FlowDefinitionRecord{
		FlowRef:    "flowpilot-core-flow-pack/vibe-tasks",
		Source:     "supabase_builtin_mirror",
		PackID:     "flowpilot-core-flow-pack",
		PackFlowID: "vibe-tasks",
	}
	store.byRef["mirror-uuid-tasks"] = FlowDefinitionRecord{
		FlowRef:    "flowpilot-core-flow-pack/vibe-tasks",
		Source:     "supabase_builtin_mirror",
		PackID:     "flowpilot-core-flow-pack",
		PackFlowID: "vibe-tasks",
	}
	store.byRef["clone-uuid-1"] = FlowDefinitionRecord{
		FlowRef:    "clone-uuid-1",
		Source:     "supabase_user_definition",
		ClonedFrom: "mirror-uuid-tasks",
	}
	store.byRef["plain-uuid"] = FlowDefinitionRecord{
		FlowRef:  "plain-uuid",
		Source:   "supabase_user_definition",
		Editable: true,
	}

	cases := []struct {
		ref   string
		store FlowDefinitionStore
		want  string
	}{
		{"clone-uuid-1", store, "vibe-tasks"},
		{"mirror-uuid-tasks", store, ""},
		{"flowpilot-core-flow-pack/vibe-tasks", store, ""},
		{"plain-uuid", store, ""},
		{"missing-uuid", store, ""},
		{"clone-uuid-1", nil, ""},
	}
	for _, tc := range cases {
		if got := vibeCloneRootPackID(ctx, tc.store, tc.ref); got != tc.want {
			t.Errorf("vibeCloneRootPackID(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}
}
