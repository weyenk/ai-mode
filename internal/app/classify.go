package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	ambiguousGap  = 0.10 // top two probabilities this close → ambiguous
	lowConfidence = 0.40 // jev's own confidence below this → low-confidence
)

type layerDef struct{ Key, Criterion string }

// testLayers is the closed set for Stage B; each key maps to
// presets/agents/qa/guidelines/<key>.md (a layer is offered only if that file exists).
var testLayers = []layerDef{
	{"unit", "Pure functions or single units in isolation, fast, no I/O"},
	{"functional", "Feature or use-case through its public boundary/API"},
	{"integration", "Modules with real collaborators (db, http, queues)"},
	{"component", "UI components rendered and exercised like a user"},
	{"contract", "Consumer/provider API compatibility (e.g. Pact)"},
	{"e2e", "Full user journeys across the deployed system"},
	{"visual-regression", "Rendered UI pixel or snapshot diffs"},
	{"mutation", "Mutation testing: would the tests catch real bugs? Assertion strength, surviving mutants"},
	{"ai-skill", "Evals for AI skills, prompts, agent behavior"},
}

type option struct{ Key, Criterion string }

type ranked struct {
	Name string  `json:"name"`
	Prob float64 `json:"probability"`
}

// roleOptions builds the Stage A closed set: ini sections that have a
// `summary:` in agents/<role>.md (jev, fast, deprecated roles have none).
func roleOptions(p Profile, only, exclude []string) []option {
	var out []option
	for _, role := range iniSectionsFile(p.Ini) {
		if len(only) > 0 && !contains(only, role) || contains(exclude, role) {
			continue
		}
		meta, _, err := parseAgentFile(filepath.Join(agentsDir(), role+".md"))
		if err != nil || meta["summary"] == "" || meta["deprecated"] == "true" {
			continue
		}
		out = append(out, option{role, meta["summary"]})
	}
	return out
}

func layerOptions(only, exclude []string) []option {
	var out []option
	for _, l := range testLayers {
		if len(only) > 0 && !contains(only, l.Key) || contains(exclude, l.Key) {
			continue
		}
		if !isFile(layerGuide(l.Key)) {
			continue
		}
		out = append(out, option{l.Key, l.Criterion})
	}
	return out
}

func layerGuide(key string) string {
	return filepath.Join(agentsDir(), "qa", "guidelines", key+".md")
}

// rankProbs sorts name→probability descending (ties by name for stability).
func rankProbs(probs map[string]any) []ranked {
	var r []ranked
	for k, v := range probs {
		r = append(r, ranked{k, toFloat(v)})
	}
	sort.Slice(r, func(i, j int) bool {
		if r[i].Prob != r[j].Prob {
			return r[i].Prob > r[j].Prob
		}
		return r[i].Name < r[j].Name
	})
	return r
}

// decide classifies how much to trust jev's answer.
func decide(r []ranked, confidence float64) string {
	switch {
	case len(r) >= 2 && r[0].Prob-r[1].Prob < ambiguousGap:
		return "ambiguous"
	case confidence < lowConfidence:
		return "low-confidence"
	}
	return "clear"
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// classifyResult is one jev answer plus the request/response for tracing.
type classifyResult struct {
	Question    string
	Choice      string
	Confidence  float64
	Decision    string
	Ranked      []ranked
	Load        []string
	InputTokens int
	Body, Raw   map[string]any
}

// questionSpec maps the CLI kind to jev's question key and instructions.
func questionSpec(kind string) (question, instructions string, ok bool) {
	switch strings.ReplaceAll(kind, "-", "_") {
	case "role":
		return "role", "Which specialist should handle this task?", true
	case "layer", "test_layer":
		return "test_layer", "Which test layer should qa use as the primary lens?", true
	}
	return "", "", false
}

func optionsFor(p Profile, question string, only, exclude []string) []option {
	if question == "role" {
		return roleOptions(p, only, exclude)
	}
	return layerOptions(only, exclude)
}

// classifyCall asks jev one closed-set question. It never writes events, so
// both `classify` (traced) and `classify --eval` (untraced) share it. Body and
// Raw are filled even on error so callers can record them.
func classifyCall(p Profile, question, instructions, task string, opts []option, timeout time.Duration) (classifyResult, error) {
	criteria := orderedJSON{}
	for _, o := range opts {
		criteria = append(criteria, struct {
			K string
			V any
		}{o.Key, o.Criterion})
	}
	res := classifyResult{Question: question, Body: map[string]any{
		"model": "jev",
		"state": task,
		"questions": map[string]any{
			question: map[string]any{"type": "choice", "instructions": instructions, "criteria": criteria},
		},
	}}
	data, err := postJSON(p.BaseURL()+"/systemone", res.Body, timeout)
	if err != nil {
		return res, err
	}
	res.Raw = data
	if u := asMap(data["usage"]); u != nil {
		res.InputTokens = toInt(u["input_tokens"])
	}
	ans := asMap(asMap(data["answers"])[question])
	if ans == nil {
		return res, fmt.Errorf("No answer in jev response")
	}
	res.Choice = asString(ans["choice"])
	res.Ranked = rankProbs(asMap(ans["probabilities"]))
	if !contains(optionKeys(opts), res.Choice) {
		return res, fmt.Errorf("jev returned %q, which is not in the closed set", res.Choice)
	}
	res.Confidence = toFloat(ans["confidence"])
	res.Decision = decide(res.Ranked, res.Confidence)
	// Guides to load (Stage B): one when clear, top two when not.
	if question == "test_layer" {
		res.Load = append(res.Load, layerGuide(res.Choice))
		if res.Decision != "clear" && len(res.Ranked) > 1 {
			res.Load = append(res.Load, layerGuide(res.Ranked[1].Name))
		}
	}
	return res, nil
}

func cmdClassify(args []string) int {
	fs := newFlags("classify")
	profile := fs.String("profile", "", "Profile (default: active)")
	only := fs.String("only", "", "Restrict the closed set to these comma-separated names")
	exclude := fs.String("exclude", "", "Drop these comma-separated names from the closed set")
	timeout := fs.Float64("timeout", 120, "Request timeout seconds")
	asJSON := fs.Bool("json", false, "Print machine-readable JSON")
	short := fs.Bool("short", false, "Print only the chosen name")
	traceF := fs.String("trace", "", "Trace id to attach this call to (default: $AI_MODE_TRACE_ID or new)")
	parentF := fs.String("parent", "", "Parent span id (default: $AI_MODE_PARENT_SPAN)")
	callerF := fs.String("caller", "", "Who is asking, e.g. chief (default: $AI_MODE_CALLER or cli)")
	evalFile := fs.String("eval", "", "Run a labelled JSONL file through jev and report accuracy/confidence (no events written)")
	minAcc := fs.Float64("min-accuracy", 0, "With --eval: exit 1 if accuracy is below this (0-1)")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if *evalFile != "" {
		return runEval(*evalFile, *profile, splitCSV(*only), splitCSV(*exclude), secs(*timeout), *minAcc)
	}
	if len(pos) < 1 {
		return fail("usage: ai-mode classify <role|layer> [task...] [--profile P] [--only a,b] [--exclude a,b] [--json] [--short] [--trace ID] [--parent SPAN] [--caller NAME]\n" +
			"       ai-mode classify --eval cases.jsonl [--profile P] [--min-accuracy 0.9]\n" +
			"  role  — Stage A: which specialist should handle the task\n  layer — Stage B: which QA test layer (after role=qa)")
	}
	question, instructions, valid := questionSpec(pos[0])
	if !valid {
		return fail("unknown classification %q (use: role | layer)", pos[0])
	}

	p, code := activeProfile(*profile, "Use: ai-mode use <name> or ai-mode classify --profile <name>")
	if code != 0 {
		return code
	}
	opts := optionsFor(p, question, splitCSV(*only), splitCSV(*exclude))
	if len(opts) < 2 {
		return fail("need at least 2 options to classify, found %d (check agents/*.md `summary:` lines and the profile's roles)", len(opts))
	}
	task := readUserMessage(pos[1:])
	if task == "" {
		return fail("Empty task: pass it as an argument or pipe stdin.")
	}

	trace, parent := traceContext(*traceF, *parentF)
	ev := Event{
		Kind: "classify", Trace: trace, Span: newSpanID(), Parent: parent,
		Caller:  firstNonEmpty(*callerF, os.Getenv("AI_MODE_CALLER"), "cli"),
		Profile: p.Name, Role: "jev", Question: question, PromptChars: len(task),
	}
	start := time.Now()
	ev.TS = start.UTC()
	var res classifyResult
	finish := func(code int, errMsg string) int {
		ev.DurationMS = time.Since(start).Milliseconds()
		if errMsg != "" {
			ev.Status, ev.Error = "error", errMsg
			fmt.Fprintln(os.Stderr, errMsg)
		}
		var resp any
		if res.Raw != nil {
			resp = res.Raw
		}
		ev.Payload = savePayload(ev.Trace, ev.Span, res.Body, resp)
		emit(ev)
		pruneTraces()
		return code
	}

	cat := fetchCatalog(p, 5*time.Second)
	if len(cat) == 0 {
		return finish(1, fmt.Sprintf("API not reachable at %s", p.BaseURL()))
	}
	e, found := cat["jev"]
	if !found {
		return finish(1, fmt.Sprintf("jev is not configured in %s", filepathBase(p.Ini)))
	}
	ev.Cold = modelStatus(e) != "loaded"

	res, err := classifyCall(p, question, instructions, task, opts, secs(*timeout))
	ev.PromptTokens = res.InputTokens
	if err != nil {
		return finish(1, err.Error())
	}
	ev.Choice, ev.Confidence, ev.Decision = res.Choice, res.Confidence, res.Decision

	switch {
	case *asJSON:
		fmt.Print(prettyJSON(orderedJSON{
			{"question", question}, {"choice", res.Choice}, {"confidence", res.Confidence}, {"decision", res.Decision},
			{"ranked", res.Ranked}, {"load", res.Load}, {"trace_id", ev.Trace}, {"span_id", ev.Span},
		}))
	case *short:
		fmt.Println(res.Choice)
	default:
		fmt.Printf("choice: %s\nconfidence: %.2f\ndecision: %s\n", res.Choice, res.Confidence, decisionAdvice(res.Decision, res.Ranked, question))
		var parts []string
		for i, r := range res.Ranked {
			if i == 4 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s %.2f", r.Name, r.Prob))
		}
		fmt.Printf("ranked: %s\n", strings.Join(parts, ", "))
		for _, g := range res.Load {
			fmt.Printf("load: %s\n", g)
		}
		fmt.Printf("trace=%s span=%s\n", ev.Trace, ev.Span)
	}
	return finish(0, "")
}

func optionKeys(opts []option) []string {
	keys := make([]string, len(opts))
	for i, o := range opts {
		keys[i] = o.Key
	}
	return keys
}

func decisionAdvice(decision string, r []ranked, question string) string {
	switch decision {
	case "ambiguous":
		return fmt.Sprintf("ambiguous — %s and %s are within %.2f; ask the human one clarifying question%s",
			r[0].Name, r[1].Name, ambiguousGap, map[bool]string{true: " or load both guides", false: ""}[question == "test_layer"])
	case "low-confidence":
		return "low-confidence — ask the human one clarifying question before routing"
	}
	return "clear"
}
