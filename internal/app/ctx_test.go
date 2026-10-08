package app

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

const ctxIni = `
version = 1
[*]
ctx-size = 8192
jinja = 1

[chief]
hf = x
ctx-size = 131072

[coder-xl]
ctx_size = 65536

[jev]
c = 16384

[fast]
hf = y
`

func TestIniRoleCtx(t *testing.T) {
	got := iniRoleCtx(ctxIni)
	want := map[string]int{"chief": 131072, "coder-xl": 65536, "jev": 16384, "fast": 8192}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// No default and no per-role value: omitted rather than guessed.
	if got := iniRoleCtx("[a]\nhf = x\n"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	// Comments and junk values are ignored.
	if got := iniRoleCtx("[a]\n; ctx-size = 5\nctx-size = abc\n"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

var ctxLog = []string{
	"srv: spawning server instance with name=chief on port 9001",
	"srv: spawning server instance with name=coder-xl on port 9002",
	"[9001] 0.01.103.788 I srv    load_model: initializing, n_slots = 4, n_ctx_slot = 40960, kv_unified = 'true'",
	"[9002] 0.01.103.788 I srv    load_model: initializing, n_slots = 4, n_ctx_slot = 65536, kv_unified = 'true'",
	"[9001] 5.00.000.000 I slot release: id 2 | n_tokens = 3414",
}

func TestEffectiveCtxFromLog(t *testing.T) {
	got := effectiveCtxFromLog(ctxLog)
	want := map[string]int{"chief": 40960, "coder-xl": 65536}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestEffectiveCtxLatestWinsAfterRestart(t *testing.T) {
	lines := append([]string{}, ctxLog...)
	lines = append(lines,
		"srv: spawning server instance with name=chief on port 9101",
		"[9101] 0.01.000.000 I srv    load_model: initializing, n_slots = 4, n_ctx_slot = 131072, kv_unified = 'true'",
	)
	if got := effectiveCtxFromLog(lines)["chief"]; got != 131072 {
		t.Fatalf("got %d, want latest 131072", got)
	}
}

func TestEffectiveCtxIgnoresUnmappedPorts(t *testing.T) {
	lines := []string{"[9999] I srv load_model: initializing, n_slots = 4, n_ctx_slot = 1234"}
	if got := effectiveCtxFromLog(lines); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestCompareCtx(t *testing.T) {
	preset := map[string]int{"chief": 131072, "coder-xl": 65536, "jev": 16384, "fast": 16384}
	eff := map[string]int{"chief": 40960, "coder-xl": 65536, "jev": 32768}
	got := compareCtx(preset, eff, []string{"jev", "chief", "coder-xl", "fast", "ghost"})
	want := []ctxMismatch{{"chief", 131072, 40960}, {"jev", 16384, 32768}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// Roles not in the list (not loaded) are never reported.
	if got := compareCtx(preset, eff, []string{"coder-xl"}); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestCtxMismatchMessage(t *testing.T) {
	capped := ctxMismatch{"chief", 131072, 40960}.String()
	for _, s := range []string{"chief", "131072", "40960", "YaRN"} {
		if !strings.Contains(capped, s) {
			t.Fatalf("capped message %q missing %q", capped, s)
		}
	}
	stale := ctxMismatch{"chief", 40960, 131072}.String()
	if !strings.Contains(stale, "ai-mode restart") {
		t.Fatalf("stale message %q should suggest restart", stale)
	}
}

func TestLogViewHyphenatedRole(t *testing.T) {
	v := newLogView(true, "")
	v.process("srv: spawning server instance with name=coder-xl on port 9002")
	out, _ := v.process("[9002] hello")
	if out != "[coder-xl:9002] hello" {
		t.Fatalf("got %q", out)
	}
}

func TestPresetChangedSince(t *testing.T) {
	start := "2026-10-08T12:00:00.000000+00:00"
	base, _ := time.Parse("2006-01-02T15:04:05.000000-07:00", start)
	if !presetChangedSince(base.Add(time.Minute), start) {
		t.Fatal("ini newer than start should be flagged")
	}
	if presetChangedSince(base.Add(-time.Minute), start) {
		t.Fatal("ini older than start should not be flagged")
	}
	if presetChangedSince(base.Add(time.Hour), "garbage") || presetChangedSince(base.Add(time.Hour), "") {
		t.Fatal("unparseable start must not flag")
	}
}
