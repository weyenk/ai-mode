package app

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	if got := slugify("A web version of this app?!"); got != "a-web-version-of-this-app" {
		t.Fatalf("got %q", got)
	}
	if got := slugify("???"); got != "topic" {
		t.Fatalf("got %q", got)
	}
}

func TestTruncateLines(t *testing.T) {
	s := strings.Repeat("line of text\n", 100)
	got := truncateLines(s, 200)
	if len(got) > 230 || !strings.HasSuffix(got, "(truncated)") {
		t.Fatalf("got %d chars: %q", len(got), got)
	}
	if truncateLines("short", 200) != "short" {
		t.Fatal("short input changed")
	}
}

func TestTeamQuestion(t *testing.T) {
	q := teamQuestion("product", "web app", "/r", "ARCH FINDINGS")
	for _, want := range []string{"TOPIC: web app", "REPO: /r", "ARCH FINDINGS", "YOUR ANGLE (product)"} {
		if !strings.Contains(q, want) {
			t.Errorf("missing %q in %q", want, q)
		}
	}
	if strings.Contains(teamQuestion("architect", "x", "/r", ""), "ARCHITECT'S REPO FINDINGS") {
		t.Error("architect question should not carry a digest")
	}
	for _, r := range defaultTeam {
		if teamBriefs[r] == "" {
			t.Errorf("default role %s has no brief", r)
		}
	}
}
