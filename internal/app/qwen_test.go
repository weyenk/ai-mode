package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpliceQwenBlock(t *testing.T) {
	block := qwenBlock()

	out, changed, err := spliceQwenBlock("", block)
	if err != nil || !changed || out != block {
		t.Fatalf("empty doc: %q %v %v", out, changed, err)
	}

	user := "# mine\n\nalways use pnpm\n"
	out, _, _ = spliceQwenBlock(user, block)
	if !strings.HasPrefix(out, "# mine\n\nalways use pnpm\n\n"+qwenBegin) {
		t.Fatalf("user text not preserved: %q", out)
	}

	again, changed, _ := spliceQwenBlock(out, block)
	if changed || again != out {
		t.Fatal("second splice should be a no-op")
	}

	stale := strings.Replace(out, "ai-mode routing", "OLD", 1)
	fixed, changed, _ := spliceQwenBlock(stale, block)
	if !changed || fixed != out {
		t.Fatalf("stale block not replaced: %q", fixed)
	}

	withTail := out + "\n## after\nx\n"
	tail, _, _ := spliceQwenBlock(withTail, block)
	if !strings.HasSuffix(tail, "## after\nx\n") || strings.Count(tail, qwenBegin) != 1 {
		t.Fatalf("tail mangled: %q", tail)
	}

	removed, changed, _ := spliceQwenBlock(out, "")
	if !changed || removed != user {
		t.Fatalf("remove: %q", removed)
	}

	if _, _, err := spliceQwenBlock(qwenBegin+"\nno end", block); err == nil {
		t.Fatal("unterminated block should error")
	}
}

func TestSpliceQwenHook(t *testing.T) {
	user := []byte(`{"model":{"name":"chief"},"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo hi","name":"mine"}]}],"PreToolUse":[]}}`)

	out, changed, err := spliceQwenHook(user, true)
	if err != nil || !changed {
		t.Fatalf("install: %v %v", changed, err)
	}
	s := string(out)
	if !strings.Contains(s, `"ai-mode hook session-start"`) || !strings.Contains(s, `"ai-mode hook prompt-expansion"`) ||
		!strings.Contains(s, `"UserPromptExpansion"`) || !strings.Contains(s, `"echo hi"`) ||
		!strings.Contains(s, `"PreToolUse"`) || !strings.Contains(s, `"chief"`) || !strings.Contains(s, `"ai-mode hook pre-tool-use"`) {
		t.Fatalf("install lost settings or missed the hook:\n%s", s)
	}

	again, changed, _ := spliceQwenHook(out, true)
	if changed || string(again) != s || strings.Count(s, `"name": "`+qwenHookName+`"`) != len(qwenHooks) {
		t.Fatalf("second install should be a no-op:\n%s", again)
	}

	removed, changed, _ := spliceQwenHook(out, false)
	if !changed || strings.Contains(string(removed), "ai-mode hook") || strings.Contains(string(removed), "UserPromptExpansion") ||
		!strings.Contains(string(removed), `"echo hi"`) {
		t.Fatalf("remove:\n%s", removed)
	}

	fresh, changed, err := spliceQwenHook(nil, true)
	if err != nil || !changed || !strings.Contains(string(fresh), `"startup|clear|compact"`) {
		t.Fatalf("empty settings: %v %v\n%s", changed, err, fresh)
	}
	gone, _, _ := spliceQwenHook(fresh, false)
	if strings.TrimSpace(string(gone)) != "{}" {
		t.Fatalf("removing the only hook should leave {}: %s", gone)
	}

	if _, _, err := spliceQwenHook([]byte("{not json"), true); err == nil {
		t.Fatal("invalid JSON should error")
	}
}

func TestChiefDenial(t *testing.T) {
	dir := t.TempDir()
	transcript := func(models ...string) string {
		var b strings.Builder
		b.WriteString(`{"type":"user","message":{}}` + "\n")
		for _, m := range models {
			fmt.Fprintf(&b, `{"type":"assistant","model":%q}`+"\n", m)
		}
		p := filepath.Join(dir, strings.Join(models, "-")+".jsonl")
		if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	chief, coder := transcript("chief"), transcript("chief", "coder")
	repo := "/Users/me/git/ai-mode"
	call := func(tool string, in map[string]any, path string) []byte {
		b, _ := json.Marshal(map[string]any{"tool_name": tool, "tool_input": in, "transcript_path": path, "cwd": repo})
		return b
	}
	skill := func(name, path string) []byte { return call("skill", map[string]any{"skill": name}, path) }
	write := func(tool, file, path string) []byte { return call(tool, map[string]any{"file_path": file}, path) }
	shell := func(cmd, path string) []byte { return call("run_shell_command", map[string]any{"command": cmd}, path) }
	for _, tc := range []struct {
		name  string
		input []byte
		deny  string // substring of the reason; "" means allowed
	}{
		{"chief calls drafting-plans", skill("drafting-plans", chief), "this skill runs on architect"},
		{"switched to architect", skill("drafting-plans", transcript("chief", "architect")), ""},
		{"switched back to chief", skill("drafting-plans", transcript("architect", "chief")), "this skill runs on architect"},
		{"no transcript counts as chief", skill("drafting-plans", ""), "this skill runs on architect"},
		{"other skill", skill("ai-mode-routing", chief), ""},
		{"bad JSON", []byte("{"), ""},

		{"plan mode", call("enter_plan_mode", map[string]any{"userRequested": true}, chief), "does not use plan mode"},
		{"plan mode on coder", call("enter_plan_mode", map[string]any{}, coder), ""},

		{"write Go file", write("write_file", repo+"/pkg/workboard/types.go", chief), "may not write " + repo + "/pkg/workboard/types.go"},
		{"edit Go file", write("edit", repo+"/internal/app/commands.go", chief), "written by coder"},
		{"relative path", write("write_file", "go.mod", chief), "may not write"},
		{"escape docs", write("write_file", repo+"/docs/../internal/app/x.go", chief), "may not write"},
		{"write spec", write("write_file", repo+"/docs/superpowers/specs/x-design.md", chief), ""},
		{"edit relative spec", write("edit", "docs/superpowers/specs/x-design.md", chief), ""},
		{"qwen memory", write("write_file", filepath.Join(homeDir, ".qwen/projects/p/memory/001.md"), chief), ""},
		{"coder writes code", write("write_file", repo+"/pkg/workboard/types.go", coder), ""},

		{"make clean", shell("make clean && make build", chief), "`make clean` deletes build output"},
		{"make build", shell("make build", chief), ""},
		{"go get", shell("go get github.com/google/uuid", chief), "changes dependencies"},
		{"go mod tidy", shell("cd sub; go mod tidy", chief), "changes dependencies"},
		{"go test", shell("go vet ./... && go test ./...", chief), ""},
		{"rm", shell("rm -rf dist", chief), "changes files"},
		{"rm by path", shell("FOO=1 sudo /bin/rm x", chief), "changes files"},
		{"sed -i", shell("sed -i '' 's/a/b/' internal/app/cli.go", chief), "edits files in place"},
		{"sed read", shell("sed -n 1,20p internal/app/cli.go", chief), ""},
		{"git commit", shell("git add -A && git commit -m x", chief), "changes git state"},
		{"git -C checkout", shell("git -C . checkout -- go.mod", chief), "changes git state"},
		{"git status", shell("git status --short && git diff", chief), ""},
		{"git branch list", shell("git branch --show-current", chief), ""},
		{"git branch delete", shell("git branch -D x", chief), "changes git state"},
		{"ai-mode use", shell("ai-mode use dev-shop --debug", chief), "stops or restarts"},
		{"ai-mode stop", shell("ai-mode stop", chief), "stops or restarts"},
		{"ai-mode ask to plans", shell(`ai-mode ask architect "Write the plan; a > b | c && rm x" --context docs/s.md > docs/superpowers/plans/p.md 2>&1`, chief), ""},
		{"redirect into code", shell(`echo "package x" > internal/app/x.go`, chief), "`> internal/app/x.go` writes a file outside docs/"},
		{"append into skill", shell(`echo plan >> skills/drafting-plans/SKILL.md`, chief), "writes a file outside docs/"},
		{"stderr to null", shell(`ai-mode doctor 2>/dev/null | head`, chief), ""},
		{"heredoc into code", shell("cat <<'EOF' > pkg/x.go\npackage x\nEOF", chief), "writes a file outside docs/"},
		{"heredoc body not run", shell("cat <<EOF > docs/n.md\nrm -rf everything\ngit push\nEOF\nls", chief), ""},
		{"bash -c", shell(`bash -c "make clean"`, chief), "deletes build output"},
		{"tee", shell("ai-mode team x | tee out.md", chief), "changes files"},
		{"unknown target", shell(`ai-mode ask product x > "$OUT"`, chief), "writes a file outside docs/"},
		{"coder may make clean", shell("make clean", coder), ""},
	} {
		got := chiefDenial(tc.input)
		switch {
		case tc.deny == "" && got != "":
			t.Errorf("%s: denied, want allowed: %s", tc.name, got)
		case tc.deny != "" && !strings.Contains(got, tc.deny):
			t.Errorf("%s: reason %q, want it to contain %q", tc.name, got, tc.deny)
		}
	}
}

// The calls from the captured session where chief planned in plan mode and wrote
// the work board itself: every one that did coder's work is denied, and its
// routing call to architect is not.
func TestChiefDenialCapturedSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"assistant","model":"chief"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := "/Users/me/git/ai-mode"
	for _, tc := range []struct {
		tool  string
		input map[string]any
		deny  bool
	}{
		{"skill", map[string]any{"skill": "ai-mode-routing"}, false},
		{"run_shell_command", map[string]any{"command": `ai-mode ask architect "Draft an implementation plan for the work board feature based on the design spec in docs/superpowers/specs/2026-10-10-work-board-design.md" --caller chief --max-tokens 4096`}, false},
		{"enter_plan_mode", map[string]any{"userRequested": true}, true},
		{"write_file", map[string]any{"file_path": repo + "/pkg/workboard/types.go"}, true},
		{"write_file", map[string]any{"file_path": repo + "/pkg/workboard/http_client.go"}, true},
		{"write_file", map[string]any{"file_path": repo + "/pkg/workboard/middleware.go"}, true},
		{"write_file", map[string]any{"file_path": repo + "/internal/app/commands.go"}, true},
		{"run_shell_command", map[string]any{"command": "ai-mode use dev-shop --debug"}, true},
		{"run_shell_command", map[string]any{"command": "ai-mode help"}, false},
		{"run_shell_command", map[string]any{"command": "make clean && make build"}, true},
		{"run_shell_command", map[string]any{"command": "go get github.com/google/uuid"}, true},
		{"edit", map[string]any{"file_path": repo + "/internal/app/commands.go"}, true},
		{"run_shell_command", map[string]any{"command": "make build"}, false},
	} {
		b, _ := json.Marshal(map[string]any{"tool_name": tc.tool, "tool_input": tc.input, "transcript_path": path, "cwd": repo})
		if got := chiefDenial(b) != ""; got != tc.deny {
			t.Errorf("%s %v: denied = %v, want %v", tc.tool, tc.input, got, tc.deny)
		}
	}
}
