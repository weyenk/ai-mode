package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func storyCard(t *testing.T, pl *pipeline) card {
	t.Helper()
	sc, _ := pl.newCard(card{Title: "Core engine", Type: "story", Stage: "amigos", Parent: "C0003", Order: 1})
	writeArtifact(t, pl, &sc, "story", "docs/pipeline/stories/C0001-core-engine.md", "# Core engine\n\n## Story\n\nAs a user...\n")
	pl.board.save(sc)
	return sc
}

func fakeAmigos(verdicts ...Verdict) (func(dir, story string, rounds int) (int, string, error), *[]int) {
	var rounds []int
	return func(dir, story string, r int) (int, string, error) {
		rounds = append(rounds, r)
		v := verdicts[0]
		if len(verdicts) > 1 {
			verdicts = verdicts[1:]
		}
		c := readyContract()
		c.Verdict, c.ReadyRoles, c.Deliverable = v, []string{"product", "qa", "architect"}, true
		if v == VerdictPaused {
			c.Questions = []Question{{Text: "Should the bell ring at the end of breaks too?", State: QuestionEscalated}}
		}
		saveStoryVerdict(story, "m-1", c)
		return exitCodeFor(v), "verdict: " + string(v), nil
	}, &rounds
}

func TestAmigosStage_ReadyIsFinal(t *testing.T) {
	pl := newTestPipeline(t)
	sc := storyCard(t, pl)
	run, rounds := fakeAmigos(VerdictReady)
	pl.amigos = run
	next, note, err := stageAmigos(pl, &sc)
	if err != nil || next != "ready" || len(*rounds) != 1 {
		t.Fatalf("next=%s err=%v rounds=%v", next, err, *rounds)
	}
	if sc.Artifacts["contract"] != "docs/pipeline/stories/C0001-core-engine.contract.json" || !strings.Contains(note.Why, "product, qa, architect") {
		t.Fatalf("card %+v note %+v", sc.Artifacts, note)
	}
}

func TestAmigosStage_NotReadyGetsOneMoreMeetingThenAHuman(t *testing.T) {
	pl := newTestPipeline(t)
	sc := storyCard(t, pl)
	run, rounds := fakeAmigos(VerdictNotReady, VerdictNotReady)
	pl.amigos = run
	next, note, err := stageAmigos(pl, &sc)
	if err != nil || next != "needs-human" || len(*rounds) != 2 || (*rounds)[1] <= (*rounds)[0] {
		t.Fatalf("next=%s err=%v rounds=%v", next, err, *rounds)
	}
	if !strings.Contains(note.Next, "pipeline answer") {
		t.Fatalf("note %+v", note)
	}
}

func TestAmigosStage_PausedThenAnswered(t *testing.T) {
	pl := newTestPipeline(t)
	sc := storyCard(t, pl)
	run, _ := fakeAmigos(VerdictPaused)
	pl.amigos = run
	next, note, err := stageAmigos(pl, &sc)
	if err != nil || next != "needs-human" || !strings.Contains(note.Why, "bell ring") {
		t.Fatalf("next=%s err=%v note=%+v", next, err, note)
	}
	sc.Stage, sc.Prev = next, "amigos"
	pl.board.save(sc)
	if err := pl.answer(sc.ID, "Yes, ring at the end of every session, breaks included."); err != nil {
		t.Fatal(err)
	}
	got, _ := pl.board.load(sc.ID)
	body, _ := os.ReadFile(pl.abs(got.Artifacts["story"]))
	if got.Stage != "amigos" || !strings.Contains(string(body), "## Answers from the requester\n\n- Yes, ring at the end of every session") {
		t.Fatalf("stage %s body:\n%s", got.Stage, body)
	}
	var v storyVerdict
	data, _ := os.ReadFile(pl.abs("docs/pipeline/stories/C0001-core-engine.contract.json"))
	if json.Unmarshal(data, &v) != nil || v.Verdict != VerdictPaused {
		t.Fatalf("verdict file %s", data)
	}
}
