package app

import (
	"os"
	"strings"
	"testing"
)

const examplePlanPath = "../../skills/drafting-plans/references/example-plan.md"

// The worked example is a regression fixture: it must always lint clean, so the skill cannot drift from the tool.
func TestExamplePlan_LintsClean(t *testing.T) {
	data, err := os.ReadFile(examplePlanPath)
	if err != nil {
		t.Fatal(err)
	}
	d, err := parsePlan(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := errs(lintPlan(d)); len(got) != 0 {
		t.Fatalf("example plan has lint errors: %v", got)
	}
	w, _ := planWaves(d.Tasks)
	if w["T01"] != 0 || w["T02"] != 1 || w["T03"] != 1 || w["T04"] != 2 {
		t.Fatalf("unexpected waves %v", w)
	}
}

// Verifying the example runs the repo's own test suite on a copy, which would recurse into this test,
// so it only runs on request: AI_MODE_VERIFY_EXAMPLE=1 go test ./internal/app -run TestExamplePlan_Verifies
// (or `make plan-example`).
func TestExamplePlan_Verifies(t *testing.T) {
	if os.Getenv("AI_MODE_PLAN_VERIFYING") != "" {
		t.Skip("running inside a plan verification")
	}
	if os.Getenv("AI_MODE_VERIFY_EXAMPLE") != "1" {
		t.Skip("set AI_MODE_VERIFY_EXAMPLE=1 to verify the example plan against this repo")
	}
	data, _ := os.ReadFile(examplePlanPath)
	d, err := parsePlan(string(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"T01", "T02", "T04"} {
		if r := verifyPlanTask(d, id, "../.."); r.Status != "verified" {
			t.Errorf("%s: want verified, got %+v", id, r)
		}
	}
	if r := verifyPlanTask(d, "T03", "../.."); r.Status != "n/a" || !strings.Contains(r.Note, "spec packet") {
		t.Errorf("T03: want n/a (spec packet), got %+v", r)
	}
}
