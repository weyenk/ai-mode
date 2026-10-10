package app

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	parkThreshold = 0.7
	sampleCount   = 3
)

var meetingRoles = []string{"product", "qa", "architect"}

type loopRun struct {
	in   Intake
	opts meetingOptions
	ra   roleAsker
	tc   turnCodec
	ue   uncertaintyEngine
	au   auditSink
	c    StoryContract
}

func runMeeting(in Intake, opts meetingOptions, ra roleAsker, tc turnCodec, ue uncertaintyEngine, au auditSink, lc layerClassifier) (StoryContract, error) {
	l := &loopRun{in: in, opts: opts, ra: ra, tc: tc, ue: ue, au: au, c: StoryContract{Story: in.Story}}
	start, resumed := 1, false
	if opts.Resume != nil {
		l.c = opts.Resume.Contract
		start, resumed = opts.Resume.State.Round+1, true
	}
	for round := start; round <= opts.Rounds; round++ {
		if round > 1 || resumed {
			paused, err := l.reaskParked(round)
			if err != nil {
				return l.c, err
			}
			if paused {
				l.c.Verdict = VerdictPaused
				return l.c, l.snapshot(round)
			}
		}
		architectReady := false
		for _, role := range meetingRoles {
			turn, err := l.askTurn(role, "")
			if err != nil {
				return l.c, err
			}
			if err := l.merge(round, role, turn); err != nil {
				return l.c, err
			}
			if role == "architect" {
				architectReady = turn.Ready
			}
		}
		for i := range l.c.Questions {
			if l.c.Questions[i].State == QuestionOpen {
				if err := l.sample(i); err != nil {
					return l.c, err
				}
			}
		}
		if l.ready(architectReady) {
			l.c.Verdict = VerdictReady
			return l.c, l.snapshot(round)
		}
		if round == opts.Rounds {
			l.c.Verdict = VerdictNotReady
			return l.c, l.snapshot(round)
		}
		if err := l.snapshot(round); err != nil {
			return l.c, err
		}
	}
	l.c.Verdict = VerdictNotReady
	return l.c, nil
}

func (l *loopRun) snapshot(round int) error {
	var parked []Question
	for _, q := range l.c.Questions {
		if q.State == QuestionParked {
			parked = append(parked, q)
		}
	}
	snap := Snapshot{Contract: l.c, State: MeetingState{MeetingID: l.opts.MeetingID, Round: round, NextRole: "product", Parked: parked}}
	if err := l.au.Write("contract", snap); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return nil
}

func (l *loopRun) ask(role, focus string) (string, string, error) {
	prompt := l.tc.Prompt(role, l.in, l.c, focus)
	return prompt, "", nil
}

func (l *loopRun) askTurn(role, focus string) (Turn, error) {
	prompt := l.tc.Prompt(role, l.in, l.c, focus)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		p := prompt
		if attempt == 1 {
			p = prompt + "\nPrevious reply was invalid: " + lastErr.Error()
		}
		raw, err := l.ra.Ask(role, p, 2048)
		if err != nil {
			return Turn{}, fmt.Errorf("%s: %w", role, err)
		}
		if err := l.au.Write("transcript", map[string]string{"role": role, "prompt": p, "reply": raw}); err != nil {
			return Turn{}, fmt.Errorf("audit: %w", err)
		}
		t, err := l.tc.Parse(role, raw)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return Turn{}, fmt.Errorf("%s: unparseable reply twice: %w", role, lastErr)
}

func (l *loopRun) setState(i int, state string) error {
	l.c.Questions[i].State = state
	if state == QuestionParked || state == QuestionEscalated {
		if err := l.au.Write("parked", l.c.Questions[i]); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
	}
	return nil
}

func (l *loopRun) merge(round int, role string, t Turn) error {
	c := &l.c
	for _, r := range t.Rules {
		if !hasString(c.Rules, r) {
			c.Rules = append(c.Rules, r)
		}
	}
	for _, e := range t.Examples {
		if !slices.Contains(c.Examples, e) {
			c.Examples = append(c.Examples, e)
		}
	}
	for _, q := range t.Questions {
		if findQuestion(c, q) < 0 {
			c.Questions = append(c.Questions, Question{Text: q, State: QuestionOpen})
		}
	}
	for _, a := range t.Answers {
		if i := findQuestion(c, a.Question); i >= 0 && (c.Questions[i].State == QuestionOpen || c.Questions[i].State == QuestionReasked) {
			c.Questions[i].State = QuestionResolved
			c.Questions[i].Answers = append(c.Questions[i].Answers, a.Text)
		}
	}
	for _, d := range t.Disagreements {
		c.Disagreements = append(c.Disagreements, d)
		if i := findQuestion(c, d.Item); i >= 0 {
			if c.Questions[i].State == QuestionOpen {
				if err := l.setState(i, QuestionParked); err != nil {
					return err
				}
			}
		} else {
			c.Questions = append(c.Questions, Question{Text: d.Item})
			if err := l.setState(len(c.Questions)-1, QuestionParked); err != nil {
				return err
			}
		}
	}
	if role == "architect" {
		if t.Size != 0 {
			c.Size = t.Size
		}
		if t.SplitInto != nil {
			c.SplitInto = t.SplitInto
		}
		if t.LayerPlan != nil {
			c.LayerPlan = t.LayerPlan
		}
	}
	opinion := "opinion: not ready"
	if t.Ready {
		opinion = "opinion: ready"
	}
	if len(t.Disagreements) > 0 {
		var items []string
		for _, d := range t.Disagreements {
			items = append(items, d.Item)
		}
		opinion += "; disagreements: " + strings.Join(items, "; ")
	}
	dec := DecisionEntry{TS: time.Now().UTC().Format(time.RFC3339), Rationale: opinion, Round: round, Role: role, Decision: fmt.Sprintf("rules=%d examples=%d questions=%d answers=%d disagreements=%d", len(t.Rules), len(t.Examples), len(t.Questions), len(t.Answers), len(t.Disagreements))}
	if err := l.au.Write("decision", dec); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return nil
}

func (l *loopRun) sample(i int) error {
	q := l.c.Questions[i]
	var texts []string
	for n := 0; n < sampleCount; n++ {
		t, err := l.askTurn("architect", q.Text)
		if err != nil {
			return err
		}
		text := ""
		for _, a := range t.Answers {
			if a.Question == q.Text {
				text = a.Text
				break
			}
		}
		texts = append(texts, text)
	}
	l.c.Questions[i].Answers = texts
	l.c.Questions[i].Score = l.ue.Score(q.Text, texts)
	if l.c.Questions[i].Score >= parkThreshold {
		l.c.Questions[i].State = QuestionResolved
		return nil
	}
	return l.setState(i, QuestionParked)
}

func (l *loopRun) reaskParked(round int) (bool, error) {
	for i := range l.c.Questions {
		if l.c.Questions[i].State != QuestionParked {
			continue
		}
		l.c.Questions[i].State = QuestionReasked
		if err := l.sample(i); err != nil {
			return false, err
		}
		if l.c.Questions[i].State == QuestionParked {
			if err := l.setState(i, QuestionEscalated); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func (l *loopRun) ready(architectReady bool) bool {
	if !architectReady {
		return false
	}
	for _, q := range l.c.Questions {
		if q.State != QuestionResolved {
			return false
		}
	}
	for _, r := range l.c.Rules {
		ok := false
		for _, e := range l.c.Examples {
			if e.Rule == r {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func hasString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func findQuestion(c *StoryContract, text string) int {
	for i, q := range c.Questions {
		if q.Text == text {
			return i
		}
	}
	return -1
}
