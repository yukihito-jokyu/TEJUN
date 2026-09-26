package acp

import (
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

func TestTakeBriefSuggestion(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{"valid", "一緒に準備します。\n```tejun-preparation\n{\"purpose\":\"デプロイ手順を共有する\"}\n```", "デプロイ手順を共有する"},
		{"plan", "案を作ります。\n```tejun-preparation\n{\"checkItems\":[{\"title\":\"起動\",\"expectedResult\":\"起動する\"}]}\n```", "plan"},
		{"invalid", "```tejun-preparation\n{broken}\n```", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := []application.ConversationItem{{Role: "agent", Content: []application.ContentPart{{Type: "text", Text: tc.text}}}}
			got := takeBriefSuggestion(messages)
			if tc.want == "" && got != nil || tc.want == "plan" && (got == nil || len(got.CheckItems) != 1) || tc.want != "" && tc.want != "plan" && (got == nil || got.Purpose != tc.want) {
				t.Fatalf("suggestion=%+v", got)
			}
			if tc.name == "valid" && messages[0].Content[0].Text != "一緒に準備します。" {
				t.Fatalf("visible message=%q", messages[0].Content[0].Text)
			}
		})
	}
}
