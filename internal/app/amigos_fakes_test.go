package app

import (
	"fmt"
)

// fakeAsker records calls and returns scripted answers.
type fakeAsker struct {
	Answers map[string]string
	Calls   []string
}

func (f *fakeAsker) Ask(role, user string, maxTokens int) (string, error) {
	f.Calls = append(f.Calls, role)
	if ans, ok := f.Answers[role]; ok {
		return ans, nil
	}
	return "", nil
}

// memAudit records audit writes in memory.
type memAudit struct {
	Entries  []string
	LevelStr string
	FailWith error // when set, Write returns it and records nothing
}

func (f *memAudit) Write(kind string, v any) error {
	if f.FailWith != nil {
		return f.FailWith
	}
	f.Entries = append(f.Entries, fmt.Sprintf("%s:%+v", kind, v))
	return nil
}

func (f *memAudit) Level() string {
	return f.LevelStr
}

// fakeEngine returns a fixed uncertainty score.
type fakeEngine struct {
	ScoreVal float64
}

func (f *fakeEngine) Score(question string, answers []string) float64 {
	return f.ScoreVal
}

// fakeClassifier returns a fixed test layer.
type fakeClassifier struct {
	LayerStr string
}

func (f *fakeClassifier) TestLayer(task string) (string, error) {
	return f.LayerStr, nil
}

// fakeCodec implements turnCodec by popping from pre-configured queues.
type fakeCodec struct {
	Turns map[string][]Turn
	Errs  map[string][]error
}

func (f *fakeCodec) Prompt(role string, in Intake, c StoryContract, focus string) string {
	return fmt.Sprintf("PROMPT role=%s focus=%s", role, focus)
}

func (f *fakeCodec) Parse(role, raw string) (Turn, error) {
	if errs, ok := f.Errs[role]; ok && len(errs) > 0 {
		err := errs[0]
		f.Errs[role] = errs[1:]
		return Turn{}, err
	}
	if turns, ok := f.Turns[role]; ok && len(turns) > 0 {
		turn := turns[0]
		f.Turns[role] = turns[1:]
		return turn, nil
	}
	return Turn{}, fmt.Errorf("no scripted turn/error for role %s", role)
}
