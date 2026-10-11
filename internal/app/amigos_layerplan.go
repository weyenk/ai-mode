package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func validateLayerPlan(c StoryContract) error {
	n := len(c.Examples)
	seen := map[int]int{}
	for _, l := range c.LayerPlan {
		for _, p := range l.Scenarios {
			if p < 1 || p > n {
				return fmt.Errorf("layer %d lists example %d, which does not exist (1..%d)", l.N, p, n)
			}
			seen[p]++
		}
	}
	for p := 1; p <= n; p++ {
		switch seen[p] {
		case 0:
			return fmt.Errorf("example %d is in no layer", p)
		case 1:
		default:
			return fmt.Errorf("example %d is in %d layers", p, seen[p])
		}
	}
	return nil
}

// repairLayerPlan fixes the layer plan's bookkeeping, not its design: positions out of range or listed twice are
// dropped, and an example in no layer joins the layer holding most examples of its rule (else the last layer).
// Examples keep arriving after architect's turn, so a plan that covered every example then left later ones out and
// downgraded a story all three amigos had agreed on. It returns one note per change.
func repairLayerPlan(c *StoryContract) []string {
	if len(c.LayerPlan) == 0 {
		return nil
	}
	var notes []string
	n := len(c.Examples)
	owner := map[int]int{} // example -> layer index
	for li := range c.LayerPlan {
		l := &c.LayerPlan[li]
		var keep []int
		for _, p := range l.Scenarios {
			if _, dup := owner[p]; p < 1 || p > n || dup {
				notes = append(notes, fmt.Sprintf("layer %d: dropped example %d (out of range or already in a layer)", l.N, p))
				continue
			}
			owner[p] = li
			keep = append(keep, p)
		}
		l.Scenarios = keep
	}
	for p := 1; p <= n; p++ {
		if _, ok := owner[p]; ok {
			continue
		}
		best, bestCount := len(c.LayerPlan)-1, 0
		for li, l := range c.LayerPlan {
			count := 0
			for _, q := range l.Scenarios {
				if c.Examples[q-1].Rule == c.Examples[p-1].Rule {
					count++
				}
			}
			if count > bestCount {
				best, bestCount = li, count
			}
		}
		c.LayerPlan[best].Scenarios = append(c.LayerPlan[best].Scenarios, p)
		owner[p] = best
		notes = append(notes, fmt.Sprintf("example %d was in no layer; added to layer %d", p, c.LayerPlan[best].N))
	}
	for li := range c.LayerPlan {
		sort.Ints(c.LayerPlan[li].Scenarios)
	}
	return notes
}

func assignTestLayers(c *StoryContract, lc layerClassifier) error {
	for i := range c.LayerPlan {
		l := &c.LayerPlan[i]
		var texts []string
		for _, p := range l.Scenarios {
			if p >= 1 && p <= len(c.Examples) {
				texts = append(texts, c.Examples[p-1].Text)
			}
		}
		lvl, err := lc.TestLayer(l.Name + ": " + strings.Join(texts, "; "))
		if err != nil {
			return fmt.Errorf("layer %d: %w", l.N, err)
		}
		l.TestLevel = lvl
	}
	return nil
}

func planGates(exposed bool) []Gate {
	dyn := "not-applicable"
	if exposed {
		dyn = "active"
	}
	return []Gate{{"static-analysis", "active"}, {"mutation", "active"}, {"performance", "not-available"}, {"ai-evals", "not-available"}, {"dynamic-security", dyn}}
}

type classifyFunc func(p Profile, question, instructions, task string, opts []option, timeout time.Duration) (classifyResult, error)

type jevClassifier struct {
	p    Profile
	call classifyFunc
}

func newJevClassifier(p Profile, call classifyFunc) layerClassifier { return &jevClassifier{p, call} }

func (j *jevClassifier) TestLayer(task string) (string, error) {
	question, instructions, _ := questionSpec("layer")
	opts := optionsFor(j.p, question, nil, nil)
	res, err := j.call(j.p, question, instructions, task, opts, 120*time.Second)
	if err != nil {
		return "", fmt.Errorf("classify layer: %w", err)
	}
	return res.Choice, nil
}
