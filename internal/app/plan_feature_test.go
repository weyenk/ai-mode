package app

import (
	"strings"
	"testing"
)

const featureSpec = "## Scenarios\n- **S1 — a.** Given x, when y, then z.\n- **S2 — b.** Given x, when y, then z.\n- **S3 — c.** Given x, when y, then z.\n"

func featurePlanMD(stories, flags string) string {
	return "# Timer — feature plan\n\n**Spec:** spec.md\n\n## Contracts\n- package main: type Clock interface\n\n## Stories\n\n```json\n" +
		`{"stories":[` + stories + `],"flags":[` + flags + `]}` + "\n```\n\n## Story notes\n### ST1: core\nDeliverable alone.\n### ST2: cli\nDeliverable alone.\n"
}

func lintFeatureText(t *testing.T, md string) string {
	t.Helper()
	fp, err := parseFeaturePlan(md)
	if err != nil {
		return "PARSE: " + err.Error()
	}
	var out []string
	for _, f := range lintFeaturePlan(fp, featureSpec) {
		out = append(out, f.Level+": "+f.Msg)
	}
	return strings.Join(out, "\n")
}

func TestFeatureLint_CleanPlan(t *testing.T) {
	md := featurePlanMD(`{"id":"ST1","title":"core","scope":"engine","scenarios":["S1","S2"],"depends_on":[],"size":"S"},
		{"id":"ST2","title":"cli","scope":"flags","scenarios":["S3"],"depends_on":["ST1"],"size":"S"}`, "")
	if got := lintFeatureText(t, md); got != "" {
		t.Fatalf("findings:\n%s", got)
	}
}

func TestFeatureLint_ScenarioCoverage(t *testing.T) {
	md := featurePlanMD(`{"id":"ST1","title":"core","scope":"engine","scenarios":["S1","S2","S9"],"depends_on":[],"size":"S"},
		{"id":"ST2","title":"cli","scope":"flags","scenarios":["S2"],"depends_on":["ST1"],"size":"S"}`, "")
	got := lintFeatureText(t, md)
	for _, want := range []string{"S2 is in several stories", "S3 is in no story", "S9 is not a scenario in the spec"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFeatureLint_CyclesAndUnknownDeps(t *testing.T) {
	md := featurePlanMD(`{"id":"ST1","title":"core","scope":"e","scenarios":["S1","S2"],"depends_on":["ST2"],"size":"S"},
		{"id":"ST2","title":"cli","scope":"f","scenarios":["S3"],"depends_on":["ST1","ST7"],"size":"XL"}`, "")
	got := lintFeatureText(t, md)
	for _, want := range []string{"cycle", "unknown story ST7", "size XL"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFeatureLint_TemporaryFlagNeedsRemovalStoryLast(t *testing.T) {
	stories := `{"id":"ST1","title":"core","scope":"e","scenarios":["S1"],"depends_on":[],"size":"S","flag":"new-timer"},
		{"id":"ST2","title":"cli","scope":"f","scenarios":["S2"],"depends_on":["ST1"],"size":"S","flag":"new-timer"},
		{"id":"ST3","title":"remove flag","scope":"g","scenarios":["S3"],"depends_on":["ST1"],"size":"S"}`
	got := lintFeatureText(t, featurePlanMD(stories, `{"name":"new-timer","kind":"temporary","used_by":["ST1","ST2"],"removal_story":"ST3"}`))
	if !strings.Contains(got, "ST3 must depend on every story that uses new-timer") {
		t.Fatalf("got:\n%s", got)
	}
	got = lintFeatureText(t, featurePlanMD(stories, `{"name":"new-timer","kind":"temporary","used_by":["ST1","ST2"]}`))
	if !strings.Contains(got, "temporary flag new-timer has no removal story") {
		t.Fatalf("got:\n%s", got)
	}
	got = lintFeatureText(t, featurePlanMD(stories, `{"name":"other","kind":"release","used_by":["ST1"]}`))
	if !strings.Contains(got, "flag new-timer") || !strings.Contains(got, "not listed") {
		t.Fatalf("a story's flag must be declared: %s", got)
	}
}

func TestFeatureLint_StoryOrderIsTopological(t *testing.T) {
	md := featurePlanMD(`{"id":"ST2","title":"cli","scope":"f","scenarios":["S3"],"depends_on":["ST1"],"size":"S"},
		{"id":"ST1","title":"core","scope":"e","scenarios":["S1","S2"],"depends_on":[],"size":"S"}`, "")
	fp, err := parseFeaturePlan(md)
	if err != nil {
		t.Fatal(err)
	}
	order, err := featureStoryOrder(fp)
	if err != nil || strings.Join(order, ",") != "ST1,ST2" {
		t.Fatalf("order %v %v", order, err)
	}
}
