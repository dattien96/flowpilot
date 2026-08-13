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

func collectReplayEvents(cl *client.Client, ctx context.Context, runID string, after, until int64, maxEvents int) []client.ProviderEvent {
	var collected []client.ProviderEvent
	for ev := range cl.StreamRun(ctx, runID, after) {
		if until > 0 && ev.Seq > until {
			break
		}
		collected = append(collected, ev)
		if maxEvents > 0 && len(collected) >= maxEvents {
			break
		}
	}
	return collected
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
