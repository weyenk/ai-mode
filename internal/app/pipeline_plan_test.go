package app

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func pomodoroFeaturePlan(specRel string) string {
	return "# Pomodoro — feature plan\n\n**Spec:** " + specRel + "\n**Card:** C0002\n\n## Contracts\n```go\ntype Clock interface{ Now() time.Time }\n```\n\n## Stories\n\n```json\n" +
		`{"stories":[{"id":"ST1","title":"Session engine","scope":"cycle logic","scenarios":["S1","S2"],"depends_on":[],"size":"S"},` +
		`{"id":"ST2","title":"Stop summary","scope":"ctrl-c","scenarios":["S3"],"depends_on":["ST1"],"size":"XS"}],"flags":[]}` +
		"\n```\n\n## Story notes\n### ST1: Session engine\nAlone.\n### ST2: Stop summary\nAlone.\n"
}

func specCardWithSpec(t *testing.T, pl *pipeline) (card, string) {
	t.Helper()
	req, _ := pl.newCard(card{Title: "Pomodoro", Type: "feature", Stage: "tracking", Context: "hobby"})
	sp, _ := pl.newCard(card{Title: "Pomodoro CLI", Type: "spec", Stage: "planning", Parent: req.ID, Context: "hobby"})
	specRel := "docs/pipeline/specs/C0002-pomodoro-cli.md"
	writeArtifact(t, pl, &sp, "spec", specRel, goodSpec)
	pl.board.save(sp)
	return sp, specRel
}

func TestFeaturePlanStage_LintCleanPlanCreatesPlanCard(t *testing.T) {
	pl := newTestPipeline(t)
	sp, specRel := specCardWithSpec(t, pl)
	planRel := "docs/pipeline/plans/C0002-pomodoro-cli-feature-plan.md"
	pl.newAgent = func(s agentSpec) (*agentSession, error) {
		if s.Role != "architect" || len(s.Owns) != 1 || s.Owns[0] != planRel {
			return nil, fmt.Errorf("unexpected agent %s owning %v", s.Role, s.Owns)
		}
		return &agentSession{spec: s, chat: &scriptedChat{replies: []agentMsg{
			call("1", "write_file", map[string]any{"path": planRel, "content": pomodoroFeaturePlan(specRel)}),
			call("2", "finish", map[string]any{"summary": "plan written, lint passes"}),
		}}}, nil
	}
	next, note, err := stageFeaturePlan(pl, &sp)
	if err != nil || next != "tracking" {
		t.Fatalf("next=%s err=%v note=%+v", next, err, note)
	}
	cards, _ := pl.board.all()
	plan := cards[len(cards)-1]
	if plan.Type != "plan" || plan.Stage != "planning" || plan.Parent != sp.ID || plan.Artifacts["plan"] != planRel {
		t.Fatalf("plan card %+v", plan)
	}
}

func TestFeaturePlanStage_LintFindingsGoBackToArchitect(t *testing.T) {
	pl := newTestPipeline(t)
	sp, specRel := specCardWithSpec(t, pl)
	planRel := "docs/pipeline/plans/C0002-pomodoro-cli-feature-plan.md"
	bad := strings.Replace(pomodoroFeaturePlan(specRel), `"scenarios":["S3"]`, `"scenarios":[]`, 1)
	var chat *scriptedChat
	pl.newAgent = func(s agentSpec) (*agentSession, error) {
		chat = &scriptedChat{replies: []agentMsg{
			call("1", "write_file", map[string]any{"path": planRel, "content": bad}),
			call("2", "finish", map[string]any{"summary": "done"}),
			call("3", "write_file", map[string]any{"path": planRel, "content": pomodoroFeaturePlan(specRel)}),
			call("4", "finish", map[string]any{"summary": "fixed"}),
		}}
		return &agentSession{spec: s, chat: chat}, nil
	}
	if next, _, err := stageFeaturePlan(pl, &sp); err != nil || next != "tracking" {
		t.Fatalf("next=%s err=%v", next, err)
	}
	var fed string
	for _, m := range chat.seen[2] {
		if m.Role == "user" {
			fed = m.Content
		}
	}
	if !strings.Contains(fed, "S3 is in no story") {
		t.Fatalf("lint findings should be fed back, got %q", fed)
	}
}

func TestStoriesStage_WritesStoryFilesInStackOrder(t *testing.T) {
	pl := newTestPipeline(t)
	sp, specRel := specCardWithSpec(t, pl)
	plan, _ := pl.newCard(card{Title: "Pomodoro CLI feature plan", Type: "plan", Stage: "planning", Parent: sp.ID, Context: "hobby"})
	writeArtifact(t, pl, &plan, "plan", "docs/pipeline/plans/C0002-plan.md", pomodoroFeaturePlan(specRel))
	pl.board.save(plan)
	pl.ask = func(role, user string, n int) (string, error) {
		if role != "product" {
			return "", fmt.Errorf("product words stories, got %s", role)
		}
		return "```json\n{\"story\":\"As a person who works in a terminal, I want the cycle to run on its own, so that I keep focus.\"}\n```", nil
	}
	next, note, err := stageStories(pl, &plan)
	if err != nil || next != "tracking" {
		t.Fatalf("next=%s err=%v note=%+v", next, err, note)
	}
	cards, _ := pl.board.all()
	var stories []card
	for _, c := range cards {
		if c.Type == "story" {
			stories = append(stories, c)
		}
	}
	if len(stories) != 2 || stories[0].Order != 1 || stories[1].Order != 2 || stories[1].DependsOn[0] != stories[0].ID || stories[0].Stage != "amigos" {
		t.Fatalf("stories %+v", stories)
	}
	body, _ := os.ReadFile(pl.abs(stories[0].Artifacts["story"]))
	for _, want := range []string{"As a person who works in a terminal", "S1 — Default cycle", "S2 — Long break", "type Clock interface", "ST1"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("story file lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), "S3 — Stop") {
		t.Error("story 1 must carry only its own scenarios")
	}
}
