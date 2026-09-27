package acp

import (
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

func TestTakeBriefSuggestion(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{"prompt example", preparationInstruction, "開発環境の構築手順を共有する"},
		{"valid", "一緒に準備します。\n```tejun-preparation\n{\"purpose\":\"デプロイ手順を共有する\"}\n```", "デプロイ手順を共有する"},
		{"plan", "案を作ります。\n```tejun-preparation\n{\"checkItems\":[{\"title\":\"起動\",\"expectedResult\":\"起動する\"}]}\n```", "plan"},
		{"question", "```tejun-preparation\r\n{\"question\":\"何の作業を手順書にしますか？\"}\r\n```", "question"},
		{"invalid", "```tejun-preparation\n{broken}\n```", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := []application.ConversationItem{
				{Role: "agent", Content: []application.ContentPart{{Type: "text", Text: tc.text}}},
			}

			got := takeBriefSuggestion(messages)
			if tc.want == "" && got != nil || tc.want == "plan" && (got == nil || len(got.CheckItems) != 1) ||
				tc.want == "question" && (got == nil || got.Question != "何の作業を手順書にしますか？") ||
				tc.want != "" && tc.want != "plan" && tc.want != "question" && (got == nil || got.Purpose != tc.want) {
				t.Fatalf("suggestion=%+v", got)
			}

			if tc.name == "valid" && messages[0].Content[0].Text != "一緒に準備します。" {
				t.Fatalf("visible message=%q", messages[0].Content[0].Text)
			}
		})
	}
}

func TestPreparationReplyOnlyShowsFieldGuidance(t *testing.T) {
	m := &Manager{newID: func() string { return "reply" }, now: func() time.Time { return time.Unix(0, 0) }}

	for _, tc := range []struct {
		name       string
		messages   []application.ConversationItem
		suggestion *application.PreparationBriefSuggestion
		want       string
	}{
		{"missing structure", []application.ConversationItem{{Role: "agent", Content: []application.ContentPart{{Type: "text", Text: "環境構築を進めました"}}}}, nil, preparationFallbackMessage},
		{"question", []application.ConversationItem{{Role: "agent", Content: []application.ContentPart{{Type: "text", Text: "何の作業ですか？"}}}}, &application.PreparationBriefSuggestion{Question: "何の作業ですか？"}, "何の作業ですか？"},
		{"filled", []application.ConversationItem{{Role: "agent", Content: []application.ContentPart{{Type: "text", Text: "READMEとTaskfileから前提を確認しました。"}}}}, &application.PreparationBriefSuggestion{Purpose: "デプロイ手順"}, "READMEとTaskfileから前提を確認しました。"},
		{"empty", nil, nil, preparationFallbackMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := m.preparationReply(tc.messages, tc.suggestion, "turn")
			if len(got) != 1 || len(got[0].Content) != 1 || got[0].Content[0].Text != tc.want {
				t.Fatalf("reply=%+v", got)
			}
		})
	}
}
