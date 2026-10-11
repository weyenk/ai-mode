package app

import (
	"strings"
	"testing"
)

func TestTruncationNotice(t *testing.T) {
	for _, fr := range []string{"stop", "", "tool_calls"} {
		if n := truncationNotice("architect", 10240, fr); n != "" {
			t.Errorf("finish_reason=%q: got notice %q, want none", fr, n)
		}
	}
	n := truncationNotice("architect", 10240, "length")
	for _, want := range []string{"architect", "--max-tokens 10240", "incomplete", "20480", "not the same command"} {
		if !strings.Contains(n, want) {
			t.Errorf("notice %q missing %q", n, want)
		}
	}
}
