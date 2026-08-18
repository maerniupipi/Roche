package unifiedqa

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/logger"
)

// progressReporter streams user-facing workflow progress through the existing
// thinking SSE contract. It never forwards model reasoning or chain-of-thought.
type progressReporter struct {
	mu         sync.Mutex
	emitMu     sync.Mutex
	eventBus   *event.EventBus
	runID      string
	sessionID  string
	requestID  string
	eventID    string
	startedAt  time.Time
	started    bool
	closed     bool
	completion progressCompletion
	interval   time.Duration
}

type progressStep struct {
	Lane    string
	Stage   string
	AgentID string
	Content string
}

type progressCompletion struct {
	Status      string
	ResultCount int
	ToolCalls   int
	ModelCalls  int
}

func newProgressReporter(eventBus *event.EventBus, runID, sessionID, requestID string, intervals ...time.Duration) *progressReporter {
	var interval time.Duration
	if len(intervals) > 0 {
		interval = intervals[0]
	}
	return &progressReporter{
		eventBus: eventBus, runID: runID, sessionID: sessionID, requestID: requestID,
		eventID: runID + "-thinking", interval: interval,
	}
}

// Update keeps the legacy call shape while producing a structured workflow
// step. New call sites should use Begin so stage and agent identity are stable.
func (p *progressReporter) Update(ctx context.Context, content string) {
	p.Begin(ctx, progressStep{Lane: "workflow", Stage: "workflow", Content: content})
}

// Begin appends another user-facing progress message to the single thinking
// stream. The shared event/step ID preserves the existing frontend contract:
// all thinking content is one block and only Close emits done=true.
func (p *progressReporter) Begin(ctx context.Context, step progressStep) {
	if p == nil || p.eventBus == nil || strings.TrimSpace(step.Content) == "" {
		return
	}
	p.emitMu.Lock()
	defer p.emitMu.Unlock()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if !p.started {
		p.startedAt = time.Now()
		p.started = true
	}
	p.mu.Unlock()

	_ = emitTextChunks(ctx, step.Content, false, p.interval, func(content string, done bool) error {
		return p.emit(ctx, p.eventID, event.AgentThoughtData{
			Content: content, Done: done, RunID: p.runID, StepID: p.eventID,
			Stage: step.Stage, AgentID: step.AgentID, Status: "running",
		})
	})
}

func (p *progressReporter) Complete(ctx context.Context, lane string, completion progressCompletion) {
	if p == nil || p.eventBus == nil {
		return
	}
	p.mu.Lock()
	if !p.closed {
		p.completion = completion
	}
	p.mu.Unlock()
}

func (p *progressReporter) Close(ctx context.Context) {
	if p == nil || p.eventBus == nil {
		return
	}
	p.emitMu.Lock()
	defer p.emitMu.Unlock()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	started := p.started
	completion := p.completion
	startedAt := p.startedAt
	if completion.Status == "" {
		completion.Status = "completed"
	}
	p.mu.Unlock()
	if started {
		_ = p.emit(ctx, p.eventID, event.AgentThoughtData{
			Done: true, RunID: p.runID, StepID: p.eventID,
			Stage: "thinking", Status: completion.Status,
			ResultCount: completion.ResultCount, ToolCalls: completion.ToolCalls,
			ModelCalls: completion.ModelCalls, DurationMS: time.Since(startedAt).Milliseconds(),
		})
	}
}

func (p *progressReporter) emit(ctx context.Context, eventID string, data event.AgentThoughtData) error {
	if err := p.eventBus.Emit(ctx, event.Event{
		ID: eventID, Type: event.EventAgentThought, SessionID: p.sessionID, RequestID: p.requestID, Data: data,
	}); err != nil {
		// Progress observability must never fail the knowledge-QA request.
		logger.Warnf(ctx, "unified QA progress event failed: %v", err)
		return err
	}
	return nil
}

func routeSelectionProgressMessage(catalog *AgentCatalog, tasks []AgentTask) string {
	names := make([]string, 0, len(tasks))
	for _, task := range tasks {
		name := catalog.NameOf(task.AgentID)
		if name != "" && !containsString(names, name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "已完成问题分析。\n"
	}
	return "已识别需要由" + strings.Join(names, "和") + "分别核对授权知识库。\n"
}

func domainProgressMessage(catalog *AgentCatalog, agentID string, stage DomainProgressStage) string {
	name := catalog.NameOf(agentID)
	switch stage {
	case DomainProgressRetrieving:
		return name + "正在检索授权知识库……\n"
	case DomainProgressReviewing:
		return name + "正在复核候选证据……\n"
	case DomainProgressRecovering:
		return "现有证据不足，" + name + "正在进行一次受限补查……\n"
	case DomainProgressReviewingRecovery:
		return name + "正在复核补查后的证据……\n"
	default:
		return ""
	}
}

func domainCompletionProgressMessage(catalog *AgentCatalog, agentID string, result DomainExecutionResult, err error) string {
	name := catalog.NameOf(agentID)
	if err != nil {
		return name + "的证据处理未完成，将仅使用已验证的信息继续回答。\n"
	}
	if result.Observation.Status == EvidenceStatusInsufficient {
		return fmt.Sprintf("%s已完成证据复核，本轮获得%d条候选证据，但证据仍不足。\n", name, len(result.Candidates))
	}
	return fmt.Sprintf("%s已完成证据复核，本轮获得%d条候选证据。\n", name, len(result.Candidates))
}

func aggregationProgressMessage(candidateCount, factCount int) string {
	if factCount == 0 {
		return fmt.Sprintf("已完成%d条候选证据的汇总，暂未确认可直接引用的事实依据。\n", candidateCount)
	}
	return fmt.Sprintf("已从%d条候选证据中确认%d条可引用事实，正在检查回答覆盖范围。\n", candidateCount, factCount)
}

func answerGenerationProgressMessage(factCount int) string {
	if factCount == 0 {
		return "正在根据已验证结果组织最终回答并校验引用……\n"
	}
	return fmt.Sprintf("正在根据%d条已验证事实组织最终回答并校验引用……\n", factCount)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
