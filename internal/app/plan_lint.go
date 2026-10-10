package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var defaultHotFiles = []string{"internal/app/cli.go", "completions/_ai-mode", "README.md", "AGENTS.md", "go.mod"}

var (
	rePlaceholderWord   = regexp.MustCompile(`\b(TBD|TODO)\b`)
	rePlaceholderPhrase = regexp.MustCompile(`(?i)implement later|add appropriate|similar to task|write tests for the above`)
	reIdent             = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	validKinds          = map[string]bool{"contract": true, "impl": true, "integration": true, "docs": true}
)

// lintPlan checks a parsed plan against the drafting-plans rules. ERR findings make the plan unfit to hand off.
func lintPlan(d *planDoc) []planFinding { return lintPlanOpts(d, false) }

// lintPlanOpts is lintPlan; skeleton skips the per-packet checks, for the stage where the graph exists but packets do not.
func lintPlanOpts(d *planDoc, skeleton bool) []planFinding {
	var out []planFinding
	errf := func(f string, a ...any) { out = append(out, planFinding{"ERR", fmt.Sprintf(f, a...)}) }
	warnf := func(f string, a ...any) { out = append(out, planFinding{"WARN", fmt.Sprintf(f, a...)}) }

	for _, h := range []string{"## Global Constraints", "## Review Focus", "## Boundaries", "## Task graph"} {
		if !d.Sections[h] {
			errf("missing section %s", h)
		}
	}
	switch d.Context {
	case "tools", "digest", "spec only":
	default:
		errf("Context line must be one of tools | digest | spec only, got %q", d.Context)
	}

	byID := map[string]planTask{}
	for _, t := range d.Tasks {
		if _, dup := byID[t.ID]; dup {
			errf("duplicate task id %s", t.ID)
		}
		byID[t.ID] = t
	}
	for _, t := range d.Tasks {
		if !validKinds[t.Kind] {
			errf("%s has kind %q; want contract | impl | integration | docs", t.ID, t.Kind)
		}
		if t.Packet != "code" && t.Packet != "spec" {
			errf("%s has packet %q; want code | spec", t.ID, t.Packet)
		}
		for _, dep := range t.DependsOn {
			dt, ok := byID[dep]
			if !ok {
				errf("%s depends on unknown task %s", t.ID, dep)
				continue
			}
			if (t.Kind == "impl" || t.Kind == "docs") && dt.Kind != "contract" {
				errf("%s (%s) depends on %s (%s): implementation tasks may depend only on contract tasks; put an interface in a contract task and inject it, so tasks never rely on each other's implementation", t.ID, t.Kind, dep, dt.Kind)
			}
		}
		if t.Kind == "contract" && t.Packet != "code" {
			errf("contract task %s must be a code packet (its code is verified, everything else builds on it)", t.ID)
		}
	}
	if _, err := planWaves(d.Tasks); err != nil {
		errf("%v", err)
		return out // ancestry below needs an acyclic graph
	}

	// ownership: tasks that can run in parallel (neither is the other's ancestor) must not overlap
	anc := ancestors(d.Tasks)
	for i, a := range d.Tasks {
		for _, b := range d.Tasks[i+1:] {
			if anc[a.ID][b.ID] || anc[b.ID][a.ID] {
				continue
			}
			for _, pa := range a.Owns {
				for _, pb := range b.Owns {
					if pathsOverlap(pa, pb) {
						errf("%s and %s can run in parallel but both own %s / %s", a.ID, b.ID, pa, pb)
					}
				}
			}
		}
	}
	hot := d.HotFiles
	if len(hot) == 0 {
		hot = defaultHotFiles
	}
	owners := map[string][]string{}
	for _, t := range d.Tasks {
		for _, p := range t.Owns {
			for _, h := range hot {
				if pathsOverlap(p, h) {
					owners[h] = append(owners[h], t.ID)
					if t.Kind != "integration" {
						errf("%s owns hot file %s but is not an integration task", t.ID, h)
					}
				}
			}
		}
	}
	for h, ids := range owners {
		if len(ids) > 1 {
			errf("hot file %s is owned by several tasks %v; exactly one integration task may own it", h, ids)
		}
	}
	seenBranch := map[string]string{}
	for _, t := range d.Tasks {
		if t.Branch == "" {
			continue
		}
		if o, ok := seenBranch[t.Branch]; ok {
			errf("branch %s is used by %s and %s", t.Branch, o, t.ID)
		}
		seenBranch[t.Branch] = t.ID
	}

	// scenario coverage: every scenario exactly one task, and test names carry the scenario id
	coverers := map[string][]string{}
	for _, t := range d.Tasks {
		for _, s := range t.Covers {
			coverers[s] = append(coverers[s], t.ID)
		}
	}
	tableScen := map[string]bool{}
	for _, c := range d.Coverage {
		tableScen[c.Scenario] = true
		if _, ok := byID[c.Task]; !ok {
			errf("scenario %s: coverage table names unknown task %s", c.Scenario, c.Task)
		}
		if !strings.Contains(c.Test, c.Scenario) {
			errf("test name %s does not contain its scenario id %s", c.Test, c.Scenario)
		}
		cs := coverers[c.Scenario]
		switch {
		case len(cs) == 0:
			errf("scenario %s is covered by no task (table says %s)", c.Scenario, c.Task)
		case len(cs) > 1:
			errf("scenario %s is covered by several tasks %v; exactly one task may cover it", c.Scenario, cs)
		case cs[0] != c.Task:
			errf("scenario %s: table says %s but task %s covers it", c.Scenario, c.Task, cs[0])
		}
	}
	var stray []string
	for s := range coverers {
		if !tableScen[s] {
			stray = append(stray, s)
		}
	}
	sort.Strings(stray)
	for _, s := range stray {
		errf("task %v covers %s which is not in the scenario coverage table", coverers[s], s)
	}

	// packets
	for _, t := range d.Tasks {
		if skeleton {
			break
		}
		pk := d.Packets[t.ID]
		if pk == nil {
			errf("task %s has no packet (expected a \"### %s: ...\" section)", t.ID, t.ID)
			continue
		}
		files := 0
		for _, b := range pk.Blocks {
			if b.File == "" {
				continue
			}
			files++
			owned := false
			for _, p := range t.Owns {
				if pathsOverlap(p, b.File) {
					owned = true
				}
			}
			if !owned {
				errf("%s writes %s which it does not own (not owned: owns %v)", t.ID, b.File, t.Owns)
			}
		}
		if t.Packet == "code" && files == 0 {
			errf("code packet %s has no file blocks (use ```go file=PATH ...)", t.ID)
		}
		if t.Packet == "code" && t.Kind != "contract" && pk.RunCmd == "" {
			errf("code packet %s has no \"Run: `command`\" line", t.ID)
		}
	}

	// spec excerpts must be verbatim quotes from the spec file
	if !skeleton && d.SpecText != "" {
		spec := normSpace(d.SpecText)
		for _, t := range d.Tasks {
			pk := d.Packets[t.ID]
			if pk == nil || pk.Excerpt == "" || strings.EqualFold(strings.Trim(pk.Excerpt, " ."), "none") {
				continue
			}
			frags, ok := excerptFragments(pk.Excerpt)
			if !ok {
				errf("%s: the spec excerpt must be quoted fragments joined by \" and \" (or none); put commentary on a separate line", t.ID)
				continue
			}
			for _, f := range frags {
				for _, piece := range regexp.MustCompile(`\.\.\.|…`).Split(f, -1) {
					if p := normSpace(piece); len(p) >= 8 && !strings.Contains(spec, p) {
						errf("%s: spec excerpt not found in the spec: %q", t.ID, p)
					}
				}
			}
		}
	}

	// the packet must use the test name the coverage table promises, so the story's intent stays traceable
	for _, c := range d.Coverage {
		if pk := d.Packets[c.Task]; pk != nil && !skeleton && !strings.Contains(pk.Text, c.Test) {
			errf("scenario %s: packet %s never mentions its test name %s from the coverage table", c.Scenario, c.Task, c.Test)
		}
	}

	// consumes should be produced by an ancestor (names only; paths into the repo are skipped)
	for _, t := range d.Tasks {
		var prod strings.Builder
		for a := range anc[t.ID] {
			prod.WriteString(strings.Join(byID[a].Produces, " ") + " ")
		}
		for _, c := range t.Consumes {
			if strings.Contains(c, "/") || strings.Contains(c, ".go") {
				continue
			}
			if id := reIdent.FindString(strings.TrimPrefix(strings.TrimPrefix(c, "func "), "type ")); id != "" && !strings.Contains(prod.String(), id) {
				warnf("%s consumes %q but no ancestor task produces it", t.ID, c)
			}
		}
	}

	if !skeleton && (rePlaceholderWord.MatchString(planText(d)) || rePlaceholderPhrase.MatchString(planText(d))) {
		errf("placeholder pattern found (TBD/TODO/implement later/add appropriate/similar to task/write tests for the above)")
	}
	return out
}

// excerptFragments splits `"a" and "b"` into [a b]; ok is false when the line is not in that form.
func excerptFragments(ex string) ([]string, bool) {
	s := strings.TrimSpace(ex)
	if !strings.HasPrefix(s, `"`) || !strings.HasSuffix(s, `"`) {
		return nil, false
	}
	parts := strings.Split(s[1:len(s)-1], `" and "`)
	return parts, true
}

func normSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func planText(d *planDoc) string {
	var b strings.Builder
	for _, p := range d.Packets {
		b.WriteString(p.Text)
	}
	return b.String()
}

// ancestors returns, for each task, the set of tasks it transitively depends on.
func ancestors(tasks []planTask) map[string]map[string]bool {
	byID := map[string]planTask{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	out := map[string]map[string]bool{}
	var walk func(id string, into map[string]bool)
	walk = func(id string, into map[string]bool) {
		for _, d := range byID[id].DependsOn {
			if _, ok := byID[d]; ok && !into[d] {
				into[d] = true
				walk(d, into)
			}
		}
	}
	for _, t := range tasks {
		out[t.ID] = map[string]bool{}
		walk(t.ID, out[t.ID])
	}
	return out
}

// cmdPlan dispatches `ai-mode plan <lint|verify>`.
func cmdPlan(args []string) int {
	if len(args) == 0 {
		return fail("usage: ai-mode plan lint <plan.md> [--json]\n       ai-mode plan verify <plan.md> --task Tnn [--repo DIR] [--json]\n       ai-mode plan apply <plan.md> --task Tnn [--phase A|B|all] [--repo DIR]")
	}
	switch args[0] {
	case "lint":
		return cmdPlanLint(args[1:])
	case "verify":
		return cmdPlanVerify(args[1:])
	case "apply":
		return cmdPlanApply(args[1:])
	}
	return fail("unknown plan subcommand %q (want lint, verify or apply)", args[0])
}

func cmdPlanLint(args []string) int {
	fs := newFlags("plan lint")
	asJSON := fs.Bool("json", false, "Print findings as JSON")
	skeleton := fs.Bool("skeleton", false, "Skip per-packet checks (the graph exists but the packets are not written yet)")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		return fail("usage: ai-mode plan lint <plan.md> [--json] [--skeleton]")
	}
	data, err := os.ReadFile(pos[0])
	if err != nil {
		return fail("%v", err)
	}
	var findings []planFinding
	if d, err := parsePlan(string(data)); err != nil {
		findings = []planFinding{{"ERR", err.Error()}}
	} else {
		d.SpecText = readSpec(d.SpecPath, pos[0])
		findings = lintPlanOpts(d, *skeleton)
	}
	waves := ""
	if d, err := parsePlan(string(data)); err == nil {
		if w, err := planWaves(d.Tasks); err == nil {
			var ws []string
			for _, t := range d.Tasks {
				ws = append(ws, fmt.Sprintf("%s=%d", t.ID, w[t.ID]))
			}
			waves = strings.Join(ws, " ")
		}
	}
	bad := false
	for _, f := range findings {
		if f.Level == "ERR" {
			bad = true
		}
	}
	if *asJSON {
		b, _ := json.MarshalIndent(map[string]any{"ok": !bad, "findings": findings, "waves": waves}, "", "  ")
		fmt.Println(string(b))
	} else {
		for _, f := range findings {
			fmt.Printf("%s: %s\n", f.Level, f.Msg)
		}
		if waves != "" {
			fmt.Println("waves: " + waves)
		}
		if bad {
			fmt.Println("RESULT: FAIL")
		} else {
			fmt.Println("RESULT: PASS")
		}
	}
	if bad {
		return 2
	}
	return 0
}

// readSpec loads the spec named in the plan header, relative to the working directory or the plan file; "" when unreadable.
func readSpec(specPath, planPath string) string {
	if specPath == "" {
		return ""
	}
	for _, p := range []string{specPath, filepath.Join(filepath.Dir(planPath), specPath)} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return ""
}

// cmdPlanVerify is implemented in plan_verify.go.
