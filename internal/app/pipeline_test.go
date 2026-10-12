package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestPipeline(t *testing.T) *pipeline {
	t.Helper()
	dir := t.TempDir()
	return &pipeline{target: dir, docs: filepath.Join(dir, "docs", "pipeline"), board: fileBoard{dir: filepath.Join(dir, "docs", "pipeline", "board")},
		stages: map[string]stageFunc{}, now: func() string { return "2026-10-11T12:00:00Z" }}
}

func TestBoard_IDsAndRoundTrip(t *testing.T) {
	pl := newTestPipeline(t)
	a, err := pl.newCard(card{Title: "Pomodoro", Type: "feature", Stage: "new"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := pl.newCard(card{Title: "Spec", Type: "spec", Stage: "spec", Parent: a.ID})
	if a.ID != "C0001" || b.ID != "C0002" {
		t.Fatalf("ids %s %s", a.ID, b.ID)
	}
	got, err := pl.board.load("C0002")
	if err != nil || got.Parent != "C0001" || got.Title != "Spec" {
		t.Fatalf("load: %+v %v", got, err)
	}
}

func TestBoard_ColumnsAndParentRollup(t *testing.T) {
	cards := []card{
		{ID: "C0001", Type: "feature", Stage: "tracking"},
		{ID: "C0002", Type: "spec", Stage: "tracking", Parent: "C0001"},
		{ID: "C0003", Type: "plan", Stage: "tracking", Parent: "C0002"},
		{ID: "C0004", Type: "story", Stage: "review", Parent: "C0003"},
		{ID: "C0005", Type: "story", Stage: "amigos", Parent: "C0003"},
	}
	if col := columnOf(cards, "C0001"); col != "building" {
		t.Fatalf("parent with a story in amigos should be building, got %s", col)
	}
	cards[4].Stage = "review"
	if col := columnOf(cards, "C0001"); col != "review" {
		t.Fatalf("parent with all stories in review should be review, got %s", col)
	}
	cards[4].Stage = "needs-human"
	if col := columnOf(cards, "C0003"); col != "review" {
		t.Fatalf("a blocked child is shown on its own; the parent follows the rest, got %s", col)
	}
	if col := columnOf(cards, "C0005"); col != "blocked" {
		t.Fatalf("blocked story column %s", col)
	}
}

func TestBoard_RenderListsColumnsAndLastNote(t *testing.T) {
	cards := []card{{ID: "C0001", Title: "Pomodoro timer", Type: "feature", Stage: "spec",
		Notes: []cardNote{{Stage: "new → spec", Done: "Viability: go.", By: "bot-product"}}}}
	md := renderBoard(cards)
	for _, want := range []string{"## ideation", "C0001", "Pomodoro timer", "Viability: go.", "## init"} {
		if !strings.Contains(md, want) {
			t.Errorf("board missing %q:\n%s", want, md)
		}
	}
}

func TestPipeline_TickRunsStageMovesCardAndNotes(t *testing.T) {
	pl := newTestPipeline(t)
	pl.stages["feature:new"] = func(pl *pipeline, c *card) (string, cardNote, error) {
		c.setArtifact("viability", "docs/pipeline/viability/C0001.md")
		return "spec", cardNote{By: "bot-product", Done: "go", Why: "cheap", Next: "spec"}, nil
	}
	pl.newCard(card{Title: "Pomodoro", Type: "feature", Stage: "new"})
	moved, err := pl.tick()
	if err != nil || moved == nil {
		t.Fatalf("tick: %v %v", moved, err)
	}
	c, _ := pl.board.load("C0001")
	if c.Stage != "spec" || len(c.Notes) != 1 || c.Notes[0].Stage != "new → spec" || c.Artifacts["viability"] == "" {
		t.Fatalf("card after tick: %+v", c)
	}
	if _, err := os.Stat(filepath.Join(pl.docs, "BOARD.md")); err != nil {
		t.Fatal("BOARD.md not rendered")
	}
}

func TestPipeline_StageErrorBlocksCardWithNote(t *testing.T) {
	pl := newTestPipeline(t)
	pl.stages["feature:new"] = func(pl *pipeline, c *card) (string, cardNote, error) {
		return "", cardNote{}, errors.New("research did not answer")
	}
	pl.newCard(card{Title: "Pomodoro", Type: "feature", Stage: "new"})
	pl.tick()
	c, _ := pl.board.load("C0001")
	if c.Stage != "needs-human" || c.Prev != "new" {
		t.Fatalf("card: %+v", c)
	}
	n := c.Notes[len(c.Notes)-1]
	if !strings.Contains(n.Why, "research did not answer") || !strings.Contains(n.Next, "pipeline retry C0001") {
		t.Fatalf("note: %+v", n)
	}
	if moved, _ := pl.tick(); moved != nil {
		t.Fatal("a blocked card must not be picked again")
	}
	if err := pl.retry("C0001"); err != nil {
		t.Fatal(err)
	}
	c, _ = pl.board.load("C0001")
	if c.Stage != "new" {
		t.Fatalf("retry should restore the previous stage, got %s", c.Stage)
	}
}

func TestPipeline_PickPrefersDeepestAndRespectsStoryOrder(t *testing.T) {
	pl := newTestPipeline(t)
	for _, k := range []string{"feature:new", "story:amigos", "story:ready"} {
		pl.stages[k] = func(pl *pipeline, c *card) (string, cardNote, error) { return "x", cardNote{}, nil }
	}
	cards := []card{
		{ID: "C0001", Type: "feature", Stage: "tracking"},
		{ID: "C0002", Type: "spec", Stage: "tracking", Parent: "C0001"},
		{ID: "C0003", Type: "plan", Stage: "tracking", Parent: "C0002"},
		{ID: "C0004", Type: "story", Stage: "ready", Parent: "C0003", Order: 2, DependsOn: []string{"C0005"}},
		{ID: "C0005", Type: "story", Stage: "amigos", Parent: "C0003", Order: 1},
		{ID: "C0009", Type: "feature", Stage: "new"},
	}
	if c := pl.pick(cards); c == nil || c.ID != "C0005" {
		t.Fatalf("story 2 waits for story 1 (earlier in the stack), want C0005, got %v", c)
	}
	cards[4].Stage = "review"
	if c := pl.pick(cards); c == nil || c.ID != "C0004" {
		t.Fatalf("want C0004, got %v", c)
	}
	cards[3].Stage = "review"
	if c := pl.pick(cards); c == nil || c.ID != "C0009" {
		t.Fatalf("C0001 is waiting for the human (all stories in review), so the next request starts; got %v", c)
	}
}

func TestPipeline_NewRequestStartsOnlyWhenNothingInFlight(t *testing.T) {
	pl := newTestPipeline(t)
	pl.stages["feature:new"] = func(pl *pipeline, c *card) (string, cardNote, error) { return "spec", cardNote{}, nil }
	cards := []card{{ID: "C0001", Type: "feature", Stage: "needs-human"}, {ID: "C0002", Type: "feature", Stage: "new"}}
	if c := pl.pick(cards); c != nil {
		t.Fatalf("one feature in flight at a time: blocked C0001 still counts, got %v", c)
	}
	cards[0].Stage = "review"
	if c := pl.pick(cards); c == nil || c.ID != "C0002" {
		t.Fatalf("want C0002, got %v", c)
	}
}

func TestPipeline_ReopenSendsAReviewedCardBack(t *testing.T) {
	pl := newTestPipeline(t)
	pl.stages["story:architect-review"] = func(pl *pipeline, c *card) (string, cardNote, error) { return "handoff", cardNote{}, nil }
	c, _ := pl.newCard(card{Title: "CLI", Type: "story", Stage: "amigos"})
	if err := pl.reopen(c.ID, "architect-review", "x", "human"); err == nil {
		t.Fatal("a card that is not in review cannot be reopened")
	}
	c.Stage = "review"
	pl.board.save(c)
	if err := pl.reopen(c.ID, "architect-review", "running it starts no timer", "human"); err != nil {
		t.Fatal(err)
	}
	got, _ := pl.board.load(c.ID)
	if got.Stage != "architect-review" || got.Notes[len(got.Notes)-1].Why != "running it starts no timer" {
		t.Fatalf("card %+v", got)
	}
}
