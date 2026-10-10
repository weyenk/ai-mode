package app

import (
	"os"
	"strings"
	"testing"
)

func TestSkill_S21_SurfacesContractNoAutoProceed(t *testing.T) {
	data, err := os.ReadFile("../../skills/three-amigos/SKILL.md")
	if err != nil {
		t.Fatalf("cannot read the skill: %v", err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		t.Errorf("the skill must start with YAML frontmatter")
	}
	for _, m := range []string{
		"name: three-amigos", "ai-mode amigos", "--caller three-amigos", "--resume",
		"exit 0", "exit 2", "exit 3", "exit 1", "Story Contract", "approval", "Never proceed",
	} {
		if !strings.Contains(text, m) {
			t.Errorf("SKILL.md does not mention %q", m)
		}
	}
}
