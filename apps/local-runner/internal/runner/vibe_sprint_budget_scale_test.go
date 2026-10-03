package runner

import "testing"

// Cap default contract (user requirement, live CP-03): the default max cap is
// 20 rounds per Task, so a CP's total cap is len(taskPlan) × 20 — CP-03 with
// 10 tasks has a total cap of 200. The sprint budget must scale with the
// detected plan so EVERY detected task triggers; a fixed budget of 8 silently
// stopped a 10-task CP after sprint 8.

func TestVibeSprintBudgetForPlan_ScalesWithTaskCount(t *testing.T) {
	if got := vibeSprintBudgetForPlan(10); got != 200 {
		t.Fatalf("10-task CP budget = %d, want 200 (10 tasks × 20 rounds)", got)
	}
	if got := vibeSprintBudgetForPlan(1); got != 20 {
		t.Fatalf("1-task CP budget = %d, want 20", got)
	}
	if got := vibeSprintBudgetForPlan(0); got != defaultVibeSprintBudget {
		t.Fatalf("empty plan budget = %d, want floor default %d", got, defaultVibeSprintBudget)
	}
}

func TestDecideNextVibeSprint_AllPlanTasksTrigger(t *testing.T) {
	plan := make([]string, 10)
	for i := range plan {
		plan[i] = "Task-x"
	}
	budget := vibeSprintBudgetForPlan(len(plan))
	for i := 0; i < len(plan); i++ {
		d := decideNextVibeSprint(false, plan, i, budget)
		if !d.Start {
			t.Fatalf("sprint %d of 10 did not trigger (budget=%d): %+v", i+1, budget, d)
		}
	}
	if d := decideNextVibeSprint(false, plan, len(plan), budget); !d.Done {
		t.Fatalf("after last task expected Done, got %+v", d)
	}
}
