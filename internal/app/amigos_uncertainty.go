package app

import "strings"

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
