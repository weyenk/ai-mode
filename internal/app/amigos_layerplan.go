package app

import (
	"fmt"
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
