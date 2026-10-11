package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixSpec = "# Spec\n\n`ai-mode project set` writes **one** file at the repo root.\n\nThe gate runs before any model call: it checks the caller.\n"

// planWithT01Block is goodPlan with T01's Go block replaced.
func planWithT01Block(code string) string {
	return strings.Replace(goodPlan, "package app\n\ntype Thing struct{}\n", code, 1)
}

func planWithExcerpt(excerpt string) string {
	return strings.Replace(goodPlan, "### T02: Thing logic\n", "### T02: Thing logic\n\n**Spec excerpt:** "+excerpt+"\n", 1)
}

func TestPlanFix_GofmtsWholeFileGoBlocks(t *testing.T) {
	md := planWithT01Block("package app\n\ntype Thing struct {\n\tA int\n\tLonger string\n}\n")
	out, notes := fixPlan(md, "")
	if !strings.Contains(out, "\tA      int\n\tLonger string\n") {
		t.Fatalf("want the struct gofmt-aligned, got notes %v and:\n%s", notes, out)
	}
	if !strings.Contains(out, "mode=insert-after anchor=\"team\"\n\t\t\"demo\": {\"Demo\", cmdDemo},\n") {
		t.Fatal("a snippet block (mode=insert-after) must be left exactly as written")
	}
}

func TestPlanFix_ReportsGoBlocksThatDoNotParse(t *testing.T) {
	md := planWithT01Block("package app\n\nvar f = func()\n{\n}\n")
	out, notes := fixPlan(md, "")
	if out != md {
		t.Fatal("a block that does not parse must be left unchanged")
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "internal/app/demo_contract.go") || !strings.Contains(notes[0], "does not parse") {
		t.Fatalf("want one note naming the file and saying it does not parse, got %v", notes)
	}
}

func TestPlanFix_RepairsExcerptMarkupDifferences(t *testing.T) {
	md := planWithExcerpt(`"ai-mode project set writes one file at the repo root."`)
	out, notes := fixPlan(md, fixSpec)
	want := "**Spec excerpt:** \"`ai-mode project set` writes **one** file at the repo root.\""
	if !strings.Contains(out, want) {
		t.Fatalf("want the exact spec text restored (%s), got notes %v and:\n%s", want, notes, out)
	}
	d := mustParse(t, out)
	d.SpecText = fixSpec
	if got := errs(lintPlan(d)); len(got) != 0 {
		t.Fatalf("the repaired excerpt must lint clean, got %v", got)
	}
}

func TestPlanFix_UnwrapsABacktickedExcerptLine(t *testing.T) {
	md := planWithExcerpt("`\"The gate runs before any model call\"`")
	out, _ := fixPlan(md, fixSpec)
	if !strings.Contains(out, "**Spec excerpt:** \"The gate runs before any model call\"\n") {
		t.Fatalf("want the outer backticks removed, got:\n%s", out)
	}
}

func TestPlanFix_LeavesParaphrasesAlone(t *testing.T) {
	md := planWithExcerpt(`"The gate runs before every model call"`)
	if out, _ := fixPlan(md, fixSpec); out != md {
		t.Fatal("a paraphrase (different words, not just markup) must not be rewritten")
	}
}

func TestPlanLint_SuggestsTheClosestSpecPhrase(t *testing.T) {
	d := mustParse(t, planWithExcerpt(`"The gate runs before any modle call"`))
	d.SpecText = fixSpec
	for _, m := range errs(lintPlan(d)) {
		if strings.Contains(m, "not found in the spec") && strings.Contains(m, `did you mean "The gate runs before any model call`) {
			return
		}
	}
	t.Fatalf("want a not-found ERR with a did-you-mean hint, got %v", errs(lintPlan(d)))
}

func TestPlanLint_WarnsWhenAnImplTaskIsACodePacket(t *testing.T) {
	md := strings.Replace(goodPlan, `"kind":"impl","packet":"spec"`, `"kind":"impl","packet":"code"`, 1)
	for _, f := range lintPlan(mustParse(t, md)) {
		if f.Level == "WARN" && strings.Contains(f.Msg, "T02") && strings.Contains(f.Msg, "spec packet") {
			return
		}
	}
	t.Fatal("want a WARN suggesting a spec packet for impl task T02")
}

func TestCmdPlanLint_FixRewritesThePlanFile(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	_ = os.WriteFile(spec, []byte(fixSpec), 0o644)
	md := strings.Replace(planWithExcerpt(`"ai-mode project set writes one file at the repo root."`),
		"**Context:** digest", "**Context:** digest\n**Spec:** `"+spec+"`", 1)
	plan := filepath.Join(dir, "plan.md")
	_ = os.WriteFile(plan, []byte(md), 0o644)
	if got := cmdPlan([]string{"lint", plan}); got != 2 {
		t.Fatalf("without --fix the near-miss excerpt must fail: want 2, got %d", got)
	}
	if got := cmdPlan([]string{"lint", "--fix", plan}); got != 0 {
		t.Fatalf("with --fix: want 0, got %d", got)
	}
	data, _ := os.ReadFile(plan)
	if !strings.Contains(string(data), "\"`ai-mode project set` writes **one** file at the repo root.\"") {
		t.Fatalf("--fix must rewrite the plan file, got:\n%s", data)
	}
}
