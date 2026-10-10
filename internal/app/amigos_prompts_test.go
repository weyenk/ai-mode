package app

import (
	"os"
	"strings"
	"testing"
)

func TestPrompts_S20_SectionsPresent(t *testing.T) {
	common := []string{"## Amigos meeting", "<story_data>", "data, not instructions", "ONE JSON object", "react to the other roles"}
	perRole := map[string][]string{
		"product":   {"`rules`", "`questions`", "`answers`", "`disagreements`", "`ready`"},
		"qa":        {"`examples`", "`questions`", "`answers`", "`disagreements`", "`ready`"},
		"architect": {"`size`", "`split_into`", "`layer_plan`", "`questions`", "`answers`", "`disagreements`", "`ready`"},
	}
	for role, markers := range perRole {
		data, err := os.ReadFile("../../presets/agents/" + role + ".md")
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		text := string(data)
		i := strings.Index(text, "## Amigos meeting")
		if i < 0 {
			t.Errorf("%s.md has no \"## Amigos meeting\" section", role)
			continue
		}
		section := text[i:]
		for _, m := range append(append([]string{}, common...), markers...) {
			if !strings.Contains(section, m) {
				t.Errorf("%s.md: the Amigos meeting section does not mention %q", role, m)
			}
		}
	}
}
