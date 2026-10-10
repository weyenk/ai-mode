package app

import (
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
		!strings.Contains(s, `"PreToolUse"`) || !strings.Contains(s, `"chief"`) {
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
