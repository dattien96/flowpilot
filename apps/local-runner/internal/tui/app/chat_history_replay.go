package app

import (
	"context"

	"flowpilot-runner/internal/tui/client"
)

// Task-290 Q-1: tail SSE replay — avoid streaming from seq 0 on long chat open.

const (
	chatReplayTailEventBudget  = 400
	chatReplayChunkEventBudget = 400
	chatReplayMaxEvents        = 8000
)

func chatReplayTailAfterSeq(lastEventSeq int64) int64 {
	if lastEventSeq <= chatReplayTailEventBudget {
		return 0
	}
	return lastEventSeq - chatReplayTailEventBudget
}

func chatReplayChunkBefore(floor int64) int64 {
	if floor <= chatReplayChunkEventBudget {
		return 0
	}
	return floor - chatReplayChunkEventBudget
}

func collectReplayEvents(ctx context.Context, cl *client.Client, runID string, after, until int64, maxEvents int) []client.ProviderEvent {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := cl.StreamRun(ctx, runID, after)
	var collected []client.ProviderEvent
	for ev := range ch {
		if until > 0 && ev.Seq > until {
			break
		}
		collected = append(collected, ev)
		if until > 0 && ev.Seq >= until {
			break
		}
		if maxEvents > 0 && len(collected) >= maxEvents {
			break
		}
	}
	return collected
}

// historyCursorAfterReplay returns the exclusive upper bound of events still
// on the server. When trim drops a mid-turn prefix, the cursor is the seq
// before the first kept event so the next chunk re-fetches that prefix.
func historyCursorAfterReplay(requestedAfter int64, collected, trimmed []client.ProviderEvent) int64 {
	if requestedAfter <= 0 {
		return 0
	}
	if len(trimmed) > 0 && len(trimmed) < len(collected) {
		cur := trimmed[0].Seq - 1
		if cur < 0 {
			return 0
		}
		return cur
	}
	return requestedAfter
}

func trimEventsFromTurnStart(evs []client.ProviderEvent) []client.ProviderEvent {
	for i, ev := range evs {
		if ev.Type == "turn_started" {
			return evs[i:]
		}
	}
	return evs
}

func prependReplayMessages(older, existing []ChatMessage) []ChatMessage {
	if len(older) == 0 {
		return existing
	}
	if len(existing) == 0 {
		return older
	}
	out := make([]ChatMessage, 0, len(older)+len(existing))
	out = append(out, older...)
	out = append(out, existing...)
	return out
}
