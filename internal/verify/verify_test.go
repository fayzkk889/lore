package verify

import (
	"strings"
	"testing"
	"time"
)

func TestRunHTTPCheckRejectsEmptyRequests(t *testing.T) {
	step := runHTTPCheck(t.TempDir(), nil, Check{
		Type:         "http",
		StartCommand: "unused",
		Port:         8080,
	}, time.Second)

	if step.Passed {
		t.Fatal("empty-request HTTP check passed")
	}
	if !strings.Contains(step.Output, "at least one request") {
		t.Fatalf("output = %q, want empty request explanation", step.Output)
	}
}
