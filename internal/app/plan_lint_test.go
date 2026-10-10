package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fence = "```"

// goodPlan is a minimal plan that lints clean: a contract task, an implementation task
// that depends only on the contract, and an integration task that owns a hot file.
var goodPlan = strings.NewReplacer("FENCE", fence).Replace(`# Demo Implementation Plan

**Context:** digest
**Lint:** manual

## Global Constraints
- Go standard library only.

## Review Focus
1. Empty input is rejected (S1).

## Boundaries
| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |
| --- | --- | --- | --- | --- | --- |
| Thing | internal/app/demo_contract.go | none | none | demo_test.go | T01 |

## Task graph

FENCEjson
{"tasks":[
 {"id":"T01","title":"Contract","kind":"contract","packet":"code","depends_on":[],"owns":["internal/app/demo_contract.go"],"produces":["type Thing"],"consumes":[],"covers":[],"test_level":"contract","stack_layer":1,"branch":"demo-01-contract-t01"},
 {"id":"T02","title":"Thing logic","kind":"impl","packet":"spec","depends_on":["T01"],"owns":["internal/app/demo_logic.go","internal/app/demo_logic_test.go"],"produces":["func Do()"],"consumes":["Thing"],"covers":["S1"],"test_level":"unit","stack_layer":2,"branch":"demo-02-logic-t02"},
 {"id":"T03","title":"Integration","kind":"integration","packet":"code","depends_on":["T01","T02"],"owns":["internal/app/cli.go"],"produces":[],"consumes":["Do"],"covers":["S2"],"test_level":"functional","stack_layer":3,"branch":"demo-03-integration-t03"}
]}
FENCE

**Scenario coverage**

| Scenario | Rule | Task | Test |
| --- | --- | --- | --- |
| S1 | does it | T02 | ` + "`TestLogic_S1_Does`" + ` |
| S2 | wired | T03 | ` + "`TestWire_S2_Registered`" + ` |

### T01: Contract

**Phase A: tests plus stubs (role: qa).**
- [ ] Step 1: add the types

FENCEgo file=internal/app/demo_contract.go
package app

type Thing struct{}
FENCE

Run: ` + "`go test ./internal/app -run Thing`" + `

**Phase B: implementation (role: coder).**
- [ ] Step 2: nothing more to implement

### T02: Thing logic

Given an empty input, when Do runs, then it returns an error. Tests: TestLogic_S1_Does in internal/app/demo_logic_test.go.

### T03: Integration

The test is TestWire_S2_Registered in internal/app/cli_wire_test.go.

**Phase A: tests plus stubs (role: qa).**

FENCEgo file=internal/app/cli.go mode=insert-after anchor="team"
		"demo": {"Demo", cmdDemo},
FENCE

Run: ` + "`go test ./internal/app -run Wire`" + `

**Phase B: implementation (role: coder).**
- [ ] Step 3: done
`)

func mustParse(t *testing.T, md string) *planDoc {
	t.Helper()
	d, err := parsePlan(md)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return d
}

func errs(fs []planFinding) []string {
	var out []string
	for _, f := range fs {
		if f.Level == "ERR" {
			out = append(out, f.Msg)
		}
	}
	return out
}

func wantErr(t *testing.T, md, substr string) {
	t.Helper()
	got := errs(lintPlan(mustParse(t, md)))
	for _, m := range got {
		if strings.Contains(m, substr) {
			return
		}
	}
	t.Fatalf("want an ERR containing %q, got %v", substr, got)
}

func TestPlanParse_GoodPlan(t *testing.T) {
	d := mustParse(t, goodPlan)
	if d.Context != "digest" || len(d.Tasks) != 3 || len(d.Coverage) != 2 {
		t.Fatalf("context=%q tasks=%d coverage=%d", d.Context, len(d.Tasks), len(d.Coverage))
	}
	p := d.Packets["T01"]
	if p == nil || len(p.Blocks) != 1 {
		t.Fatalf("T01 packet/blocks wrong: %+v", p)
	}
	b := p.Blocks[0]
	if b.Lang != "go" || b.File != "internal/app/demo_contract.go" || b.Phase != "A" || !strings.Contains(b.Code, "type Thing struct{}") {
		t.Fatalf("block parsed wrong: %+v", b)
	}
	if p.RunCmd != "go test ./internal/app -run Thing" {
		t.Fatalf("run command: %q", p.RunCmd)
	}
	b3 := d.Packets["T03"].Blocks[0]
	if b3.Mode != "insert-after" || b3.Anchor != "team" {
		t.Fatalf("mode/anchor not parsed: %+v", b3)
	}
}

func TestPlanWaves(t *testing.T) {
	d := mustParse(t, goodPlan)
	w, err := planWaves(d.Tasks)
	if err != nil || w["T01"] != 0 || w["T02"] != 1 || w["T03"] != 2 {
		t.Fatalf("waves %v, err %v", w, err)
	}
	cyc := []planTask{{ID: "A", DependsOn: []string{"B"}}, {ID: "B", DependsOn: []string{"A"}}}
	if _, err := planWaves(cyc); err == nil {
		t.Fatal("want a cycle error")
	}
}

func TestPlanLint_GoodPlanIsClean(t *testing.T) {
	if got := errs(lintPlan(mustParse(t, goodPlan))); len(got) != 0 {
		t.Fatalf("want no errors, got %v", got)
	}
}

func TestPlanLint_MissingContextAndSections(t *testing.T) {
	wantErr(t, strings.Replace(goodPlan, "**Context:** digest", "**Context:** vibes", 1), "Context")
	wantErr(t, strings.Replace(goodPlan, "## Review Focus", "## Something Else", 1), "## Review Focus")
}

func TestPlanLint_ImplementationMayDependOnlyOnContractTasks(t *testing.T) {
	md := strings.Replace(goodPlan, `{"id":"T03"`, `{"id":"T04","title":"More logic","kind":"impl","packet":"spec","depends_on":["T02"],"owns":["internal/app/more.go"],"produces":[],"consumes":[],"covers":[],"test_level":"unit","stack_layer":2,"branch":"b4"},
 {"id":"T03"`, 1)
	md += "\n### T04: More logic\n\nGiven x when y then z.\n"
	wantErr(t, md, "may depend only on contract")
}

func TestPlanLint_ParallelTasksMayNotOverlapOwnership(t *testing.T) {
	md := strings.Replace(goodPlan, `{"id":"T03"`, `{"id":"T04","title":"Twin","kind":"impl","packet":"spec","depends_on":["T01"],"owns":["internal/app/demo_logic.go"],"produces":[],"consumes":[],"covers":[],"test_level":"unit","stack_layer":2,"branch":"b4"},
 {"id":"T03"`, 1)
	md += "\n### T04: Twin\n\nGiven x when y then z.\n"
	wantErr(t, md, "both own")
}

func TestPlanLint_HotFilesBelongToOneIntegrationTask(t *testing.T) {
	md := strings.Replace(goodPlan, `"owns":["internal/app/demo_logic.go","internal/app/demo_logic_test.go"]`, `"owns":["internal/app/demo_logic.go","README.md"]`, 1)
	wantErr(t, md, "hot file")
}

func TestPlanLint_ScenarioCoverage(t *testing.T) {
	// S2 claimed by two tasks
	md := strings.Replace(goodPlan, `"covers":["S1"]`, `"covers":["S1","S2"]`, 1)
	wantErr(t, md, "S2")
	// a scenario in the table with no task covering it
	md = strings.Replace(goodPlan, "| S2 | wired | T03 |", "| S2 | wired | T09 |", 1)
	wantErr(t, md, "S2")
	// test name without its scenario id
	md = strings.Replace(goodPlan, "`TestLogic_S1_Does`", "`TestLogicDoes`", 1)
	wantErr(t, md, "TestLogicDoes")
}

func TestPlanLint_UnknownDependencyAndCycle(t *testing.T) {
	wantErr(t, strings.Replace(goodPlan, `"depends_on":["T01","T02"]`, `"depends_on":["T01","T77"]`, 1), "T77")
	wantErr(t, strings.Replace(goodPlan, `"depends_on":[],"owns":["internal/app/demo_contract.go"]`, `"depends_on":["T03"],"owns":["internal/app/demo_contract.go"]`, 1), "cycle")
}

func TestPlanLint_CodeBlocksMustWriteOwnedFiles(t *testing.T) {
	wantErr(t, strings.Replace(goodPlan, "file=internal/app/demo_contract.go", "file=internal/app/stray.go", 1), "not owned")
}

func TestPlanLint_ContractTasksMustBeCodePackets(t *testing.T) {
	wantErr(t, strings.Replace(goodPlan, `"kind":"contract","packet":"code"`, `"kind":"contract","packet":"spec"`, 1), "contract")
}

func TestPlanLint_PlaceholdersAreRejected(t *testing.T) {
	wantErr(t, goodPlan+"\nTODO fill this in later\n", "placeholder")
}

func TestPlanLint_EveryTaskNeedsAPacket(t *testing.T) {
	wantErr(t, strings.Replace(goodPlan, "### T02: Thing logic", "### Not a packet", 1), "no packet")
}

func TestPlanLint_ConsumesMustBeProducedByAnAncestor(t *testing.T) {
	md := strings.Replace(goodPlan, `"consumes":["Thing"]`, `"consumes":["Ghost"]`, 1)
	for _, f := range lintPlan(mustParse(t, md)) {
		if f.Level == "WARN" && strings.Contains(f.Msg, "Ghost") {
			return
		}
	}
	t.Fatal("want a WARN about a consumed name no ancestor produces")
}

func TestApplyPlanBlock_Modes(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		_ = os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755)
		_ = os.WriteFile(filepath.Join(root, name), []byte(body), 0o644)
	}
	read := func(name string) string { t.Helper(); b, _ := os.ReadFile(filepath.Join(root, name)); return string(b) }

	if err := applyPlanBlock(root, planBlock{File: "a/new.go", Code: "package a\n"}); err != nil || read("a/new.go") != "package a\n" {
		t.Fatalf("create: %v %q", err, read("a/new.go"))
	}
	write("f.txt", "one\ntwo\nthree\n")
	if err := applyPlanBlock(root, planBlock{File: "f.txt", Mode: "append", Code: "four\n"}); err != nil || read("f.txt") != "one\ntwo\nthree\nfour\n" {
		t.Fatalf("append: %v %q", err, read("f.txt"))
	}
	if err := applyPlanBlock(root, planBlock{File: "f.txt", Mode: "insert-after", Anchor: "two", Code: "two-and-a-half\n"}); err != nil || read("f.txt") != "one\ntwo\ntwo-and-a-half\nthree\nfour\n" {
		t.Fatalf("insert-after: %v %q", err, read("f.txt"))
	}
	if err := applyPlanBlock(root, planBlock{File: "f.txt", Mode: "replace-line", Anchor: "three", Code: "THREE\n"}); err != nil || read("f.txt") != "one\ntwo\ntwo-and-a-half\nTHREE\nfour\n" {
		t.Fatalf("replace-line: %v %q", err, read("f.txt"))
	}
	if err := applyPlanBlock(root, planBlock{File: "f.txt", Mode: "insert-after", Anchor: "absent", Code: "x\n"}); err == nil || !strings.Contains(err.Error(), "anchor") {
		t.Fatalf("missing anchor must be an error, got %v", err)
	}
	if err := applyPlanBlock(root, planBlock{File: "missing.txt", Mode: "append", Code: "x\n"}); err == nil {
		t.Fatal("append to a missing file must be an error")
	}
	if err := applyPlanBlock(root, planBlock{File: "../escape.txt", Code: "x\n"}); err == nil {
		t.Fatal("a path escaping the repo must be an error")
	}
}

func TestCmdPlanLint_ExitCodes(t *testing.T) {
	dir := t.TempDir()
	good, bad := filepath.Join(dir, "good.md"), filepath.Join(dir, "bad.md")
	_ = os.WriteFile(good, []byte(goodPlan), 0o644)
	_ = os.WriteFile(bad, []byte(strings.Replace(goodPlan, "**Context:** digest", "**Context:** vibes", 1)), 0o644)
	if got := cmdPlan([]string{"lint", good}); got != 0 {
		t.Fatalf("good plan: want exit 0, got %d", got)
	}
	if got := cmdPlan([]string{"lint", bad}); got != 2 {
		t.Fatalf("bad plan: want exit 2, got %d", got)
	}
	if got := cmdPlan([]string{"lint", filepath.Join(dir, "missing.md")}); got != 1 {
		t.Fatalf("missing file: want exit 1, got %d", got)
	}
	if got := cmdPlan(nil); got != 1 {
		t.Fatalf("no subcommand: want exit 1, got %d", got)
	}
}

func TestCmdPlanLint_PrintsComputedWaves(t *testing.T) {
	file := filepath.Join(t.TempDir(), "good.md")
	_ = os.WriteFile(file, []byte(goodPlan), 0o644)
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := cmdPlan([]string{"lint", file})
	w.Close()
	os.Stdout = old
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if code != 0 || !strings.Contains(b.String(), "waves: T01=0 T02=1 T03=2") {
		t.Fatalf("want exit 0 and a waves line, got %d: %q", code, b.String())
	}
}

func TestLintPlanSkeleton_SkipsPacketChecksButKeepsGraphChecks(t *testing.T) {
	skeleton := goodPlan[:strings.Index(goodPlan, "### T01:")]
	d := mustParse(t, skeleton)
	if got := errs(lintPlan(d)); len(got) == 0 {
		t.Fatal("a full lint must complain that the packets are missing")
	}
	if got := errs(lintPlanOpts(d, true)); len(got) != 0 {
		t.Fatalf("a skeleton lint must ignore missing packets, got %v", got)
	}
	bad := mustParse(t, strings.Replace(skeleton, `"depends_on":["T01","T02"]`, `"depends_on":["T01","T77"]`, 1))
	if got := errs(lintPlanOpts(bad, true)); len(got) == 0 {
		t.Fatal("a skeleton lint must still check the graph")
	}
}

func TestCmdPlanLint_SkeletonFlag(t *testing.T) {
	file := filepath.Join(t.TempDir(), "skeleton.md")
	_ = os.WriteFile(file, []byte(goodPlan[:strings.Index(goodPlan, "### T01:")]), 0o644)
	if got := cmdPlan([]string{"lint", file}); got != 2 {
		t.Fatalf("without --skeleton: want exit 2, got %d", got)
	}
	if got := cmdPlan([]string{"lint", "--skeleton", file}); got != 0 {
		t.Fatalf("with --skeleton: want exit 0, got %d", got)
	}
}

func TestPlanLint_PacketMustUseTheTestNameFromTheCoverageTable(t *testing.T) {
	wantErr(t, strings.Replace(goodPlan, "TestLogic_S1_Does in", "TestLogic_Different in", 1), "TestLogic_S1_Does")
}

func TestPlanParse_HeadingsInsideFencedBlocksDoNotEndAPacket(t *testing.T) {
	md := strings.Replace(goodPlan, "type Thing struct{}\n", "type Thing struct{}\n\n## a heading inside a fenced block\n### T99: not a packet\n", 1)
	d := mustParse(t, md)
	p := d.Packets["T01"]
	if p == nil || len(p.Blocks) != 1 || !strings.Contains(p.Blocks[0].Code, "## a heading inside a fenced block") {
		t.Fatalf("the fenced block must keep its heading line, got %+v", p)
	}
	if p.RunCmd != "go test ./internal/app -run Thing" {
		t.Fatalf("content after the fence must still belong to T01, run command %q", p.RunCmd)
	}
	if d.Packets["T99"] != nil {
		t.Fatal("a ### T99 line inside a fence is not a packet")
	}
	if d.Packets["T02"] == nil || d.Packets["T03"] == nil {
		t.Fatal("later packets must still be found")
	}
}

const demoSpec = "# Spec\n\nThe gate runs before any model call: it checks the caller.\n\n- Exit codes: 0 ready,\n  2 not-ready, 3 paused.\n"

func lintWithSpec(t *testing.T, excerpt string) []string {
	t.Helper()
	md := strings.Replace(goodPlan, "### T02: Thing logic\n", "### T02: Thing logic\n\n**Spec excerpt:** "+excerpt+"\n", 1)
	d := mustParse(t, md)
	d.SpecText = demoSpec
	return errs(lintPlan(d))
}

func TestPlanLint_SpecExcerptsMustBeVerbatim(t *testing.T) {
	if got := lintWithSpec(t, `"The gate runs before any model call" and "Exit codes: 0 ready, 2 not-ready, 3 paused."`); len(got) != 0 {
		t.Fatalf("verbatim quotes (whitespace-insensitive) must pass, got %v", got)
	}
	if got := lintWithSpec(t, `"The gate runs ... it checks the caller."`); len(got) != 0 {
		t.Fatalf("an ellipsis may elide text, got %v", got)
	}
	if got := lintWithSpec(t, "none"); len(got) != 0 {
		t.Fatalf("none is allowed, got %v", got)
	}
	found := false
	for _, m := range lintWithSpec(t, `"The gate runs before every model call"`) {
		found = found || strings.Contains(m, "not found in the spec")
	}
	if !found {
		t.Fatal("a paraphrased quote must be an ERR saying it is not found in the spec")
	}
	found = false
	for _, m := range lintWithSpec(t, "The gate runs before any model call") {
		found = found || strings.Contains(m, "quoted")
	}
	if !found {
		t.Fatal("an unquoted excerpt must be an ERR")
	}
}

func TestCmdPlanLint_ReadsTheSpecNamedInTheHeader(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	_ = os.WriteFile(spec, []byte(demoSpec), 0o644)
	plan := strings.Replace(goodPlan, "**Context:** digest", "**Context:** digest\n**Spec:** `"+spec+"`", 1)
	good := strings.Replace(plan, "### T02: Thing logic\n", "### T02: Thing logic\n\n**Spec excerpt:** \"The gate runs before any model call\"\n", 1)
	bad := strings.Replace(plan, "### T02: Thing logic\n", "### T02: Thing logic\n\n**Spec excerpt:** \"The gate never runs\"\n", 1)
	g, b := filepath.Join(dir, "g.md"), filepath.Join(dir, "b.md")
	_ = os.WriteFile(g, []byte(good), 0o644)
	_ = os.WriteFile(b, []byte(bad), 0o644)
	if got := cmdPlan([]string{"lint", g}); got != 0 {
		t.Fatalf("verbatim: want 0, got %d", got)
	}
	if got := cmdPlan([]string{"lint", b}); got != 2 {
		t.Fatalf("paraphrased: want 2, got %d", got)
	}
}
