package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// `ai-mode agent` runs one role as a tool-using agent: the role gets a fixed tool
// set instead of a shell, may write only the paths its task owns, and may run only
// allowlisted commands (argv, never a shell). Pipeline stages that need a role to
// read the repo, write files or run tests (architect planning, qa red, coder green)
// use it; consults stay on `ai-mode ask`.

type agentToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type agentMsg struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []agentToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type agentRequest struct {
	Model           string
	Messages        []agentMsg
	Tools           []map[string]any
	MaxTokens       int
	ReasoningEffort string // chat_template_kwargs.reasoning_effort for this request; "" keeps the preset's
}

type agentReply struct {
	Message          agentMsg
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
}

// chatClient is one chat/completions call with tools; the fake in tests replays a script.
type chatClient interface {
	Complete(req agentRequest) (agentReply, error)
}

type agentSpec struct {
	Role      string
	System    string     // full system prompt; "" uses the role's agent guide plus agentRules
	Root      string     // every path is relative to it and must stay inside it
	Owns      []string   // writable path patterns (file, dir/**, or a path.Match glob)
	ReadOnly  []string   // patterns that stay read-only even when Owns matches (e.g. tests during green)
	Commands  [][]string // allowlisted argv prefixes for the run tool
	MaxTurns  int
	MaxWall   time.Duration
	MaxTokens int
	// ReasoningEffort is sent per request (low | medium | high); "" keeps the preset's. A per-request token budget is
	// ignored by llama-server, but the effort level is honoured and is what keeps a thinking model's turns short.
	ReasoningEffort string
	Transcript      string    // JSONL file of every message; "" for none
	Log             io.Writer // one progress line per tool call; nil for none
}

type agentResult struct {
	Summary   string   `json:"summary"`
	Stopped   string   `json:"stopped"` // finished | max-turns | timeout | no-tool-call
	Turns     int      `json:"turns"`
	ToolCalls int      `json:"tool_calls"`
	Writes    []string `json:"writes"`
}

type agentSession struct {
	spec      agentSpec
	chat      chatClient
	msgs      []agentMsg
	writes    []string
	turns     int
	toolCalls int
	start     time.Time
}

// defaultAgentCommands is what `ai-mode agent` allows when no --allow is given.
var defaultAgentCommands = [][]string{{"go", "test"}, {"go", "build"}, {"go", "vet"}, {"gofmt", "-l"}, {"gofmt", "-d"}, {"gofmt", "-w"}}

const agentRules = `

## Agent mode (ai-mode tool loop)

You are running unattended as a tool-using agent. Nobody reads your messages until you call finish: act through tools.
- Paths are relative to the repository root; you cannot leave it.
- You may write only these paths: %s. Everything else is read-only.
- The run tool executes one command as argv (no shell, no pipes, no &&). Allowed command prefixes: %s.
- Read a file before you change it. Use edit_file for small changes to an existing file and write_file for new files or full rewrites.
- When the task is done and its checks pass, call finish with a short summary: what you changed and the result of the last
  check you ran. If you cannot finish, call finish and say exactly what blocked you. Do not stop without calling finish.`

func agentToolDefs() []map[string]any {
	fn := func(name, desc string, props map[string]any, req ...string) map[string]any {
		if req == nil {
			req = []string{}
		}
		return map[string]any{"type": "function", "function": map[string]any{
			"name": name, "description": desc,
			"parameters": map[string]any{"type": "object", "properties": props, "required": req},
		}}
	}
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	num := func(d string) map[string]any { return map[string]any{"type": "integer", "description": d} }
	return []map[string]any{
		fn("list_files", "List files under a directory (recursive, relative paths; directories end in /).", map[string]any{"path": str("directory, default the repo root")}),
		fn("read_file", "Read a text file. Long files are cut; pass start_line/end_line (1-based, inclusive) to read a range.",
			map[string]any{"path": str("file path"), "start_line": num("first line"), "end_line": num("last line")}, "path"),
		fn("search", "Search file contents with a Go regular expression; prints path:line: text.",
			map[string]any{"pattern": str("RE2 regular expression"), "path": str("directory or file, default the repo root")}, "pattern"),
		fn("write_file", "Create or overwrite a file you own with the full content.", map[string]any{"path": str("file path"), "content": str("full file content")}, "path", "content"),
		fn("edit_file", "Replace one exact, unique occurrence of old_string with new_string in a file you own.",
			map[string]any{"path": str("file path"), "old_string": str("exact text to replace, unique in the file"), "new_string": str("replacement")}, "path", "old_string", "new_string"),
		fn("run", "Run one allowlisted command in the repo root (argv only, no shell). Returns the exit code and the output tail.",
			map[string]any{"command": str("for example: go test ./...")}, "command"),
		fn("finish", "End the task with a short summary of what you did and the last check's result.", map[string]any{"summary": str("summary")}, "summary"),
	}
}

func agentRulesFor(owns []string, cmds [][]string) string {
	o := "none (read-only task)"
	if len(owns) > 0 {
		o = strings.Join(owns, ", ")
	}
	var cs []string
	for _, c := range cmds {
		cs = append(cs, "`"+strings.Join(c, " ")+"`")
	}
	if len(cs) == 0 {
		cs = []string{"none"}
	}
	return fmt.Sprintf(agentRules, o, strings.Join(cs, ", "))
}

// Run starts a session on task.
func (a *agentSession) Run(task string) (agentResult, error) {
	a.start = time.Now()
	sys := a.spec.System
	if sys == "" {
		sys = "You are a careful software agent." + agentRulesFor(a.spec.Owns, a.spec.Commands)
	}
	a.msgs = []agentMsg{{Role: "system", Content: sys}, {Role: "user", Content: task}}
	a.record(a.msgs...)
	return a.loop()
}

// Continue appends a user message (for example a failed check) to the same conversation and resumes.
func (a *agentSession) Continue(msg string) (agentResult, error) {
	if a.start.IsZero() {
		a.start = time.Now()
	}
	m := agentMsg{Role: "user", Content: msg}
	a.msgs = append(a.msgs, m)
	a.record(m)
	return a.loop()
}

func (a *agentSession) result(stopped, summary string) agentResult {
	return agentResult{Summary: summary, Stopped: stopped, Turns: a.turns, ToolCalls: a.toolCalls, Writes: append([]string(nil), a.writes...)}
}

func (a *agentSession) loop() (agentResult, error) {
	maxTurns := a.spec.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 40
	}
	maxTokens := a.spec.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 32768
	}
	nudges, turnsThisRun := 0, 0
	for {
		if turnsThisRun >= maxTurns {
			return a.result("max-turns", ""), nil
		}
		if a.spec.MaxWall > 0 && time.Since(a.start) > a.spec.MaxWall {
			return a.result("timeout", ""), nil
		}
		rep, err := a.chat.Complete(agentRequest{Model: a.spec.Role, Messages: a.msgs, Tools: agentToolDefs(), MaxTokens: maxTokens, ReasoningEffort: a.spec.ReasoningEffort})
		a.turns++
		turnsThisRun++
		if err != nil {
			return a.result("error", ""), err
		}
		m := agentMsg{Role: "assistant", Content: rep.Message.Content, ToolCalls: rep.Message.ToolCalls}
		a.msgs = append(a.msgs, m)
		a.record(m)
		if len(m.ToolCalls) == 0 {
			if nudges < 2 {
				nudges++
				n := agentMsg{Role: "user", Content: "No tool was called. Do the work with the tools; when it is done and checked, call finish with a summary."}
				if rep.FinishReason == "length" {
					n.Content = "Your reply hit the token limit before any tool call. Think less and call a tool now; call finish when done."
				}
				a.msgs = append(a.msgs, n)
				a.record(n)
				continue
			}
			return a.result("no-tool-call", strings.TrimSpace(m.Content)), nil
		}
		finished, summary := false, ""
		for _, tc := range m.ToolCalls {
			a.toolCalls++
			var args map[string]any
			out := ""
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil && strings.TrimSpace(tc.Function.Arguments) != "" {
				out = "error: arguments are not valid JSON: " + err.Error()
			} else if tc.Function.Name == "finish" {
				finished, summary, out = true, strings.TrimSpace(argStr(args, "summary")), "ok"
			} else {
				out = a.dispatch(tc.Function.Name, args)
			}
			a.logf("%s %s → %s", tc.Function.Name, shortArgs(args), firstLine(out))
			r := agentMsg{Role: "tool", ToolCallID: tc.ID, Content: out}
			a.msgs = append(a.msgs, r)
			a.record(r)
		}
		if finished {
			return a.result("finished", summary), nil
		}
	}
}

func (a *agentSession) dispatch(name string, args map[string]any) string {
	switch name {
	case "list_files":
		return a.toolList(args)
	case "read_file":
		return a.toolRead(args)
	case "search":
		return a.toolSearch(args)
	case "write_file":
		return a.toolWrite(args)
	case "edit_file":
		return a.toolEdit(args)
	case "run":
		return a.toolRun(args)
	}
	return fmt.Sprintf("error: unknown tool %q (tools: list_files, read_file, search, write_file, edit_file, run, finish)", name)
}

func (a *agentSession) logf(format string, args ...any) {
	if a.spec.Log != nil {
		fmt.Fprintf(a.spec.Log, "  [%s] "+format+"\n", append([]any{a.spec.Role}, args...)...)
	}
}

func (a *agentSession) record(ms ...agentMsg) {
	if a.spec.Transcript == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(a.spec.Transcript), 0o755)
	f, err := os.OpenFile(a.spec.Transcript, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	for _, m := range ms {
		b, _ := json.Marshal(m)
		f.Write(append(b, '\n'))
	}
}

func argStr(args map[string]any, k string) string {
	if args == nil {
		return ""
	}
	s, _ := args[k].(string)
	return s
}

func argInt(args map[string]any, k string) int {
	if args == nil {
		return 0
	}
	switch v := args[k].(type) {
	case float64:
		return int(v)
	case string:
		var n int
		fmt.Sscan(v, &n)
		return n
	}
	return 0
}

func shortArgs(args map[string]any) string {
	for _, k := range []string{"path", "command", "pattern", "summary"} {
		if s := argStr(args, k); s != "" {
			return truncateRunes(s, 80)
		}
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return truncateRunes(s, 100)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

var errOutside = errors.New("path is outside the repository root")

// resolve maps a model-supplied path to (relative, absolute), refusing anything outside Root (also through symlinks).
func (a *agentSession) resolve(p string) (string, string, error) {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		p = "."
	}
	root, err := filepath.EvalSymlinks(a.spec.Root)
	if err != nil {
		root = a.spec.Root
	}
	if filepath.IsAbs(p) {
		rp := p
		if r, err := filepath.EvalSymlinks(p); err == nil {
			rp = r
		}
		r, err := filepath.Rel(root, rp)
		if err != nil {
			return "", "", errOutside
		}
		p = r
	}
	rel := filepath.Clean(p)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errOutside
	}
	full := filepath.Join(root, rel)
	if real, err := filepath.EvalSymlinks(full); err == nil {
		if r, err := filepath.Rel(root, real); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return "", "", errOutside
		}
	}
	return filepath.ToSlash(rel), full, nil
}

func (a *agentSession) owned(rel string) bool {
	for _, ro := range a.spec.ReadOnly {
		if pathsOverlap(ro, rel) {
			return false
		}
	}
	for _, o := range a.spec.Owns {
		if pathsOverlap(o, rel) {
			return true
		}
	}
	return false
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, ".pipeline": true}

func (a *agentSession) toolList(args map[string]any) string {
	rel, full, err := a.resolve(argStr(args, "path"))
	if err != nil {
		return "error: " + err.Error()
	}
	var out []string
	_ = filepath.WalkDir(full, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == full {
			return nil
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		r, _ := filepath.Rel(full, p)
		r = filepath.ToSlash(filepath.Join(rel, r))
		if d.IsDir() {
			r += "/"
		}
		out = append(out, r)
		if len(out) >= 400 {
			return fs.SkipAll
		}
		return nil
	})
	if len(out) == 0 {
		return "(empty)"
	}
	s := strings.Join(out, "\n")
	if len(out) >= 400 {
		s += "\n…(listing cut at 400 entries; list a subdirectory)"
	}
	return s
}

const agentReadMax = 60000

func (a *agentSession) toolRead(args map[string]any) string {
	rel, full, err := a.resolve(argStr(args, "path"))
	if err != nil {
		return "error: " + err.Error()
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "error: " + err.Error()
	}
	s := string(data)
	from, to := argInt(args, "start_line"), argInt(args, "end_line")
	if from > 0 || to > 0 {
		lines := strings.SplitAfter(s, "\n")
		if from < 1 {
			from = 1
		}
		if to < from || to > len(lines) {
			to = len(lines)
		}
		if from > len(lines) {
			return fmt.Sprintf("error: %s has %d lines", rel, len(lines))
		}
		s = strings.Join(lines[from-1:to], "")
	}
	if len(s) > agentReadMax {
		s = s[:agentReadMax] + fmt.Sprintf("\n…(cut at %d characters; read a line range)", agentReadMax)
	}
	return s
}

func (a *agentSession) toolSearch(args map[string]any) string {
	re, err := regexp.Compile(argStr(args, "pattern"))
	if err != nil {
		return "error: " + err.Error()
	}
	_, full, err := a.resolve(argStr(args, "path"))
	if err != nil {
		return "error: " + err.Error()
	}
	root, _ := filepath.EvalSymlinks(a.spec.Root)
	if root == "" {
		root = a.spec.Root
	}
	var out []string
	_ = filepath.WalkDir(full, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] && p != full {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1<<20 {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || strings.ContainsRune(string(data[:min(len(data), 512)]), 0) {
			return nil
		}
		r, _ := filepath.Rel(root, p)
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				out = append(out, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(r), i+1, truncateRunes(strings.TrimSpace(line), 200)))
				if len(out) >= 80 {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	if len(out) == 0 {
		return "no matches"
	}
	return strings.Join(out, "\n")
}

func (a *agentSession) writable(p string) (string, string, string) {
	rel, full, err := a.resolve(p)
	if err != nil {
		return "", "", "error: " + err.Error()
	}
	if !a.owned(rel) {
		return "", "", fmt.Sprintf("refused: %s is not owned by this task (you may write only: %s)", rel, strings.Join(a.spec.Owns, ", "))
	}
	return rel, full, ""
}

func (a *agentSession) noteWrite(rel string) {
	for _, w := range a.writes {
		if w == rel {
			return
		}
	}
	a.writes = append(a.writes, rel)
	sort.Strings(a.writes)
}

func (a *agentSession) toolWrite(args map[string]any) string {
	rel, full, refusal := a.writable(argStr(args, "path"))
	if refusal != "" {
		return refusal
	}
	content := argStr(args, "content")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "error: " + err.Error()
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "error: " + err.Error()
	}
	a.noteWrite(rel)
	return fmt.Sprintf("wrote %s (%d bytes)", rel, len(content))
}

func (a *agentSession) toolEdit(args map[string]any) string {
	rel, full, refusal := a.writable(argStr(args, "path"))
	if refusal != "" {
		return refusal
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "error: " + err.Error()
	}
	old, repl := argStr(args, "old_string"), argStr(args, "new_string")
	if old == "" {
		return "error: old_string is empty; use write_file to create or rewrite a file"
	}
	switch n := strings.Count(string(data), old); n {
	case 0:
		return "error: old_string not found in " + rel + " (read the file and copy the exact text, including indentation)"
	case 1:
	default:
		return fmt.Sprintf("error: old_string appears %d times in %s; include more surrounding lines so it is unique", n, rel)
	}
	if err := os.WriteFile(full, []byte(strings.Replace(string(data), old, repl, 1)), 0o644); err != nil {
		return "error: " + err.Error()
	}
	a.noteWrite(rel)
	return "edited " + rel
}

// hasShellOps reports a shell operator outside quotes; inside quotes (a -run regexp like '^(A|B)$') it is just a character,
// since the command runs as argv without a shell.
func hasShellOps(cmd string) bool {
	quote := rune(0)
	rs := []rune(cmd)
	for i, r := range rs {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case strings.ContainsRune(";|&<>`\n", r):
			return true
		case r == '$' && i+1 < len(rs) && rs[i+1] == '(':
			return true
		}
	}
	return false
}

func (a *agentSession) toolRun(args map[string]any) string {
	cmd := strings.TrimSpace(argStr(args, "command"))
	if hasShellOps(cmd) {
		return "refused: no shell here, so pipes, redirects, ;, && and $(...) are not supported. Run one command."
	}
	argv, err := splitArgv(cmd)
	if err != nil || len(argv) == 0 {
		return "error: could not parse the command"
	}
	allowed := false
	for _, pre := range a.spec.Commands {
		if len(argv) >= len(pre) && strings.Join(argv[:len(pre)], "\x00") == strings.Join(pre, "\x00") {
			allowed = true
		}
	}
	if !allowed {
		var cs []string
		for _, c := range a.spec.Commands {
			cs = append(cs, strings.Join(c, " "))
		}
		return fmt.Sprintf("refused: command not allowed in this task. Allowed prefixes: %s", strings.Join(cs, "; "))
	}
	if argv[0] == "gofmt" {
		for _, x := range argv[1:] {
			if argv[1] == "-w" && !strings.HasPrefix(x, "-") {
				if rel, _, err := a.resolve(x); err != nil || !a.owned(rel) {
					return "refused: gofmt -w may only rewrite files this task owns"
				}
			}
		}
	}
	exe := argv[0]
	if exe == appName {
		if self, err := os.Executable(); err == nil {
			exe = self
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c := exec.CommandContext(ctx, exe, argv[1:]...)
	c.Dir = a.spec.Root
	out, err := c.CombinedOutput()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			return "error: " + err.Error()
		}
	}
	s := string(out)
	if len(s) > 12000 {
		s = "…(output cut; showing the end)\n" + s[len(s)-12000:]
	}
	if strings.TrimSpace(s) == "" {
		s = "(no output)"
	}
	return fmt.Sprintf("exit code %d\n%s", code, s)
}

// splitArgv splits a command line into words, honouring single and double quotes.
func splitArgv(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	in, quote := false, rune(0)
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, in = r, true
		case r == ' ' || r == '\t':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unclosed quote")
	}
	if in {
		out = append(out, cur.String())
	}
	return out, nil
}

// ---- llama-server client ---------------------------------------------------

type llamaChat struct {
	baseURL string
	timeout time.Duration
	trace   string
	caller  string
	profile string
}

func (c llamaChat) Complete(req agentRequest) (agentReply, error) {
	body := map[string]any{"model": req.Model, "messages": req.Messages, "tools": req.Tools, "max_tokens": req.MaxTokens}
	if req.ReasoningEffort != "" {
		body["chat_template_kwargs"] = map[string]any{"reasoning_effort": req.ReasoningEffort}
	}
	ev := Event{Kind: "agent", Trace: c.trace, Span: newSpanID(), Caller: c.caller, Profile: c.profile, Role: req.Model, MaxTokens: req.MaxTokens}
	start := time.Now()
	ev.TS = start.UTC()
	data, err := postJSON(c.baseURL+"/chat/completions", body, c.timeout)
	ev.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		ev.Status, ev.Error = "error", err.Error()
		emit(ev)
		return agentReply{}, err
	}
	recordUsage(&ev, data)
	var parsed struct {
		Choices []struct {
			Message      agentMsg `json:"message"`
			FinishReason string   `json:"finish_reason"`
		} `json:"choices"`
	}
	b, _ := json.Marshal(data)
	if err := json.Unmarshal(b, &parsed); err != nil || len(parsed.Choices) == 0 {
		ev.Status, ev.Error = "error", "no choices in response"
		emit(ev)
		return agentReply{}, errors.New("no choices in response")
	}
	ch := parsed.Choices[0]
	ev.Status = "ok"
	ev.ToolCalls = len(ch.Message.ToolCalls)
	emit(ev)
	return agentReply{Message: ch.Message, FinishReason: ch.FinishReason, PromptTokens: ev.PromptTokens, CompletionTokens: ev.CompletionTokens}, nil
}

// newRoleAgent builds a session for role on the active profile, with the role's guide plus the agent rules as system prompt.
func newRoleAgent(p Profile, spec agentSpec, trace, caller string) (*agentSession, error) {
	if spec.Role == "jev" || !contains(iniSectionsFile(p.Ini), spec.Role) {
		return nil, fmt.Errorf("role %q is not a chat role in %s", spec.Role, filepathBase(p.Ini))
	}
	if spec.System == "" {
		_, _, body, code := agentForRole(spec.Role)
		if code != 0 {
			return nil, fmt.Errorf("no agent guide for %s", spec.Role)
		}
		spec.System = strings.TrimSpace(body) + agentRulesFor(spec.Owns, spec.Commands)
	}
	if spec.Transcript == "" {
		spec.Transcript = filepath.Join(stateDir(), "agent", time.Now().Format("20060102-150405")+"-"+spec.Role+"-"+newSpanID()+".jsonl")
	}
	if trace == "" {
		trace = newTraceID()
	}
	return &agentSession{spec: spec, chat: llamaChat{baseURL: p.BaseURL(), timeout: 30 * time.Minute, trace: trace, caller: caller, profile: p.Name}}, nil
}

func cmdAgent(args []string) int {
	fs := newFlags("agent")
	repo := fs.String("repo", ".", "Repository root the agent works in")
	var owns, readOnly, allow, ctxPaths stringList
	fs.Var(&owns, "own", "Path the agent may write (file, dir/**, or glob); repeatable")
	fs.Var(&readOnly, "read-only", "Path that stays read-only even if --own matches; repeatable")
	fs.Var(&allow, "allow", `Allowed command prefix for the run tool, e.g. "go test"; repeatable (default: go test/build/vet, gofmt)`)
	fs.Var(&ctxPaths, "context", "File or directory added to the task as reference material; repeatable")
	maxTurns := fs.Int("max-turns", 40, "Maximum model turns")
	maxWall := fs.Duration("max-wall", 30*time.Minute, "Wall-clock budget")
	maxTokens := fs.Int("max-tokens", 32768, "max_tokens per model turn (thinking roles need room)")
	effort := fs.String("effort", "", "reasoning_effort per request: low | medium | high (default: the preset's)")
	transcript := fs.String("transcript", "", "Write the conversation as JSONL here (default: <state>/agent/...)")
	asJSON := fs.Bool("json", false, "Print the result as JSON")
	profile := fs.String("profile", "", "Profile (default: active)")
	callerF := fs.String("caller", "", "Who is running the agent (default: $AI_MODE_CALLER or cli)")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) < 1 {
		return fail("usage: ai-mode agent <role> [task...] [--repo DIR] [--own PATH]... [--read-only PATH]... [--allow CMD]... [--context PATH]... [--max-turns N] [--max-wall D] [--json]")
	}
	task := readUserMessage(pos[1:])
	if task == "" {
		return fail("Empty task: pass it as arguments or on stdin.")
	}
	block, err := buildContext(ctxPaths, defaultContextMaxChars)
	if err != nil {
		return fail("%v", err)
	}
	task = withContext(task, block)
	cmds := defaultAgentCommands
	if len(allow) > 0 {
		cmds = nil
		for _, c := range allow {
			argv, err := splitArgv(c)
			if err != nil || len(argv) == 0 {
				return fail("bad --allow %q", c)
			}
			cmds = append(cmds, argv)
		}
	}
	p, code := activeProfile(*profile, "Use: ai-mode use <name>")
	if code != 0 {
		return code
	}
	root, _ := filepath.Abs(*repo)
	a, err := newRoleAgent(p, agentSpec{Role: pos[0], Root: root, Owns: owns, ReadOnly: readOnly, Commands: cmds,
		MaxTurns: *maxTurns, MaxWall: *maxWall, MaxTokens: *maxTokens, ReasoningEffort: *effort, Transcript: *transcript, Log: os.Stderr},
		os.Getenv("AI_MODE_TRACE_ID"), firstNonEmpty(*callerF, os.Getenv("AI_MODE_CALLER"), "cli"))
	if err != nil {
		return fail("%v", err)
	}
	res, err := a.Run(task)
	if err != nil {
		return fail("agent: %v", err)
	}
	if *asJSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
	} else {
		fmt.Printf("stopped: %s after %d turns, %d tool calls\nwrites: %s\ntranscript: %s\n\n%s\n", res.Stopped, res.Turns, res.ToolCalls,
			strings.Join(res.Writes, ", "), a.spec.Transcript, res.Summary)
	}
	if res.Stopped != "finished" {
		return 2
	}
	return 0
}
