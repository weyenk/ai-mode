package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scriptedChat replays canned assistant messages and records what it was sent.
type scriptedChat struct {
	replies []agentMsg
	seen    [][]agentMsg
}

func (s *scriptedChat) Complete(req agentRequest) (agentReply, error) {
	s.seen = append(s.seen, append([]agentMsg(nil), req.Messages...))
	if len(s.replies) == 0 {
		return agentReply{Message: agentMsg{Role: "assistant", Content: "done"}}, nil
	}
	m := s.replies[0]
	s.replies = s.replies[1:]
	return agentReply{Message: m}, nil
}

func call(id, name string, args map[string]any) agentMsg {
	b, _ := json.Marshal(args)
	tc := agentToolCall{ID: id, Type: "function"}
	tc.Function.Name = name
	tc.Function.Arguments = string(b)
	return agentMsg{Role: "assistant", ToolCalls: []agentToolCall{tc}}
}

func newTestAgent(t *testing.T, chat chatClient, owns ...string) (*agentSession, string) {
	t.Helper()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644)
	return &agentSession{spec: agentSpec{
		Role: "coder", Root: root, Owns: owns, MaxTurns: 10,
		Commands: [][]string{{"go", "test"}, {"go", "vet"}, {"gofmt", "-l"}},
	}, chat: chat}, root
}

func lastToolResult(s *scriptedChat) string {
	last := s.seen[len(s.seen)-1]
	return last[len(last)-1].Content
}

func TestAgent_WritesOnlyOwnedPaths(t *testing.T) {
	chat := &scriptedChat{replies: []agentMsg{
		call("1", "write_file", map[string]any{"path": "secret.go", "content": "package demo\n"}),
		call("2", "write_file", map[string]any{"path": "timer.go", "content": "package demo\n"}),
		call("3", "finish", map[string]any{"summary": "wrote timer.go"}),
	}}
	a, root := newTestAgent(t, chat, "timer.go")
	res, err := a.Run("write the timer")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stopped != "finished" || res.Summary != "wrote timer.go" {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "secret.go")); err == nil {
		t.Fatal("wrote an unowned file")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "timer.go")); string(b) != "package demo\n" {
		t.Fatalf("timer.go = %q", b)
	}
	if got := chat.seen[1][len(chat.seen[1])-1].Content; !strings.Contains(got, "not owned") {
		t.Fatalf("unowned write should be refused with a reason, got %q", got)
	}
	if strings.Join(res.Writes, ",") != "timer.go" {
		t.Fatalf("writes = %v", res.Writes)
	}
}

func TestAgent_ReadRefusesPathsOutsideRoot(t *testing.T) {
	chat := &scriptedChat{replies: []agentMsg{
		call("1", "read_file", map[string]any{"path": "../../etc/passwd"}),
		call("2", "finish", map[string]any{"summary": "x"}),
	}}
	a, _ := newTestAgent(t, chat)
	if _, err := a.Run("read"); err != nil {
		t.Fatal(err)
	}
	if got := chat.seen[1][len(chat.seen[1])-1].Content; !strings.Contains(got, "outside") {
		t.Fatalf("got %q", got)
	}
}

func TestAgent_RunAllowsOnlyListedCommands(t *testing.T) {
	a, _ := newTestAgent(t, nil)
	for cmd, want := range map[string]string{
		"rm -rf .":                  "not allowed",
		"go test ./...; rm -rf /":   "shell",
		"go test ./... && echo hi":  "shell",
		"go test $(echo x)":         "shell",
		"git push origin main":      "not allowed",
		"gh pr merge 1":             "not allowed",
		"go test ./... -run TestNo": "exit",
	} {
		got := a.toolRun(map[string]any{"command": cmd})
		if !strings.Contains(got, want) {
			t.Errorf("%q: got %q, want it to mention %q", cmd, got, want)
		}
	}
}

func TestAgent_NudgesWhenModelAnswersWithoutTools(t *testing.T) {
	chat := &scriptedChat{replies: []agentMsg{
		{Role: "assistant", Content: "I will now write the file."},
		call("1", "write_file", map[string]any{"path": "a.go", "content": "package demo\n"}),
		call("2", "finish", map[string]any{"summary": "ok"}),
	}}
	a, _ := newTestAgent(t, chat, "a.go")
	res, err := a.Run("task")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stopped != "finished" {
		t.Fatalf("stopped = %s", res.Stopped)
	}
	if !strings.Contains(chat.seen[1][len(chat.seen[1])-1].Content, "finish") {
		t.Fatal("expected a nudge telling the model to use tools and call finish")
	}
}

func TestAgent_StopsAtTurnCap(t *testing.T) {
	var replies []agentMsg
	for i := 0; i < 20; i++ {
		replies = append(replies, call("x", "list_files", map[string]any{}))
	}
	a, _ := newTestAgent(t, &scriptedChat{replies: replies})
	a.spec.MaxTurns = 3
	res, err := a.Run("loop")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stopped != "max-turns" || res.Turns != 3 {
		t.Fatalf("result = %+v", res)
	}
}

func TestAgent_EditFileNeedsUniqueMatch(t *testing.T) {
	a, root := newTestAgent(t, nil, "a.go")
	os.WriteFile(filepath.Join(root, "a.go"), []byte("x := 1\nx := 1\ny := 2\n"), 0o644)
	if got := a.toolEdit(map[string]any{"path": "a.go", "old_string": "x := 1", "new_string": "x := 3"}); !strings.Contains(got, "2 times") {
		t.Fatalf("ambiguous edit: %q", got)
	}
	if got := a.toolEdit(map[string]any{"path": "a.go", "old_string": "y := 2", "new_string": "y := 5"}); !strings.Contains(got, "edited") {
		t.Fatalf("edit: %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.go")); !strings.Contains(string(b), "y := 5") {
		t.Fatalf("file = %q", b)
	}
}

func TestAgent_OwnedPatterns(t *testing.T) {
	a := &agentSession{spec: agentSpec{Owns: []string{"timer/**", "*_test.go", "cmd/main.go"}}}
	for p, want := range map[string]bool{
		"timer/clock.go": true, "timer/sub/x.go": true, "a_test.go": true, "cmd/main.go": true,
		"cmd/other.go": false, "main.go": false, "timerx/a.go": false,
	} {
		if got := a.owned(p); got != want {
			t.Errorf("owned(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestAgent_ContinueKeepsHistory(t *testing.T) {
	chat := &scriptedChat{replies: []agentMsg{
		call("1", "finish", map[string]any{"summary": "first"}),
		call("2", "finish", map[string]any{"summary": "second"}),
	}}
	a, _ := newTestAgent(t, chat)
	if _, err := a.Run("task"); err != nil {
		t.Fatal(err)
	}
	res, err := a.Continue("lint failed: fix it")
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary != "second" {
		t.Fatalf("summary = %q", res.Summary)
	}
	sent := chat.seen[1]
	if sent[len(sent)-1].Content != "lint failed: fix it" || len(sent) < 4 {
		t.Fatalf("continue should append to the history, sent %d messages ending %q", len(sent), sent[len(sent)-1].Content)
	}
}
