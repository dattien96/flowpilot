package runner

import (
	"testing"
)

// BUG-389 runner half: reproduceFailuresExerciseTarget resolves the failing
// test to its file and checks whether the file calls a symbol declared in the
// step's frozen scope.
func TestBug389_FabricatedTestDoesNotExerciseDeclaredSymbol(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newReproduceFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})
	writeRepoFile(t, dir, "calc/calc.go", "package calc\n\nfunc Add(a, b int) int { return a + b }\n")
	// The fabricated RED from live run-4501: self-contained, never calls Add.
	writeRepoFile(t, dir, "calc/repro_test.go",
		"package calc\n\nimport \"testing\"\n\nfunc TestSimulatedBuggy(t *testing.T) {\n\tsimulatedBuggy := 3 + 4\n\tif simulatedBuggy != 12 { t.Fatalf(\"want 12\") }\n}\n")

	checked, hit := svc.reproduceFailuresExerciseTarget(dir, parentID, []string{"TestSimulatedBuggy"}, []string{"calc/repro_test.go"})
	if !checked {
		t.Fatal("test file resolved — the check must be checked=true")
	}
	if hit {
		t.Fatal("a self-contained fabricated failure must not exercise the declared symbol")
	}
}

func TestBug389_RealReproductionExercisesDeclaredSymbol(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newReproduceFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})
	writeRepoFile(t, dir, "calc/calc.go", "package calc\n\nfunc Add(a, b int) int { return a + b }\n")
	writeRepoFile(t, dir, "calc/repro_test.go",
		"package calc\n\nimport \"testing\"\n\nfunc TestAdd_Wrong(t *testing.T) {\n\tif Add(2, 3) != 6 { t.Fatalf(\"Add wrong\") }\n}\n")

	checked, hit := svc.reproduceFailuresExerciseTarget(dir, parentID, []string{"TestAdd_Wrong"}, []string{"calc/repro_test.go"})
	if !checked || !hit {
		t.Fatalf("genuine reproduction must be checked+hit; got checked=%v hit=%v", checked, hit)
	}
}

// No frozen contract / no Go declared files -> unchecked (typed degradation).
func TestBug389_NoContractDegradesToUnchecked(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc, parentID := newReproduceFixture(t, dir)
	checked, _ := svc.reproduceFailuresExerciseTarget(dir, parentID, []string{"TestX"}, []string{"calc/x_test.go"})
	if checked {
		t.Fatal("no frozen contract -> check must be skipped, not fabricated")
	}
}
