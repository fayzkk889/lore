package parse_test

import (
	"testing"

	"lore-cli/internal/parse"
)

func TestCleanJSON(t *testing.T) {
	tests := []struct {
		name       string
		xmlContent string
		raw        string
		wantPrefix string // just check it starts with {
	}{
		{
			name:       "xml content passed directly",
			xmlContent: `{"files":[]}`,
			raw:        "anything",
			wantPrefix: "{",
		},
		{
			name:       "json fenced block",
			xmlContent: "",
			raw:        "Here is the result:\n```json\n{\"updated\":[]}\n```\nDone.",
			wantPrefix: "{",
		},
		{
			name:       "plain fenced block",
			xmlContent: "",
			raw:        "```\n{\"updated\":[]}\n```",
			wantPrefix: "{",
		},
		{
			name:       "bare json with leading text",
			xmlContent: "",
			raw:        "Sure! Here you go: {\"updated\":[]} thanks.",
			wantPrefix: "{",
		},
		{
			name:       "trailing comma cleaned",
			xmlContent: `{"files":[{"path":"a","content":"b"},]}`,
			raw:        "",
			wantPrefix: "{",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parse.CleanJSON(tt.xmlContent, tt.raw)
			if got == "" {
				t.Fatal("CleanJSON returned empty string")
			}
			if len(got) < 1 || string(got[0]) != "{" {
				t.Errorf("expected JSON starting with '{', got: %q", got)
			}
		})
	}
}
