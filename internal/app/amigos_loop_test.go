package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type loopOut struct {
	c     StoryContract
	err   error
	asker *fakeAsker
	audit *memAudit
}

func runLoop(rounds int, score float64, turns map[string][]Turn, errs map[string][]error, mod func(*meetingOptions), au *memAudit) loopOut {
	fa := &fakeAsker{Answers: map[string]string{"product": "x", "qa": "x", "architect": "x"}}
	if au == nil {
		au = &memAudit{LevelStr: "full"}
	}
	opts := meetingOptions{Rounds: rounds, MeetingID: "m1"}
	if mod != nil {
		mod(&opts)
	}
	c, err := runMeeting(Intake{Story: "s"}, opts, fa, &fakeCodec{Turns: turns, Errs: errs}, &fakeEngine{ScoreVal: score}, au, &fakeClassifier{LayerStr: "unit"})
	return loopOut{c, err, fa, au}
}

func ans(q, text string) Turn { return Turn{Answers: []Answer{{Question: q, Text: text}}} }

func count(xs []string, prefix string) int {
	n := 0
	for _, x := range xs {
		if strings.HasPrefix(x, prefix) {
			n++
		}
	}
	return n
}

func s12turns() map[string][]Turn {
	return map[string][]Turn{
		"product":   {{Rules: []string{"r1"}, Questions: []string{"Q1"}}},
		"qa":        {{Examples: []Example{{Rule: "r1", Text: "e1"}}}},
		"architect": {{Ready: true}, ans("Q1", "A"), ans("Q1", "A"), ans("Q1", "A")},
	}
}

func TestLoop_S12_ReadyWhenResolved(t *testing.T) {
	o := runLoop(1, 1.0, s12turns(), nil, nil, nil)
	c := o.c
	if o.err != nil || c.Verdict != VerdictReady {
		t.Fatalf("want ready, got %q, %v", c.Verdict, o.err)
	}
	if c.Questions[0].State != QuestionResolved || !reflect.DeepEqual(c.Questions[0].Answers, []string{"A", "A", "A"}) || !reflect.DeepEqual(c.Rules, []string{"r1"}) {
		t.Fatalf("contract wrong: %+v", c)
	}
	if want := []string{"product", "qa", "architect", "architect", "architect", "architect"}; !reflect.DeepEqual(o.asker.Calls, want) {
		t.Fatalf("calls %v want %v", o.asker.Calls, want)
	}
}

func TestLoop_S13_RoundCapStopsNotReady(t *testing.T) {
	o := runLoop(2, 1.0, map[string][]Turn{"product": {{Rules: []string{"r1"}}, {}}, "qa": {{}, {}}, "architect": {{Ready: false}, {Ready: false}}}, nil, nil, nil)
	if o.err != nil || o.c.Verdict != VerdictNotReady || len(o.asker.Calls) != 6 {
		t.Fatalf("want not-ready after 6 calls, got %q, %v, %d", o.c.Verdict, o.err, len(o.asker.Calls))
	}
}

func TestLoop_S14_ParkedThenEscalated(t *testing.T) {
	arch := []Turn{{Ready: false}}
	for i := 0; i < 6; i++ {
		arch = append(arch, ans("Q1", "A"))
	}
	o := runLoop(2, 0.2, map[string][]Turn{"product": {{Rules: []string{"r1"}, Questions: []string{"Q1"}}}, "qa": {{Examples: []Example{{Rule: "r1", Text: "e1"}}}}, "architect": arch}, nil, nil, nil)
	if o.err != nil || o.c.Verdict != VerdictPaused || o.c.Questions[0].State != QuestionEscalated {
		t.Fatalf("got %q %+v %v", o.c.Verdict, o.c.Questions, o.err)
	}
	if count(o.asker.Calls, "product") != 1 || count(o.asker.Calls, "qa") != 1 || count(o.asker.Calls, "architect") != 7 {
		t.Fatalf("calls %v", o.asker.Calls)
	}
}

func TestLoop_S15_DisagreementParksRegardless(t *testing.T) {
	o := runLoop(1, 1.0, map[string][]Turn{
		"product":   {{Rules: []string{"r1"}, Questions: []string{"Q1"}}},
		"qa":        {{Examples: []Example{{Rule: "r1", Text: "e1"}}, Disagreements: []Disagreement{{Item: "Q1", Roles: []string{"product", "qa"}, Positions: []string{"yes", "no"}}}}},
		"architect": {{Ready: true}},
	}, nil, nil, nil)
	if o.err != nil || o.c.Questions[0].State != QuestionParked || len(o.c.Disagreements) != 1 || o.c.Verdict != VerdictNotReady || count(o.asker.Calls, "architect") != 1 {
		t.Fatalf("got %q %+v %v %v", o.c.Verdict, o.c.Questions, o.err, o.asker.Calls)
	}
}

func TestLoop_S27_UnparseableReplyReaskedOnce(t *testing.T) {
	turns := func() map[string][]Turn {
		return map[string][]Turn{"product": {{Rules: []string{"r1"}}}, "qa": {{Examples: []Example{{Rule: "r1", Text: "e1"}}}}, "architect": {{Ready: true}}}
	}
	o := runLoop(1, 1.0, turns(), map[string][]error{"product": {errors.New("bad json")}}, nil, nil)
	if o.err != nil || o.c.Verdict != VerdictReady || !reflect.DeepEqual(o.asker.Calls, []string{"product", "product", "qa", "architect"}) {
		t.Fatalf("part 1: %q %v %v", o.c.Verdict, o.err, o.asker.Calls)
	}
	o = runLoop(1, 1.0, turns(), map[string][]error{"product": {errors.New("bad json"), errors.New("still bad")}}, nil, nil)
	if o.err == nil || !strings.Contains(o.err.Error(), "product") {
		t.Fatalf("part 2: %v", o.err)
	}
}

func TestLoop_S28_WritesAuditEntries(t *testing.T) {
	o := runLoop(1, 1.0, s12turns(), nil, nil, nil)
	e := o.audit.Entries
	if count(e, "transcript:") != 6 || count(e, "decision:") != 3 || count(e, "parked:") != 0 || count(e, "contract:") != 1 {
		t.Fatalf("entries: transcript=%d decision=%d parked=%d contract=%d", count(e, "transcript:"), count(e, "decision:"), count(e, "parked:"), count(e, "contract:"))
	}
	var last string
	for _, x := range e {
		if strings.HasPrefix(x, "contract:") {
			last = x
		}
	}
	if !strings.Contains(last, "MeetingID:m1") || !strings.Contains(last, "Round:1") {
		t.Fatalf("contract entry: %s", last)
	}
	o = runLoop(1, 1.0, s12turns(), nil, nil, &memAudit{LevelStr: "full", FailWith: errors.New("disk full")})
	if o.err == nil || !strings.Contains(o.err.Error(), "audit") {
		t.Fatalf("part 2: want an audit error, got %v", o.err)
	}
}

func TestLoop_S29_ResumeContinuesFromSnapshot(t *testing.T) {
	snap := &Snapshot{
		Contract: StoryContract{Story: "s", Rules: []string{"r1"}, Examples: []Example{{Rule: "r1", Text: "e1"}}, Questions: []Question{{Text: "Q1", State: QuestionParked}}},
		State:    MeetingState{MeetingID: "m1", Round: 1},
	}
	o := runLoop(2, 1.0, map[string][]Turn{
		"product":   {{}},
		"qa":        {{}},
		"architect": {ans("Q1", "A"), ans("Q1", "A"), ans("Q1", "A"), {Ready: true}},
	}, nil, func(op *meetingOptions) { op.Resume = snap }, nil)
	if o.err != nil || o.c.Verdict != VerdictReady || o.c.Questions[0].State != QuestionResolved {
		t.Fatalf("got %q %+v %v", o.c.Verdict, o.c.Questions, o.err)
	}
	if want := []string{"architect", "architect", "architect", "product", "qa", "architect"}; !reflect.DeepEqual(o.asker.Calls, want) {
		t.Fatalf("calls %v want %v", o.asker.Calls, want)
	}
	var last string
	for _, x := range o.audit.Entries {
		if strings.HasPrefix(x, "contract:") {
			last = x
		}
	}
	if !strings.Contains(last, "Round:2") {
		t.Fatalf("last contract entry: %s", last)
	}
}

func TestLoop_S32_OnlyArchitectSetsSizeSplitAndLayerPlan(t *testing.T) {
	o := runLoop(1, 1.0, map[string][]Turn{
		"product":   {{Rules: []string{"r1"}, Size: 5, SplitInto: []string{"from-product"}, LayerPlan: []Layer{{N: 9, Name: "bad"}}}},
		"qa":        {{Examples: []Example{{Rule: "r1", Text: "e1"}}}},
		"architect": {{Ready: true, Size: 2, SplitInto: []string{"a", "b"}, LayerPlan: []Layer{{N: 1, Name: "core", Scenarios: []int{1}}}}},
	}, nil, nil, nil)
	if o.err != nil || o.c.Size != 2 || !reflect.DeepEqual(o.c.SplitInto, []string{"a", "b"}) || len(o.c.LayerPlan) != 1 || o.c.LayerPlan[0].Name != "core" {
		t.Fatalf("got size=%d split=%v plan=%+v err=%v", o.c.Size, o.c.SplitInto, o.c.LayerPlan, o.err)
	}
}

func TestLoop_RepeatedExamplesAreNotDuplicated(t *testing.T) {
	e := Example{Rule: "r1", Text: "e1"}
	o := runLoop(2, 1.0, map[string][]Turn{
		"product":   {{Rules: []string{"r1"}}, {Rules: []string{"r1"}}},
		"qa":        {{Examples: []Example{e}}, {Examples: []Example{e, {Rule: "r1", Text: "e2"}}}},
		"architect": {{}, {Ready: true}},
	}, nil, nil, nil)
	if o.err != nil || len(o.c.Examples) != 2 {
		t.Fatalf("want 2 distinct examples, got %+v (%v)", o.c.Examples, o.err)
	}
}

func TestLoop_AuditRecordsScoreTimestampAndRationale(t *testing.T) {
	au := &memAudit{LevelStr: "full"}
	o := runLoop(1, 0.9, s12turns(), nil, nil, au)
	if o.err != nil || o.c.Questions[0].Score != 0.9 {
		t.Fatalf("score not recorded: %+v %v", o.c.Questions, o.err)
	}
	n := 0
	for _, e := range au.Entries {
		if !strings.HasPrefix(e, "decision:") {
			continue
		}
		n++
		if strings.Contains(e, "TS: ") || strings.Contains(e, "TS:}") || strings.Contains(e, "Rationale:}") || !strings.Contains(e, "Rationale:opinion") {
			t.Errorf("decision lacks a timestamp or rationale: %s", e)
		}
	}
	if n != 3 {
		t.Fatalf("want 3 decisions, got %d", n)
	}
}
