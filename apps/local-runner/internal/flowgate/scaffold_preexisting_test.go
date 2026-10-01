package flowgate

import (
	"strings"
	"testing"
)

// CA-1094 (bounded stubs): a scaffold turn that modified a production file
// which existed at contract freeze violates r-scaffold-red regardless of the
// suite outcome — including a declared zero-red (waived) contract.

func TestRuleScaffoldRedRejectsPreExistingFileModification(t *testing.T) {
	tr := scaffoldTurnResult()
	tr.ScaffoldPreExistingTouched = []string{"settings.gradle.kts"}
	v := checkScaffoldRedRule(ScaffoldRedRule(), tr)
	if v == nil {
		t.Fatal("a scaffold turn that modified a pre-existing declared file must violate r-scaffold-red")
	}
	if !strings.Contains(v.Detail, "settings.gradle.kts") {
		t.Fatalf("violation detail must name the touched files, got %q", v.Detail)
	}
}

func TestRuleScaffoldRedRejectsPreExistingFileModificationWhenWaived(t *testing.T) {
	tr := scaffoldTurnResult()
	tr.Tests.Failed = nil
	tr.ScaffoldRedWaived = true
	tr.ScaffoldPreExistingTouched = []string{"libs.versions.toml"}
	if v := checkScaffoldRedRule(ScaffoldRedRule(), tr); v == nil {
		t.Fatal("the waived-contract path must still reject modification of pre-existing files")
	}
}

func TestRuleScaffoldRedAllowsNewStubCreation(t *testing.T) {
	tr := scaffoldTurnResult()
	// No pre-existing files touched: only new declared files were created.
	if v := checkScaffoldRedRule(ScaffoldRedRule(), tr); v != nil {
		t.Fatalf("creating new stub files must stay legal, got %q", v.Detail)
	}
}
