package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const demoStoryPlanJSON = `{"goal": "add two numbers", "architecture": "one file", "review_focus": ["negative numbers (S1)"],
 "boundaries": [{"boundary": "adder", "artifact": "contract.go", "fixtures": "none", "fake": "none", "provider_check": "add_test.go", "owner": "T01"}],
 "tasks": [
  {"id": "T01", "title": "Contract", "kind": "contract", "packet": "code", "owns": ["contract.go"], "produces": ["type Adder"], "stack_layer": 1, "test_level": "contract",
   "files": [{"path": "contract.go", "phase": "A", "code": "package demo\n\n// Adder adds two numbers.\ntype Adder func(a, b int) int\n"}]},
  {"id": "T02", "title": "Add", "kind": "impl", "packet": "spec", "depends_on": ["T01"], "owns": ["add.go", "add_test.go"], "produces": ["func Add(a, b int) int"],
   "consumes": ["Adder"], "stack_layer": 1, "test_level": "unit",
   "scenarios": [{"id": "S1", "test": "TestAdd_S1_SumsTwoNumbers", "check": "Given 2 and 3, when Add runs, then it returns 5"}]}
 ]}`

const demoTest = "package demo\n\nimport \"testing\"\n\nfunc TestAdd_S1_SumsTwoNumbers(t *testing.T) {\n\tif got := Add(2, 3); got != 5 {\n\t\tt.Fatalf(\"want 5, got %d\", got)\n\t}\n}\n"

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// buildFixture: a git repo with the module at examples/demo, cards up to a ready story, on branch "pipe".
func buildFixture(t *testing.T) (*pipeline, card) {
	t.Helper()
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t",
		"GIT_COMMITTER_EMAIL": "t@example.com", "AI_MODE_STATE": t.TempDir()} {
		t.Setenv(k, v)
	}
	root := t.TempDir()
	gitT(t, root, "init", "-q", "-b", "main")
	mod := filepath.Join(root, "examples", "demo")
	os.MkdirAll(mod, 0o755)
	os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644)
	pl := &pipeline{target: mod, docs: filepath.Join(mod, "docs", "pipeline"), board: fileBoard{dir: filepath.Join(mod, "docs", "pipeline", "board")},
		stages: defaultStages(), now: func() string { return "2026-10-11T12:00:00Z" }}
	req, _ := pl.newCard(card{Title: "Demo request", Type: "feature", Stage: "tracking"})
	sp, _ := pl.newCard(card{Title: "Demo spec", Type: "spec", Stage: "tracking", Parent: req.ID})
	plan, _ := pl.newCard(card{Title: "Demo plan", Type: "plan", Stage: "tracking", Parent: sp.ID})
	st, _ := pl.newCard(card{Title: "Add", Type: "story", Stage: "ready", Parent: plan.ID, Order: 1})
	writeArtifact(t, pl, &st, "story", "docs/pipeline/stories/C0004-add.md", "# Add\n\n## Story\n\nAs a user, I want sums.\n\n## Global Constraints\n\n- stdlib only\n")
	c := readyContract()
	c.ReadyRoles, c.Deliverable = []string{"product", "qa", "architect"}, true
	if err := saveStoryVerdict(pl.abs(st.Artifacts["story"]), "m1", c); err != nil {
		t.Fatal(err)
	}
	pl.board.save(st)
	pl.ask = func(role, user string, n int) (string, error) {
		return "Looks consistent.\n```json\n{\"approve\": true, \"problems\": []}\n```", nil
	}
	gitT(t, root, "checkout", "-q", "-b", "pipe")
	gitT(t, root, "add", "-A")
	gitT(t, root, "commit", "-q", "-m", "fixture")
	return pl, st
}

func scriptedAgents(t *testing.T, calls *[]string) func(agentSpec) (*agentSession, error) {
	return func(s agentSpec) (*agentSession, error) {
		*calls = append(*calls, s.Role)
		var replies []agentMsg
		switch s.Role {
		case "architect":
			replies = []agentMsg{call("1", "write_file", map[string]any{"path": "docs/pipeline/stories/C0004-add.plan.json", "content": demoStoryPlanJSON})}
		case "qa":
			replies = []agentMsg{
				call("1", "write_file", map[string]any{"path": "add_test.go", "content": demoTest}),
				call("2", "write_file", map[string]any{"path": "add.go", "content": "package demo\n\n// Add is a stub.\nfunc Add(a, b int) int { return 0 }\n"}),
			}
		case "coder":
			replies = []agentMsg{
				call("1", "write_file", map[string]any{"path": "add_test.go", "content": "package demo\n"}), // refused: read-only
				call("2", "write_file", map[string]any{"path": "add.go", "content": "package demo\n\n// Add returns a+b.\nfunc Add(a, b int) int { return a + b }\n"}),
			}
		default:
			return nil, fmt.Errorf("unexpected role %s", s.Role)
		}
		replies = append(replies, call("9", "finish", map[string]any{"summary": "done"}))
		return &agentSession{spec: s, chat: &scriptedChat{replies: replies}}, nil
	}
}

func TestImplementStage_PlanRedGreenOnAStackLayer(t *testing.T) {
	pl, st := buildFixture(t)
	pl.git = true // the pipeline commits notes to the base branch mid-stage; the stack must not care
	var calls []string
	pl.newAgent = scriptedAgents(t, &calls)
	next, note, err := stageImplement(pl, &st)
	if err != nil || next != "architect-review" {
		t.Fatalf("next=%s err=%v note=%+v", next, err, note)
	}
	if strings.Join(calls, ",") != "architect,qa,coder" {
		t.Fatalf("agents %v", calls)
	}
	stack, _ := pl.loadStack("C0003")
	if len(stack.Layers) != 1 || stack.Layers[0].Branch != "demo-request-01-core" || strings.Join(stack.Layers[0].Tasks, ",") != "T01,T02" || stack.Base != "pipe" {
		t.Fatalf("stack %+v", stack)
	}
	wtMod := filepath.Join(stack.Worktree, "examples", "demo")
	log := gitT(t, wtMod, "log", "--format=%s", stack.BaseSHA+"..demo-request-01-core")
	if log != "feat(T02): Add\ntest(T02): red — Add\ncontract(T01): Contract" {
		t.Fatalf("commits:\n%s", log)
	}
	if b, _ := os.ReadFile(filepath.Join(wtMod, "add_test.go")); string(b) != demoTest {
		t.Fatal("coder must not be able to change the test")
	}
	if _, err := os.Stat(pl.abs("docs/pipeline/stories/C0004-add.plan.md")); err != nil {
		t.Fatal("story plan not copied to the pipeline docs")
	}
	if cm, _ := os.ReadFile(pl.abs(st.Artifacts["contract-md"])); !strings.Contains(string(cm), "- **S1** (R1) e1") {
		t.Fatalf("contract.md:\n%s", cm)
	}
	// the main checkout stays on the pipeline branch; the code lives only on the stack
	if b := gitT(t, pl.target, "rev-parse", "--abbrev-ref", "HEAD"); b != "pipe" {
		t.Fatalf("main checkout moved to %s", b)
	}
	if _, err := os.Stat(pl.abs("add.go")); err == nil {
		t.Fatal("code leaked into the pipeline checkout")
	}
}

func TestImplementStage_RefusesStoryThatIsNotReady(t *testing.T) {
	pl, st := buildFixture(t)
	c := readyContract()
	c.Verdict = VerdictNotReady
	saveStoryVerdict(pl.abs(st.Artifacts["story"]), "m2", c)
	pl.newAgent = func(agentSpec) (*agentSession, error) {
		t.Fatal("no agent may start on a story that is not ready")
		return nil, nil
	}
	if _, _, err := stageImplement(pl, &st); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("err = %v", err)
	}
}

func TestTDDCheck_FlagsTestsEditedAfterRed(t *testing.T) {
	pl, st := buildFixture(t)
	var calls []string
	pl.newAgent = scriptedAgents(t, &calls)
	if _, _, err := stageImplement(pl, &st); err != nil {
		t.Fatal(err)
	}
	stack, _ := pl.loadStack("C0003")
	wtMod := filepath.Join(stack.Worktree, "examples", "demo")
	planMD, _ := os.ReadFile(pl.abs("docs/pipeline/stories/C0004-add.plan.md"))
	d, _ := parsePlan(string(planMD))
	if len(d.Tasks) != 2 {
		t.Fatalf("rendered plan has %d tasks", len(d.Tasks))
	}
	if res := tddCheck(d, wtMod, "C0004"); !res.ok() {
		t.Fatalf("clean history should pass: %v", res.problems())
	}
	os.WriteFile(filepath.Join(wtMod, "add_test.go"), []byte(strings.Replace(demoTest, "want 5", "want five", 1)), 0o644)
	gitT(t, wtMod, "commit", "-q", "-am", "edit test")
	if res := tddCheck(d, wtMod, "C0004"); res.ok() || !strings.Contains(strings.Join(res.problems(), " "), "changed after the red commit") {
		t.Fatalf("an edited test must fail tdd-check: %v", res.problems())
	}
}

func TestRedCheck_RejectsBuildFailuresAndPassingTests(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "add_test.go"), []byte(demoTest), 0o644)
	if f, _ := redCheck(dir, []string{"TestAdd_S1_SumsTwoNumbers"}); len(f) == 0 || !strings.Contains(f[0], "do not compile") {
		t.Fatalf("missing Add must be a compile failure, got %v", f)
	}
	os.WriteFile(filepath.Join(dir, "add.go"), []byte("package demo\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	if f, _ := redCheck(dir, []string{"TestAdd_S1_SumsTwoNumbers"}); len(f) == 0 || !strings.Contains(f[0], "passed against the stubs") {
		t.Fatalf("a passing test is not red, got %v", f)
	}
	os.WriteFile(filepath.Join(dir, "add.go"), []byte("package demo\n\nfunc Add(a, b int) int { return 0 }\n"), 0o644)
	if f, _ := redCheck(dir, []string{"TestAdd_S1_SumsTwoNumbers"}); len(f) != 0 {
		t.Fatalf("stub returning 0 is red: %v", f)
	}
}

func TestReviewStage_BlockingCommentFixedInTheLayerThenApproved(t *testing.T) {
	pl, st := buildFixture(t)
	var calls []string
	pl.newAgent = scriptedAgents(t, &calls)
	if _, _, err := stageImplement(pl, &st); err != nil {
		t.Fatal(err)
	}
	reviews := []string{
		"Missing doc.\n```json\n{\"approve\": false, \"comments\": [{\"file\": \"add.go\", \"line\": 3, \"severity\": \"blocking\", \"comment\": \"say what Add does with overflow\"}]}\n```",
		"Good.\n```json\n{\"approve\": true, \"comments\": [{\"file\": \"add.go\", \"line\": 1, \"severity\": \"nit\", \"comment\": \"fine\"}]}\n```",
	}
	pl.ask = func(role, user string, n int) (string, error) {
		if role != "architect" || !strings.Contains(user, "func Add") {
			return "", fmt.Errorf("review needs architect and the diff, got %s", role)
		}
		r := reviews[0]
		reviews = reviews[1:]
		return r, nil
	}
	var fixers []string
	pl.newAgent = func(s agentSpec) (*agentSession, error) {
		fixers = append(fixers, s.Role)
		if s.Role == "qa" {
			if strings.Join(s.Owns, ",") != "add_test.go" {
				return nil, fmt.Errorf("qa owns the layer's tests, got %v", s.Owns)
			}
			return &agentSession{spec: s, chat: &scriptedChat{replies: []agentMsg{call("1", "finish", map[string]any{"summary": "tests already require it"})}}}, nil
		}
		if s.Role != "coder" || strings.Join(s.Owns, ",") != "add.go,contract.go" || strings.Join(s.ReadOnly, ",") != "add_test.go" {
			return nil, fmt.Errorf("fix goes to coder owning the layer's code, got %s %v %v", s.Role, s.Owns, s.ReadOnly)
		}
		return &agentSession{spec: s, chat: &scriptedChat{replies: []agentMsg{
			call("1", "write_file", map[string]any{"path": "add.go", "content": "package demo\n\n// Add returns a+b; it wraps on overflow like Go's int.\nfunc Add(a, b int) int { return a + b }\n"}),
			call("2", "finish", map[string]any{"summary": "documented"}),
		}}}, nil
	}
	next, note, err := stageReview(pl, &st)
	if err != nil || next != "handoff" {
		t.Fatalf("next=%s err=%v note=%+v", next, err, note)
	}
	stack, _ := pl.loadStack("C0003")
	wtMod := filepath.Join(stack.Worktree, "examples", "demo")
	if top := gitT(t, wtMod, "log", "-1", "--format=%s", stack.Layers[0].Branch); top != "fix(review): layer 1 round 1" {
		t.Fatalf("top commit %q", top)
	}
	if _, err := os.Stat(pl.abs("docs/pipeline/reviews/C0004/layer-1-round-2.json")); err != nil {
		t.Fatal("second review round not saved")
	}
	if !strings.Contains(note.Done, "1 fix round") || strings.Join(fixers, ",") != "qa,coder" {
		t.Fatalf("note %+v fixers %v", note, fixers)
	}
}

func TestImplementStage_TestReviewSendsContradictionsBackToQA(t *testing.T) {
	pl, st := buildFixture(t)
	wrong := strings.Replace(demoTest, "Add(2, 3); got != 5", "Add(2, 3); got != 6", 1)
	reviews := []string{
		"```json\n{\"approve\": false, \"problems\": [{\"test\": \"TestAdd_S1_SumsTwoNumbers\", \"scenario\": \"S1\", \"problem\": \"2+3 is 5, not 6\", \"fix\": \"assert 5\"}]}\n```",
		"```json\n{\"approve\": true, \"problems\": []}\n```",
	}
	pl.ask = func(role, user string, n int) (string, error) {
		r := reviews[0]
		reviews = reviews[1:]
		return r, nil
	}
	pl.newAgent = func(s agentSpec) (*agentSession, error) {
		var replies []agentMsg
		switch s.Role {
		case "architect":
			replies = []agentMsg{call("1", "write_file", map[string]any{"path": "docs/pipeline/stories/C0004-add.plan.json", "content": demoStoryPlanJSON})}
		case "qa":
			replies = []agentMsg{
				call("1", "write_file", map[string]any{"path": "add_test.go", "content": wrong}),
				call("2", "write_file", map[string]any{"path": "add.go", "content": "package demo\n\n// Add is a stub.\nfunc Add(a, b int) int { return 0 }\n"}),
				call("3", "finish", map[string]any{"summary": "red"}),
				call("4", "write_file", map[string]any{"path": "add_test.go", "content": demoTest}),
			}
		case "coder":
			replies = []agentMsg{call("1", "write_file", map[string]any{"path": "add.go", "content": "package demo\n\n// Add returns a+b.\nfunc Add(a, b int) int { return a + b }\n"})}
		}
		return &agentSession{spec: s, chat: &scriptedChat{replies: append(replies, call("9", "finish", map[string]any{"summary": "done"}))}}, nil
	}
	if next, note, err := stageImplement(pl, &st); err != nil || next != "architect-review" {
		t.Fatalf("next=%s err=%v note=%+v", next, err, note)
	}
	cm, _ := os.ReadFile(pl.abs(st.Artifacts["contract-md"]))
	if !strings.Contains(string(cm), "## Amendments") || !strings.Contains(string(cm), "2+3 is 5, not 6 → assert 5") {
		t.Fatalf("amendment not recorded:\n%s", cm)
	}
}

func TestImplementStage_StuckGreenIsTriagedAndAWrongTestFixedAsReview(t *testing.T) {
	pl, st := buildFixture(t)
	wrong := strings.Replace(demoTest, "Add(2, 3); got != 5", "Add(2, 3); got != 6", 1)
	pl.ask = func(role, user string, n int) (string, error) {
		if strings.Contains(user, "could not make task") {
			return "```json\n{\"cause\": \"test\", \"problems\": [{\"test\": \"TestAdd_S1_SumsTwoNumbers\", \"scenario\": \"S1\", \"problem\": \"expects 6\", \"fix\": \"expect 5\"}], \"diagnosis\": \"the code is right\"}\n```", nil
		}
		return "```json\n{\"approve\": true, \"problems\": []}\n```", nil // a review that missed it
	}
	qaRuns := 0
	pl.newAgent = func(s agentSpec) (*agentSession, error) {
		var replies []agentMsg
		switch s.Role {
		case "architect":
			replies = []agentMsg{call("1", "write_file", map[string]any{"path": "docs/pipeline/stories/C0004-add.plan.json", "content": demoStoryPlanJSON})}
		case "qa":
			qaRuns++
			if qaRuns == 1 {
				replies = []agentMsg{
					call("1", "write_file", map[string]any{"path": "add_test.go", "content": wrong}),
					call("2", "write_file", map[string]any{"path": "add.go", "content": "package demo\n\n// Add is a stub.\nfunc Add(a, b int) int { return 0 }\n"}),
				}
			} else {
				replies = []agentMsg{call("1", "write_file", map[string]any{"path": "add_test.go", "content": demoTest})}
			}
		case "coder":
			replies = []agentMsg{call("1", "write_file", map[string]any{"path": "add.go", "content": "package demo\n\n// Add returns a+b.\nfunc Add(a, b int) int { return a + b }\n"})}
		}
		return &agentSession{spec: s, chat: &scriptedChat{replies: append(replies, call("9", "finish", map[string]any{"summary": "done"}))}}, nil
	}
	if next, note, err := stageImplement(pl, &st); err != nil || next != "architect-review" {
		t.Fatalf("next=%s err=%v note=%+v", next, err, note)
	}
	stack, _ := pl.loadStack("C0003")
	wtMod := filepath.Join(stack.Worktree, "examples", "demo")
	log := gitT(t, wtMod, "log", "--format=%s", stack.BaseSHA+".."+stack.Layers[0].Branch)
	if !strings.Contains(log, "fix(contract): T02 tests follow architect's ruling") || !strings.HasPrefix(log, "feat(T02)") {
		t.Fatalf("commits:\n%s", log)
	}
}

func TestReplan_CommittedTasksMustStayAsCommitted(t *testing.T) {
	pl, st := buildFixture(t)
	var calls []string
	pl.newAgent = scriptedAgents(t, &calls)
	if _, _, err := stageImplement(pl, &st); err != nil {
		t.Fatal(err)
	}
	stack, _ := pl.loadStack("C0003")
	wtMod := filepath.Join(stack.Worktree, "examples", "demo")
	got := committedTasks(&stack, wtMod, "C0004")
	if got["T02"] == nil || strings.Join(got["T02"].Files, ",") != "add.go,add_test.go" || strings.Join(got["T02"].Tests, ",") != "TestAdd_S1_SumsTwoNumbers" {
		t.Fatalf("committed %+v", got["T02"])
	}
	moved := strings.Replace(strings.Replace(demoStoryPlanJSON, `"add.go", "add_test.go"`, `"sum.go", "sum_test.go"`, 1), "TestAdd_S1_SumsTwoNumbers", "TestSum_S1_Two", 1)
	os.WriteFile(filepath.Join(wtMod, "docs/pipeline/stories/C0004-add.plan.json"), []byte(moved), 0o644)
	f := strings.Join(committedPlanCheck(wtMod, "docs/pipeline/stories/C0004-add.plan.json", got), "\n")
	for _, want := range []string{"owns must include add.go", "must name an existing test (TestAdd_S1_SumsTwoNumbers), not TestSum_S1_Two"} {
		if !strings.Contains(f, want) {
			t.Errorf("missing %q in:\n%s", want, f)
		}
	}
	if note := committedNote(got); !strings.Contains(note, "T02: files add.go, add_test.go; tests TestAdd_S1_SumsTwoNumbers") {
		t.Fatalf("note %q", note)
	}
}

func TestWiring_LastStoryMustWireAnEmptyMain(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module p\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "types.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	rel, empty := mainFile(dir)
	if rel != "types.go" || !empty {
		t.Fatalf("mainFile = %q %v", rel, empty)
	}
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	plan := `{"goal":"g","architecture":"a","tasks":[{"id":"T01","title":"Flags","owns":["flags.go","flags_test.go"],"stack_layer":1,
		"scenarios":[{"id":"S1","test":"TestFlags_S1_X","check":"x"}]}]}`
	os.WriteFile(filepath.Join(dir, "docs", "p.plan.json"), []byte(plan), 0o644)
	if f := wiringPlanCheck(dir, "docs/p.plan.json", "types.go"); len(f) != 1 || !strings.Contains(f[0], "own types.go") {
		t.Fatalf("findings %v", f)
	}
	os.WriteFile(filepath.Join(dir, "docs", "p.plan.json"), []byte(strings.Replace(plan, `"flags.go"`, `"types.go", "flags.go"`, 1)), 0o644)
	if f := wiringPlanCheck(dir, "docs/p.plan.json", "types.go"); len(f) != 0 {
		t.Fatalf("a task owning main's file with a scenario passes: %v", f)
	}
	os.WriteFile(filepath.Join(dir, "types.go"), []byte("package main\n\nimport \"os\"\n\nfunc main() { os.Exit(run()) }\n\nfunc run() int { return 0 }\n"), 0o644)
	if _, empty := mainFile(dir); empty {
		t.Fatal("a wired main is not empty")
	}
}

func TestStoryPlan_CodePacketMayNotRewriteAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "types.go"), []byte("package main\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs", "p.plan.json"), []byte(`{"goal":"g","architecture":"a","tasks":[
		{"id":"T01","title":"c","kind":"contract","owns":["types.go","new.go"],"stack_layer":1,
		 "files":[{"path":"types.go","phase":"A","code":"package main\n"},{"path":"new.go","phase":"A","code":"package main\n"}]}]}`), 0o644)
	f := existingFileBlocks(dir, "docs/p.plan.json", nil)
	if len(f) != 1 || !strings.Contains(f[0], "whole of types.go") {
		t.Fatalf("findings %v", f)
	}
	if f := existingFileBlocks(dir, "docs/p.plan.json", map[string]string{"T01": "contract"}); len(f) != 0 {
		t.Fatalf("a committed task's files exist because it wrote them: %v", f)
	}
}

func TestObservedRun_ReportsOutputAndCtrlC(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module p\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import (
	"fmt"
	"os"
	"os/signal"
)

func main() {
	fmt.Print("tick\r\x1b[K")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	fmt.Println("Completed 0 focus sessions")
}
`), 0o644)
	got := observedRun(dir)
	for _, want := range []string{"received Ctrl-C (SIGINT) and exited (exit status 0)", `"tick\r\x1b[KCompleted 0 focus sessions\n"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
