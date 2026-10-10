package app

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed qwen_block.md
var qwenBlockBody string

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
	printOnly := fs.Bool("print", false, "Print the routing block and exit")
	remove := fs.Bool("remove", false, "Remove the managed block")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 0 {
		return fail("usage: ai-mode setup-qwen [--file PATH] [--print] [--remove]")
	}
	if *printOnly {
		fmt.Print(qwenBlock())
		return 0
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
