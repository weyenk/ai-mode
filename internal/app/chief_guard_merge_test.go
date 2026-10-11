package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuard_P8c_MergeBlockedForEveryModel(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "t.jsonl")
	os.WriteFile(transcript, []byte(`{"type":"assistant","model":"coder","message":{"role":"assistant"}}`+"\n"), 0o644)
	payload := func(cmd string) []byte {
		b, _ := json.Marshal(map[string]any{"tool_name": "run_shell_command", "tool_input": map[string]any{"command": cmd},
			"transcript_path": transcript, "cwd": dir})
		return b
	}
	for _, cmd := range []string{"gh pr merge 12 --squash", "gh -R weyenk/ai-mode pr merge 3", "GH_X=1 gh stack merge --yes", "cd x && gh stack merge --yes --squash", `bash -c "gh pr merge 1"`} {
		if got := chiefDenial(payload(cmd)); !strings.Contains(got, "Only a human merges") {
			t.Errorf("%q from coder should be blocked, got %q", cmd, got)
		}
	}
	for _, cmd := range []string{"gh pr create --draft", "gh pr view 3", "gh stack push", "git status"} {
		if got := chiefDenial(payload(cmd)); got != "" {
			t.Errorf("%q from coder should pass, got %q", cmd, got)
		}
	}
}
