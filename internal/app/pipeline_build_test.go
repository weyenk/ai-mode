package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const demoStoryPlan = "# Add story plan\n\n" +
	"**Goal:** add two numbers\n**Architecture:** one file\n**Tech Stack:** Go\n" +
	"**Spec:** docs/pipeline/stories/C0004-add.contract.md    **Story Contract:** docs/pipeline/stories/C0004-add.contract.json\n" +
	"**Context:** tools\n**Lint:** ok · **Verify:** ok\n\n" +
	"## Global Constraints\n- Go standard library only.\n\n" +
	"## Review Focus\n1. negative numbers (S1)\n\n" +
	"## Boundaries\n| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |\n| --- | --- | --- | --- | --- | --- |\n| adder | `contract.go` | none | none | add_test.go | T01 |\n\n" +
	"## Task graph\n\n```json\n" +
	`{"tasks":[` + "\n" +
	` {"id":"T01","title":"Contract","kind":"contract","packet":"code","depends_on":[],"owns":["contract.go"],"produces":["type Adder"],"consumes":[],"covers":[],"test_level":"contract","stack_layer":1},` + "\n" +
	` {"id":"T02","title":"Add","kind":"impl","packet":"spec","depends_on":["T01"],"owns":["add.go","add_test.go"],"produces":["func Add(a, b int) int"],"consumes":["Adder"],"covers":["S1"],"test_level":"unit","stack_layer":1}` + "\n" +
	`],"hot_files":["go.mod","README.md"]}` + "\n```\n\n" +
	"**Scenario coverage**\n\n| Scenario | Rule | Task | Test |\n| --- | --- | --- | --- |\n| S1 | sums | T02 | `TestAdd_S1_SumsTwoNumbers` |\n\n" +
	"### T01: Contract\n\n**Spec excerpt:** none\n\n**Phase A: the types.**\n\n```go file=contract.go\npackage demo\n\n// Adder adds two numbers.\ntype Adder func(a, b int) int\n```\n\nRun: `go vet ./...`\n\n" +
	"### T02: Add\n\n**Spec excerpt:** none\n\n- S1 `TestAdd_S1_SumsTwoNumbers`: Given 2 and 3, when Add runs, then it returns 5.\n\nRun: `go test ./... -run TestAdd_S1`\n"

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
			replies = []agentMsg{call("1", "write_file", map[string]any{"path": "docs/pipeline/stories/C0004-add.plan.md", "content": demoStoryPlan})}
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
	log := gitT(t, wtMod, "log", "--format=%s", "pipe..demo-request-01-core")
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
	d, _ := parsePlan(demoStoryPlan)
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
	pl.newAgent = func(s agentSpec) (*agentSession, error) {
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
	if !strings.Contains(note.Done, "1 fix round") {
		t.Fatalf("note %+v", note)
	}
}
