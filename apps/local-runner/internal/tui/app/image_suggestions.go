package app

import (
	"fmt"
	"strconv"
	"strings"

	"flowpilot-runner/internal/tui/client"
)

// imageSubcommands are the Tab/Enter picker rows for `/image <sub>`.
var imageSubcommands = []struct {
	name   string
	detail string
}{
	{"open", "Preview a pending image (then pick index)"},
	{"paste", "Attach from clipboard (Alt+V)"},
	{"list", "List pending + open manage panel"},
	{"clear", "Remove all pending images"},
	{"rm", "Remove one pending image (then pick index)"},
}

// parseImagePicker reports `/image` arg mode.
//
//	mode "sub"  — picking a subcommand (query may filter paste/open/…)
//	mode "open" — picking which pending image to open
//	mode "rm"   — picking which pending image to remove
func parseImagePicker(input string) (mode, query string, ok bool) {
	okPrefix, rest := parseSlashArgPrefix(input, "/image")
	if !okPrefix {
		return "", "", false
	}
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return "sub", "", true
	}
	head := strings.ToLower(parts[0])
	switch head {
	case "open", "view", "show":
		if len(parts) > 1 {
			return "open", strings.Join(parts[1:], " "), true
		}
		return "open", "", true
	case "rm", "remove", "del", "delete", "x":
		if len(parts) > 1 {
			return "rm", strings.Join(parts[1:], " "), true
		}
		return "rm", "", true
	default:
		// Partial subcommand filter, e.g. "/image o" → open
		return "sub", rest, true
	}
}

// filterImageSuggestions drives Tab pickers for /image (subcommand + open/rm targets).
func filterImageSuggestions(input string, pending []client.PromptAttachment) []suggestItem {
	mode, query, ok := parseImagePicker(input)
	if !ok {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	switch mode {
	case "open":
		return filterImageIndexSuggestions(pending, q, "image-open", "open")
	case "rm":
		return filterImageIndexSuggestions(pending, q, "image-rm", "remove")
	default: // "sub"
		out := make([]suggestItem, 0, len(imageSubcommands)+1)
		for _, sc := range imageSubcommands {
			if q != "" && !strings.HasPrefix(sc.name, q) && !strings.Contains(sc.name, q) {
				// also match detail lightly
				if !strings.Contains(strings.ToLower(sc.detail), q) {
					continue
				}
			}
			// "open"/"rm" expand to next picker (trailing space) like provider-action.
			kind := "image-sub"
			if sc.name == "open" || sc.name == "rm" {
				kind = "image-sub-next"
			}
			detail := sc.detail
			if (sc.name == "open" || sc.name == "rm" || sc.name == "list" || sc.name == "clear") && len(pending) == 0 {
				if sc.name == "paste" {
					// still useful
				} else if sc.name != "paste" {
					detail = sc.detail + " · (none pending)"
				}
			}
			if len(pending) > 0 && (sc.name == "open" || sc.name == "list" || sc.name == "rm" || sc.name == "clear") {
				detail = fmt.Sprintf("%s · %d pending", sc.detail, len(pending))
			}
			out = append(out, suggestItem{value: sc.name, detail: detail, kind: kind})
		}
		return out
	}
}

func filterImageIndexSuggestions(pending []client.PromptAttachment, q, kind, verb string) []suggestItem {
	if len(pending) == 0 {
		return []suggestItem{{value: "", detail: "(no pending images — Alt+V / /image paste first)", kind: kind}}
	}
	out := make([]suggestItem, 0, len(pending))
	for i, a := range pending {
		n := strconv.Itoa(i + 1)
		name := a.OriginalName
		if name == "" {
			name = fmt.Sprintf("image-%d", i+1)
		}
		hay := strings.ToLower(n + " " + name + " " + a.MimeType)
		if q != "" && !strings.Contains(hay, q) && !strings.HasPrefix(n, q) {
			continue
		}
		detail := fmt.Sprintf("%s · %s %dx%d %s", verb, a.MimeType, a.Width, a.Height, humanBytes(a.SizeBytes))
		out = append(out, suggestItem{value: n, detail: name + " · " + detail, kind: kind})
	}
	if len(out) == 0 {
		return []suggestItem{{value: "", detail: "(no matching image)", kind: kind}}
	}
	return out
}
