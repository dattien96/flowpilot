package app

import (
	"strings"
)

// initSubcommands are the Tab picker rows for `/init <kind>`.
var initSubcommands = []struct {
	name   string
	detail string
}{
	{"skill", "Install flow-pack skills for this project's platform"},
	{"all", "Full engine init — skills + scaffold + ledger + catalog"},
}

// filterInitSuggestions returns picker rows for `/init ...` (skill | all).
// Returns nil for non-/init input so the generic slash picker still wins.
func filterInitSuggestions(input string) []suggestItem {
	ok, query := parseSlashArgPrefix(input, "/init")
	if !ok {
		// Bare "/init" (no trailing space) should still show picker on Tab.
		if strings.EqualFold(strings.TrimSpace(input), "/init") {
			out := make([]suggestItem, 0, len(initSubcommands))
			for _, sc := range initSubcommands {
				out = append(out, suggestItem{value: sc.name, detail: sc.detail, kind: "init", slash: "/init"})
			}
			return out
		}
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]suggestItem, 0, len(initSubcommands))
	for _, sc := range initSubcommands {
		if q != "" && !strings.HasPrefix(sc.name, q) {
			continue
		}
		out = append(out, suggestItem{value: sc.name, detail: sc.detail, kind: "init", slash: "/init"})
	}
	return out
}
