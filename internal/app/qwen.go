package app

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed qwen_block.md
var qwenBlockBody string

// usingAIMode is injected by the Qwen Code SessionStart hook, the way
// Superpowers injects using-superpowers. Qwen Code's built-in prompt carries
// tool calls written as text ("[tool_call: read_file for ...]") and chief copies
// them unless its first move is a real call; replaying a captured failing
// request, this text gave a real `skill` call 10/10 and then a real
// `ai-mode team` call 10/10, against 0/5 without it.
//
//go:embed using_ai_mode.md
var usingAIMode string

const qwenHookName = "ai-mode"

// qwenHookGroup is the SessionStart entry setup-qwen adds to Qwen Code's
// settings. The matcher mirrors Superpowers (resumed sessions keep their context).
func qwenHookGroup() map[string]any {
	return map[string]any{
		"matcher": "startup|clear|compact",
		"hooks": []any{map[string]any{
			"type":        "command",
			"name":        qwenHookName,
			"command":     "ai-mode hook session-start",
			"description": "Inject the ai-mode routing rule (managed by `ai-mode setup-qwen`)",
			"timeout":     10,
		}},
	}
}

// isQwenHookGroup reports whether a SessionStart matcher group is ours.
func isQwenHookGroup(g any) bool {
	hooks, _ := asMap(g)["hooks"].([]any)
	for _, h := range hooks {
		if asString(asMap(h)["name"]) == qwenHookName {
			return true
		}
	}
	return false
}

// spliceQwenHook returns settings JSON with our SessionStart hook installed
// (or removed when install is false), keeping every other setting and hook.
// Top-level keys come back sorted; values are otherwise unchanged.
func spliceQwenHook(doc []byte, install bool) (out []byte, changed bool, err error) {
	settings := map[string]any{}
	if len(bytes.TrimSpace(doc)) > 0 {
		if err := json.Unmarshal(doc, &settings); err != nil {
			return nil, false, fmt.Errorf("not valid JSON (%v); fix the file by hand", err)
		}
	}
	hooks := asMap(settings["hooks"])
	if hooks == nil {
		hooks = map[string]any{}
	}
	groups, _ := hooks["SessionStart"].([]any)
	var kept []any
	for _, g := range groups {
		if !isQwenHookGroup(g) {
			kept = append(kept, g)
		}
	}
	if install {
		kept = append(kept, qwenHookGroup())
	}
	switch {
	case len(kept) > 0:
		hooks["SessionStart"] = kept
	default:
		delete(hooks, "SessionStart")
	}
	switch {
	case len(hooks) > 0:
		settings["hooks"] = hooks
	default:
		delete(settings, "hooks")
	}
	out, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, false, err
	}
	out = append(out, '\n')
	if len(bytes.TrimSpace(doc)) > 0 {
		var before any
		_ = json.Unmarshal(doc, &before)
		var after any
		_ = json.Unmarshal(out, &after)
		b1, _ := json.Marshal(before)
		b2, _ := json.Marshal(after)
		if bytes.Equal(b1, b2) {
			return doc, false, nil
		}
	}
	return out, true, nil
}

// cmdHook runs as a Qwen Code hook. Only session-start exists: it prints the
// using-ai-mode rule as additionalContext while an ai-mode server is running,
// and nothing otherwise, so Qwen Code sessions without ai-mode are untouched.
func cmdHook(args []string) int {
	if len(args) != 1 || args[0] != "session-start" {
		return fail("usage: ai-mode hook session-start  (run by Qwen Code; installed by `ai-mode setup-qwen`)")
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	st := loadState()
	if st.Profile == "" || managedPID(st) == 0 {
		fmt.Println("{}")
		return 0
	}
	out, err := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":     "SessionStart",
		"additionalContext": strings.TrimSpace(usingAIMode),
	}})
	if err != nil {
		return fail("%v", err)
	}
	fmt.Println(string(out))
	return 0
}

const (
	qwenBegin = "<!-- BEGIN ai-mode (managed by `ai-mode setup-qwen`; edits inside are overwritten) -->"
	qwenEnd   = "<!-- END ai-mode -->"
)

func qwenBlock() string {
	return qwenBegin + "\n" + strings.TrimSpace(qwenBlockBody) + "\n" + qwenEnd + "\n"
}

// spliceQwenBlock returns doc with the managed block replaced by block (or
// appended when absent). An empty block removes it. changed is false when the
// result equals doc.
func spliceQwenBlock(doc, block string) (out string, changed bool, err error) {
	start := strings.Index(doc, qwenBegin)
	var before, after string
	switch {
	case start < 0:
		before, after = doc, ""
	default:
		rel := strings.Index(doc[start:], qwenEnd)
		if rel < 0 {
			return "", false, fmt.Errorf("found %q without a matching END marker; fix the file by hand", qwenBegin)
		}
		before, after = doc[:start], doc[start+rel+len(qwenEnd):]
		after = strings.TrimPrefix(after, "\n")
	}
	before = strings.TrimRight(before, "\n")
	after = strings.TrimLeft(after, "\n")
	var parts []string
	if before != "" {
		parts = append(parts, before)
	}
	if block != "" {
		parts = append(parts, strings.TrimRight(block, "\n"))
	}
	if after != "" {
		parts = append(parts, strings.TrimRight(after, "\n"))
	}
	out = strings.Join(parts, "\n\n")
	if out != "" {
		out += "\n"
	}
	return out, out != doc, nil
}

func cmdSetupQwen(args []string) int {
	fs := newFlags("setup-qwen")
	file := fs.String("file", "", "Target context file (default ~/.qwen/QWEN.md)")
	settingsFile := fs.String("settings", "", "Qwen Code settings file for the SessionStart hook (default ~/.qwen/settings.json)")
	printOnly := fs.Bool("print", false, "Print the routing block and the hook's injected rule, and exit")
	remove := fs.Bool("remove", false, "Remove the managed block and the hook")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 0 {
		return fail("usage: ai-mode setup-qwen [--file PATH] [--settings PATH] [--print] [--remove]")
	}
	if *printOnly {
		fmt.Print(qwenBlock())
		fmt.Printf("\n<!-- injected at session start by `ai-mode hook session-start` -->\n%s", usingAIMode)
		return 0
	}
	if rc := setupQwenHook(*settingsFile, *remove); rc != 0 {
		return rc
	}
	path := *file
	if path == "" {
		path = filepath.Join(homeDir, ".qwen", "QWEN.md")
	}
	path = expandHome(path)

	var doc string
	if data, err := os.ReadFile(path); err == nil {
		doc = string(data)
	} else if !os.IsNotExist(err) {
		return fail("%v", err)
	}
	block := qwenBlock()
	if *remove {
		block = ""
	}
	out, changed, err := spliceQwenBlock(doc, block)
	if err != nil {
		return fail("%s: %v", path, err)
	}
	if !changed {
		fmt.Printf("Up to date: %s\n", path)
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail("%v", err)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fail("%v", err)
	}
	if *remove {
		fmt.Printf("Removed ai-mode block from %s\n", path)
	} else {
		fmt.Printf("Wrote ai-mode routing block to %s (restart Qwen Code to load it)\n", path)
	}
	return 0
}

func setupQwenHook(path string, remove bool) int {
	if path == "" {
		path = filepath.Join(homeDir, ".qwen", "settings.json")
	}
	path = expandHome(path)
	doc, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fail("%v", err)
	}
	out, changed, err := spliceQwenHook(doc, !remove)
	if err != nil {
		return fail("%s: %v", path, err)
	}
	if !changed {
		fmt.Printf("Up to date: %s\n", path)
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail("%v", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fail("%v", err)
	}
	if remove {
		fmt.Printf("Removed ai-mode SessionStart hook from %s\n", path)
	} else {
		fmt.Printf("Added ai-mode SessionStart hook to %s\n", path)
	}
	return 0
}
