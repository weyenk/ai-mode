package app

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const goodSpec = `# Pomodoro timer — design spec

## Goal
A terminal pomodoro timer.

## Users and success metrics
The owner; uses it daily.

## Scope
### In scope
- cycles
### Out of scope
- sync

## User flows
1. Start, watch, stop.

## Behaviour rules
- R1 Focus 25, short 5, long 15 after every 4th focus.

## Scenarios
- **S1 — Default cycle.** Given defaults, when a focus ends, then a 5-minute break starts.
- **S2 — Long break.** Given 3 focus sessions done, when the 4th ends, then a 15-minute break starts.
- **S3 — Stop.** Given 2 focus sessions done, when Ctrl-C is pressed, then it prints 2.

## Global Constraints
- Go standard library only.

## Technical approach
A clock interface so tests run fast.

## Open questions
None.
`

func TestSpecCheck_AcceptsCompleteSpec(t *testing.T) {
	if f := checkSpec(goodSpec); len(f) != 0 {
		t.Fatalf("findings: %v", f)
	}
	ids := specScenarioIDs(goodSpec)
	if strings.Join(ids, ",") != "S1,S2,S3" {
		t.Fatalf("ids = %v", ids)
	}
}

func TestSpecCheck_FindsMissingSectionsAndBadScenarios(t *testing.T) {
	bad := strings.Replace(goodSpec, "## User flows\n1. Start, watch, stop.\n", "", 1)
	bad = strings.Replace(bad, "**S2 — Long break.**", "**S5 — Long break.**", 1)
	bad = strings.Replace(bad, "then it prints 2", "it prints 2", 1)
	f := strings.Join(checkSpec(bad), "\n")
	for _, want := range []string{"User flows", "S1, S5, S3", "S3"} {
		if !strings.Contains(f, want) {
			t.Errorf("findings lack %q:\n%s", want, f)
		}
	}
}

func TestSpecSplit_CreatesSpecCards(t *testing.T) {
	pl := newTestPipeline(t)
	pl.ask = func(role, user string, n int) (string, error) {
		if role != "architect" {
			return "", fmt.Errorf("split is architect's call, got %s", role)
		}
		return "One subsystem.\n```json\n{\"specs\":[{\"title\":\"Pomodoro timer CLI\",\"scope\":\"the whole request\"}],\"reason\":\"one deliverable\"}\n```", nil
	}
	req, _ := pl.newCard(card{Title: "Pomodoro", Type: "feature", Stage: "spec"})
	writeArtifact(t, pl, &req, "request", "docs/pipeline/requests/C0001.md", "# Pomodoro\nA timer.")
	writeArtifact(t, pl, &req, "viability", "docs/pipeline/viability/C0001.md", "# Viability\nGO")
	next, note, err := stageSpecSplit(pl, &req)
	if err != nil || next != "tracking" {
		t.Fatalf("next=%s err=%v", next, err)
	}
	cards, _ := pl.board.all()
	if len(cards) != 2 || cards[1].Type != "spec" || cards[1].Stage != "spec" || cards[1].Parent != req.ID || cards[1].Context != req.Context {
		t.Fatalf("cards: %+v", cards)
	}
	if !strings.Contains(note.Done, "1 spec") {
		t.Fatalf("note: %+v", note)
	}
}

func writeArtifact(t *testing.T, pl *pipeline, c *card, kind, rel, body string) {
	t.Helper()
	os.MkdirAll(pl.abs(rel[:strings.LastIndex(rel, "/")]), 0o755)
	if err := os.WriteFile(pl.abs(rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c.setArtifact(kind, rel)
}

func TestSpecWrite_WritesReviewsAndMerges(t *testing.T) {
	pl := newTestPipeline(t)
	req, _ := pl.newCard(card{Title: "Pomodoro", Type: "feature", Stage: "tracking"})
	writeArtifact(t, pl, &req, "request", "docs/pipeline/requests/C0001.md", "# Pomodoro\nA timer.")
	writeArtifact(t, pl, &req, "viability", "docs/pipeline/viability/C0001.md", "# Viability\nGO")
	pl.board.save(req)
	sp, _ := pl.newCard(card{Title: "Pomodoro timer CLI", Type: "spec", Stage: "spec", Parent: req.ID})
	specRel := "docs/pipeline/specs/C0002-pomodoro-timer-cli.md"
	var agents []string
	pl.newAgent = func(s agentSpec) (*agentSession, error) {
		agents = append(agents, s.Role)
		if len(s.Owns) != 1 || s.Owns[0] != specRel {
			return nil, fmt.Errorf("%s may write only the spec, owns %v", s.Role, s.Owns)
		}
		content := goodSpec
		if s.Role == "chief" {
			content = goodSpec + "\n## Review log\n- product: applied.\n"
		}
		return &agentSession{spec: s, chat: &scriptedChat{replies: []agentMsg{
			call("1", "write_file", map[string]any{"path": specRel, "content": content}),
			call("2", "finish", map[string]any{"summary": "done"}),
		}}}, nil
	}
	var reviewers []string
	pl.ask = func(role, user string, n int) (string, error) {
		reviewers = append(reviewers, role)
		if !strings.Contains(user, "S1 — Default cycle") {
			return "", fmt.Errorf("review prompt lacks the spec text")
		}
		return "## " + role + " review\n**Blocking**\n- none\n**Non-blocking**\n- fine\n**Proposed edits**\n- none", nil
	}
	next, note, err := stageSpecWrite(pl, &sp)
	if err != nil || next != "planning" {
		t.Fatalf("next=%s err=%v note=%+v", next, err, note)
	}
	if strings.Join(agents, ",") != "architect,chief" {
		t.Fatalf("agents = %v", agents)
	}
	if strings.Join(reviewers, ",") != "product,research,security,ux,design,architect" {
		t.Fatalf("reviewers = %v", reviewers)
	}
	if sp.Artifacts["spec"] != specRel {
		t.Fatalf("artifacts = %v", sp.Artifacts)
	}
	if _, err := os.Stat(pl.abs("docs/pipeline/specs/reviews/C0002-security.md")); err != nil {
		t.Fatal("security review not saved")
	}
	if b, _ := os.ReadFile(pl.abs(specRel)); !strings.Contains(string(b), "## Review log") {
		t.Fatal("merged spec lacks the review log")
	}
}
