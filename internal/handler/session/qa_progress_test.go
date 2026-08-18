package session

import (
	"testing"

	"roche.local/knowledge-agent-platform/internal/event"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestAppendQuickAnswerProgressUpdatesStructuredStep(t *testing.T) {
	message := &types.Message{}
	appendQuickAnswerProgress(message, event.AgentThoughtData{
		Content: "正在检索", RunID: "run", StepID: "step", Stage: "retrieving", AgentID: "finance", Status: "running",
	})
	appendQuickAnswerProgress(message, event.AgentThoughtData{
		RunID: "run", StepID: "step", Stage: "retrieving", AgentID: "finance", Status: "completed",
		Done: true, ResultCount: 5, ToolCalls: 2, ModelCalls: 1, DurationMS: 1200,
	})

	if len(message.AgentSteps) != 1 {
		t.Fatalf("steps = %+v", message.AgentSteps)
	}
	step := message.AgentSteps[0]
	if step.ReasoningContent != "正在检索" || step.ProgressStatus != "completed" || step.ProgressResultCount != 5 ||
		step.ProgressToolCalls != 2 || step.ProgressModelCalls != 1 || step.Duration != 1200 {
		t.Fatalf("step = %+v", step)
	}
}

func TestAppendQuickAnswerProgressPersistsResponseType(t *testing.T) {
	message := &types.Message{}
	appendQuickAnswerProgressWithType(message, types.ResponseTypeQuestionUnderstood, event.AgentThoughtData{
		Content: "已完成问题理解", RunID: "run", StepID: "question", Stage: "question_understood",
		Status: "completed", Done: true,
	})

	if len(message.AgentSteps) != 1 {
		t.Fatalf("steps = %+v", message.AgentSteps)
	}
	if message.AgentSteps[0].ProgressResponseType != types.ResponseTypeQuestionUnderstood {
		t.Fatalf("response type = %q", message.AgentSteps[0].ProgressResponseType)
	}
}
