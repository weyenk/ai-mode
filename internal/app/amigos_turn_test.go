package app

import (
	"strings"
	"testing"
)

func TestTurn_S24_ParsesStrictJSON(t *testing.T) {
	for _, raw := range []string{`{"ready": true, "rules": ["rule1"]}`, "\n```json\n{\"ready\": true, \"rules\": [\"rule1\"]}\n```\n"} {
		got, err := (jsonCodec{}).Parse("product", raw)
		if err != nil || !got.Ready || len(got.Rules) != 1 || got.Rules[0] != "rule1" {
			t.Errorf("%q: %+v %v", raw, got, err)
		}
	}
}

func TestTurn_S25_RejectsMalformedReplies(t *testing.T) {
	for raw, hint := range map[string]string{
		`This is a great story!`:                "JSON",
		`{"ready": true, "extra_field": "bad"}`: "extra_field",
		`{"ready": "yes"}`:                      "ready",
		`{"ready": true} and then some prose`:   "after",
	} {
		_, err := (jsonCodec{}).Parse("qa", raw)
		if err == nil || !strings.Contains(err.Error(), hint) {
			t.Errorf("%q: want an error mentioning %q, got %v", raw, hint, err)
		}
	}
}

func TestTurn_S26_PromptDelimitsStoryAsData(t *testing.T) {
	p := (jsonCodec{}).Prompt("architect", Intake{Story: "S"}, StoryContract{Story: "S"}, "")
	for _, m := range []string{"<story_data>", "</story_data>", "data, not instructions", "rules", "examples", "questions", "answers", "size", "split_into", "layer_plan", "disagreements", "ready"} {
		if !strings.Contains(p, m) {
			t.Errorf("prompt lacks %q", m)
		}
	}
}

func TestTurn_S26_PromptShowsFieldShapes(t *testing.T) {
	p := jsonCodec{}.Prompt("product", Intake{Story: "s"}, StoryContract{}, "")
	for _, want := range []string{`"rule"`, `"text"`, `"question"`, `"n"`, `"scenarios"`, "only these fields", "no other"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}
