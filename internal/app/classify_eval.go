package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// evalCase is one labelled line of a classify eval file (JSONL).
//
//	{"question":"role","task":"Write edge-case tests for login","expect":"qa"}
//
// question defaults to "role"; "layer" / "test_layer" evaluates Stage B.
type evalCase struct {
	Question string `json:"question"`
	Task     string `json:"task"`
	Expect   string `json:"expect"`
}

func parseEvalCases(text string) ([]evalCase, error) {
	var out []evalCase
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var c evalCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("line %d: %v", i+1, err)
		}
		if c.Question == "" {
			c.Question = "role"
		}
		if _, _, ok := questionSpec(c.Question); !ok {
			return nil, fmt.Errorf("line %d: unknown question %q", i+1, c.Question)
		}
		if c.Task == "" || c.Expect == "" {
			return nil, fmt.Errorf("line %d: task and expect are required", i+1)
		}
		out = append(out, c)
	}
	return out, nil
}

type evalRow struct {
	evalCase
	Got        string
	Confidence float64
	Margin     float64
	Decision   string
	Err        string
}

func (r evalRow) correct() bool { return r.Err == "" && r.Got == r.Expect }

type evalSummary struct {
	N, Correct, Clear  int
	WrongButClear      int // dangerous: confidently wrong
	RightButUnclear    int // wasted human clarifications
	Errors             int
	AvgConf, AvgMargin float64
	Confusions         map[string]int // "expect→got"
}

func summarizeEval(rows []evalRow) evalSummary {
	s := evalSummary{Confusions: map[string]int{}}
	for _, r := range rows {
		s.N++
		if r.Err != "" {
			s.Errors++
			continue
		}
		s.AvgConf += r.Confidence
		s.AvgMargin += r.Margin
		if r.Decision == "clear" {
			s.Clear++
		}
		switch {
		case r.correct():
			s.Correct++
			if r.Decision != "clear" {
				s.RightButUnclear++
			}
		default:
			s.Confusions[r.Expect+"→"+r.Got]++
			if r.Decision == "clear" {
				s.WrongButClear++
			}
		}
	}
	if scored := s.N - s.Errors; scored > 0 {
		s.AvgConf /= float64(scored)
		s.AvgMargin /= float64(scored)
	}
	return s
}

func runEval(file, profile string, only, exclude []string, timeout time.Duration, minAcc float64) int {
	data, err := os.ReadFile(file)
	if err != nil {
		return fail("%v", err)
	}
	cases, err := parseEvalCases(string(data))
	if err != nil {
		return fail("%s: %v", file, err)
	}
	if len(cases) == 0 {
		return fail("%s: no cases", file)
	}
	p, code := activeProfile(profile, "Use: ai-mode use <name> or ai-mode classify --eval FILE --profile <name>")
	if code != 0 {
		return code
	}
	if len(fetchCatalog(p, 5*time.Second)) == 0 {
		return fail("API not reachable at %s", p.BaseURL())
	}

	var rows []evalRow
	skipped := 0
	for _, c := range cases {
		question, instructions, _ := questionSpec(c.Question)
		opts := optionsFor(p, question, only, exclude)
		if !contains(optionKeys(opts), c.Expect) {
			skipped++ // expected answer is not routable under this profile/filters
			continue
		}
		row := evalRow{evalCase: c}
		res, err := classifyCall(p, question, instructions, c.Task, opts, timeout)
		if err != nil {
			row.Err = err.Error()
		} else {
			row.Got, row.Confidence, row.Decision = res.Choice, res.Confidence, res.Decision
			if len(res.Ranked) > 1 {
				row.Margin = res.Ranked[0].Prob - res.Ranked[1].Prob
			}
		}
		rows = append(rows, row)
		mark := "✓"
		if !row.correct() {
			mark = "✗"
		}
		if row.Err != "" {
			fmt.Printf("%s %-9s error: %s | %s\n", mark, row.Expect, row.Err, truncate(c.Task, 60))
			continue
		}
		fmt.Printf("%s %-9s → %-9s conf=%.2f margin=%.2f %-14s | %s\n", mark, row.Expect, row.Got,
			row.Confidence, row.Margin, row.Decision, truncate(c.Task, 60))
	}

	s := summarizeEval(rows)
	if s.N == 0 {
		return fail("no scoreable cases (all %d skipped: expected answers not in the closed set)", skipped)
	}
	acc := float64(s.Correct) / float64(s.N)
	fmt.Printf("\ncases: %d (skipped %d)   accuracy: %d/%d (%.0f%%)   avg confidence: %.2f   avg margin: %.2f\n",
		s.N, skipped, s.Correct, s.N, acc*100, s.AvgConf, s.AvgMargin)
	fmt.Printf("clear: %d/%d   wrong-but-clear: %d   right-but-unclear: %d   errors: %d\n",
		s.Clear, s.N, s.WrongButClear, s.RightButUnclear, s.Errors)
	if len(s.Confusions) > 0 {
		var cs []string
		for k, n := range s.Confusions {
			cs = append(cs, fmt.Sprintf("%s ×%d", k, n))
		}
		sort.Strings(cs)
		fmt.Printf("confusions: %s\n", strings.Join(cs, ", "))
	}
	if minAcc > 0 && acc < minAcc {
		fmt.Fprintf(os.Stderr, "accuracy %.2f is below --min-accuracy %.2f\n", acc, minAcc)
		return 1
	}
	return 0
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
