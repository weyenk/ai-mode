package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Stage 8, implement (`ai-mode work start`): only a story whose saved amigos verdict is ready. architect writes the
// story plan (drafting-plans story mode) with plan lint and verify in its loop; then each task runs test-first in a
// dedicated git worktree that holds the feature's linear stack of layer branches: qa writes the tests and stubs and
// they must fail on their assertions (committed as the red commit), coder makes them pass without touching the tests
// (the green commit), and the module's gates must stay green. `ai-mode plan tdd-check` re-checks red before green.

type stackLayer struct {
	N      int      `json:"n"` // position in the whole stack, 1 = bottom
	Branch string   `json:"branch"`
	Story  string   `json:"story"` // story card
	Layer  int      `json:"layer"` // the story plan's stack_layer
	Name   string   `json:"name"`
	Tasks  []string `json:"tasks"`
	PR     string   `json:"pr,omitempty"`
}

type stackState struct {
	Plan     string       `json:"plan"` // plan card that owns the stack
	Base     string       `json:"base"` // branch the stack starts from (the pipeline branch)
	Slug     string       `json:"slug"`
	Worktree string       `json:"worktree"`
	Module   string       `json:"module"` // the target's path inside the git repo
	Layers   []stackLayer `json:"layers"`
}

func (pl *pipeline) stackRel(planID string) string { return "docs/pipeline/stack/" + planID + ".json" }

func (pl *pipeline) loadStack(planID string) (stackState, error) {
	st := stackState{Plan: planID}
	data, err := os.ReadFile(pl.abs(pl.stackRel(planID)))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(data, &st)
}

func (pl *pipeline) saveStack(st stackState) error {
	p := pl.abs(pl.stackRel(st.Plan))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(p, append(data, '\n'), 0o644)
}

func (st *stackState) tip() string {
	if n := len(st.Layers); n > 0 {
		return st.Layers[n-1].Branch
	}
	return st.Base
}

// ensureWorktree creates (or reuses) the plan's git worktree, checked out at the stack tip, and returns the module dir in it.
func (pl *pipeline) ensureWorktree(st *stackState) (string, error) {
	root, err := gitIn(pl.target, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("target is not in a git repo: %s", root)
	}
	if st.Base == "" {
		b, err := gitIn(pl.target, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil || b == "HEAD" {
			return "", fmt.Errorf("the target's checkout must be on a branch to base the stack on")
		}
		st.Base = b
	}
	if st.Module == "" {
		real, _ := filepath.EvalSymlinks(pl.target)
		rr, _ := filepath.EvalSymlinks(root)
		m, err := filepath.Rel(rr, real)
		if err != nil {
			return "", err
		}
		st.Module = filepath.ToSlash(m)
	}
	if st.Worktree == "" {
		st.Worktree = filepath.Join(stateDir(), "pipeline", "worktrees", st.Plan+"-"+filepath.Base(pipelineLockPath(pl.target))[5:])
	}
	if _, err := os.Stat(filepath.Join(st.Worktree, ".git")); err != nil {
		_ = os.MkdirAll(filepath.Dir(st.Worktree), 0o755)
		if out, err := gitIn(root, "worktree", "add", "--detach", st.Worktree, st.tip()); err != nil {
			return "", fmt.Errorf("git worktree add: %s", out)
		}
	}
	co := []string{"checkout", "-q", st.tip()}
	if len(st.Layers) == 0 {
		co = []string{"checkout", "-q", "--detach", st.Base} // the base branch stays checked out in the main checkout
	}
	if out, err := gitIn(st.Worktree, co...); err != nil {
		return "", fmt.Errorf("checkout %s in the worktree: %s", st.tip(), out)
	}
	return filepath.Join(st.Worktree, filepath.FromSlash(st.Module)), nil
}

// startLayer creates the next branch of the stack on top of the current tip.
func (pl *pipeline) startLayer(st *stackState, story string, layer int, name string) (string, error) {
	n := len(st.Layers) + 1
	slug := slugify(name)
	if len(slug) > 32 {
		slug = strings.Trim(slug[:32], "-")
	}
	branch := fmt.Sprintf("%s-%02d-%s", st.Slug, n, slug)
	if out, err := gitIn(st.Worktree, "checkout", "-q", "-b", branch, st.tip()); err != nil {
		return "", fmt.Errorf("create branch %s: %s", branch, out)
	}
	st.Layers = append(st.Layers, stackLayer{N: n, Branch: branch, Story: story, Layer: layer, Name: name})
	return branch, pl.saveStack(*st)
}

// renderContractMD is the readable Story Contract: the story plan's **Spec:** and the source of its scenario ids.
// Scenario S<n> is example n of the contract (the numbering amigos' layer_plan uses).
func renderContractMD(title, storyRel string, v storyVerdict, storyText string) string {
	k := v.Contract
	var b strings.Builder
	flag := "none"
	if k.Flag != "" {
		flag = k.Flag
	}
	fmt.Fprintf(&b, "# Story Contract: %s\n\n- **Story:** %s · **Verdict:** %s (%s) · **Meeting:** %s · **Flag:** %s\n\n",
		title, storyRel, v.Verdict, strings.Join(k.ReadyRoles, ", "), v.MeetingID, flag)
	if s, ok := specSection(storyText, "## Story"); ok {
		fmt.Fprintf(&b, "## Story\n\n%s\n\n", strings.TrimSpace(s))
	}
	b.WriteString("## Rules\n\n")
	for i, r := range k.Rules {
		fmt.Fprintf(&b, "- R%d %s\n", i+1, r)
	}
	b.WriteString("\n## Scenarios\n\nEach scenario is one example from the Story Contract; S<n> is example n. Tests carry the id in their name.\n\n")
	for i, e := range k.Examples {
		rule := ""
		for j, r := range k.Rules {
			if r == e.Rule {
				rule = fmt.Sprintf(" (R%d)", j+1)
			}
		}
		fmt.Fprintf(&b, "- **S%d**%s %s\n", i+1, rule, strings.Join(strings.Fields(e.Text), " "))
	}
	b.WriteString("\n## Layer plan\n\n| Layer | Name | Scenarios | Test level |\n| --- | --- | --- | --- |\n")
	for _, l := range k.LayerPlan {
		var ss []string
		for _, s := range l.Scenarios {
			ss = append(ss, fmt.Sprintf("S%d", s))
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", l.N, l.Name, strings.Join(ss, ", "), l.TestLevel)
	}
	if len(k.Gates) > 0 {
		b.WriteString("\n## Gates\n\n")
		for _, g := range k.Gates {
			fmt.Fprintf(&b, "- %s: %s\n", g.Name, g.State)
		}
	}
	var resolved []string
	for _, q := range k.Questions {
		if q.State == QuestionResolved && len(q.Answers) > 0 {
			resolved = append(resolved, fmt.Sprintf("- %s → %s", q.Text, strings.Join(strings.Fields(q.Answers[0]), " ")))
		}
	}
	if len(resolved) > 0 {
		fmt.Fprintf(&b, "\n## Resolved questions\n\n%s\n", strings.Join(resolved, "\n"))
	}
	for _, h := range []string{"## Global Constraints", "## Contracts (from the feature plan)", "## Answers from the requester"} {
		if s, ok := specSection(storyText, h); ok {
			fmt.Fprintf(&b, "\n%s\n\n%s\n", h, strings.TrimSpace(s))
		}
	}
	return b.String()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func stageImplement(pl *pipeline, c *card) (string, cardNote, error) {
	storyRel := c.Artifacts["story"]
	v, err := loadStoryVerdict(pl.abs(storyRel))
	if err != nil || v.Verdict != VerdictReady {
		return "", cardNote{}, fmt.Errorf("refusing to start: the story's saved amigos verdict is %q, not ready (%v)", v.Verdict, err)
	}
	planCard, err := pl.board.load(c.Parent)
	if err != nil {
		return "", cardNote{}, err
	}
	st, err := pl.loadStack(planCard.ID)
	if err != nil {
		return "", cardNote{}, err
	}
	if st.Slug == "" {
		st.Slug = pl.featureSlug(planCard)
	}
	mod, err := pl.ensureWorktree(&st)
	if err != nil {
		return "", cardNote{}, err
	}
	if err := pl.saveStack(st); err != nil {
		return "", cardNote{}, err
	}
	c.setArtifact("stack", pl.stackRel(planCard.ID))

	// the readable contract: on the pipeline branch, and in the worktree so plan lint can check excerpts against it
	storyText, _ := os.ReadFile(pl.abs(storyRel))
	contractMD := strings.TrimSuffix(storyRel, ".md") + ".contract.md"
	if err := os.WriteFile(pl.abs(contractMD), []byte(renderContractMD(c.Title, storyRel, v, string(storyText))), 0o644); err != nil {
		return "", cardNote{}, err
	}
	c.setArtifact("contract-md", contractMD)
	if err := copyFile(pl.abs(contractMD), filepath.Join(mod, contractMD)); err != nil {
		return "", cardNote{}, err
	}

	// 1. architect writes the story plan
	planRel := strings.TrimSuffix(storyRel, ".md") + ".plan.md"
	if err := pl.writeStoryPlan(c, mod, planRel, contractMD); err != nil {
		return "", cardNote{}, fmt.Errorf("story plan: %w", err)
	}
	if err := copyFile(filepath.Join(mod, planRel), pl.abs(planRel)); err != nil {
		return "", cardNote{}, err
	}
	c.setArtifact("story-plan", planRel)
	pl.sync(fmt.Sprintf("pipeline: %s story plan written", c.ID))

	// 2. tasks, test-first, layer by layer
	data, _ := os.ReadFile(filepath.Join(mod, planRel))
	d, err := parsePlan(string(data))
	if err != nil {
		return "", cardNote{}, err
	}
	contractText, _ := os.ReadFile(pl.abs(contractMD))
	ex := &executor{pl: pl, st: &st, mod: mod, d: d, story: c, contract: string(contractText)}
	if err := ex.run(); err != nil {
		_ = pl.saveStack(st)
		return "", cardNote{}, err
	}
	// 3. the TDD gate over everything this story committed
	res := tddCheck(d, mod, c.ID)
	if !res.ok() {
		return "", cardNote{}, fmt.Errorf("tdd-check failed: %s", strings.Join(res.problems(), "; "))
	}
	var branches []string
	for _, l := range st.Layers {
		if l.Story == c.ID {
			branches = append(branches, l.Branch)
		}
	}
	return "architect-review", cardNote{By: "bot-coder",
		Done:      fmt.Sprintf("Implemented %d tasks test-first in %d layer(s): %s.", len(d.Tasks), len(branches), strings.Join(branches, ", ")),
		Artifacts: []string{planRel, contractMD, pl.stackRel(planCard.ID)},
		Why:       "Every task was seen red (qa's tests failing on their assertions) before green (coder), test files untouched since red; `plan tdd-check` passes; gofmt, go vet and go test ./... are green on every layer. " + ex.summary(),
		Next:      "architect reviews each layer against the Story Contract; coder fixes in the owning layer."}, nil
}

func (pl *pipeline) featureSlug(planCard card) string {
	// the request at the root of the tree names the feature
	c := planCard
	for c.Parent != "" {
		p, err := pl.board.load(c.Parent)
		if err != nil {
			break
		}
		c = p
	}
	s := slugify(c.Title)
	if len(s) > 24 {
		s = strings.Trim(s[:24], "-")
	}
	return s
}

const storyPlanAdaptations = `Pipeline adaptations of the skill (they win over the skill text where they differ):
- The repository root is this Go module; every path in the plan is relative to it. Owned paths like "main.go" or "timer_test.go".
- Header lines: **Spec:** %s (the Story Contract rendered as markdown; quote it for spec excerpts) and **Story Contract:** %s, and **Context:** tools.
- Scenarios are the Story Contract's: S<n> is example n of the contract (listed under "## Scenarios" in the spec file). Every one
  is covered by exactly one task, and its test name carries the id, e.g. TestTimer_S3_LongBreakAfterFourth.
- Put "hot_files":["go.mod","README.md"] in the task graph. Leave out "branch": the pipeline names the stack branches.
- stack_layer is the Story Contract's layer number (its "## Layer plan"); every task gets one.
- Keep it small: one contract task (code packet: shared types, interfaces, and stubbed function variables or stub functions
  other tasks build on), then a few impl tasks as spec packets (qa writes their tests and stubs, coder implements; you write
  no implementation code), and an integration task only if main.go needs wiring. A spec packet must say exactly what each
  test asserts (Given/When/Then with concrete values) and which fakes to use.
- Run ` + "`ai-mode plan lint --fix %s`" + ` and ` + "`ai-mode plan verify %s --task all --repo .`" + ` until both pass, then finish.`

// writeStoryPlan has architect write the drafting-plans story plan inside the worktree module, checked by lint and verify.
func (pl *pipeline) writeStoryPlan(c *card, mod, planRel, contractMD string) error {
	if _, err := os.Stat(filepath.Join(mod, planRel)); err == nil {
		if len(planCheck(mod, planRel, contractMD)) == 0 {
			return nil // a retried stage keeps a plan that already passes
		}
	}
	skill := drafttingPlansSkillBody()
	contract, _ := os.ReadFile(filepath.Join(mod, contractMD))
	task := fmt.Sprintf(`Write the implementation plan for story %s ("%s") to %s, following the drafting-plans skill below in story mode.
It is the only file you may write. Read the module's code first (list_files, read_file): the plan builds on what exists.

%s

<skill name="drafting-plans">
%s
</skill>

The Story Contract (data from the amigos meeting, your spec for this plan):
<contract path=%q>
%s
</contract>`, c.ID, c.Title, planRel, fmt.Sprintf(storyPlanAdaptations, contractMD, strings.TrimSuffix(contractMD, ".md")+".json", planRel, planRel),
		skill, contractMD, strings.TrimSpace(string(contract)))
	cmds := [][]string{{"ai-mode", "plan", "lint"}, {"ai-mode", "plan", "verify"}, {"go", "build"}, {"go", "vet"}}
	_, err := pl.runAgentIn(mod, "architect", "", task, []string{planRel}, nil, cmds, 40, func() []string { return planCheck(mod, planRel, contractMD) })
	return err
}

// planCheck is plan lint (with --fix) plus plan verify --task all, in-process, against the worktree module.
func planCheck(mod, planRel, specRel string) []string {
	path := filepath.Join(mod, planRel)
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{"the plan file " + planRel + " does not exist yet; write it with write_file"}
	}
	spec, _ := os.ReadFile(filepath.Join(mod, specRel))
	if fixed, _ := fixPlan(string(data), string(spec)); fixed != string(data) {
		_ = os.WriteFile(path, []byte(fixed), 0o644)
		data = []byte(fixed)
	}
	d, err := parsePlan(string(data))
	if err != nil {
		return []string{err.Error()}
	}
	d.SpecText = string(spec)
	var out []string
	if d.SpecPath != specRel {
		out = append(out, fmt.Sprintf("the **Spec:** line must name %s (found %q)", specRel, d.SpecPath))
	}
	for _, f := range lintPlanOpts(d, false) {
		if f.Level == "ERR" {
			out = append(out, "plan lint: "+f.Msg)
		}
	}
	if len(d.Tasks) == 0 {
		out = append(out, "the task graph has no tasks")
	}
	for _, t := range d.Tasks {
		if t.StackLayer < 1 {
			out = append(out, fmt.Sprintf("%s has no stack_layer (use the Story Contract's layer number)", t.ID))
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, t := range d.Tasks {
		if r := verifyPlanTask(d, t.ID, mod); r.Status == "failed" {
			for _, s := range r.Steps {
				if !s.OK {
					out = append(out, fmt.Sprintf("plan verify %s failed at %q: %s", t.ID, s.Name, truncateRunes(s.Detail, 1200)))
					break
				}
			}
		}
	}
	return out
}

// drafttingPlansSkillBody is the drafting-plans skill without its frontmatter, chief block and handoff sections.
func drafttingPlansSkillBody() string {
	data, err := os.ReadFile(filepath.Join(filepath.Dir(presetsDir()), "skills", "drafting-plans", "SKILL.md"))
	if err != nil {
		return "(skills/drafting-plans/SKILL.md not found next to the presets)"
	}
	s := string(data)
	if i := strings.Index(s, "## The four rules"); i >= 0 {
		s = s[i:]
	}
	if i := strings.Index(s, "## Handoff"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ---- executor ---------------------------------------------------------------

type executor struct {
	pl       *pipeline
	st       *stackState
	mod      string
	d        *planDoc
	story    *card
	contract string
	log      []string
}

func (ex *executor) summary() string { return strings.Join(ex.log, " ") }

// orderedTasks sorts tasks by stack layer, then wave, then id.
func orderedTasks(d *planDoc) ([]planTask, error) {
	waves, err := planWaves(d.Tasks)
	if err != nil {
		return nil, err
	}
	ts := append([]planTask(nil), d.Tasks...)
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		if a.StackLayer != b.StackLayer {
			return a.StackLayer < b.StackLayer
		}
		if waves[a.ID] != waves[b.ID] {
			return waves[a.ID] < waves[b.ID]
		}
		return a.ID < b.ID
	})
	return ts, nil
}

func (ex *executor) layerName(n int) string {
	var v storyVerdict
	if data, err := os.ReadFile(storyContractPath(ex.pl.abs(ex.story.Artifacts["story"]))); err == nil {
		_ = json.Unmarshal(data, &v)
	}
	for _, l := range v.Contract.LayerPlan {
		if l.N == n && l.Name != "" {
			return l.Name
		}
	}
	return fmt.Sprintf("layer %d", n)
}

func (ex *executor) run() error {
	tasks, err := orderedTasks(ex.d)
	if err != nil {
		return err
	}
	done := ex.donePhases()
	// a retry starts from the last commit: drop half-done edits to tracked files in the module
	if out, err := gitIn(ex.mod, "checkout", "--", "."); err != nil {
		ex.pl.logf("  reset module: %s", out)
	}
	cur := -1
	for _, t := range tasks {
		if t.StackLayer != cur {
			cur = t.StackLayer
			if l := ex.layerFor(cur); l != nil {
				if out, err := gitIn(ex.st.Worktree, "checkout", "-q", l.Branch); err != nil {
					return fmt.Errorf("checkout %s: %s", l.Branch, out)
				}
			} else if _, err := ex.pl.startLayer(ex.st, ex.story.ID, cur, ex.layerName(cur)); err != nil {
				return err
			}
		}
		if done[t.ID] == "green" || done[t.ID] == "contract" {
			continue
		}
		ex.pl.logf("  %s %s (%s, %s packet, layer %d)", t.ID, t.Title, t.Kind, t.Packet, t.StackLayer)
		if err := ex.runTask(t, done[t.ID] == "red"); err != nil {
			return fmt.Errorf("%s: %w", t.ID, err)
		}
		l := ex.layerFor(cur)
		l.Tasks = append(l.Tasks, t.ID)
		if err := ex.pl.saveStack(*ex.st); err != nil {
			return err
		}
		if ex.pl.push {
			if out, err := gitIn(ex.st.Worktree, "push", "-q", "-u", "origin", l.Branch); err != nil {
				ex.pl.logf("  push %s: %s", l.Branch, out)
			}
		}
	}
	return nil
}

func (ex *executor) layerFor(n int) *stackLayer {
	for i := range ex.st.Layers {
		if ex.st.Layers[i].Story == ex.story.ID && ex.st.Layers[i].Layer == n {
			return &ex.st.Layers[i]
		}
	}
	return nil
}

// donePhases reads the Task/Phase trailers already on the stack, so a retried stage resumes: "green" or "contract"
// means the task is done, "red" means coder still has to make it pass.
func (ex *executor) donePhases() map[string]string {
	out := map[string]string{}
	log, err := gitIn(ex.st.Worktree, "log", "--format=%B%x00", ex.st.Base+".."+ex.st.tip())
	if err != nil {
		return out
	}
	for _, msg := range strings.Split(log, "\x00") {
		if !strings.Contains(msg, "Story: "+ex.story.ID) {
			continue
		}
		m := reTaskTrailer.FindStringSubmatch(msg)
		ph := rePhaseTrailer.FindStringSubmatch(msg)
		if m == nil || ph == nil {
			continue
		}
		if out[m[1]] == "" || ph[1] != "red" {
			out[m[1]] = ph[1]
		}
	}
	return out
}

var rePhaseTrailer = regexp.MustCompile(`(?m)^Phase: (\S+)`)

var reTaskTrailer = regexp.MustCompile(`(?m)^Task: (\S+)`)

func (ex *executor) commit(t planTask, phase, subject string) error {
	var paths []string
	for _, p := range t.Owns {
		if _, err := os.Stat(filepath.Join(ex.mod, p)); err == nil {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return fmt.Errorf("nothing to commit: none of %v exists", t.Owns)
	}
	if out, err := gitIn(ex.mod, append([]string{"add", "--"}, paths...)...); err != nil {
		return fmt.Errorf("git add: %s", out)
	}
	msg := fmt.Sprintf("%s\n\nTask: %s\nPhase: %s\nStory: %s\n\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>", subject, t.ID, phase, ex.story.ID)
	if out, err := gitIn(ex.mod, "commit", "-q", "-m", msg); err != nil {
		if strings.Contains(out, "nothing to commit") || strings.Contains(out, "nothing added") {
			return fmt.Errorf("%s phase left no change to commit", phase)
		}
		return fmt.Errorf("git commit: %s", out)
	}
	return nil
}

// scenarioTests are the test names the coverage table promises for a task.
func scenarioTests(d *planDoc, taskID string) []string {
	var out []string
	for _, c := range d.Coverage {
		if c.Task == taskID {
			out = append(out, strings.Trim(c.Test, "` "))
		}
	}
	return out
}

func testFilter(names []string) string {
	return "^(" + strings.Join(names, "|") + ")$"
}

func runIn(dir string, argv ...string) (string, int) {
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return err.Error(), -1
	}
	return string(out), code
}

var reBuildFailure = regexp.MustCompile(`\[build failed\]|\[setup failed\]|undefined: |cannot use |declared and not used|imported and not used|syntax error`)

// redCheck: the tests compile, every named test exists, and every one fails on an assertion.
func redCheck(mod string, names []string) ([]string, string) {
	var out []string
	if o, code := runIn(mod, "go", "vet", "./..."); code != 0 {
		return []string{"the tests or stubs do not compile (go vet ./... failed): " + tail(o)}, o
	}
	for _, n := range names {
		if !testDefined(mod, n) {
			out = append(out, fmt.Sprintf("test %s is not defined (write exactly `func %s(t *testing.T)` in a _test.go file you own)", n, n))
		}
	}
	if len(out) > 0 {
		return out, ""
	}
	o, code := runIn(mod, "go", "test", "./...", "-count=1", "-run", testFilter(names))
	if reBuildFailure.MatchString(o) {
		return []string{"the run failed to build, not on an assertion: " + tail(o)}, o
	}
	if code == 0 {
		return []string{"every named test passed against the stubs: the stubs implement behaviour or the tests assert too little; make stubs return zero values or a not-implemented error and assert the scenario's concrete values"}, o
	}
	for _, n := range names {
		if !strings.Contains(o, "--- FAIL: "+n) {
			out = append(out, fmt.Sprintf("%s did not fail against the stubs (it passes or did not run); it must fail on its assertion until coder implements the behaviour", n))
		}
	}
	return out, o
}

func testDefined(mod, name string) bool {
	re := regexp.MustCompile(`func\s+` + regexp.QuoteMeta(name) + `\s*\(\s*\w+\s+\*testing\.T\s*\)`)
	found := false
	_ = filepath.WalkDir(mod, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if strings.HasSuffix(p, "_test.go") {
			if data, err := os.ReadFile(p); err == nil && re.Match(data) {
				found = true
			}
		}
		return nil
	})
	return found
}

// greenCheck: gofmt-clean owned Go files, vet, build, the named tests and the whole module green.
func greenCheck(mod string, names, owned []string) []string {
	var out []string
	var gofiles []string
	for _, p := range owned {
		if strings.HasSuffix(p, ".go") {
			if _, err := os.Stat(filepath.Join(mod, p)); err == nil {
				gofiles = append(gofiles, p)
			}
		}
	}
	if len(gofiles) > 0 {
		if o, _ := runIn(mod, append([]string{"gofmt", "-l"}, gofiles...)...); strings.TrimSpace(o) != "" {
			out = append(out, "not gofmt-clean (run gofmt -w on): "+strings.Join(strings.Fields(o), ", "))
		}
	}
	if o, code := runIn(mod, "go", "vet", "./..."); code != 0 {
		return append(out, "go vet ./... failed: "+tail(o))
	}
	if o, code := runIn(mod, "go", "build", "./..."); code != 0 {
		return append(out, "go build ./... failed: "+tail(o))
	}
	if len(names) > 0 {
		if o, code := runIn(mod, "go", "test", "./...", "-count=1", "-run", testFilter(names)); code != 0 {
			return append(out, "the task's tests still fail: "+tail(o))
		}
	}
	if o, code := runIn(mod, "go", "test", "./...", "-count=1"); code != 0 {
		out = append(out, "go test ./... fails: "+tail(o))
	}
	return out
}

func splitTests(owns []string) (tests, impl []string) {
	for _, p := range owns {
		if strings.HasSuffix(p, "_test.go") {
			tests = append(tests, p)
		} else {
			impl = append(impl, p)
		}
	}
	return
}

func applyPhase(mod string, pk *planPacket, phase string) error {
	for _, b := range pk.Blocks {
		if b.File == "" || (phase != "" && b.Phase != phase) {
			continue
		}
		if err := applyPlanBlock(mod, b); err != nil {
			return err
		}
	}
	return nil
}

func (ex *executor) runTask(t planTask, redDone bool) error {
	pk := ex.d.Packets[t.ID]
	if pk == nil {
		return fmt.Errorf("no packet")
	}
	names := scenarioTests(ex.d, t.ID)
	_, impl := splitTests(t.Owns)
	switch {
	case t.Kind == "contract" || (t.Packet == "code" && len(names) == 0):
		if err := applyPhase(ex.mod, pk, ""); err != nil {
			return err
		}
		if f := greenCheck(ex.mod, nil, t.Owns); len(f) > 0 {
			return fmt.Errorf("contract code fails the gates: %s", strings.Join(f, "; "))
		}
		ex.log = append(ex.log, t.ID+" contract applied.")
		return ex.commit(t, "contract", fmt.Sprintf("contract(%s): %s", t.ID, t.Title))
	case t.Packet == "code":
		if err := applyPhase(ex.mod, pk, "A"); err != nil {
			return err
		}
		if f, _ := redCheck(ex.mod, names); len(f) > 0 {
			return fmt.Errorf("phase A is not red: %s", strings.Join(f, "; "))
		}
		if err := ex.commit(t, "red", fmt.Sprintf("test(%s): red — %s", t.ID, t.Title)); err != nil {
			return err
		}
		if err := applyPhase(ex.mod, pk, "B"); err != nil {
			return err
		}
		if f := greenCheck(ex.mod, names, t.Owns); len(f) > 0 {
			return fmt.Errorf("phase B is not green: %s", strings.Join(f, "; "))
		}
		ex.log = append(ex.log, t.ID+" code packet red then green.")
		return ex.commit(t, "green", fmt.Sprintf("feat(%s): %s", t.ID, t.Title))
	}
	if len(names) == 0 {
		// a docs or wiring task with no scenario: coder does it, the gates check it
		if err := ex.coderGreen(t, nil, impl, ""); err != nil {
			return err
		}
		ex.log = append(ex.log, t.ID+" done by coder (no scenario).")
		return ex.commit(t, "green", fmt.Sprintf("feat(%s): %s", t.ID, t.Title))
	}
	var redOut string
	if redDone {
		redOut, _ = runIn(ex.mod, "go", "test", "./...", "-count=1", "-run", testFilter(names))
	} else {
		out, err := ex.qaRed(t, names)
		if err != nil {
			return err
		}
		redOut = out
		if err := ex.commit(t, "red", fmt.Sprintf("test(%s): red — %s", t.ID, t.Title)); err != nil {
			return err
		}
	}
	if err := ex.coderGreen(t, names, impl, redOut); err != nil {
		return err
	}
	tests, _ := splitTests(t.Owns)
	if out, err := gitIn(ex.mod, append([]string{"diff", "--quiet", "HEAD", "--"}, tests...)...); err != nil {
		return fmt.Errorf("coder changed the tests after red: %s", out)
	}
	ex.log = append(ex.log, fmt.Sprintf("%s red (qa) then green (coder), %d scenario test(s).", t.ID, len(names)))
	return ex.commit(t, "green", fmt.Sprintf("feat(%s): %s", t.ID, t.Title))
}

func (ex *executor) taskBrief(t planTask) string {
	pk := ex.d.Packets[t.ID]
	var cov []string
	for _, c := range ex.d.Coverage {
		if c.Task == t.ID {
			cov = append(cov, fmt.Sprintf("- %s `%s`: %s", c.Scenario, strings.Trim(c.Test, "` "), scenarioText(ex.contract, c.Scenario)))
		}
	}
	return fmt.Sprintf("<packet task=%q>\n%s\n</packet>\n\nScenario tests for this task:\n%s\n\n<story_contract>\n%s\n</story_contract>",
		t.ID, strings.TrimSpace(pk.Text), strings.Join(cov, "\n"), strings.TrimSpace(ex.contract))
}

func scenarioText(contract, id string) string {
	for _, l := range strings.Split(contract, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "- **"+id+"**") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "- **"+id+"**"))
		}
	}
	return ""
}

func guideline(level string) string {
	if level == "" {
		level = "unit"
	}
	data, err := os.ReadFile(filepath.Join(agentsDir(), "qa", "guidelines", level+".md"))
	if err != nil {
		data, err = os.ReadFile(filepath.Join(agentsDir(), "qa", "guidelines", "unit.md"))
		if err != nil {
			return ""
		}
	}
	return string(data)
}

func goCmds() [][]string {
	return [][]string{{"go", "test"}, {"go", "vet"}, {"go", "build"}, {"gofmt", "-l"}, {"gofmt", "-d"}, {"gofmt", "-w"}}
}

func (ex *executor) qaRed(t planTask, names []string) (string, error) {
	tests, impl := splitTests(t.Owns)
	task := fmt.Sprintf(`Phase A of task %s, "%s": write the failing tests, test-first. You are qa.
You own: %s. Tests go in %s; minimal stubs go in %s (only what the tests need to compile: zero values or a not-implemented
error, no behaviour), so every test fails on its assertion.
Write exactly these test functions, one per scenario, asserting the scenario's concrete values: %s.
Test layer: %s. Follow the guideline below. Use fakes for time and output as the packet says; no sleeping in tests.
When done, run: go test ./... -count=1 -run '%s'
Every named test must FAIL on an assertion (not a build error). Then call finish.

%s

<guideline layer=%q>
%s
</guideline>`, t.ID, t.Title, strings.Join(t.Owns, ", "), strings.Join(tests, ", "), strings.Join(impl, ", "), strings.Join(names, ", "),
		firstNonEmpty(t.TestLevel, "unit"), testFilter(names), ex.taskBrief(t), firstNonEmpty(t.TestLevel, "unit"), guideline(t.TestLevel))
	var last string
	_, err := ex.pl.runAgentIn(ex.mod, "qa", "", task, t.Owns, nil, goCmds(), 35, func() []string {
		f, o := redCheck(ex.mod, names)
		last = o
		return f
	})
	return last, err
}

func (ex *executor) coderGreen(t planTask, names, impl []string, redOut string) error {
	tests, _ := splitTests(t.Owns)
	readOnly := tests
	owns := impl
	if len(names) == 0 {
		owns, readOnly = t.Owns, nil
	}
	what := "Make the failing tests pass. The test files are read-only: " + strings.Join(tests, ", ") + "."
	if len(names) == 0 {
		what = "Do the task as the packet describes."
	}
	task := fmt.Sprintf(`Phase B of task %s, "%s": implementation. You are coder.
%s You own: %s.
When the task's tests pass, run gofmt -l ., go vet ./... and go test ./... and fix anything they report in your files. Then call finish.

The red run (qa's tests against the stubs):
%s

%s`, t.ID, t.Title, what, strings.Join(owns, ", "), tail(redOut), ex.taskBrief(t))
	_, err := ex.pl.runAgentIn(ex.mod, "coder", "", task, owns, readOnly, goCmds(), 40, func() []string { return greenCheck(ex.mod, names, t.Owns) })
	return err
}

// ---- tdd-check (P8b) ----------------------------------------------------------

type tddTaskResult struct {
	Task     string   `json:"task"`
	Status   string   `json:"status"` // ok | failed | skipped
	Problems []string `json:"problems,omitempty"`
}

type tddResult struct{ Tasks []tddTaskResult }

func (r tddResult) ok() bool {
	for _, t := range r.Tasks {
		if t.Status == "failed" {
			return false
		}
	}
	return true
}

func (r tddResult) problems() []string {
	var out []string
	for _, t := range r.Tasks {
		for _, p := range t.Problems {
			out = append(out, t.Task+": "+p)
		}
	}
	return out
}

// tddCheck, for every task with scenario tests: the tests exist at the tip with their scenario ids, the red commit
// (Task/Phase trailers) fails them in a scratch checkout, the tip passes them, and no later commit touched the task's
// test files. story limits the commits searched to one story ("" for any).
func tddCheck(d *planDoc, mod, story string) tddResult {
	var res tddResult
	log, _ := gitIn(mod, "log", "--format=%H%x01%B%x00")
	commits := map[string]map[string]string{} // task -> phase -> sha (newest wins only if none yet)
	for _, rec := range strings.Split(log, "\x00") {
		parts := strings.SplitN(strings.TrimSpace(rec), "\x01", 2)
		if len(parts) != 2 {
			continue
		}
		sha, msg := parts[0], parts[1]
		if story != "" && !strings.Contains(msg, "Story: "+story) {
			continue
		}
		m := reTaskTrailer.FindStringSubmatch(msg)
		ph := rePhaseTrailer.FindStringSubmatch(msg)
		if m == nil || ph == nil {
			continue
		}
		if commits[m[1]] == nil {
			commits[m[1]] = map[string]string{}
		}
		if _, seen := commits[m[1]][ph[1]]; !seen {
			commits[m[1]][ph[1]] = sha
		}
	}
	for _, t := range d.Tasks {
		names := scenarioTests(d, t.ID)
		r := tddTaskResult{Task: t.ID, Status: "ok"}
		if len(names) == 0 {
			r.Status = "skipped"
			res.Tasks = append(res.Tasks, r)
			continue
		}
		bad := func(f string, a ...any) { r.Status = "failed"; r.Problems = append(r.Problems, fmt.Sprintf(f, a...)) }
		for _, n := range names {
			if !testDefined(mod, n) {
				bad("test %s is missing at the tip", n)
			}
		}
		red, green := commits[t.ID]["red"], commits[t.ID]["green"]
		if red == "" || green == "" {
			bad("no red and green commits with Task: %s trailers", t.ID)
			res.Tasks = append(res.Tasks, r)
			continue
		}
		tests, _ := splitTests(t.Owns)
		// no commit after red may touch the tests, except an architect-requested review fix (Phase: review)
		later, _ := gitIn(mod, append([]string{"log", "--format=%H%x01%B%x00", red + "..HEAD", "--"}, tests...)...)
		for _, rec := range strings.Split(later, "\x00") {
			parts := strings.SplitN(strings.TrimSpace(rec), "\x01", 2)
			if len(parts) != 2 {
				continue
			}
			if ph := rePhaseTrailer.FindStringSubmatch(parts[1]); ph == nil || ph[1] != "review" {
				bad("test files changed after the red commit %s by %s (%s)", red[:8], parts[0][:8], firstLine(parts[1]))
			}
		}
		if o, code := runIn(mod, "go", "test", "./...", "-count=1", "-run", testFilter(names)); code != 0 {
			bad("the tests fail at the tip: %s", tail(o))
		}
		if f := redAt(mod, red, names); f != "" {
			bad("%s", f)
		}
		res.Tasks = append(res.Tasks, r)
	}
	return res
}

// redAt checks out sha in a scratch worktree and requires the named tests to fail there on assertions.
func redAt(mod, sha string, names []string) string {
	root, err := gitIn(mod, "rev-parse", "--show-toplevel")
	if err != nil {
		return "not a git repo"
	}
	prefix, _ := gitIn(mod, "rev-parse", "--show-prefix")
	tmp, err := os.MkdirTemp("", "tdd-check-")
	if err != nil {
		return err.Error()
	}
	defer os.RemoveAll(tmp)
	wt := filepath.Join(tmp, "wt")
	if out, err := gitIn(root, "worktree", "add", "--detach", wt, sha); err != nil {
		return "cannot check out the red commit: " + out
	}
	defer gitIn(root, "worktree", "remove", "--force", wt)
	o, code := runIn(filepath.Join(wt, prefix), "go", "test", "./...", "-count=1", "-run", testFilter(names))
	if code == 0 {
		return fmt.Sprintf("at the red commit %s the tests pass; they were never red", sha[:8])
	}
	if reBuildFailure.MatchString(o) {
		return fmt.Sprintf("at the red commit %s the tests do not build: %s", sha[:8], tail(o))
	}
	return ""
}

// ---- commands -----------------------------------------------------------------

// cmdWork is `ai-mode work start <story card>`: stage 8 for one story, refusing anything not ready.
func cmdWork(args []string) int {
	if len(args) == 0 || args[0] != "start" {
		return fail("usage: ai-mode work start <story-card> [--repo DIR] [--push]")
	}
	fs := newFlags("work start")
	repo := fs.String("repo", ".", "Target Go module the pipeline works on")
	push := fs.Bool("push", false, "Push the pipeline branch and layer branches as they are committed")
	pos, code, ok := parse(fs, args[1:])
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return fail("usage: ai-mode work start <story-card> [--repo DIR] [--push]")
	}
	pl, err := newPipeline(*repo, true, *push)
	if err != nil {
		return fail("%v", err)
	}
	c, err := pl.board.load(pos[0])
	if err != nil {
		return fail("%v", err)
	}
	if c.Type != "story" || c.Stage != "ready" {
		return fail("refusing: %s is a %s at %s; work start takes a story whose amigos verdict is ready", c.ID, c.Type, c.Stage)
	}
	release, err := acquireLock(pipelineLockPath(pl.target))
	if err != nil {
		return fail("%v", err)
	}
	defer release()
	if code := pl.ensureProfile(); code != 0 {
		return code
	}
	if err := pl.runStage(&c); err != nil {
		fmt.Fprintf(os.Stderr, "%s blocked: %v\n", c.ID, err)
		return 2
	}
	return 0
}

// cmdPlanTDDCheck is `ai-mode plan tdd-check <plan.md> [--repo DIR] [--story CARD] [--json]` (P8b).
func cmdPlanTDDCheck(args []string) int {
	fs := newFlags("plan tdd-check")
	repo := fs.String("repo", ".", "The module whose git history holds the red and green commits")
	story := fs.String("story", "", "Only commits with this Story: trailer")
	asJSON := fs.Bool("json", false, "Print the result as JSON")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return fail("usage: ai-mode plan tdd-check <plan.md> [--repo DIR] [--story CARD] [--json]")
	}
	data, err := os.ReadFile(pos[0])
	if err != nil {
		return fail("%v", err)
	}
	d, err := parsePlan(string(data))
	if err != nil {
		return fail("%v", err)
	}
	res := tddCheck(d, *repo, *story)
	if *asJSON {
		b, _ := json.MarshalIndent(map[string]any{"ok": res.ok(), "tasks": res.Tasks}, "", "  ")
		fmt.Println(string(b))
	} else {
		for _, t := range res.Tasks {
			fmt.Printf("%s: %s\n", t.Task, t.Status)
			for _, p := range t.Problems {
				fmt.Printf("  %s\n", p)
			}
		}
	}
	if !res.ok() {
		return 2
	}
	return 0
}
