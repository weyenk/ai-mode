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
