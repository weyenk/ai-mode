package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

type seen struct {
	in   Intake
	opts meetingOptions
	n    int
}

func fakeRun(c StoryContract, err error) (meetingRunner, *seen) {
	s := &seen{}
	return func(in Intake, opts meetingOptions) (StoryContract, error) {
		s.in, s.opts, s.n = in, opts, s.n+1
		return c, err
	}, s
}

func amigos(args []string, stdin string, run meetingRunner) (int, string, string) {
	var o, e strings.Builder
	code := runAmigos(args, run, strings.NewReader(stdin), &o, &e)
	return code, o.String(), e.String()
}

func readyContract() StoryContract {
	return StoryContract{Story: "s", Verdict: VerdictReady, Rules: []string{"r1"}, Examples: []Example{{Rule: "r1", Text: "e1"}},
		Questions: []Question{{Text: "Q1", State: QuestionParked}}, LayerPlan: []Layer{{N: 1, Name: "core", Scenarios: []int{1}, TestLevel: "unit"}}}
}

func TestCmd_S17_ExitCodes(t *testing.T) {
	for v, want := range map[Verdict]int{VerdictReady: 0, VerdictNotReady: 2, VerdictPaused: 3, Verdict("bogus"): 1} {
		if got := exitCodeFor(v); got != want {
			t.Errorf("%s: %d want %d", v, got, want)
		}
		run, _ := fakeRun(StoryContract{Verdict: v}, nil)
		if code, _, _ := amigos([]string{"a story"}, "", run); code != want {
			t.Errorf("runAmigos %s: %d want %d", v, code, want)
		}
	}
}

func TestCmd_S18_JsonIndependentOfAudit(t *testing.T) {
	c := readyContract()
	run, s := fakeRun(c, nil)
	_, j1, _ := amigos([]string{"story", "--json", "--audit", "off"}, "", run)
	_, j2, _ := amigos([]string{"story", "--json", "--audit", "full"}, "", run)
	var got StoryContract
	if err := json.Unmarshal([]byte(j1), &got); err != nil || !reflect.DeepEqual(got, c) || strings.Contains(j1, "verdict:") {
		t.Fatalf("json output: %v %s", err, j1)
	}
	if j1 != j2 || s.opts.Audit != "full" || !s.opts.JSON {
		t.Fatalf("json must not depend on audit: %v %v", j1 == j2, s.opts)
	}
	out := filepath.Join(t.TempDir(), "o.txt")
	_, t1, _ := amigos([]string{"story", "--audit", "off", "--out", out}, "", run)
	_, t2, _ := amigos([]string{"story", "--audit", "full"}, "", run)
	if !strings.HasPrefix(t1, "verdict: ready") || t1 != t2 {
		t.Fatalf("text: %q", t1)
	}
	a, b, d := strings.Index(t1, "layer plan:"), strings.Index(t1, "questions:"), strings.Index(t1, "contract:")
	if !(0 < a && a < b && b < d) {
		t.Fatalf("order: %d %d %d", a, b, d)
	}
	if data, _ := os.ReadFile(out); string(data) != t1 {
		t.Fatalf("--out content differs")
	}
}

func TestCmd_S19_UsageAndIntakeErrors(t *testing.T) {
	run, s := fakeRun(StoryContract{}, nil)
	for _, args := range [][]string{{}, {"x", "--rounds", "1"}, {"x", "--audit", "bogus"}} {
		code, _, e := amigos(args, "", run)
		if code != 1 || !strings.Contains(e, "usage: ai-mode amigos") {
			t.Errorf("%v: %d %q", args, code, e)
		}
	}
	if s.n != 0 {
		t.Fatal("runner must not be called on a usage error")
	}
	bad, _ := fakeRun(StoryContract{}, &IntakeError{Code: ErrUnauthorizedCaller, Detail: "d"})
	code, o, e := amigos([]string{"x"}, "", bad)
	if code != 1 || strings.TrimSpace(e) != `{"code":"unauthorized_caller","detail":"d"}` || o != "" {
		t.Fatalf("%d %q %q", code, e, o)
	}
	boom, _ := fakeRun(StoryContract{}, errors.New("boom"))
	code, o, e = amigos([]string{"x"}, "", boom)
	if code != 1 || !strings.Contains(e, "error: boom") || o != "" {
		t.Fatalf("%d %q %q", code, e, o)
	}
}

func TestCmd_S33_IntakeSourcesAndPrecedence(t *testing.T) {
	run, s := fakeRun(StoryContract{Verdict: VerdictReady}, nil)
	amigos([]string{"As a user I can X"}, "", run)
	if s.in.Story != "As a user I can X" || s.in.Provenance != (Provenance{Kind: "human", CallerID: "human"}) || s.opts.Rounds != 2 || s.opts.Audit != "summary" {
		t.Fatalf("%+v %+v", s.in, s.opts)
	}
	if !regexp.MustCompile(`^\d{8}-\d{6}-[0-9a-f]{6}$`).MatchString(s.opts.MeetingID) {
		t.Fatalf("meeting id %q", s.opts.MeetingID)
	}
	amigos([]string{"x", "--caller", "three-amigos"}, "", run)
	if s.in.Provenance.CallerID != "three-amigos" {
		t.Fatal("caller")
	}
	f := filepath.Join(t.TempDir(), "s.txt")
	os.WriteFile(f, []byte("from file\n"), 0o644)
	amigos([]string{"positional", "--story", f}, "", run)
	if s.in.Story != "positional" {
		t.Fatal("positional must win")
	}
	amigos([]string{"--story", "-"}, "from stdin", run)
	if s.in.Story != "from stdin" {
		t.Fatal("stdin")
	}
	amigos([]string{"--story", f}, "", run)
	if s.in.Story != "from file" {
		t.Fatal("file")
	}
	js := `{"story":"S","provenance":{"kind":"skill","caller_id":"brainstorming"},"repo":"/r","constraints":{"exposed":true}}`
	amigos([]string{"--story", "-"}, js, run)
	want := Intake{Story: "S", Provenance: Provenance{Kind: "skill", CallerID: "brainstorming"}, Repo: "/r", Constraints: Constraints{Exposed: true}}
	if s.in != want {
		t.Fatalf("%+v", s.in)
	}
	amigos([]string{"--story", "-", "--caller", "x"}, js, run)
	want.Provenance.CallerID = "x"
	if s.in != want {
		t.Fatalf("%+v", s.in)
	}
	amigos([]string{"--resume", "m1"}, "", run)
	if s.opts.ResumeID != "m1" || s.opts.MeetingID != "m1" || s.in.Story != "" {
		t.Fatalf("%+v %+v", s.in, s.opts)
	}
	prun, _ := fakeRun(StoryContract{Verdict: VerdictPaused}, nil)
	_, o, _ := amigos([]string{"x", "--resume", "m9"}, "", prun)
	if !strings.HasSuffix(strings.TrimSpace(o), "resume with: ai-mode amigos --resume m9") {
		t.Fatalf("%q", o)
	}
}
