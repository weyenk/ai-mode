package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// In the dev pipeline architect writes the story plan as JSON and `ai-mode plan render` turns it into the
// drafting-plans markdown that plan lint, plan verify, plan apply and tdd-check read. A local model got the markdown's
// format details wrong for a dozen lint rounds (backtick-quoted excerpts, a truncated packet, edits that no longer
// matched); rendered, the format is right by construction and the lint findings are about substance.

type storyScenarioJSON struct {
	ID    string `json:"id"`
	Test  string `json:"test"`
	Check string `json:"check"` // what the test asserts: Given/When/Then with concrete values
}

type storyFileJSON struct {
	Path  string `json:"path"`
	Phase string `json:"phase"` // A (tests and stubs, or a contract's code) | B (implementation)
	Code  string `json:"code"`
}

type storyTaskJSON struct {
	ID          string              `json:"id"`
	Title       string              `json:"title"`
	Kind        string              `json:"kind"`   // contract | impl | integration | docs
	Packet      string              `json:"packet"` // spec (qa and coder write the code) | code (files below)
	DependsOn   []string            `json:"depends_on"`
	Owns        []string            `json:"owns"`
	Produces    []string            `json:"produces"`
	Consumes    []string            `json:"consumes"`
	StackLayer  int                 `json:"stack_layer"`
	TestLevel   string              `json:"test_level"`
	Scenarios   []storyScenarioJSON `json:"scenarios"`
	SpecExcerpt []string            `json:"spec_excerpt"`
	Notes       string              `json:"notes"`
	Files       []storyFileJSON     `json:"files"`
	Run         string              `json:"run"`
}

type boundaryJSON struct {
	Boundary      string `json:"boundary"`
	Artifact      string `json:"artifact"`
	Fixtures      string `json:"fixtures"`
	Fake          string `json:"fake"`
	ProviderCheck string `json:"provider_check"`
	Owner         string `json:"owner"`
}

type storyPlanJSON struct {
	Goal         string          `json:"goal"`
	Architecture string          `json:"architecture"`
	ReviewFocus  []string        `json:"review_focus"`
	Boundaries   []boundaryJSON  `json:"boundaries"`
	Tasks        []storyTaskJSON `json:"tasks"`
}

const storyPlanJSONFields = `top level: goal, architecture, review_focus, boundaries, tasks; a task: id, title, kind, packet, depends_on, owns, produces, consumes, stack_layer, test_level, scenarios [{id, test, check}], spec_excerpt, notes, files [{path, phase, code}], run`

func parseStoryPlanJSON(data []byte) (*storyPlanJSON, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var pj storyPlanJSON
	if err := dec.Decode(&pj); err != nil {
		return nil, fmt.Errorf("the plan JSON does not match the schema: %v (fields: %s)", err, storyPlanJSONFields)
	}
	for i := range pj.Tasks {
		t := &pj.Tasks[i]
		if t.Kind == "" {
			t.Kind = "impl"
		}
		if t.Packet == "" {
			t.Packet = "spec"
			if len(t.Files) > 0 || t.Kind == "contract" {
				t.Packet = "code"
			}
		}
	}
	return &pj, nil
}

var reContractScenario = regexp.MustCompile(`(?m)^- \*\*(S\d+)\*\*(?: \((R\d+)\))?`)

// contractScenarios maps each scenario id of a rendered Story Contract to its rule id ("" when it has none), in order.
func contractScenarios(contractMD string) ([]string, map[string]string) {
	var ids []string
	rule := map[string]string{}
	for _, m := range reContractScenario.FindAllStringSubmatch(contractMD, -1) {
		ids = append(ids, m[1])
		rule[m[1]] = m[2]
	}
	return ids, rule
}

func scenarioRunCmd(tests []string) string {
	if len(tests) == 0 {
		return "go vet ./..."
	}
	return "go test ./... -count=1 -run '" + testFilter(tests) + "'"
}

func uniq(xs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func renderStoryPlan(pj *storyPlanJSON, title, specRel, contractJSONRel, contractMD string) string {
	_, rules := contractScenarios(contractMD)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — story plan\n\n> **For agentic workers:** execute wave by wave; qa writes tests, coder implements.\n\n", title)
	fmt.Fprintf(&b, "**Goal:** %s\n**Architecture:** %s\n**Tech Stack:** Go, standard library only\n", oneLine(pj.Goal), oneLine(pj.Architecture))
	fmt.Fprintf(&b, "**Spec:** %s    **Story Contract:** %s\n**Context:** tools\n", specRel, contractJSONRel)
	b.WriteString("**Lint:** `ai-mode plan lint` exit 0 · **Verify:** `ai-mode plan verify --task all` exit 0\n\n")
	b.WriteString("_Rendered by `ai-mode plan render` from the plan JSON next to this file; change the JSON, not this file._\n\n")

	b.WriteString("## Global Constraints\n")
	if gc, ok := specSection(contractMD, "## Global Constraints"); ok && strings.TrimSpace(gc) != "" {
		b.WriteString(strings.TrimSpace(gc) + "\n")
	} else {
		b.WriteString("- Go standard library only.\n")
	}
	b.WriteString("\n## Review Focus\n")
	for i, r := range pj.ReviewFocus {
		fmt.Fprintf(&b, "%d. %s\n", i+1, oneLine(r))
	}
	b.WriteString("\n## Boundaries\n| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, x := range pj.Boundaries {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", mdCell(x.Boundary), mdCell(x.Artifact), mdCell(x.Fixtures), mdCell(x.Fake), mdCell(x.ProviderCheck), mdCell(x.Owner))
	}

	var tasks []planTask
	for _, t := range pj.Tasks {
		var covers []string
		for _, s := range t.Scenarios {
			covers = append(covers, s.ID)
		}
		tasks = append(tasks, planTask{ID: t.ID, Title: t.Title, Kind: t.Kind, Packet: t.Packet, DependsOn: nonNil(t.DependsOn), Owns: nonNil(t.Owns),
			Produces: nonNil(t.Produces), Consumes: nonNil(t.Consumes), Covers: nonNil(uniq(covers)), TestLevel: t.TestLevel, StackLayer: t.StackLayer})
	}
	graph, _ := json.MarshalIndent(map[string]any{"tasks": tasks, "hot_files": []string{"go.mod", "README.md"}}, "", " ")
	fmt.Fprintf(&b, "\n## Task graph\n\n```json\n%s\n```\n\n**Scenario coverage**\n\n| Scenario | Rule | Task | Test |\n| --- | --- | --- | --- |\n", graph)
	type row struct{ id, rule, task, test string }
	var rows []row
	for _, t := range pj.Tasks {
		for _, s := range t.Scenarios {
			rows = append(rows, row{s.ID, firstNonEmpty(rules[s.ID], "-"), t.ID, s.Test})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return scenarioNum(rows[i].id) < scenarioNum(rows[j].id) })
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | `%s` |\n", r.id, r.rule, r.task, r.test)
	}

	b.WriteString("\n## Task packets\n")
	for _, t := range pj.Tasks {
		var tests, covers []string
		for _, s := range t.Scenarios {
			tests = append(tests, s.Test)
			covers = append(covers, s.ID)
		}
		tests = uniq(tests)
		fmt.Fprintf(&b, "\n### %s: %s\n\n", t.ID, oneLine(t.Title))
		fmt.Fprintf(&b, "**Kind / packet / stack layer / test level / depends on:** %s / %s / %d / %s / %s\n", t.Kind, t.Packet, t.StackLayer,
			firstNonEmpty(t.TestLevel, "unit"), firstNonEmpty(strings.Join(t.DependsOn, ", "), "none"))
		fmt.Fprintf(&b, "**Owns (exclusive):** %s\n", backticks(t.Owns))
		fmt.Fprintf(&b, "**Covers:** %s\n", firstNonEmpty(strings.Join(uniq(covers), ", "), "none"))
		var quotes []string
		for _, q := range t.SpecExcerpt {
			if q = strings.Trim(oneLine(q), "\"` "); q != "" {
				quotes = append(quotes, `"`+q+`"`)
			}
		}
		fmt.Fprintf(&b, "**Spec excerpt:** %s\n\n", firstNonEmpty(strings.Join(quotes, " and "), "none"))
		fmt.Fprintf(&b, "**Interfaces**\n- Consumes: %s\n- Produces: %s\n\n", firstNonEmpty(strings.Join(t.Consumes, "; "), "nothing"), firstNonEmpty(strings.Join(t.Produces, "; "), "nothing"))
		if n := strings.TrimSpace(t.Notes); n != "" {
			b.WriteString(n + "\n\n")
		}
		if len(t.Scenarios) > 0 {
			b.WriteString("**Scenarios (write these tests first):**\n")
			for _, s := range t.Scenarios {
				fmt.Fprintf(&b, "- %s `%s`: %s\n", s.ID, s.Test, oneLine(s.Check))
			}
			b.WriteString("\n")
		}
		for _, phase := range []string{"A", "B"} {
			var blocks []storyFileJSON
			for _, f := range t.Files {
				if strings.ToUpper(firstNonEmpty(f.Phase, "A")) == phase {
					blocks = append(blocks, f)
				}
			}
			if len(blocks) == 0 {
				continue
			}
			if phase == "A" {
				if t.Kind == "contract" {
					b.WriteString("**Phase A: the contract code.**\n\n")
				} else {
					b.WriteString("**Phase A: tests plus stubs (role: qa).**\n\n")
				}
			} else {
				b.WriteString("**Phase B: implementation (role: coder).**\n\n")
			}
			for _, f := range blocks {
				code := f.Code
				if strings.HasSuffix(f.Path, ".go") {
					if formatted, err := format.Source([]byte(code)); err == nil {
						code = string(formatted) // the mechanical repair plan lint --fix makes on markdown plans
					}
				}
				code = strings.TrimRight(code, "\n")
				fmt.Fprintf(&b, "```go file=%s\n%s\n```\n\n", f.Path, code)
			}
		}
		fmt.Fprintf(&b, "Run: `%s`\n", firstNonEmpty(strings.TrimSpace(t.Run), scenarioRunCmd(tests)))
	}
	return b.String()
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func backticks(xs []string) string {
	var out []string
	for _, x := range xs {
		out = append(out, "`"+x+"`")
	}
	return strings.Join(out, ", ")
}

func scenarioNum(id string) int {
	var n int
	fmt.Sscanf(strings.TrimPrefix(id, "S"), "%d", &n)
	return n
}

var reTestName = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)

// maxScenariosPerTask keeps a task small enough for one qa or coder turn to hold in mind.
const maxScenariosPerTask = 6

// storyPlanChecks are the pipeline's rules on top of plan lint: qa's tests and coder's code for a scenario live in one
// task, every Story Contract scenario is covered, and names and paths are usable.
func storyPlanChecks(pj *storyPlanJSON, contractMD string) []string {
	var out []string
	ids, _ := contractScenarios(contractMD)
	known := map[string]bool{}
	for _, id := range ids {
		known[id] = true
	}
	covered := map[string]bool{}
	if len(pj.Tasks) == 0 {
		out = append(out, "the plan has no tasks")
	}
	for _, t := range pj.Tasks {
		if t.ID == "" || t.Title == "" {
			out = append(out, fmt.Sprintf("every task needs an id and a title (got id %q)", t.ID))
		}
		if t.StackLayer < 1 {
			out = append(out, fmt.Sprintf("%s has no stack_layer (use the Story Contract's layer number, from 1)", t.ID))
		}
		for _, p := range t.Owns {
			if filepath.IsAbs(p) || strings.HasPrefix(filepath.Clean(p), "..") {
				out = append(out, fmt.Sprintf("%s owns %q: paths are relative to the module root", t.ID, p))
			}
		}
		if len(t.Scenarios) > 0 {
			tests, code := splitTests(t.Owns)
			var goCode []string
			for _, c := range code {
				if strings.HasSuffix(c, ".go") {
					goCode = append(goCode, c)
				}
			}
			if len(tests) == 0 {
				out = append(out, fmt.Sprintf("%s covers scenarios but owns no _test.go file (qa writes the tests in the task that covers them)", t.ID))
			}
			if len(goCode) == 0 {
				out = append(out, fmt.Sprintf("%s covers scenarios but owns no non-test .go file (qa writes the stubs and coder the code in the same task)", t.ID))
			}
		}
		if n := len(t.Scenarios); n > maxScenariosPerTask {
			out = append(out, fmt.Sprintf("%s covers %d scenarios; at most %d per task (split it: qa and coder slow down sharply on big tasks)", t.ID, n, maxScenariosPerTask))
		}
		for _, s := range t.Scenarios {
			covered[s.ID] = true
			if len(ids) > 0 && !known[s.ID] {
				out = append(out, fmt.Sprintf("%s lists %s, which is not a scenario of the Story Contract (%s)", t.ID, s.ID, strings.Join(ids, ", ")))
			}
			if !reTestName.MatchString(s.Test) || !strings.Contains(s.Test, s.ID) {
				out = append(out, fmt.Sprintf("%s: test name %q must be a Go test name (Test...) containing %s", t.ID, s.Test, s.ID))
			}
			if strings.TrimSpace(s.Check) == "" {
				out = append(out, fmt.Sprintf("%s: scenario %s needs a check (what the test asserts, with values)", t.ID, s.ID))
			}
		}
		if t.Packet == "code" && len(t.Scenarios) > 0 {
			hasTest := false
			for _, f := range t.Files {
				if strings.HasSuffix(f.Path, "_test.go") && strings.ToUpper(firstNonEmpty(f.Phase, "A")) == "A" {
					hasTest = true
				}
			}
			if !hasTest {
				out = append(out, fmt.Sprintf("%s is a code packet that covers scenarios but has no phase A _test.go file; make it a spec packet (qa writes the tests, coder the code) or give the tests in phase A", t.ID))
			}
		}
		if t.Packet == "code" && len(t.Files) == 0 {
			out = append(out, fmt.Sprintf("%s is a code packet without files; give files or make it a spec packet", t.ID))
		}
	}
	for _, id := range ids {
		if !covered[id] {
			out = append(out, fmt.Sprintf("%s is not covered by any task", id))
		}
	}
	return out
}

// renderStoryPlanFile renders <x>.plan.json to <x>.plan.md beside it and returns the lint and pipeline findings.
// specRel is the rendered Story Contract, relative to root (the module); excerpts are checked against it.
func renderStoryPlanFile(jsonPath, specRel, root string) (string, []string, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return "", nil, err
	}
	contract, err := os.ReadFile(filepath.Join(root, specRel))
	if err != nil {
		return "", nil, fmt.Errorf("the Story Contract %s: %w", specRel, err)
	}
	mdPath := strings.TrimSuffix(jsonPath, ".json") + ".md"
	pj, err := parseStoryPlanJSON(data)
	if err != nil {
		return mdPath, []string{err.Error()}, nil
	}
	title := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(contract), "\n", 2)[0], "# Story Contract:"))
	md := renderStoryPlan(pj, title, specRel, strings.TrimSuffix(specRel, ".md")+".json", string(contract))
	if err := os.WriteFile(mdPath, []byte(md), 0o644); err != nil {
		return mdPath, nil, err
	}
	findings := storyPlanChecks(pj, string(contract))
	d, err := parsePlan(md)
	if err != nil {
		return mdPath, append(findings, err.Error()), nil
	}
	d.SpecText = string(contract)
	for _, f := range lintPlanOpts(d, false) {
		if f.Level == "ERR" {
			findings = append(findings, "plan lint: "+f.Msg)
		}
	}
	return mdPath, findings, nil
}

// cmdPlanRender is `ai-mode plan render <x.plan.json> [--spec CONTRACT.md] [--root DIR]`.
func cmdPlanRender(args []string) int {
	fs := newFlags("plan render")
	spec := fs.String("spec", "", "The rendered Story Contract, relative to --root (default: <x>.contract.md next to the JSON)")
	root := fs.String("root", ".", "Module root the plan's paths are relative to")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 || !strings.HasSuffix(pos[0], ".plan.json") {
		return fail("usage: ai-mode plan render <x.plan.json> [--spec CONTRACT.md] [--root DIR]")
	}
	rootAbs, _ := filepath.Abs(*root)
	specRel := *spec
	if specRel == "" {
		abs, _ := filepath.Abs(strings.TrimSuffix(pos[0], ".plan.json") + ".contract.md")
		r, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			return fail("%v", err)
		}
		specRel = filepath.ToSlash(r)
	}
	md, findings, err := renderStoryPlanFile(pos[0], specRel, rootAbs)
	if err != nil {
		return fail("%v", err)
	}
	fmt.Println("rendered: " + md)
	for _, f := range findings {
		fmt.Println("ERR: " + f)
	}
	if len(findings) > 0 {
		fmt.Println("RESULT: FAIL")
		return 2
	}
	fmt.Println("RESULT: PASS")
	return 0
}
