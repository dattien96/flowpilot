package app

import (
	"fmt"
	"strings"
	"time"

	"flowpilot-runner/internal/tui/client"
)

// Quota gate rendering (Task-450 / CP-87 P-6): the TUI renders the runner's
// candidate table verbatim — runner order is contractual, no re-ranking or
// re-parsing. Missing headroom renders "unknown", never a fake zero.

// renderQuotaCandidateTable renders the contractual columns: Provider,
// Model, Workload, Account, Headroom, Reset, Confidence, Reason. Candidates
// keep their runner order; rows in an open cooldown window get a
// determinate countdown line beneath them.
func renderQuotaCandidateTable(decision client.QuotaRouteDecision, width int) []string {
	var lines []string
	header := fmt.Sprintf("quota route required — %s/%s unusable (%s)",
		decision.ProviderKey, decision.AccountID, decision.Reason)
	lines = append(lines, truncateVisual(header, width))
	lines = append(lines, truncateVisual(
		fmt.Sprintf("  %-8s %-18s %-14s %-10s %-9s %-16s %-10s %s",
			"provider", "model", "workload", "account", "headroom", "reset", "confidence", "reason"), width))
	for _, c := range decision.Candidates {
		head := headroomCell(c.Headroom)
		conf := c.Headroom.Confidence
		if conf == "" {
			conf = "—"
		}
		reason := strings.Join(c.RejectionReasons, ",")
		if reason == "" {
			reason = "—"
		} else if c.AutoEligible {
			reason = "eligible," + reason
		}
		row := fmt.Sprintf("  %-8s %-18s %-14s %-10s %-9s %-16s %-10s %s",
			c.ProviderKey, dashOr(c.Model), dashOr(c.WorkloadClass), c.AccountID,
			head, dashOr(shortReset(c.Headroom.ResetAt)), conf, reason)
		lines = append(lines, truncateVisual(row, width))
		if c.CooldownUntil != "" {
			lines = append(lines, "    "+renderQuotaCooldownBar(c.CooldownStartedAt, c.CooldownUntil, time.Now()))
		}
	}
	if len(decision.Candidates) == 0 {
		lines = append(lines, "  (no candidates)")
	}
	return lines
}

// renderQuotaCooldownBar renders the determinate same-provider cooldown bar:
// a fixed window from the server's StartedAt/Until stamps — remounts resume
// at the true remaining time, never restart at full width.
func renderQuotaCooldownBar(startedAt, until string, now time.Time) string {
	untilT, err := time.Parse(time.RFC3339, until)
	if err != nil {
		return "safety cooldown — waiting (same-IP provider risk controls)"
	}
	remaining := untilT.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	total := 20 * time.Second
	if st, err := time.Parse(time.RFC3339, startedAt); err == nil && untilT.After(st) {
		total = untilT.Sub(st)
	}
	frac := 1.0
	if total > 0 {
		frac = remaining.Seconds() / total.Seconds()
		if frac > 1 {
			frac = 1
		}
		if frac < 0 {
			frac = 0
		}
	}
	const barW = 20
	filled := int(frac * barW)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barW-filled)
	return fmt.Sprintf("[%s] %ds — safety cooldown: rapid same-provider account switching on one IP may trigger provider risk controls", bar, int(remaining.Seconds()))
}

// headroomCell renders headroom honestly: percent when exact, the state word
// otherwise — "unknown" is never rendered as a token count or zero.
func headroomCell(h client.AccountHeadroomInfo) string {
	if h.RemainingPercent != nil && h.State != "" && h.State != "unknown" {
		return fmt.Sprintf("%d%%", *h.RemainingPercent)
	}
	if h.State != "" {
		return h.State
	}
	return "unknown"
}

// dashOr renders an empty cell as "—" (orDash already exists rendering
// "(none)" — too wide for the table's column budget).
func dashOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// shortReset renders the headroom reset timestamp compactly (HH:MM when it
// parses), matching the table's column budget.
func shortReset(resetAt string) string {
	if resetAt == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, resetAt); err == nil {
		return t.Format("15:04")
	}
	return resetAt
}

// renderQuotaRoutingSettings renders the machine-global rotation policy for
// /quota — a faithful read of GET /client/quota-routing-settings (edits are
// Desktop Settings → Engine; the runner owns the document).
func renderQuotaRoutingSettings(s client.QuotaRoutingSettings, width int) []string {
	mode := s.Mode
	if mode == "" {
		mode = "manual"
	}
	lines := []string{
		truncateVisual(fmt.Sprintf("quota routing — mode: %s (policy v%d)", mode, s.PolicyVersion), width),
		truncateVisual(fmt.Sprintf("  provider priority: %s", dashOr(strings.Join(s.ProviderPriority, ", "))), width),
	}
	if len(s.ModelBindings) == 0 {
		lines = append(lines, truncateVisual("  model bindings: —", width))
	} else {
		lines = append(lines, truncateVisual("  model bindings:", width))
		for _, b := range s.ModelBindings {
			lines = append(lines, truncateVisual(fmt.Sprintf("    %s / %s → %s", b.ProviderKey, b.WorkloadClass, b.Model), width))
		}
	}
	lines = append(lines,
		truncateVisual(fmt.Sprintf("  headroom low threshold: %d%%", s.HeadroomLowPercent), width),
		truncateVisual(fmt.Sprintf("  telemetry TTL: %ds", s.TelemetryTTLSeconds), width),
		truncateVisual(fmt.Sprintf("  same-provider cooldown: %ds (same_provider_ip_safety)", s.SameProviderCooldownSeconds), width),
		truncateVisual("  edit in Desktop → Settings → Engine → Quota Routing", width),
	)
	return lines
}

// renderQuotaAudit renders the forensic route record for /quota audit —
// requested→resolved binding, reason, policy, headroom, est/max/actual usage.
func renderQuotaAudit(rec client.QuotaRoutingAuditRecord, width int) []string {
	tok := func(p *int64) string {
		if p == nil {
			return "—"
		}
		return fmt.Sprintf("%d", *p)
	}
	usage := "—"
	if rec.ActualUsage != nil {
		usage = fmt.Sprintf("%d", rec.ActualUsage.TotalTokens)
	}
	head := "—"
	if rec.Headroom != nil {
		head = rec.Headroom.State
		if rec.Headroom.RemainingPercent != nil {
			head += fmt.Sprintf(" %d%%", *rec.Headroom.RemainingPercent)
		}
		head += fmt.Sprintf(" (%s)", rec.Headroom.Confidence)
	}
	lines := []string{
		truncateVisual(fmt.Sprintf("quota audit — %s (policy v%d)", dashOr(rec.Outcome), rec.PolicyVersion), width),
		truncateVisual(fmt.Sprintf("  requested: %s / %s", dashOr(rec.FromProvider), dashOr(rec.FromAccount)), width),
		truncateVisual(fmt.Sprintf("  resolved:  %s / %s / %s", dashOr(rec.ToProvider), dashOr(rec.ToAccount), dashOr(rec.ToModel)), width),
		truncateVisual(fmt.Sprintf("  scope: %s · reason: %s", dashOr(rec.Scope), dashOr(rec.Reason)), width),
		truncateVisual(fmt.Sprintf("  headroom: %s", head), width),
		truncateVisual(fmt.Sprintf("  est prompt: %s · max usage: %s · actual: %s", tok(rec.EstPromptTokens), tok(rec.MaxUsageTokens), usage), width),
	}
	return lines
}
