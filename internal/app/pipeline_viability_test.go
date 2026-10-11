package app

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestLastJSONObject(t *testing.T) {
	for in, want := range map[string]string{
		"Findings...\n```json\n{\"a\": 1}\n```\n":          `{"a": 1}`,
		"text {\"x\": {\"y\": 2}} more text":               `{"x": {"y": 2}}`,
		"first {\"a\":1} then ```json\n{\"b\":2}\n``` end": `{"b":2}`,
		"no json here":                    "",
		"quoted brace {\"s\": \"a } b\"}": `{"s": "a } b"}`,
	} {
		got, _ := lastJSONObject(in)
		if got != want {
			t.Errorf("lastJSONObject(%q) = %q, want %q", in, got, want)
		}
	}
}

func goodEstimates() viabilityEstimates {
	return viabilityEstimates{
		Research:  &researchEstimate{Feasible: true, Confidence: 0.9},
		Product:   &productEstimate{Value: 3, SuccessMetrics: []string{"used for 5 sessions in the first week"}},
		Architect: &architectEstimate{EffortDays: 1, Complexity: 1},
	}
}

func TestViability_GoForSmallHobbyRequest(t *testing.T) {
	d := decideViability(viabilityRubricFor("hobby"), goodEstimates())
	if !d.Go || d.ROI != 3 {
		t.Fatalf("decision %+v", d)
	}
}

func TestViability_NoGoWhenTooBigForContext(t *testing.T) {
	e := goodEstimates()
	e.Architect.EffortDays = 40
	d := decideViability(viabilityRubricFor("hobby"), e)
	if d.Go || !strings.Contains(strings.Join(d.Failed, " "), "effort") {
		t.Fatalf("decision %+v", d)
	}
}

func TestViability_UnknownWhenARoleGaveNoEstimate(t *testing.T) {
	e := goodEstimates()
	e.Architect = nil
	d := decideViability(viabilityRubricFor("hobby"), e)
	if d.Go || !d.Unknown {
		t.Fatalf("missing architect estimate must not be a go: %+v", d)
	}
}

func TestViability_HealthNeedsSecurityAndLowRisk(t *testing.T) {
	r := viabilityRubricFor("health")
	if !r.NeedSecurity {
		t.Fatal("health requests get a security consult")
	}
	e := goodEstimates()
	d := decideViability(r, e)
	if d.Go || !d.Unknown {
		t.Fatalf("no security estimate for health: %+v", d)
	}
	e.Security = &securityEstimate{Risk: 4}
	d = decideViability(r, e)
	if d.Go || d.Unknown {
		t.Fatalf("risk 4 is a no-go for health: %+v", d)
	}
}

func TestViability_StageWritesReportAndMovesToSpec(t *testing.T) {
	pl := newTestPipeline(t)
	replies := map[string]string{
		"research":  "Findings: timers are common.\n```json\n{\"feasible\":true,\"prior_art\":[\"tomato apps\"],\"known_pitfalls\":[],\"confidence\":0.9}\n```",
		"product":   "Findings: one user.\n```json\n{\"value\":3,\"target_user\":\"the owner\",\"success_metrics\":[\"owner uses it daily\"],\"mvp_scope\":[\"25/5 cycle\"],\"out_of_scope\":[\"sync\"]}\n```",
		"architect": "Findings: stdlib only.\n```json\n{\"effort_days\":1,\"complexity\":1,\"approach\":\"state machine + ticker\",\"technical_risks\":[]}\n```",
	}
	pl.ask = func(role, user string, maxTokens int) (string, error) {
		if !strings.Contains(user, "pomodoro") || !strings.Contains(user, "<request>") {
			return "", fmt.Errorf("prompt lacks the request: %q", user)
		}
		return replies[role], nil
	}
	c, _ := pl.newCard(card{Title: "Pomodoro timer", Type: "feature", Stage: "new", Context: "hobby"})
	os.MkdirAll(pl.abs("docs/pipeline/requests"), 0o755)
	os.WriteFile(pl.abs("docs/pipeline/requests/C0001.md"), []byte("# Pomodoro timer\n\nA simple pomodoro timer."), 0o644)
	c.setArtifact("request", "docs/pipeline/requests/C0001.md")
	next, note, err := stageViability(pl, &c)
	if err != nil || next != "spec" {
		t.Fatalf("next=%s err=%v note=%+v", next, err, note)
	}
	report, err := os.ReadFile(pl.abs(c.Artifacts["viability"]))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GO", "owner uses it daily", "ROI", "state machine + ticker"} {
		if !strings.Contains(string(report), want) {
			t.Errorf("report lacks %q", want)
		}
	}
	if !strings.Contains(note.Why, "ROI") {
		t.Errorf("note should give the basis: %+v", note)
	}
}
