package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureRepo is a tiny Go module the verifier can copy and run `go` in.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func codeBlock(file, code string) string {
	return fence + "go file=" + file + "\n" + code + fence + "\n"
}

// onePacketPlan builds a plan with one contract task whose packet has the given Phase A and B blocks.
func onePacketPlan(phaseA, phaseB, run string) string {
	return `# Demo Implementation Plan

**Context:** digest

## Task graph

` + fence + `json
{"tasks":[{"id":"T01","title":"Adder","kind":"impl","packet":"code","depends_on":[],"owns":["thing.go","thing_test.go"],"covers":[],"branch":"b1"}]}
` + fence + `

### T01: Adder

**Phase A: tests plus stubs (role: qa).**

` + phaseA + `
Run: ` + "`" + run + "`" + `

**Phase B: implementation (role: coder).**

` + phaseB
}

const adderStub = "package demo\n\nfunc Add(a, b int) int { return 0 }\n"
const adderImpl = "package demo\n\nfunc Add(a, b int) int { return a + b }\n"
const adderTest = "package demo\n\nimport \"testing\"\n\nfunc TestAdd_Thing(t *testing.T) {\n\tif got := Add(1, 2); got != 3 {\n\t\tt.Fatalf(\"want 3, got %d\", got)\n\t}\n}\n"

func stepNamed(r planVerifyResult, sub string) *planStep {
	for i := range r.Steps {
		if strings.Contains(r.Steps[i].Name, sub) {
			return &r.Steps[i]
		}
	}
	return nil
}

func runVerify(t *testing.T, repo, md, task string) planVerifyResult {
	t.Helper()
	return verifyPlanTask(mustParse(t, md), task, repo)
}

func TestPlanVerify_RedThenGreenIsVerified(t *testing.T) {
	repo := fixtureRepo(t)
	md := onePacketPlan(codeBlock("thing.go", adderStub)+codeBlock("thing_test.go", adderTest), codeBlock("thing.go", adderImpl), "go test ./... -run Thing")
	r := runVerify(t, repo, md, "T01")
	if r.Status != "verified" {
		t.Fatalf("want verified, got %+v", r)
	}
	// the real repo must be untouched: verification runs on a copy
	if _, err := os.Stat(filepath.Join(repo, "thing.go")); err == nil {
		t.Fatal("verify wrote into the real repo")
	}
}

func TestPlanVerify_RedThatIsABuildErrorFails(t *testing.T) {
	repo := fixtureRepo(t)
	md := onePacketPlan(codeBlock("thing_test.go", adderTest), codeBlock("thing.go", adderImpl), "go test ./... -run Thing")
	r := runVerify(t, repo, md, "T01")
	if r.Status != "failed" {
		t.Fatalf("want failed, got %+v", r)
	}
	if s := stepNamed(r, "compile"); s == nil || s.OK {
		t.Fatalf("the Phase A compile step must fail, steps: %+v", r.Steps)
	}
}

func TestPlanVerify_TestsThatPassAgainstTheStubAreNotRed(t *testing.T) {
	repo := fixtureRepo(t)
	md := onePacketPlan(codeBlock("thing.go", adderImpl)+codeBlock("thing_test.go", adderTest), codeBlock("thing.go", adderImpl), "go test ./... -run Thing")
	r := runVerify(t, repo, md, "T01")
	if s := stepNamed(r, "red"); r.Status != "failed" || s == nil || s.OK || !strings.Contains(s.Detail, "pass") {
		t.Fatalf("want a failed red step saying the tests pass, got %+v", r)
	}
}

func TestPlanVerify_RunPatternThatMatchesNothingIsNotRed(t *testing.T) {
	repo := fixtureRepo(t)
	md := onePacketPlan(codeBlock("thing.go", adderStub)+codeBlock("thing_test.go", adderTest), codeBlock("thing.go", adderImpl), "go test ./... -run NoSuchTest")
	r := runVerify(t, repo, md, "T01")
	if r.Status != "failed" || stepNamed(r, "red") == nil || stepNamed(r, "red").OK {
		t.Fatalf("a -run pattern matching no test must fail the red step, got %+v", r)
	}
}

func TestPlanVerify_ImplementationThatStaysRedFails(t *testing.T) {
	repo := fixtureRepo(t)
	wrong := "package demo\n\nfunc Add(a, b int) int { return a - b }\n"
	md := onePacketPlan(codeBlock("thing.go", adderStub)+codeBlock("thing_test.go", adderTest), codeBlock("thing.go", wrong), "go test ./... -run Thing")
	r := runVerify(t, repo, md, "T01")
	if s := stepNamed(r, "green"); r.Status != "failed" || s == nil || s.OK {
		t.Fatalf("want a failed green step, got %+v", r)
	}
}

func TestPlanVerify_UnformattedGoFails(t *testing.T) {
	repo := fixtureRepo(t)
	ugly := "package demo\nfunc Add(a, b int) int {\nreturn a + b }\n"
	md := onePacketPlan(codeBlock("thing.go", adderStub)+codeBlock("thing_test.go", adderTest), codeBlock("thing.go", ugly), "go test ./... -run Thing")
	r := runVerify(t, repo, md, "T01")
	if s := stepNamed(r, "gofmt"); r.Status != "failed" || s == nil || s.OK {
		t.Fatalf("want a failed gofmt step, got %+v", r)
	}
}

func TestPlanVerify_SpecPacketIsNotApplicable(t *testing.T) {
	repo := fixtureRepo(t)
	md := strings.Replace(onePacketPlan("", "", "go test ./..."), `"packet":"code"`, `"packet":"spec"`, 1)
	r := runVerify(t, repo, md, "T01")
	if r.Status != "n/a" || !strings.Contains(r.Note, "spec packet") {
		t.Fatalf("want n/a with a note, got %+v", r)
	}
}

func TestPlanVerify_AppliesContractDependenciesFirst(t *testing.T) {
	repo := fixtureRepo(t)
	md := `# Demo

**Context:** digest

## Task graph

` + fence + `json
{"tasks":[
 {"id":"T01","title":"Contract","kind":"contract","packet":"code","depends_on":[],"owns":["shape.go"],"covers":[],"branch":"b1"},
 {"id":"T02","title":"Uses it","kind":"impl","packet":"code","depends_on":["T01"],"owns":["use.go","use_test.go"],"covers":[],"branch":"b2"}
]}
` + fence + `

### T01: Contract

**Phase A: stub (role: qa).**

` + codeBlock("shape.go", "package demo\n\ntype Shape struct{ W, H int }\n") + `
Run: ` + "`go vet ./...`" + `

**Phase B: nothing more.**

### T02: Uses it

**Phase A: tests plus stubs (role: qa).**

` + codeBlock("use.go", "package demo\n\nfunc Area(s Shape) int { return 0 }\n") +
		codeBlock("use_test.go", "package demo\n\nimport \"testing\"\n\nfunc TestArea_Thing(t *testing.T) {\n\tif got := Area(Shape{W: 2, H: 3}); got != 6 {\n\t\tt.Fatalf(\"want 6, got %d\", got)\n\t}\n}\n") + `
Run: ` + "`go test ./... -run Area`" + `

**Phase B: implementation (role: coder).**

` + codeBlock("use.go", "package demo\n\nfunc Area(s Shape) int { return s.W * s.H }\n")
	if r := runVerify(t, repo, md, "T02"); r.Status != "verified" {
		t.Fatalf("T02 should verify with T01's types applied first, got %+v", r)
	}
}

func TestPlanVerify_CommandExitCodes(t *testing.T) {
	repo := fixtureRepo(t)
	dir := t.TempDir()
	good, bad := filepath.Join(dir, "good.md"), filepath.Join(dir, "bad.md")
	_ = os.WriteFile(good, []byte(onePacketPlan(codeBlock("thing.go", adderStub)+codeBlock("thing_test.go", adderTest), codeBlock("thing.go", adderImpl), "go test ./... -run Thing")), 0o644)
	_ = os.WriteFile(bad, []byte(onePacketPlan(codeBlock("thing.go", adderStub)+codeBlock("thing_test.go", adderTest), codeBlock("thing.go", adderStub), "go test ./... -run Thing")), 0o644)
	if got := cmdPlan([]string{"verify", good, "--task", "T01", "--repo", repo}); got != 0 {
		t.Fatalf("verified: want exit 0, got %d", got)
	}
	if got := cmdPlan([]string{"verify", bad, "--task", "T01", "--repo", repo}); got != 2 {
		t.Fatalf("failed: want exit 2, got %d", got)
	}
	if got := cmdPlan([]string{"verify", good, "--repo", repo}); got != 1 {
		t.Fatalf("missing --task: want exit 1, got %d", got)
	}
	if got := cmdPlan([]string{"verify", good, "--task", "T99", "--repo", repo}); got != 1 {
		t.Fatalf("unknown task: want exit 1, got %d", got)
	}
}

func TestPlanVerify_AllRunsEveryTaskInWaveOrder(t *testing.T) {
	repo := fixtureRepo(t)
	file := filepath.Join(t.TempDir(), "p.md")
	_ = os.WriteFile(file, []byte(onePacketPlan(codeBlock("thing.go", adderStub)+codeBlock("thing_test.go", adderTest), codeBlock("thing.go", adderImpl), "go test ./... -run Thing")), 0o644)
	if got := cmdPlan([]string{"verify", file, "--task", "all", "--repo", repo}); got != 0 {
		t.Fatalf("want exit 0, got %d", got)
	}
}

func TestPlan_RegisteredInCommandsUsageAndCompletions(t *testing.T) {
	if _, ok := commands()["plan"]; !ok {
		t.Fatal(`"plan" is not registered in commands()`)
	}
	var b strings.Builder
	usage(&b)
	if !strings.Contains(b.String(), "plan") {
		t.Fatalf("usage does not list plan:\n%s", b.String())
	}
	comp, err := os.ReadFile("../../completions/_ai-mode")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(comp), "'plan:") {
		t.Fatal("completions/_ai-mode has no 'plan:' entry")
	}
}

func contractPlan(code string) string {
	return `# Demo

**Context:** digest

## Task graph

` + fence + `json
{"tasks":[{"id":"T01","title":"Types","kind":"contract","packet":"code","depends_on":[],"owns":["shape.go"],"covers":[],"branch":"b1"}]}
` + fence + `

### T01: Types

**Phase A: the types.**

` + codeBlock("shape.go", code) + `
**Phase B: nothing more.**
`
}

func TestPlanVerify_ContractTaskNeedsNoRedButMustCompileAndBeFormatted(t *testing.T) {
	repo := fixtureRepo(t)
	if r := runVerify(t, repo, contractPlan("package demo\n\ntype Shape struct{ W, H int }\n"), "T01"); r.Status != "verified" {
		t.Fatalf("a types-only contract must verify without a red step, got %+v", r)
	}
	if r := runVerify(t, repo, contractPlan("package demo\n\ntype Shape struct{ W, H int \n"), "T01"); r.Status != "failed" {
		t.Fatalf("a contract that does not compile must fail, got %+v", r)
	}
	if r := runVerify(t, repo, contractPlan("package demo\ntype Shape struct{W,H int}\n"), "T01"); r.Status != "failed" || stepNamed(r, "gofmt") == nil {
		t.Fatalf("an unformatted contract must fail the gofmt step, got %+v", r)
	}
}

func TestPlanVerify_NestedCommandsAreMarkedSoVerificationCannotRecurse(t *testing.T) {
	out, code := runShell(t.TempDir(), "echo $AI_MODE_PLAN_VERIFYING")
	if code != 0 || strings.TrimSpace(out) != "1" {
		t.Fatalf("commands run by the verifier must see AI_MODE_PLAN_VERIFYING=1, got %q (exit %d)", out, code)
	}
}

func TestPlanVerify_ProductionCodeMayNotDependOnTestOnlyDeclarations(t *testing.T) {
	repo := fixtureRepo(t)
	// go vet compiles tests too, so it accepts this; go build does not: shape.go uses a type only shape_test.go declares.
	md := `# Demo

**Context:** digest

## Task graph

` + fence + `json
{"tasks":[{"id":"T01","title":"Types","kind":"contract","packet":"code","depends_on":[],"owns":["shape.go","shape_test.go"],"covers":[],"branch":"b1"}]}
` + fence + `

### T01: Types

**Phase A: the types.**

` + codeBlock("shape.go", "package demo\n\nfunc Area(s Shape) int { return s.W * s.H }\n") +
		codeBlock("shape_test.go", "package demo\n\ntype Shape struct{ W, H int }\n") + `
**Phase B: nothing more.**
`
	r := runVerify(t, repo, md, "T01")
	if s := stepNamed(r, "build"); r.Status != "failed" || s == nil || s.OK {
		t.Fatalf("a go build failure must fail the build step, got %+v", r)
	}
}
