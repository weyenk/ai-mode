package app

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func contractWith(n int, layers ...Layer) StoryContract {
	c := StoryContract{LayerPlan: layers}
	for i := 0; i < n; i++ {
		c.Examples = append(c.Examples, Example{Rule: "r", Text: "e"})
	}
	return c
}

func TestLayerPlan_S10_EachScenarioInOneLayer(t *testing.T) {
	if err := validateLayerPlan(contractWith(3, Layer{N: 1, Scenarios: []int{1, 2}}, Layer{N: 2, Scenarios: []int{3}})); err != nil {
		t.Fatalf("valid: %v", err)
	}
	cases := map[string]struct {
		c    StoryContract
		want string
	}{
		"missing":   {contractWith(3, Layer{N: 1, Scenarios: []int{1, 2}}), "3"},
		"duplicate": {contractWith(3, Layer{N: 1, Scenarios: []int{1, 2}}, Layer{N: 2, Scenarios: []int{2, 3}}), "2"},
		"range":     {contractWith(3, Layer{N: 1, Scenarios: []int{1, 2, 3, 7}}), "7"},
	}
	for name, tc := range cases {
		if err := validateLayerPlan(tc.c); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want error naming %s, got %v", name, tc.want, err)
		}
	}
	if err := validateLayerPlan(StoryContract{}); err != nil {
		t.Fatalf("empty: %v", err)
	}
}

type failSecond struct{ n int }

func (f *failSecond) TestLayer(string) (string, error) {
	f.n++
	if f.n == 2 {
		return "", errors.New("jev down")
	}
	return "unit", nil
}

func TestLayerPlan_S11_TestLayerPerLayer(t *testing.T) {
	c := contractWith(2, Layer{N: 1, Name: "a", Scenarios: []int{1}}, Layer{N: 2, Name: "b", Scenarios: []int{2}})
	if err := assignTestLayers(&c, &fakeClassifier{LayerStr: "unit"}); err != nil || c.LayerPlan[0].TestLevel != "unit" || c.LayerPlan[1].TestLevel != "unit" {
		t.Fatalf("%+v %v", c.LayerPlan, err)
	}
	c = contractWith(2, Layer{N: 1, Name: "a", Scenarios: []int{1}}, Layer{N: 2, Name: "b", Scenarios: []int{2}})
	err := assignTestLayers(&c, &failSecond{})
	if err == nil || !strings.Contains(err.Error(), "layer 2") || c.LayerPlan[0].TestLevel != "unit" {
		t.Fatalf("%+v %v", c.LayerPlan, err)
	}
}

func TestLayerPlan_S31_GatesFollowExposure(t *testing.T) {
	a, b := planGates(true), planGates(false)
	names := []string{"static-analysis", "mutation", "performance", "ai-evals", "dynamic-security"}
	for i, n := range names {
		if a[i].Name != n || b[i].Name != n {
			t.Fatalf("order: %v %v", a, b)
		}
	}
	if a[4].State != "active" || b[4].State != "not-applicable" || a[2].State != "not-available" || b[3].State != "not-available" {
		t.Fatalf("states: %v %v", a, b)
	}
}

func TestLayerPlan_S36_JevAdapterMapsChoice(t *testing.T) {
	var gotQ, gotTask string
	fn := func(p Profile, question, instructions, task string, opts []option, timeout time.Duration) (classifyResult, error) {
		gotQ, gotTask = question, task
		return classifyResult{Choice: "functional"}, nil
	}
	got, err := newJevClassifier(Profile{}, fn).TestLayer("do x")
	if err != nil || got != "functional" || gotQ != "test_layer" || gotTask != "do x" {
		t.Fatalf("%q %q %q %v", got, gotQ, gotTask, err)
	}
	bad := func(Profile, string, string, string, []option, time.Duration) (classifyResult, error) {
		return classifyResult{}, errors.New("down")
	}
	if _, err := newJevClassifier(Profile{}, bad).TestLayer("x"); err == nil {
		t.Fatal("classifier error must surface")
	}
}
