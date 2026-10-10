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
