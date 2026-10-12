package app

import (
	"encoding/json"
	"fmt"
	"strings"
)

type agreementEngine struct{}

var _ uncertaintyEngine = agreementEngine{}

func (agreementEngine) Score(question string, answers []string) float64 {
	if len(answers) == 0 {
		return 0
	}
	counts := map[string]int{}
	best := 0
	for _, a := range answers {
		k := strings.ToLower(strings.TrimSpace(a))
		counts[k]++
		if counts[k] > best {
			best = counts[k]
		}
	}
	return float64(best) / float64(len(answers))
}

// judgedEngine asks a small model which sampled answers make the same decision, so answers that agree in different
// words count as agreement (exact match scored 0.33 on three answers that all said "exit code 0", which parked and
// escalated settled questions). Exact-match agreement is the fallback when the judge gives no usable reply.
type judgedEngine struct {
	ask  roleAsker
	role string
}

var _ uncertaintyEngine = judgedEngine{}

func (j judgedEngine) Score(question string, answers []string) float64 {
	// an empty sample is a turn that did not answer, not a dissenting answer
	var real []string
	for _, a := range answers {
		if strings.TrimSpace(a) != "" {
			real = append(real, a)
		}
	}
	if len(real) < 2 {
		return 0
	}
	answers = real
	var b strings.Builder
	fmt.Fprintf(&b, "Several team members answered the same question. Group the answers that give the same decision or value, ignoring wording, length and rationale. An empty answer belongs to no group.\n\nQuestion: %s\n\nAnswers:\n", question)
	for i, a := range answers {
		fmt.Fprintf(&b, "%d. %s\n", i+1, strings.Join(strings.Fields(a), " "))
	}
	b.WriteString("\nReply with ONE JSON object and nothing else, for example {\"groups\": [[1, 2], [3]]}.")
	raw, err := j.ask.Ask(j.role, b.String(), 512)
	if err == nil {
		var g struct {
			Groups [][]int `json:"groups"`
		}
		if js, ok := lastJSONObject(raw); ok && json.Unmarshal([]byte(js), &g) == nil {
			best := 0
			for _, grp := range g.Groups {
				n, seen := 0, map[int]bool{}
				for _, i := range grp {
					if i >= 1 && i <= len(answers) && !seen[i] && strings.TrimSpace(answers[i-1]) != "" {
						seen[i] = true
						n++
					}
				}
				if n > best {
					best = n
				}
			}
			if best > 0 {
				return float64(best) / float64(len(answers))
			}
		}
	}
	return agreementEngine{}.Score(question, answers)
}
