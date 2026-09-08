package flowgate

import "testing"

func TestComputeRequirementDrift_MissingAC(t *testing.T) {
	drift, detail := ComputeRequirementDrift(
		"## AC\n- AC-1 login\n- AC-2 timeout\n",
		"func TestLogin(t *testing.T) {}\n",
	)
	if !drift {
		t.Fatal("want drift")
	}
	if !containsAll(detail, "AC-1", "AC-2") && !containsAll(detail, "AC-2") {
		t.Fatalf("detail=%q", detail)
	}
}

func TestComputeRequirementDrift_Covered(t *testing.T) {
	drift, _ := ComputeRequirementDrift(
		"- AC-1 login\n",
		"func TestLogin(t *testing.T) {}\n// AC-1\n",
	)
	if drift {
		t.Fatal("covered AC must not drift")
	}
}

func TestComputeRequirementDrift_NoACs(t *testing.T) {
	drift, _ := ComputeRequirementDrift("just prose", "func TestFoo(t *testing.T) {}")
	if drift {
		t.Fatal("no AC ids → no drift")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !containsStr(s, p) {
			return false
		}
	}
	return true
}

func containsStr(s, p string) bool {
	return len(s) >= len(p) && (s == p || len(p) == 0 || stringIndex(s, p) >= 0)
}

func stringIndex(s, p string) int {
	for i := 0; i+len(p) <= len(s); i++ {
		if s[i:i+len(p)] == p {
			return i
		}
	}
	return -1
}
