package handler

import (
	"strings"
	"testing"
)

func TestNormaliseAgentConversationStarters(t *testing.T) {
	t.Run("trims complete prompts", func(t *testing.T) {
		got, err := normaliseAgentConversationStarters([]AgentConversationStarter{{
			Label:  "  Review a PR  ",
			Prompt: "  Review the open pull request.  ",
		}})
		if err != nil {
			t.Fatalf("normaliseAgentConversationStarters() error = %v", err)
		}
		want := []AgentConversationStarter{{Label: "Review a PR", Prompt: "Review the open pull request."}}
		if len(got) != 1 || got[0] != want[0] {
			t.Fatalf("normaliseAgentConversationStarters() = %#v, want %#v", got, want)
		}
	})

	for name, prompts := range map[string][]AgentConversationStarter{
		"too many": {
			{Label: "One", Prompt: "One"},
			{Label: "Two", Prompt: "Two"},
			{Label: "Three", Prompt: "Three"},
			{Label: "Four", Prompt: "Four"},
		},
		"blank label":  {{Label: " ", Prompt: "Prompt"}},
		"blank prompt": {{Label: "Label", Prompt: " "}},
		"long label":   {{Label: strings.Repeat("a", maxAgentConversationStarterLabel+1), Prompt: "Prompt"}},
		"long prompt":  {{Label: "Label", Prompt: strings.Repeat("a", maxAgentConversationStarterLength+1)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normaliseAgentConversationStarters(prompts); err == nil {
				t.Fatal("normaliseAgentConversationStarters() error = nil, want validation error")
			}
		})
	}
}
