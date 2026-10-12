package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleContractMD = `# Story Contract: Count words

## Rules

- R1 Words are runs of non-space characters separated by Unicode whitespace.
- R2 An empty or all-space text has zero words.

## Scenarios

- **S1** (R1) Given "hello world", when counting, then the count is 2.
- **S2** (R1) Given "  a\tb\nc  ", when counting, then the count is 3.
- **S3** (R2) Given "   ", when counting, then the count is 0.

## Global Constraints

- Go standard library only; package kata.
`

const renderPlanJSON = `{
 "goal": "Count words in a text.",
 "architecture": "One file at the module root.",
 "review_focus": ["Unicode spaces other than ASCII (S2)"],
 "boundaries": [{"boundary": "none", "artifact": "none", "fixtures": "none", "fake": "none", "provider_check": "counter_test.go", "owner": "T01"}],
 "tasks": [
  {"id": "T01", "title": "Word counter", "kind": "impl", "packet": "spec", "depends_on": [], "owns": ["counter.go", "counter_test.go"],
   "produces": ["func CountWords(s string) int"], "consumes": [], "stack_layer": 1, "test_level": "unit",
   "scenarios": [
     {"id": "S1", "test": "TestCountWords_S1_TwoWords", "check": "CountWords(\"hello world\") == 2"},
     {"id": "S2", "test": "TestCountWords_S2_MixedWhitespace", "check": "CountWords(\"  a\\tb\\nc  \") == 3"},
     {"id": "S3", "test": "TestCountWords_S3_AllSpaces", "check": "CountWords(\"   \") == 0"}],
   "spec_excerpt": ["Words are runs of non-space characters separated by Unicode whitespace."],
   "notes": "Use strings.Fields."}
 ]
}`

func TestPlanRender_ProducesALintCleanDraftingPlan(t *testing.T) {
	pj, err := parseStoryPlanJSON([]byte(renderPlanJSON))
	if err != nil {
		t.Fatal(err)
	}
	md := renderStoryPlan(pj, "Count words", "docs/pipeline/stories/C1.contract.md", "docs/pipeline/stories/C1.contract.json", sampleContractMD)
	d, err := parsePlan(md)
	if err != nil {
		t.Fatalf("rendered plan does not parse: %v\n%s", err, md)
	}
	d.SpecText = sampleContractMD
	for _, f := range lintPlanOpts(d, false) {
		if f.Level == "ERR" {
			t.Errorf("lint: %s", f.Msg)
		}
	}
	if len(d.Coverage) != 3 || d.Coverage[1].Test != "TestCountWords_S2_MixedWhitespace" || d.Tasks[0].Covers[2] != "S3" {
		t.Fatalf("coverage %+v tasks %+v", d.Coverage, d.Tasks)
	}
	if !strings.Contains(md, "| S3 | R2 | T01 |") || !strings.Contains(md, "- Go standard library only; package kata.") {
		t.Fatalf("rule ids and global constraints come from the contract:\n%s", md)
	}
	if d.Packets["T01"].RunCmd != "go test ./... -count=1 -run '^(TestCountWords_S1_TwoWords|TestCountWords_S2_MixedWhitespace|TestCountWords_S3_AllSpaces)$'" {
		t.Fatalf("run %q", d.Packets["T01"].RunCmd)
	}
}

func TestPlanRender_CodePacketPhasesBecomeFileBlocks(t *testing.T) {
	pj, err := parseStoryPlanJSON([]byte(`{"goal":"g","architecture":"a","tasks":[{"id":"T01","title":"Contract","kind":"contract","packet":"code",
		"owns":["clock.go"],"produces":["type Clock"],"stack_layer":1,"files":[{"path":"clock.go","phase":"A","code":"package kata\n\ntype Clock interface{\nNow() int\n}\n"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	d, err := parsePlan(renderStoryPlan(pj, "x", "c.md", "c.json", sampleContractMD))
	if err != nil {
		t.Fatal(err)
	}
	b := d.Packets["T01"].Blocks
	if len(b) != 1 || b[0].File != "clock.go" || b[0].Phase != "A" || !strings.Contains(b[0].Code, "type Clock interface {\n\tNow() int\n}") {
		t.Fatalf("blocks %+v", b)
	}
}

func TestPlanRender_RejectsUnknownFieldsWithAHint(t *testing.T) {
	_, err := parseStoryPlanJSON([]byte(`{"goal":"g","tasks":[{"id":"T01","covers":["S1"]}]}`))
	if err == nil || !strings.Contains(err.Error(), "covers") {
		t.Fatalf("err = %v", err)
	}
}

func TestStoryPlanChecks_TestsAndCodeInOneTaskAndEveryScenarioCovered(t *testing.T) {
	pj, _ := parseStoryPlanJSON([]byte(`{"goal":"g","architecture":"a","tasks":[
		{"id":"T01","title":"Tests","kind":"impl","packet":"spec","owns":["counter_test.go"],"stack_layer":1,
		 "scenarios":[{"id":"S1","test":"TestC_S1_X","check":"x"}]}]}`))
	f := strings.Join(storyPlanChecks(pj, sampleContractMD), "\n")
	for _, want := range []string{"T01 covers scenarios but owns no non-test .go file", "S2 is not covered", "S3 is not covered"} {
		if !strings.Contains(f, want) {
			t.Errorf("missing %q in:\n%s", want, f)
		}
	}
}

func TestPlanRenderCommand_WritesMarkdownNextToTheJSON(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs", "C1.contract.md"), []byte(sampleContractMD), 0o644)
	os.WriteFile(filepath.Join(dir, "docs", "C1.plan.json"), []byte(renderPlanJSON), 0o644)
	md, findings, err := renderStoryPlanFile(filepath.Join(dir, "docs", "C1.plan.json"), "docs/C1.contract.md", dir)
	if err != nil || len(findings) != 0 {
		t.Fatalf("err %v findings %v", err, findings)
	}
	if md != filepath.Join(dir, "docs", "C1.plan.md") {
		t.Fatalf("md path %s", md)
	}
	if b, _ := os.ReadFile(md); !strings.Contains(string(b), "**Spec:** docs/C1.contract.md") {
		t.Fatalf("plan:\n%s", b)
	}
}

func TestStoryPlanChecks_CodePacketCoveringScenariosNeedsItsTests(t *testing.T) {
	pj, _ := parseStoryPlanJSON([]byte(`{"goal":"g","architecture":"a","tasks":[
		{"id":"T01","title":"Main","kind":"integration","packet":"code","owns":["main.go","main_test.go"],"stack_layer":1,
		 "scenarios":[{"id":"S1","test":"TestMain_S1_X","check":"x"}],"files":[{"path":"main.go","phase":"A","code":"package main\n\nfunc main() {}\n"}]}]}`))
	if f := strings.Join(storyPlanChecks(pj, "- **S1** (R1) x\n"), "\n"); !strings.Contains(f, "no phase A _test.go file") {
		t.Fatalf("findings:\n%s", f)
	}
}
