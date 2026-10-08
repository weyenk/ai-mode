package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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

func TestDecide(t *testing.T) {
	cases := []struct {
		r    []ranked
		conf float64
		want string
	}{
		{[]ranked{{"qa", 0.7}, {"coder", 0.1}}, 0.6, "clear"},
		{[]ranked{{"functional", 0.34}, {"integration", 0.32}}, 0.26, "ambiguous"},
		{[]ranked{{"ux", 0.5}, {"qa", 0.2}}, 0.17, "low-confidence"},
	}
	for _, c := range cases {
		if got := decide(c.r, c.conf); got != c.want {
			t.Fatalf("decide(%v,%v)=%s want %s", c.r, c.conf, got, c.want)
		}
	}
}

func TestRankProbs(t *testing.T) {
	got := rankProbs(map[string]any{"b": 0.2, "a": 0.2, "c": 0.6})
	want := []ranked{{"c", 0.6}, {"a", 0.2}, {"b", 0.2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRoleOptionsUsesSummaries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AI_MODE_PRESETS", dir)
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("p.ini", "[*]\njinja = 1\n[chief]\nhf = x\n[jev]\nhf = x\n[qa]\nhf = x\n[old]\nhf = x\n[fast]\nhf = x\n")
	write("agents/chief.md", "---\nrole: chief\nsummary: Talk to human\n---\nbody")
	write("agents/jev.md", "---\nrole: jev\n---\nbody") // no summary → excluded
	write("agents/qa.md", "---\nrole: qa\nsummary: Tests: plans, cases\n---\nbody")
	write("agents/old.md", "---\nrole: old\nsummary: legacy\ndeprecated: true\n---\nbody")
	// fast has no agent file at all
	p := Profile{Name: "p", Ini: filepath.Join(dir, "p.ini")}

	got := roleOptions(p, nil, nil)
	want := []option{{"chief", "Talk to human"}, {"qa", "Tests: plans, cases"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := roleOptions(p, nil, []string{"chief"}); len(got) != 1 || got[0].Key != "qa" {
		t.Fatalf("exclude: %v", got)
	}
	if got := roleOptions(p, []string{"chief"}, nil); len(got) != 1 || got[0].Key != "chief" {
		t.Fatalf("only: %v", got)
	}
}

func TestLayerOptionsRequireGuideFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AI_MODE_PRESETS", dir)
	gdir := filepath.Join(dir, "agents", "qa", "guidelines")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"unit.md", "e2e.md"} {
		if err := os.WriteFile(filepath.Join(gdir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := optionKeys(layerOptions(nil, nil))
	if !reflect.DeepEqual(got, []string{"unit", "e2e"}) {
		t.Fatalf("got %v", got)
	}
}

func TestParseEvalCases(t *testing.T) {
	cases, err := parseEvalCases("# c\n\n{\"task\":\"a\",\"expect\":\"qa\"}\n{\"question\":\"layer\",\"task\":\"b\",\"expect\":\"unit\"}\n")
	if err != nil || len(cases) != 2 || cases[0].Question != "role" || cases[1].Question != "layer" {
		t.Fatalf("%v %+v", err, cases)
	}
	for _, bad := range []string{"{not json}", "{\"task\":\"a\"}", "{\"question\":\"x\",\"task\":\"a\",\"expect\":\"b\"}"} {
		if _, err := parseEvalCases(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestSummarizeEval(t *testing.T) {
	mk := func(exp, got, dec string, conf float64) evalRow {
		return evalRow{evalCase: evalCase{Expect: exp}, Got: got, Decision: dec, Confidence: conf, Margin: conf}
	}
	s := summarizeEval([]evalRow{
		mk("qa", "qa", "clear", 0.8),
		mk("qa", "qa", "ambiguous", 0.2), // right but unclear
		mk("ux", "design", "clear", 0.6), // wrong but clear
		mk("docs", "coder", "low-confidence", 0.2),
		{evalCase: evalCase{Expect: "x"}, Err: "boom"},
	})
	if s.N != 5 || s.Correct != 2 || s.Errors != 1 || s.WrongButClear != 1 || s.RightButUnclear != 1 || s.Clear != 2 {
		t.Fatalf("%+v", s)
	}
	if s.Confusions["ux→design"] != 1 || s.Confusions["docs→coder"] != 1 {
		t.Fatalf("%v", s.Confusions)
	}
}

func TestParseClassifyAnswer(t *testing.T) {
	cands := []string{"architect", "coder", "security", "qa"}
	cases := []struct {
		in      string
		act     reviewAction
		correct string
		note    string
	}{
		{"", actOK, "", ""},
		{"y looks fine", actOK, "", "looks fine"},
		{"n", actBad, "", ""},
		{"sec", actFix, "security", ""},
		{"QA tests not code", actFix, "qa", "tests not code"},
		{"a", actInvalid, "", ""}, // ambiguous prefix? only architect starts with "a" → fix
		{"zzz", actInvalid, "", ""},
		{"s", actSkip, "", ""},
		{"q", actQuit, "", ""},
		{"i", actIgnore, "", ""},
	}
	for _, c := range cases {
		act, correct, note := parseClassifyAnswer(c.in, cands)
		want := c.act
		if c.in == "a" { // unique prefix of "architect"
			want, c.correct = actFix, "architect"
		}
		if act != want || correct != c.correct || note != c.note {
			t.Fatalf("%q → %v %q %q; want %v %q %q", c.in, act, correct, note, want, c.correct, c.note)
		}
	}
	if _, ok := matchCandidate("c", []string{"coder", "chief"}); ok {
		t.Fatal("ambiguous prefix must not match")
	}
}

func TestParseAskAnswer(t *testing.T) {
	if a, n := parseAskAnswer("n too long"); a != actBad || n != "too long" {
		t.Fatalf("%v %q", a, n)
	}
	if a, _ := parseAskAnswer(""); a != actOK {
		t.Fatal("empty should be ok")
	}
	if a, _ := parseAskAnswer("f"); a != actFull {
		t.Fatal("f should be full")
	}
	if a, _ := parseAskAnswer("???"); a != actInvalid {
		t.Fatal("garbage should be invalid")
	}
}

func TestPendingReviews(t *testing.T) {
	now := time.Now().UTC()
	evs := []Event{
		{Kind: "classify", Span: "a", TS: now.Add(-3 * time.Hour), Status: "ok", Payload: "t/a.json"},
		{Kind: "ask", Span: "b", TS: now.Add(-2 * time.Hour), Status: "ok", Payload: "t/b.json", Role: "qa"},
		{Kind: "ask", Span: "c", TS: now.Add(-1 * time.Hour), Status: "error", Payload: "t/c.json"}, // failed: skip
		{Kind: "ask", Span: "d", TS: now.Add(-30 * time.Minute), Status: "ok"},                      // no payload: skip
		{Kind: "use", Span: "e", TS: now, Status: "ok", Payload: "x"},                               // not a call
		{Kind: "ask", Span: "f", TS: now.Add(-100 * time.Hour), Status: "ok", Payload: "t/f.json"},  // too old
	}
	got := pendingReviews(evs, map[string]Review{}, reviewFilter{since: 24 * time.Hour})
	if len(got) != 2 || got[0].Span != "a" || got[1].Span != "b" {
		t.Fatalf("%+v", got)
	}
	got = pendingReviews(evs, map[string]Review{"a": {Span: "a"}}, reviewFilter{since: 24 * time.Hour})
	if len(got) != 1 || got[0].Span != "b" {
		t.Fatalf("reviewed span should be excluded: %+v", got)
	}
	got = pendingReviews(evs, map[string]Review{"a": {Span: "a"}}, reviewFilter{since: 24 * time.Hour, again: true, limit: 1})
	if len(got) != 1 || got[0].Span != "b" {
		t.Fatalf("limit keeps newest: %+v", got)
	}
	if got := pendingReviews(evs, nil, reviewFilter{kind: "classify", since: 24 * time.Hour}); len(got) != 1 || got[0].Span != "a" {
		t.Fatalf("kind filter: %+v", got)
	}
}

func TestExportReviews(t *testing.T) {
	t.Setenv("AI_MODE_STATE", t.TempDir())
	appendReview(Review{Span: "1", Kind: "classify", Question: "role", Task: "t1", Choice: "qa", Verdict: "ok"})
	appendReview(Review{Span: "2", Kind: "classify", Question: "role", Task: "t2", Choice: "coder", Verdict: "fixed", Correct: "qa"})
	appendReview(Review{Span: "3", Kind: "classify", Question: "role", Task: "t3", Choice: "ux", Verdict: "bad"})
	appendReview(Review{Span: "4", Kind: "ask", Role: "qa", Task: "q", Verdict: "ok"})
	appendReview(Review{Span: "2", Kind: "classify", Question: "role", Task: "t2", Choice: "coder", Verdict: "fixed", Correct: "security"}) // re-review wins
	path := filepath.Join(t.TempDir(), "r.jsonl")
	if code := exportReviews(path); code != 0 {
		t.Fatal(code)
	}
	data, _ := os.ReadFile(path)
	cases, err := parseEvalCases(string(data))
	if err != nil || len(cases) != 2 || cases[0].Expect != "qa" || cases[1].Expect != "security" {
		t.Fatalf("%v %+v", err, cases)
	}
	exportReviews(path) // idempotent
	data2, _ := os.ReadFile(path)
	if string(data) != string(data2) {
		t.Fatal("second export must not duplicate")
	}
}
