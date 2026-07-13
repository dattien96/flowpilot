package changecontract

import "testing"

func TestSpecDriftedFalseWhenNoGoverningDocs(t *testing.T) {
	dir := t.TempDir()
	h := CanonicalHead{FeatureKey: "calc-core"}
	if SpecDrifted(dir, h) {
		t.Fatal("expected no drift when the Head has no governing docs")
	}
}

func TestSpecDriftedFalseWhenDocUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeGoverningDoc(t, dir, "SS-14-Code-Context-And-Regression-Safety", "spec content v1")

	h := BuildHeadForDriftTest(t, dir)
	if SpecDrifted(dir, h) {
		t.Fatal("expected no drift when the governing doc has not changed")
	}
}

func TestSpecDriftedTrueWhenDocChanges(t *testing.T) {
	dir := t.TempDir()
	writeGoverningDoc(t, dir, "SS-14-Code-Context-And-Regression-Safety", "spec content v1")
	h := BuildHeadForDriftTest(t, dir)

	writeGoverningDoc(t, dir, "SS-14-Code-Context-And-Regression-Safety", "spec content v2 - changed")

	if !SpecDrifted(dir, h) {
		t.Fatal("expected drift after the governing doc content changed")
	}
}

func TestSpecDriftedTrueWhenDocDeleted(t *testing.T) {
	dir := t.TempDir()
	h := CanonicalHead{
		FeatureKey:         "calc-core",
		GoverningDocIDs:    []string{"SS-14-Code-Context-And-Regression-Safety"},
		GoverningDocHashes: map[string]string{"SS-14-Code-Context-And-Regression-Safety": "somehash"},
	}
	if !SpecDrifted(dir, h) {
		t.Fatal("expected drift (missing doc) not a panic when the governing doc file does not exist")
	}
}

func TestCodeDriftedTrueOnlyWhenOutOfContractAndNotSpecDrifted(t *testing.T) {
	if !CodeDrifted(true, false) {
		t.Fatal("expected code_drifted=true for an out-of-contract change with no spec drift")
	}
	if CodeDrifted(true, true) {
		t.Fatal("expected spec-drift to take precedence: code_drifted should be false when spec also drifted")
	}
	if CodeDrifted(false, false) {
		t.Fatal("expected code_drifted=false when there is no out-of-contract change")
	}
}

// BuildHeadForDriftTest mints a minimal spec-backed Head for drift tests
// without pulling in changeledger/featurecatalog test scaffolding.
func BuildHeadForDriftTest(t *testing.T, workspace string) CanonicalHead {
	t.Helper()
	docID := "SS-14-Code-Context-And-Regression-Safety"
	h := CanonicalHead{
		FeatureKey:      "calc-core",
		GoverningDocIDs: []string{docID},
	}
	h.GoverningDocHashes = hashGoverningDocs(workspace, h.GoverningDocIDs)
	return h
}
