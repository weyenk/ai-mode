package app

import (
	"math"
	"testing"
)

func TestUncertainty_S6_AgreementRatio(t *testing.T) {
	cases := []struct {
		in   []string
		want float64
	}{
		{[]string{"yes", "yes", "no"}, 2.0 / 3.0}, {[]string{"a", "b"}, 0.5}, {[]string{"a"}, 1}, {[]string{}, 0}, {nil, 0},
		{[]string{"Yes ", " yes", "NO"}, 2.0 / 3.0}, {[]string{"a", "a", "a"}, 1},
	}
	for _, c := range cases {
		if got := (agreementEngine{}).Score("q", c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%v: got %v want %v", c.in, got, c.want)
		}
	}
}

func TestUncertainty_JudgedAgreementGroupsSameDecision(t *testing.T) {
	answers := []string{
		"The program will return exit code 0, since the shutdown is graceful.",
		"Exit code 0. The program intercepts SIGINT on purpose.",
		"It exits with status 0 after printing the summary.",
	}
	fa := &fakeAsker{Answers: map[string]string{"fast": "Same value.\n```json\n{\"groups\": [[1, 2, 3]]}\n```"}}
	if got := (judgedEngine{ask: fa, role: "fast"}).Score("exit code?", answers); got != 1 {
		t.Fatalf("three answers with the same decision: got %v", got)
	}
	fa.Answers["fast"] = `{"groups": [[1, 3], [2]]}`
	if got := (judgedEngine{ask: fa, role: "fast"}).Score("q", answers); math.Abs(got-2.0/3.0) > 1e-9 {
		t.Fatalf("2 of 3: got %v", got)
	}
	fa.Answers["fast"] = `{"groups": [[1, 2, 3]]}`
	if got := (judgedEngine{ask: fa, role: "fast"}).Score("q", []string{"a", "a", ""}); math.Abs(got-2.0/3.0) > 1e-9 {
		t.Fatalf("an empty answer agrees with nothing: got %v", got)
	}
	fa.Answers["fast"] = "I cannot tell."
	if got := (judgedEngine{ask: fa, role: "fast"}).Score("q", []string{"yes", "yes", "no"}); math.Abs(got-2.0/3.0) > 1e-9 {
		t.Fatalf("an unusable judge falls back to exact match: got %v", got)
	}
}
