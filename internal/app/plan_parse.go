package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reTitle    = regexp.MustCompile(`(?m)^# (.+)$`)
	reContext  = regexp.MustCompile(`\*\*Context:\*\*\s*([^\n]+)`)
	reSpec     = regexp.MustCompile("\\*\\*Spec:\\*\\*\\s*`?([^`\\s]+)`?")
	reExcerpt  = regexp.MustCompile(`(?m)^\*\*Spec excerpt:\*\*[ \t]*(.*)$`)
	reHeading2 = regexp.MustCompile(`(?m)^## .+$`)
	rePacket   = regexp.MustCompile(`(?m)^### (T\d+):`)
	reFence    = regexp.MustCompile("^```(\\w*)(.*)$")
	reAttr     = regexp.MustCompile(`(\w+)=("[^"]*"|\S+)`)
	reCovRow   = regexp.MustCompile("^\\|\\s*(S\\d+)\\s*\\|.*\\|\\s*(T\\d+)\\s*\\|\\s*`?([A-Za-z0-9_]+)`?\\s*\\|\\s*$")
	reRunCmd   = regexp.MustCompile("Run: `([^`]+)`")
)

// parsePlan reads a drafting-plans document. The task graph is the first ```json block under
// "## Task graph" (Go's standard library cannot parse YAML); waves are computed, not written.
func parsePlan(md string) (*planDoc, error) {
	d := &planDoc{Sections: map[string]bool{}, Packets: map[string]*planPacket{}}
	if m := reTitle.FindStringSubmatch(md); m != nil {
		d.Title = strings.TrimSpace(m[1])
	}
	if m := reContext.FindStringSubmatch(md); m != nil {
		d.Context = strings.Trim(strings.TrimSpace(m[1]), "`")
	}
	if m := reSpec.FindStringSubmatch(md); m != nil {
		d.SpecPath = m[1]
	}
	for _, h := range reHeading2.FindAllString(md, -1) {
		d.Sections[strings.TrimSpace(h)] = true
	}

	lines := strings.Split(md, "\n")
	// task graph
	gi := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "## Task graph") {
			gi = i
			break
		}
	}
	if gi < 0 {
		return nil, fmt.Errorf("no \"## Task graph\" section")
	}
	graphJSON, _, ok := firstFence(lines, gi+1, "json")
	if !ok {
		return nil, fmt.Errorf("no ```json block under \"## Task graph\"")
	}
	var g struct {
		Tasks    []planTask `json:"tasks"`
		HotFiles []string   `json:"hot_files"`
	}
	if err := json.Unmarshal([]byte(graphJSON), &g); err != nil {
		return nil, fmt.Errorf("task graph is not valid JSON: %v", err)
	}
	d.Tasks, d.HotFiles = g.Tasks, g.HotFiles

	// scenario coverage table
	for _, l := range lines {
		if m := reCovRow.FindStringSubmatch(l); m != nil {
			d.Coverage = append(d.Coverage, planCov{Scenario: m[1], Task: m[2], Test: m[3]})
		}
	}

	// packets: "### Tnn:" up to the next "### Tnn:" or "## " heading, ignoring lines inside fenced blocks
	type mark struct {
		line int
		id   string // "" for a "## " heading that ends the current packet
	}
	var marks []mark
	inFence := false
	for i, l := range lines {
		if strings.HasPrefix(l, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := rePacket.FindStringSubmatch(l); m != nil && strings.HasPrefix(l, "### ") {
			marks = append(marks, mark{i, m[1]})
		} else if strings.HasPrefix(l, "## ") {
			marks = append(marks, mark{i, ""})
		}
	}
	for n, mk := range marks {
		if mk.id == "" {
			continue
		}
		end := len(lines)
		if n+1 < len(marks) {
			end = marks[n+1].line
		}
		text := strings.Join(lines[mk.line:end], "\n")
		pk := &planPacket{ID: mk.id, Text: text}
		pk.Blocks = parseBlocks(lines[mk.line:end])
		if m := reRunCmd.FindStringSubmatch(text); m != nil {
			pk.RunCmd = m[1]
		}
		if m := reExcerpt.FindStringSubmatch(text); m != nil {
			pk.Excerpt = strings.TrimSpace(m[1])
		}
		d.Packets[mk.id] = pk
	}
	return d, nil
}

// firstFence returns the body of the first fenced block with the given language at or after line from.
func firstFence(lines []string, from int, lang string) (body string, next int, ok bool) {
	for i := from; i < len(lines); i++ {
		if m := reFence.FindStringSubmatch(lines[i]); m != nil && m[1] == lang {
			j := i + 1
			var buf []string
			for j < len(lines) && !strings.HasPrefix(lines[j], "```") {
				buf = append(buf, lines[j])
				j++
			}
			return strings.Join(buf, "\n"), j + 1, true
		}
	}
	return "", 0, false
}

// parseBlocks extracts fenced blocks. A block that writes a file declares it in the info string:
// ```go file=path/x.go [mode=create|append|insert-after|replace-line] [anchor="text"]
func parseBlocks(lines []string) []planBlock {
	var out []planBlock
	phase := ""
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "**Phase A"):
			phase = "A"
		case strings.HasPrefix(l, "**Phase B"):
			phase = "B"
		}
		m := reFence.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		b := planBlock{Lang: m[1], Phase: phase}
		for _, a := range reAttr.FindAllStringSubmatch(m[2], -1) {
			v := strings.Trim(a[2], `"`)
			switch a[1] {
			case "file":
				b.File = v
			case "mode":
				b.Mode = v
			case "anchor":
				b.Anchor = v
			}
		}
		var buf []string
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(lines[j], "```") {
			buf = append(buf, lines[j])
			j++
		}
		b.Code = strings.Join(buf, "\n") + "\n"
		out = append(out, b)
		i = j
	}
	return out
}

// planWaves computes each task's wave from depends_on: 0 with no dependencies, else 1 + the deepest dependency.
func planWaves(tasks []planTask) (map[string]int, error) {
	byID := map[string]planTask{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	waves := map[string]int{}
	state := map[string]int{} // 1 visiting, 2 done
	var visit func(id string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("dependency cycle involving %s", id)
		case 2:
			return nil
		}
		state[id] = 1
		w := 0
		for _, dep := range byID[id].DependsOn {
			if _, ok := byID[dep]; !ok {
				continue // reported separately by lint
			}
			if err := visit(dep); err != nil {
				return err
			}
			if waves[dep]+1 > w {
				w = waves[dep] + 1
			}
		}
		waves[id] = w
		state[id] = 2
		return nil
	}
	for _, t := range tasks {
		if err := visit(t.ID); err != nil {
			return nil, err
		}
	}
	return waves, nil
}

// applyPlanBlock writes one block into root according to its mode. It never leaves root.
func applyPlanBlock(root string, b planBlock) error {
	rel := filepath.Clean(b.File)
	if b.File == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("block path %q is empty or escapes the repo", b.File)
	}
	full := filepath.Join(root, rel)
	switch b.Mode {
	case "", "create":
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		return os.WriteFile(full, []byte(b.Code), 0o644)
	case "append", "insert-after", "replace-line":
		data, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("%s: mode %s needs an existing file: %v", b.File, b.Mode, err)
		}
		if b.Mode == "append" {
			s := string(data)
			if s != "" && !strings.HasSuffix(s, "\n") {
				s += "\n"
			}
			return os.WriteFile(full, []byte(s+b.Code), 0o644)
		}
		ls := strings.SplitAfter(string(data), "\n")
		at := -1
		for i, l := range ls {
			if strings.Contains(l, b.Anchor) {
				at = i
				break
			}
		}
		if b.Anchor == "" || at < 0 {
			return fmt.Errorf("%s: anchor %q not found", b.File, b.Anchor)
		}
		var out []string
		out = append(out, ls[:at]...)
		if b.Mode == "insert-after" {
			out = append(out, ls[at])
		}
		out = append(out, b.Code)
		out = append(out, ls[at+1:]...)
		return os.WriteFile(full, []byte(strings.Join(out, "")), 0o644)
	}
	return fmt.Errorf("%s: unknown mode %q", b.File, b.Mode)
}

// pathsOverlap reports whether two owned-path patterns can refer to the same file.
func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	for _, p := range [][2]string{{a, b}, {b, a}} {
		pat, other := p[0], p[1]
		if strings.HasSuffix(pat, "/**") {
			if strings.HasPrefix(other, strings.TrimSuffix(pat, "**")) || other == strings.TrimSuffix(pat, "/**") {
				return true
			}
		}
		if ok, _ := path.Match(pat, other); ok {
			return true
		}
	}
	return false
}
