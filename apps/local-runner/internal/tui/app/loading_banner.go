package app

import (
	"strings"
)

// Compact FlowPilot wordmark (fits ~56 cols). Animation waves a highlight across it.
var flowpilotWordmark = []string{
	`  _____ _                 ____  _ _       _   `,
	` |  ___| | _____      __ |  _ \(_) | ___ | |_ `,
	` | |_  | |/ _ \ \ /\ / / | |_) | | |/ _ \| __|`,
	` |  _| | | (_) \ V  V /  |  __/| | | (_) | |_ `,
	` |_|   |_|\___/ \_/\_/   |_|   |_|_|\___/ \__|`,
}

var flowpilotWordmarkASCII = []string{
	`  ##### #                 ####  # #       #   `,
	`  #     # ###   # # # #   #   # # # ###   ### `,
	`  ####  # # # # # # # # # ####  # # # # # # # `,
	`  #     # # # # #  # #    #     # # # # # # # `,
	`  #     # ### # #  # #    #     # # ### # ### `,
}

const loadingBannerHeight = 8

func renderFlowpilotLoader(frame int, asciiMode bool) string {
	art := flowpilotWordmark
	if asciiMode {
		art = flowpilotWordmarkASCII
	}
	width := 0
	for _, line := range art {
		if len(line) > width {
			width = len(line)
		}
	}
	if width < 40 {
		width = 40
	}

	// Horizontal shimmer: a bright window slides left→right across each row.
	win := 8
	pos := frame % (width + win)
	var sb strings.Builder
	for _, line := range art {
		padded := line
		if len(padded) < width {
			padded += strings.Repeat(" ", width-len(padded))
		}
		runes := []rune(padded)
		for i, r := range runes {
			inWave := i >= pos-win && i < pos
			if inWave {
				if asciiMode {
					if r == ' ' {
						sb.WriteByte('.')
					} else {
						sb.WriteRune(r)
					}
				} else {
					// Dense block under the wave for a “scanline” feel.
					switch r {
					case ' ':
						sb.WriteRune('·')
					default:
						sb.WriteRune(r)
					}
				}
			} else {
				sb.WriteRune(r)
			}
		}
		sb.WriteByte('\n')
	}

	barW := width - 4
	if barW < 12 {
		barW = 12
	}
	fill := (frame * 2) % (barW + 1)
	bar := strings.Repeat(ternStr(asciiMode, "#", "█"), fill) + strings.Repeat(ternStr(asciiMode, "-", "░"), barW-fill)
	sb.WriteString("  FlowPilot\n")
	sb.WriteString("  [")
	sb.WriteString(bar)
	sb.WriteString("]\n")
	sb.WriteString("  loading session · project · providers — chat locked")
	return sb.String()
}

func ternStr(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
