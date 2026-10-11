package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Stage 7, amigos: `ai-mode amigos --story <file> --caller pipeline` on each story. A ready verdict is final
// (roadmap D2); not-ready gets one more meeting with an extra round, then goes to a human; paused goes to a human
// with the escalated questions. `ai-mode pipeline answer` appends the requester's answer to the story file.

// storyVerdict is what `ai-mode amigos --story FILE` saves next to the story.
type storyVerdict struct {
	MeetingID string        `json:"meeting_id"`
	Verdict   Verdict       `json:"verdict"`
	StoryFile string        `json:"story_file"`
	Contract  StoryContract `json:"contract"`
}

func loadStoryVerdict(story string) (storyVerdict, error) {
	var v storyVerdict
	data, err := os.ReadFile(storyContractPath(story))
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(data, &v)
}

// amigosExec runs the real command; the exit code is the verdict (0 ready, 2 not-ready, 3 paused, 1 error).
func amigosExec(dir, story string, rounds int) (int, string, error) {
	self, err := os.Executable()
	if err != nil {
		return 1, "", err
	}
	c := exec.Command(self, "amigos", "--story", story, "--caller", "pipeline", "--rounds", fmt.Sprint(rounds), "--audit", "summary")
	c.Dir = dir
	var out bytes.Buffer
	c.Stdout, c.Stderr = &out, &out
	err = c.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), out.String(), nil
	}
	if err != nil {
		return 1, out.String(), err
	}
	return 0, out.String(), nil
}

func stageAmigos(pl *pipeline, c *card) (string, cardNote, error) {
	storyRel := c.Artifacts["story"]
	story := pl.abs(storyRel)
	if _, err := os.Stat(story); err != nil {
		return "", cardNote{}, fmt.Errorf("story file: %w", err)
	}
	var code int
	var out string
	for _, rounds := range []int{3, 4} {
		pl.logf("  amigos: %d rounds on %s …", rounds, storyRel)
		start := time.Now()
		var err error
		code, out, err = pl.amigos(pl.target, story, rounds)
		if err != nil {
			return "", cardNote{}, fmt.Errorf("amigos: %w", err)
		}
		pl.logf("  amigos: exit %d in %s", code, time.Since(start).Round(time.Second))
		if code != ExitNotReady {
			break
		}
	}
	if code == ExitError {
		return "", cardNote{}, fmt.Errorf("amigos failed: %s", truncateRunes(strings.TrimSpace(out), 600))
	}
	v, err := loadStoryVerdict(story)
	if err != nil {
		return "", cardNote{}, fmt.Errorf("amigos left no verdict next to the story: %w", err)
	}
	contractRel := pl.rel(storyContractPath(story))
	c.setArtifact("contract", contractRel)
	k := v.Contract
	var layers []string
	for _, l := range k.LayerPlan {
		layers = append(layers, fmt.Sprintf("%d %s [%s] examples %v", l.N, l.Name, l.TestLevel, l.Scenarios))
	}
	var open []string
	for _, q := range k.Questions {
		if q.State != QuestionResolved {
			open = append(open, fmt.Sprintf("[%s] %s", q.State, q.Text))
		}
	}
	flag := "no flag"
	if k.Flag != "" {
		flag = "flag " + k.Flag
	}
	why := fmt.Sprintf("Verdict %s from meeting %s; ready: %s; %d rules, %d examples; layers: %s; %s.", v.Verdict, v.MeetingID,
		strings.Join(k.ReadyRoles, ", "), len(k.Rules), len(k.Examples), strings.Join(layers, "; "), flag)
	switch code {
	case ExitReady:
		return "ready", cardNote{By: "bot-amigos", Done: "Amigos: ready. Product, qa and architect agree on the Story Contract.", Artifacts: []string{contractRel}, Why: why,
			Next: "Stage 8: architect writes the story plan, qa writes failing tests, coder makes them pass, one stack layer at a time."}, nil
	case ExitPaused:
		return "needs-human", cardNote{By: "bot-amigos", Done: "Amigos paused: a question needs the requester.", Artifacts: []string{contractRel},
			Why:  why + " Escalated: " + strings.Join(open, " | "),
			Next: fmt.Sprintf("The requester answers with `ai-mode pipeline answer %s \"<answer>\"`, which adds it to the story and re-runs amigos.", c.ID)}, nil
	}
	return "needs-human", cardNote{By: "bot-amigos", Done: "Amigos: not ready after two meetings.", Artifacts: []string{contractRel},
		Why:  why + " Open: " + strings.Join(open, " | "),
		Next: fmt.Sprintf("A human clarifies the story (`ai-mode pipeline answer %s \"<clarification>\"`) or splits it.", c.ID)}, nil
}

// answer appends the requester's answer to a story that amigos sent to a human, and moves it back to amigos.
func (pl *pipeline) answer(id, text string) error {
	c, err := pl.board.load(id)
	if err != nil {
		return err
	}
	if c.Type != "story" || c.Stage != "needs-human" || c.Prev != "amigos" {
		return fmt.Errorf("%s is not a story waiting on a human after amigos (type %s, stage %s)", id, c.Type, c.Stage)
	}
	path := pl.abs(c.Artifacts["story"])
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := strings.TrimRight(string(data), "\n") + "\n"
	if !strings.Contains(s, "\n## Answers from the requester\n") {
		s += "\n## Answers from the requester\n\n"
	}
	s += fmt.Sprintf("- %s (%s)\n", strings.TrimSpace(text), pl.now()[:10])
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		return err
	}
	c.Stage, c.Prev = "amigos", ""
	pl.addNote(&c, cardNote{By: "human", Stage: "needs-human → amigos", Done: "The requester answered.", Artifacts: []string{c.Artifacts["story"]},
		Why: truncateRunes(strings.TrimSpace(text), 300), Next: "Amigos runs again with the answer in the story."})
	if err := pl.board.save(c); err != nil {
		return err
	}
	pl.sync(fmt.Sprintf("pipeline: %s answered, back to amigos", id))
	return nil
}
