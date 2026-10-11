package app

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	// parkThreshold: a question resolves when at least two thirds of the non-empty samples agree (by decision, see judgedEngine)
	parkThreshold = 0.66
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
	// round is the meeting round the next turn belongs to (0 while sampling outside a round)
	round int
}

// roundNote tells a role where the meeting stands, so turns converge instead of adding polish every round:
// in the last round a story is ready only if all three amigos set ready.
func roundNote(round, rounds int) string {
	if round <= 0 || rounds <= 0 {
		return ""
	}
	if round < rounds {
		return fmt.Sprintf("\nThis is round %d of %d. Raise what matters now; set ready when you have no open concern.\n", round, rounds)
	}
	return fmt.Sprintf("\nThis is round %d of %d, the last. The story is ready only if product, qa and architect all set ready in this round. "+
		"Set ready to true unless something would make the story impossible to build or test as written; do not add questions, rules or "+
		"examples for polish now (they become a follow-up story).\n", round, rounds)
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
		readyRoles := map[string]bool{}
		l.round = round
		for _, role := range meetingRoles {
			turn, err := l.askTurn(role, "")
			if err != nil {
				return l.c, err
			}
			if err := l.merge(round, role, turn); err != nil {
				return l.c, err
			}
			readyRoles[role] = turn.Ready
		}
		l.c.ReadyRoles = nil
		for _, role := range meetingRoles {
			if readyRoles[role] {
				l.c.ReadyRoles = append(l.c.ReadyRoles, role)
			}
		}
		l.round = 0
		for i := range l.c.Questions {
			if l.c.Questions[i].State == QuestionOpen {
				if err := l.sample(i); err != nil {
					return l.c, err
				}
			}
		}
		if l.ready(len(l.c.ReadyRoles) == len(meetingRoles)) {
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
	prompt := l.tc.Prompt(role, l.in, l.c, focus) + roundNote(l.round, l.opts.Rounds)
	if focus == "" {
		prompt += l.gapNote()
	}
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
			if _, sim := closestRule(c.Rules, r); sim >= ruleSameThreshold {
				continue // a paraphrase or a truncated copy of an existing rule is not a new rule
			}
			c.Rules = append(c.Rules, r)
		}
	}
	for _, e := range t.Examples {
		if !hasString(c.Rules, e.Rule) {
			if best, sim := closestRule(c.Rules, e.Rule); sim >= ruleLinkThreshold {
				e.Rule = best // roles cite rules in their own words; attach the example to the rule it means
			}
		}
		if !slices.Contains(c.Examples, e) && !hasExampleText(c.Examples, e.Text) {
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
		c.Deliverable = t.Deliverable
		if t.Flag != "" && t.Flag != "none" {
			c.Flag = t.Flag
		}
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
		if text == "" && len(t.Answers) == 1 {
			text = t.Answers[0].Text // the turn was asked this one question; a reworded echo of it is still its answer
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

// ready: product, qa and architect all reported ready this round, architect confirmed the story is deliverable on
// its own, every question is resolved and every rule has an example.
func (l *loopRun) ready(allReady bool) bool {
	if !allReady || !l.c.Deliverable {
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

// hasExampleText reports whether an example with the same text (ignoring case, spacing and end punctuation) exists,
// whatever rule it was filed under: roles re-send examples under reworded rules, which duplicated scenarios.
func hasExampleText(xs []Example, text string) bool {
	norm := func(s string) string {
		return strings.Trim(strings.Join(strings.Fields(strings.ToLower(s)), " "), ".!; ")
	}
	n := norm(text)
	for _, x := range xs {
		if norm(x.Text) == n {
			return true
		}
	}
	return false
}

const (
	ruleSameThreshold = 0.8 // word overlap at which a "new" rule is a restatement of an existing one
	ruleLinkThreshold = 0.5 // word overlap at which an example's cited rule is taken to mean an existing rule
)

func ruleWords(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(w) >= 3 {
			out[w] = true
		}
	}
	return out
}

// ruleSimilarity is 1 when one rule's normalized text contains the other's, else the word-set Jaccard overlap.
func ruleSimilarity(a, b string) float64 {
	norm := func(s string) string {
		return strings.Trim(strings.Join(strings.Fields(strings.ToLower(s)), " "), ".!;: ")
	}
	na, nb := norm(a), norm(b)
	if na == "" || nb == "" {
		return 0
	}
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return 1
	}
	wa, wb := ruleWords(a), ruleWords(b)
	inter := 0
	for w := range wa {
		if wb[w] {
			inter++
		}
	}
	union := len(wa) + len(wb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func closestRule(rules []string, r string) (string, float64) {
	best, bestSim := "", 0.0
	for _, x := range rules {
		if s := ruleSimilarity(x, r); s > bestSim {
			best, bestSim = x, s
		}
	}
	return best, bestSim
}

// gapNote lists the rules that still have no example, so turns close the gap the ready check measures.
func (l *loopRun) gapNote() string {
	var gaps []string
	for _, r := range l.c.Rules {
		ok := false
		for _, e := range l.c.Examples {
			if e.Rule == r {
				ok = true
			}
		}
		if !ok {
			gaps = append(gaps, "- "+r)
		}
	}
	if len(gaps) == 0 {
		return ""
	}
	return "Rules with no example yet (the story cannot be ready until each has one; quote the rule exactly in the example's \"rule\" field):\n" + strings.Join(gaps, "\n") + "\n"
}
