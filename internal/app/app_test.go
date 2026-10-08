package app

import (
	"reflect"
	"testing"
)

func TestParseKV(t *testing.T) {
	got := parseKV("# c\n; c\nPort = 8080\nmodels_max = 5\ndescription = a = b\nwarm = \"x,y\"\nnoequals\n")
	want := map[string]string{"port": "8080", "models-max": "5", "description": "a = b", "warm": "x,y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestIniSections(t *testing.T) {
	got := iniSections("version = 1\n[*]\njinja = 1\n[chief]\nhf = x\n  [jev]  \n")
	if !reflect.DeepEqual(got, []string{"chief", "jev"}) {
		t.Fatalf("got %v", got)
	}
}

func TestParseAgent(t *testing.T) {
	meta, body := parseAgent("---\nrole: chief\ntrigger: \"x: y\"\n---\n\nBody here\n")
	if meta["role"] != "chief" || meta["trigger"] != "x: y" {
		t.Fatalf("meta %v", meta)
	}
	if body != "Body here\n" {
		t.Fatalf("body %q", body)
	}
	meta, body = parseAgent("no frontmatter")
	if len(meta) != 0 || body != "no frontmatter" {
		t.Fatalf("got %v %q", meta, body)
	}
}

func TestParseArgsInterspersed(t *testing.T) {
	fs := newFlags("t")
	force := fs.Bool("force", false, "")
	to := fs.Float64("timeout", 0, "")
	pos, err := parseArgs(fs, []string{"dev-shop", "--force", "extra", "--timeout", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if !*force || *to != 5 || !reflect.DeepEqual(pos, []string{"dev-shop", "extra"}) {
		t.Fatalf("force=%v to=%v pos=%v", *force, *to, pos)
	}
}

func TestLogViewAnnotateAndFilter(t *testing.T) {
	v := newLogView(true, "chief")
	lines := []string{
		"srv: spawning server instance with name=chief on port 9001",
		"srv: spawning server instance with name=jev on port 9002",
		"[9001] loaded",
		"[9002] other",
	}
	var out []string
	for _, l := range lines {
		if s, ok := v.process(l); ok {
			out = append(out, s)
		}
	}
	want := []string{lines[0], "[chief:9001] loaded"}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestExtractAssistantText(t *testing.T) {
	if got := assistantText(map[string]any{"content": " ", "reasoning_content": " hmm "}); got != "hmm" {
		t.Fatalf("got %q", got)
	}
}

func TestParseSince(t *testing.T) {
	for in, want := range map[string]string{"30m": "30m0s", "24h": "24h0m0s", "7d": "168h0m0s", "2": "2h0m0s", "": "0s"} {
		d, err := parseSince(in)
		if err != nil || d.String() != want {
			t.Fatalf("%q -> %v %v want %s", in, d, err, want)
		}
	}
	if _, err := parseSince("abc"); err == nil {
		t.Fatal("expected error")
	}
}

func TestEmitAndReadEvents(t *testing.T) {
	t.Setenv("AI_MODE_STATE", t.TempDir())
	emit(Event{Kind: "ask", Trace: "t1", Span: "s1", Role: "fast", DurationMS: 5})
	emit(Event{Kind: "ask", Trace: "t1", Span: "s2", Parent: "s1", Role: "fast", Status: "error", Error: "boom"})
	evs := readEvents()
	if len(evs) != 2 || evs[0].Status != "ok" || evs[1].Error != "boom" {
		t.Fatalf("got %+v", evs)
	}
	if got := filterEvents(evs, "", "", "", "error", 0); len(got) != 1 {
		t.Fatalf("filter got %d", len(got))
	}
	sums := summarizeTraces(evs)
	if len(sums) != 1 || sums[0].calls != 2 || sums[0].errs != 1 {
		t.Fatalf("sums %+v", sums)
	}
}

func TestPayloadCaptureToggle(t *testing.T) {
	t.Setenv("AI_MODE_STATE", t.TempDir())
	if p := savePayload("t", "s", map[string]any{"a": 1}, map[string]any{"b": 2}); p == "" {
		t.Fatal("expected payload path")
	}
	t.Setenv("AI_MODE_CAPTURE", "0")
	if p := savePayload("t", "s2", nil, nil); p != "" {
		t.Fatal("capture should be off")
	}
}

func TestRecordUsage(t *testing.T) {
	var ev Event
	recordUsage(&ev, map[string]any{
		"usage":   map[string]any{"prompt_tokens": 10.0, "completion_tokens": 4.0, "prompt_tokens_details": map[string]any{"cached_tokens": 3.0}},
		"timings": map[string]any{"predicted_per_second": 50.0},
		"choices": []any{map[string]any{"finish_reason": "length"}},
	})
	if ev.PromptTokens != 10 || ev.CompletionTokens != 4 || ev.CachedTokens != 3 || ev.GenTPS != 50 || ev.FinishReason != "length" {
		t.Fatalf("%+v", ev)
	}
}
