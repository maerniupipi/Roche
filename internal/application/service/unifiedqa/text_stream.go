package unifiedqa

import (
	"context"
	"time"

	"roche.local/knowledge-agent-platform/internal/event"
)

// Keep unified-QA text events close to the token-sized chunks produced by the
// smart-reasoning pipeline. Structural SSE events (references, milestones and
// complete) remain atomic.
const (
	unifiedQATextChunkRunes    = 4
	unifiedQATextChunkInterval = 40 * time.Millisecond
)

func emitTextChunks(
	ctx context.Context,
	content string,
	done bool,
	interval time.Duration,
	emit func(content string, done bool) error,
) error {
	chunks := splitTextChunks(content, unifiedQATextChunkRunes)
	if len(chunks) == 0 {
		if done {
			return emit("", true)
		}
		return nil
	}
	for index, chunk := range chunks {
		if index > 0 && interval > 0 {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ctx.Err()
			case <-timer.C:
			}
		}
		if err := emit(chunk, done && index == len(chunks)-1); err != nil {
			return err
		}
	}
	return nil
}

func splitTextChunks(content string, maxRunes int) []string {
	if content == "" || maxRunes <= 0 {
		return nil
	}
	runes := []rune(content)
	chunks := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for len(runes) > 0 {
		chunkSize := min(maxRunes, len(runes))
		chunks = append(chunks, string(runes[:chunkSize]))
		runes = runes[chunkSize:]
	}
	return chunks
}

func emitFinalAnswerChunks(
	ctx context.Context,
	eventBus *event.EventBus,
	eventID string,
	sessionID string,
	requestID string,
	content string,
	done bool,
	isFallback bool,
	interval time.Duration,
) error {
	return emitTextChunks(ctx, content, done, interval, func(chunk string, chunkDone bool) error {
		return eventBus.Emit(ctx, event.Event{
			ID: eventID, Type: event.EventAgentFinalAnswer, SessionID: sessionID, RequestID: requestID,
			Data: event.AgentFinalAnswerData{Content: chunk, Done: chunkDone, IsFallback: isFallback},
		})
	})
}
