package flowgate

import (
	"regexp"
	"strings"
)

var acIDPattern = regexp.MustCompile(`AC-\d+`)
var testFuncPattern = regexp.MustCompile(`func Test[A-Za-z0-9_]+`)

// ComputeRequirementDrift reports green-suite signature drift vs locked SS ACs.
func ComputeRequirementDrift(ssBody, testSource string) (bool, string) {
	acs := uniqueStrings(acIDPattern.FindAllString(ssBody, -1))
	if len(acs) == 0 {
		return false, ""
	}
	haystack := testSource
	for _, tfn := range testFuncPattern.FindAllString(testSource, -1) {
		haystack += " " + tfn
	}
	var missing []string
	for _, ac := range acs {
		if !strings.Contains(haystack, ac) {
			missing = append(missing, ac)
		}
	}
	if len(missing) == 0 {
		return false, ""
	}
	return true, FormatRequirementCard(missing)
}

// FormatRequirementCard is the non-tech r-requirement copy (which AC drifted + SS edit).
func FormatRequirementCard(missingACs []string) string {
	if len(missingACs) == 0 {
		return ""
	}
	return "Requirement drift: " + strings.Join(missingACs, ", ") +
		" have no matching test. SS edit that fixes it: add a test named after each AC, or remove/rewrite those AC lines in the locked SS."
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
