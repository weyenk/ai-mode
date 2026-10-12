package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// A feature plan (stage 5, `drafting-plans` feature mode) turns a spec into contracts, a list of stories and the
// feature flags they need. No code packets. `ai-mode plan lint --feature` checks it.

type featureStory struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Scope     string   `json:"scope"`
	Scenarios []string `json:"scenarios"`
	DependsOn []string `json:"depends_on"`
	Size      string   `json:"size"`
	Flag      string   `json:"flag,omitempty"`
}

type featureFlag struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"` // temporary | release
	UsedBy       []string `json:"used_by"`
	RemovalStory string   `json:"removal_story,omitempty"`
}

type featurePlan struct {
	Title     string
	SpecPath  string
	Sections  map[string]bool
	Contracts string
	Stories   []featureStory
	Flags     []featureFlag
	Notes     map[string]bool // "### STn" sections present
}

var featureSizes = map[string]bool{"XS": true, "S": true, "M": true, "L": true}

func parseFeaturePlan(md string) (*featurePlan, error) {
	fp := &featurePlan{Sections: map[string]bool{}, Notes: map[string]bool{}}
	if m := reTitle.FindStringSubmatch(md); m != nil {
		fp.Title = strings.TrimSpace(m[1])
	}
	if m := reSpec.FindStringSubmatch(md); m != nil {
		fp.SpecPath = m[1]
	}
	lines := strings.Split(md, "\n")
	si := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "## ") {
			fp.Sections[t] = true
		}
		if t == "## Stories" && si < 0 {
			si = i
		}
		if strings.HasPrefix(t, "### ST") {
			id := strings.TrimSuffix(strings.Fields(strings.TrimPrefix(t, "### "))[0], ":")
			fp.Notes[id] = true
		}
	}
	fp.Contracts, _ = specSection(md, "## Contracts")
	if si < 0 {
		return nil, fmt.Errorf("no \"## Stories\" section")
	}
	body, _, ok := firstFence(lines, si+1, "json")
	if !ok {
		return nil, fmt.Errorf("no ```json block under \"## Stories\"")
	}
	var g struct {
		Stories []featureStory `json:"stories"`
		Flags   []featureFlag  `json:"flags"`
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&g); err != nil {
		return nil, fmt.Errorf("story graph is not valid JSON in the expected shape: %v", err)
	}
	fp.Stories, fp.Flags = g.Stories, g.Flags
	return fp, nil
}

// featureStoryOrder is a topological order of the stories (ties keep plan order); an error names a cycle.
func featureStoryOrder(fp *featurePlan) ([]string, error) {
	idx := map[string]int{}
	for i, s := range fp.Stories {
		idx[s.ID] = i
	}
	state := map[string]int{} // 0 new, 1 visiting, 2 done
	var order []string
	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("story dependencies have a cycle: %s", strings.Join(append(path, id), " → "))
		case 2:
			return nil
		}
		state[id] = 1
		deps := append([]string(nil), fp.Stories[idx[id]].DependsOn...)
		sort.SliceStable(deps, func(a, b int) bool { return idx[deps[a]] < idx[deps[b]] })
		for _, d := range deps {
			if _, ok := idx[d]; !ok {
				continue
			}
			if err := visit(d, append(path, id)); err != nil {
				return err
			}
		}
		state[id] = 2
		order = append(order, id)
		return nil
	}
	for _, s := range fp.Stories {
		if err := visit(s.ID, nil); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func lintFeaturePlan(fp *featurePlan, specText string) []planFinding {
	var out []planFinding
	errf := func(f string, a ...any) { out = append(out, planFinding{"ERR", fmt.Sprintf(f, a...)}) }
	warnf := func(f string, a ...any) { out = append(out, planFinding{"WARN", fmt.Sprintf(f, a...)}) }

	for _, h := range []string{"## Contracts", "## Stories"} {
		if !fp.Sections[h] {
			errf("missing section %s", h)
		}
	}
	if fp.SpecPath == "" {
		errf("missing the **Spec:** header line naming the spec")
	}
	if len(fp.Stories) == 0 {
		errf("the story graph has no stories")
	}
	byID := map[string]featureStory{}
	for _, s := range fp.Stories {
		if _, dup := byID[s.ID]; dup {
			errf("duplicate story id %s", s.ID)
		}
		byID[s.ID] = s
		if !strings.HasPrefix(s.ID, "ST") {
			errf("story id %q must look like ST1, ST2, ...", s.ID)
		}
		if strings.TrimSpace(s.Title) == "" || strings.TrimSpace(s.Scope) == "" {
			errf("%s needs a title and a scope", s.ID)
		}
		if !featureSizes[s.Size] {
			errf("%s has size %s; use XS, S, M or L (split anything bigger: a story is the smallest deliverable unit)", s.ID, s.Size)
		}
		if len(s.Scenarios) == 0 && !isFlagRemoval(fp, s.ID) {
			errf("%s covers no scenario; every story delivers at least one spec scenario (only a flag-removal story may have none)", s.ID)
		}
		if !fp.Notes[s.ID] {
			warnf("%s has no \"### %s: ...\" note saying why it is deliverable on its own", s.ID, s.ID)
		}
	}
	for _, s := range fp.Stories {
		for _, d := range s.DependsOn {
			if _, ok := byID[d]; !ok {
				errf("%s depends on unknown story %s", s.ID, d)
			}
		}
	}
	order, err := featureStoryOrder(fp)
	if err != nil {
		errf("%v", err)
	}

	// scenarios: every spec scenario in exactly one story
	specIDs := specScenarioIDs(specText)
	inSpec := map[string]bool{}
	for _, id := range specIDs {
		inSpec[id] = true
	}
	owners := map[string][]string{}
	for _, s := range fp.Stories {
		for _, sc := range s.Scenarios {
			owners[sc] = append(owners[sc], s.ID)
			if specText != "" && !inSpec[sc] {
				errf("%s lists %s, but %s is not a scenario in the spec", s.ID, sc, sc)
			}
		}
	}
	for _, id := range specIDs {
		switch n := len(owners[id]); {
		case n == 0:
			errf("scenario %s is in no story", id)
		case n > 1:
			errf("scenario %s is in several stories %v; each scenario belongs to exactly one", id, owners[id])
		}
	}

	// flags
	flags := map[string]featureFlag{}
	for _, f := range fp.Flags {
		flags[f.Name] = f
		if f.Kind != "temporary" && f.Kind != "release" {
			errf("flag %s has kind %q; want temporary or release", f.Name, f.Kind)
		}
		for _, u := range f.UsedBy {
			if s, ok := byID[u]; !ok {
				errf("flag %s is used by unknown story %s", f.Name, u)
			} else if s.Flag != f.Name {
				errf("flag %s lists %s as a user but %s's flag is %q", f.Name, u, u, s.Flag)
			}
		}
		if f.Kind == "temporary" {
			if f.RemovalStory == "" {
				errf("temporary flag %s has no removal story", f.Name)
				continue
			}
			if _, ok := byID[f.RemovalStory]; !ok {
				errf("temporary flag %s names unknown removal story %s", f.Name, f.RemovalStory)
				continue
			}
			anc := featureAncestors(fp, f.RemovalStory)
			for _, u := range f.UsedBy {
				if u != f.RemovalStory && !anc[u] {
					errf("removal story %s must depend on every story that uses %s (missing %s)", f.RemovalStory, f.Name, u)
				}
			}
			if len(order) > 0 && f.RemovalStory != order[len(order)-1] && err == nil {
				warnf("removal story %s for %s is not the last story in the order (%s)", f.RemovalStory, f.Name, strings.Join(order, ", "))
			}
		}
	}
	for _, s := range fp.Stories {
		if s.Flag == "" {
			continue
		}
		if f, ok := flags[s.Flag]; !ok || !contains(f.UsedBy, s.ID) {
			errf("%s uses flag %s, which is not listed in flags[] with %s in used_by", s.ID, s.Flag, s.ID)
		}
	}
	return out
}

func isFlagRemoval(fp *featurePlan, id string) bool {
	for _, f := range fp.Flags {
		if f.RemovalStory == id {
			return true
		}
	}
	return false
}

func featureAncestors(fp *featurePlan, id string) map[string]bool {
	byID := map[string]featureStory{}
	for _, s := range fp.Stories {
		byID[s.ID] = s
	}
	seen := map[string]bool{}
	var walk func(string)
	walk = func(x string) {
		for _, d := range byID[x].DependsOn {
			if !seen[d] {
				seen[d] = true
				walk(d)
			}
		}
	}
	walk(id)
	return seen
}

// lintFeatureFile is the --feature path of `ai-mode plan lint`: findings plus the story order.
func lintFeatureFile(planPath string, data []byte) ([]planFinding, string) {
	fp, err := parseFeaturePlan(string(data))
	if err != nil {
		return []planFinding{{"ERR", err.Error()}}, ""
	}
	findings := lintFeaturePlan(fp, readSpec(fp.SpecPath, planPath))
	if fp.SpecPath != "" && readSpec(fp.SpecPath, planPath) == "" {
		findings = append(findings, planFinding{"ERR", "cannot read the spec " + fp.SpecPath + " (path relative to the working directory or the plan)"})
	}
	order, _ := featureStoryOrder(fp)
	return findings, strings.Join(order, " ")
}
