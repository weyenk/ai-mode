package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func deps(t *testing.T, turns map[string][]Turn) (wireDeps, *fakeAsker) {
	t.Helper()
	fa := &fakeAsker{Answers: map[string]string{"product": "x", "qa": "x", "architect": "x"}}
	return wireDeps{Asker: fa, Codec: &fakeCodec{Turns: turns}, Engine: &fakeEngine{ScoreVal: 1}, Classifier: &fakeClassifier{LayerStr: "unit"},
		Callers: []string{"human"}, Roots: []string{t.TempDir()}, StateDir: t.TempDir()}, fa
}

func TestWire_S22_Registered(t *testing.T) {
	if _, ok := commands()["amigos"]; !ok {
		t.Fatal(`"amigos" is not registered in commands()`)
	}
	var b strings.Builder
	usage(&b)
	if !strings.Contains(b.String(), "amigos") {
		t.Fatal("usage")
	}
	comp, err := os.ReadFile("../../completions/_ai-mode")
	if err != nil || !strings.Contains(string(comp), "'amigos:") {
		t.Fatal("completions")
	}
}

func TestWire_S23_GateRunsBeforeAnyModelCall(t *testing.T) {
	d, fa := deps(t, nil)
	run := newMeetingRunnerWith(d)
	opts := meetingOptions{Rounds: 2, Audit: "summary", MeetingID: "m1"}
	cases := []struct {
		in   Intake
		want string
	}{
		{Intake{Story: "ok", Provenance: Provenance{CallerID: "stranger"}}, ErrUnauthorizedCaller},
		{Intake{Story: "tk_" + strings.Repeat("a", 24), Provenance: Provenance{CallerID: "human"}}, ErrSecretDetected},
		{Intake{Story: "ok", Provenance: Provenance{CallerID: "human"}, Repo: "https://example.com/r.git"}, ErrPathOutsideRoots},
	}
	for _, c := range cases {
		_, err := run(c.in, opts)
		var ie *IntakeError
		if !errors.As(err, &ie) || ie.Code != c.want {
			t.Errorf("want %s, got %v", c.want, err)
		}
		if len(fa.Calls) != 0 || exists(filepath.Join(d.StateDir, "amigos")) {
			t.Fatalf("model call or side effect before the gate: %v", fa.Calls)
		}
	}
}

func TestWire_S34_ResumeLoadsSnapshot(t *testing.T) {
	d, fa := deps(t, map[string][]Turn{"product": {{}}, "qa": {{}}, "architect": {{Ready: true}}})
	snap := Snapshot{Contract: StoryContract{Story: "saved story", Rules: []string{"r1"}, Examples: []Example{{Rule: "r1", Text: "e1"}}, LayerPlan: []Layer{{N: 1, Name: "core", Scenarios: []int{1}}}}, State: MeetingState{MeetingID: "m1", Round: 1}}
	s, _ := newFileAudit("summary", filepath.Join(d.StateDir, "amigos", "m1"), 0)
	s.Write("contract", snap)
	run := newMeetingRunnerWith(d)
	c, err := run(Intake{Provenance: Provenance{CallerID: "human"}}, meetingOptions{Rounds: 2, Audit: "summary", ResumeID: "m1", MeetingID: "m1"})
	if err != nil || c.Story != "saved story" || c.Verdict != VerdictReady || strings.Join(fa.Calls, ",") != "product,qa,architect" {
		t.Fatalf("%q %q %v %v", c.Story, c.Verdict, err, fa.Calls)
	}
	got, err := loadSnapshot(filepath.Join(d.StateDir, "amigos"), "m1")
	if err != nil || got.State.Round != 2 {
		t.Fatalf("round after resume: %+v %v", got.State, err)
	}
	n := len(fa.Calls)
	if _, err := run(Intake{Provenance: Provenance{CallerID: "human"}}, meetingOptions{Rounds: 2, ResumeID: "nope", MeetingID: "nope"}); err == nil || !strings.Contains(err.Error(), "nope") || len(fa.Calls) != n {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestWire_S35_InvalidLayerPlanDowngradesReady(t *testing.T) {
	mk := func(scen []int) map[string][]Turn {
		return map[string][]Turn{"product": {{Rules: []string{"r1"}}}, "qa": {{Examples: []Example{{Rule: "r1", Text: "e1"}, {Rule: "r1", Text: "e2"}}}},
			"architect": {{Ready: true, LayerPlan: []Layer{{N: 1, Name: "core", Scenarios: scen}}}}}
	}
	d, _ := deps(t, mk([]int{1}))
	c, err := newMeetingRunnerWith(d)(Intake{Story: "s", Provenance: Provenance{CallerID: "human"}}, meetingOptions{Rounds: 1, Audit: "off", MeetingID: "m1"})
	if err != nil || c.Verdict != VerdictNotReady || len(c.Questions) == 0 || c.Questions[len(c.Questions)-1].State != QuestionOpen || !strings.HasPrefix(c.Questions[len(c.Questions)-1].Text, "layer plan:") {
		t.Fatalf("%q %+v %v", c.Verdict, c.Questions, err)
	}
	for exposed, want := range map[bool]string{true: "active", false: "not-applicable"} {
		d, _ := deps(t, mk([]int{1, 2}))
		c, err := newMeetingRunnerWith(d)(Intake{Story: "s", Provenance: Provenance{CallerID: "human"}, Constraints: Constraints{Exposed: exposed}}, meetingOptions{Rounds: 1, Audit: "off", MeetingID: "m1"})
		if err != nil || c.Verdict != VerdictReady || c.LayerPlan[0].TestLevel != "unit" || c.Gates[4].Name != "dynamic-security" || c.Gates[4].State != want {
			t.Fatalf("exposed=%v: %q %+v %+v %v", exposed, c.Verdict, c.LayerPlan, c.Gates, err)
		}
	}
}

func TestWire_SavedContractHasFinalGatesAndVerdict(t *testing.T) {
	d, _ := deps(t, map[string][]Turn{"product": {{Rules: []string{"r1"}}}, "qa": {{Examples: []Example{{Rule: "r1", Text: "e1"}}}},
		"architect": {{Ready: true, LayerPlan: []Layer{{N: 1, Name: "core", Scenarios: []int{1}}}}}})
	c, err := newMeetingRunnerWith(d)(Intake{Story: "s", Provenance: Provenance{CallerID: "human"}, Constraints: Constraints{Exposed: true}}, meetingOptions{Rounds: 1, Audit: "summary", MeetingID: "m9"})
	if err != nil || c.Verdict != VerdictReady {
		t.Fatalf("%q %v", c.Verdict, err)
	}
	snap, err := loadSnapshot(filepath.Join(d.StateDir, "amigos"), "m9")
	if err != nil || len(snap.Contract.Gates) == 0 || snap.Contract.LayerPlan[0].TestLevel != "unit" || snap.Contract.Verdict != VerdictReady || snap.State.Round != 1 {
		t.Fatalf("saved contract is not the final one: %+v %v", snap, err)
	}
}
