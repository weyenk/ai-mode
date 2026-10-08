package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Review is one human verdict on a traced call. Append-only; the latest
// record for a span wins. Fields are denormalised so `stats` and `--export`
// don't need to join against events or payloads.
type Review struct {
	TS         time.Time `json:"ts"`
	Span       string    `json:"span_id"`
	Trace      string    `json:"trace_id,omitempty"`
	Kind       string    `json:"kind"` // classify | ask
	Profile    string    `json:"profile,omitempty"`
	Role       string    `json:"role,omitempty"` // ask: the specialist; classify: jev
	Question   string    `json:"question,omitempty"`
	Task       string    `json:"task,omitempty"`
	Choice     string    `json:"choice,omitempty"` // classify: jev's pick
	Decision   string    `json:"decision,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	Verdict    string    `json:"verdict"` // ok | bad | fixed | ignore
	Correct    string    `json:"correct,omitempty"`
	Note       string    `json:"note,omitempty"`
}

func reviewsFile() string { return filepath.Join(stateDir(), "reviews.jsonl") }

func appendReview(r Review) {
	if r.TS.IsZero() {
		r.TS = time.Now().UTC()
	}
	line, err := json.Marshal(r)
	if err != nil {
		return
	}
	ensureDirs()
	f, err := os.OpenFile(reviewsFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// readReviews returns the latest review per span, plus all records in order.
func readReviews() (bySpan map[string]Review, all []Review) {
	bySpan = map[string]Review{}
	data, err := os.ReadFile(reviewsFile())
	if err != nil {
		return bySpan, nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var r Review
		if json.Unmarshal([]byte(line), &r) == nil && r.Span != "" {
			bySpan[r.Span] = r
			all = append(all, r)
		}
	}
	return bySpan, all
}

// ---- pending selection -----------------------------------------------------

type reviewFilter struct {
	kind, role, trace string // kind: classify | ask | "" (both)
	since             time.Duration
	again             bool // include already-reviewed spans
	limit             int
}

// pendingReviews picks successful, payload-backed calls that still need a
// verdict: the newest `limit`, returned oldest-first so traces read in order.
func pendingReviews(evs []Event, done map[string]Review, f reviewFilter) []Event {
	cutoff := time.Time{}
	if f.since > 0 {
		cutoff = time.Now().Add(-f.since)
	}
	var out []Event
	for _, e := range evs {
		if !isCall(e) || e.Status != "ok" || e.Payload == "" {
			continue
		}
		if f.kind != "" && e.Kind != f.kind || f.trace != "" && e.Trace != f.trace {
			continue
		}
		if f.role != "" && e.Role != f.role && e.Choice != f.role {
			continue
		}
		if !cutoff.IsZero() && e.TS.Before(cutoff) {
			continue
		}
		if _, reviewed := done[e.Span]; reviewed && !f.again {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS.Before(out[j].TS) })
	if f.limit > 0 && len(out) > f.limit {
		out = out[len(out)-f.limit:]
	}
	return out
}

// ---- answer parsing --------------------------------------------------------

type reviewAction int

const (
	actInvalid reviewAction = iota
	actOK
	actBad
	actFix
	actIgnore
	actSkip
	actQuit
	actFull
)

// matchCandidate resolves an exact or unique-prefix name from the closed set.
func matchCandidate(in string, cands []string) (string, bool) {
	in = strings.ToLower(strings.TrimSpace(in))
	if in == "" {
		return "", false
	}
	var hits []string
	for _, c := range cands {
		if strings.ToLower(c) == in {
			return c, true
		}
		if strings.HasPrefix(strings.ToLower(c), in) {
			hits = append(hits, c)
		}
	}
	if len(hits) == 1 {
		return hits[0], true
	}
	return "", false
}

func firstWord(s string) (head, rest string) {
	s = strings.TrimSpace(s)
	head, rest, _ = strings.Cut(s, " ")
	return strings.ToLower(head), strings.TrimSpace(rest)
}

// parseClassifyAnswer: ""/y = jev was right; n = wrong (caller asks which);
// a role name = wrong, and this is the right one; i/s/q as usual.
func parseClassifyAnswer(in string, cands []string) (reviewAction, string, string) {
	head, rest := firstWord(in)
	switch head {
	case "", "y", "yes":
		return actOK, "", rest
	case "n", "no":
		return actBad, "", rest
	case "i", "ignore":
		return actIgnore, "", rest
	case "s", "skip":
		return actSkip, "", ""
	case "q", "quit":
		return actQuit, "", ""
	}
	if c, ok := matchCandidate(head, cands); ok {
		return actFix, c, rest
	}
	return actInvalid, "", ""
}

// parseAskAnswer: ""/y = helpful; n [note] = not; f = show full answer.
func parseAskAnswer(in string) (reviewAction, string) {
	head, rest := firstWord(in)
	switch head {
	case "", "y", "yes":
		return actOK, rest
	case "n", "no":
		return actBad, rest
	case "f", "full":
		return actFull, ""
	case "i", "ignore":
		return actIgnore, rest
	case "s", "skip":
		return actSkip, ""
	case "q", "quit":
		return actQuit, ""
	}
	return actInvalid, ""
}

// ---- payload helpers -------------------------------------------------------

type payload struct {
	Request  map[string]any `json:"request"`
	Response map[string]any `json:"response"`
}

func loadPayload(rel string) (payload, bool) {
	var p payload
	data, err := os.ReadFile(filepath.Join(tracesDir(), rel))
	if err != nil || json.Unmarshal(data, &p) != nil {
		return p, false
	}
	return p, true
}

// classifyTask and classifyCandidates read the task and the closed set from a saved request.
func classifyTask(p payload) string { return asString(p.Request["state"]) }

func classifyCandidates(p payload, question string) []string {
	crit := asMap(asMap(asMap(p.Request["questions"])[question])["criteria"])
	keys := make([]string, 0, len(crit))
	for k := range crit {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func askQA(p payload) (q, a string) {
	if msgs, ok := p.Request["messages"].([]any); ok {
		for _, m := range msgs {
			if mm := asMap(m); asString(mm["role"]) == "user" {
				q = asString(mm["content"])
			}
		}
	}
	if ch, _ := p.Response["choices"].([]any); len(ch) > 0 {
		a = assistantText(asMap(asMap(ch[0])["message"]))
	}
	return q, a
}

// ---- command ---------------------------------------------------------------

func cmdReview(args []string) int {
	fs := newFlags("review")
	sinceF := fs.String("since", "7d", "Only calls newer than e.g. 2h, 24h, 7d")
	kind := fs.String("kind", "", "classify or ask (default: both)")
	role := fs.String("role", "", "Only this specialist, or this jev pick")
	trace := fs.String("trace", "", "Only this trace id")
	limit := fs.Int("limit", 20, "Review at most this many (newest)")
	again := fs.Bool("again", false, "Include calls you already reviewed")
	list := fs.Bool("list", false, "Just count what is waiting for review")
	export := fs.String("export", "", "Append reviewed routing calls to FILE as eval cases (see classify --eval)")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	if *kind != "" && *kind != "classify" && *kind != "ask" {
		return fail("--kind must be classify or ask")
	}
	since, err := parseSince(*sinceF)
	if err != nil {
		return fail("%v", err)
	}
	if *export != "" {
		return exportReviews(*export)
	}

	done, _ := readReviews()
	f := reviewFilter{kind: *kind, role: *role, trace: *trace, since: since, again: *again, limit: *limit}
	pending := pendingReviews(readEvents(), done, f)
	f.limit = 0
	total := len(pendingReviews(readEvents(), done, f))

	if *list || len(pending) == 0 {
		fmt.Printf("%d call(s) waiting for review (since %s)", total, *sinceF)
		if len(pending) == 0 {
			fmt.Println(" — nothing to do.")
		} else {
			fmt.Println(". Run: ai-mode review")
		}
		return 0
	}
	if !stdinIsTerminal() {
		return fail("review is interactive; run it in a terminal (or use --list / --export)")
	}

	fmt.Printf("%d call(s) to review (showing %d). Enter = looks right. q = quit.\n", total, len(pending))
	rd := bufio.NewReader(os.Stdin)
	counts := map[string]int{}
	for i, e := range pending {
		p, ok := loadPayload(e.Payload)
		if !ok {
			counts["no payload"]++
			continue
		}
		fmt.Printf("\n[%d/%d] ", i+1, len(pending))
		var r Review
		var quit bool
		if e.Kind == "classify" {
			r, quit = reviewClassify(rd, e, p)
		} else {
			r, quit = reviewAsk(rd, e, p)
		}
		if quit {
			break
		}
		if r.Verdict == "" { // skipped
			counts["skipped"]++
			continue
		}
		appendReview(r)
		counts[r.Verdict]++
	}
	var parts []string
	if len(counts) == 0 {
		fmt.Println("\nNothing saved.")
		return 0
	}
	for _, k := range []string{"ok", "fixed", "bad", "ignore", "skipped", "no payload"} {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
		}
	}
	fmt.Printf("\nSaved: %s. Next: ai-mode stats · ai-mode review --export evals/reviewed.jsonl\n", strings.Join(parts, ", "))
	return 0
}

func readLine(rd *bufio.Reader) (string, bool) {
	line, err := rd.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", false
	}
	return strings.TrimSpace(line), true
}

func reviewHeader(e Event) string {
	h := fmt.Sprintf("%s · %s · %s · by=%s · trace=%s", e.Kind, e.Profile, e.TS.Local().Format("01-02 15:04"), firstNonEmpty(e.Caller, "?"), e.Trace)
	if w := warnings(e); len(w) > 0 {
		h += "  ⚠ " + strings.Join(w, ", ")
	}
	return h
}

func reviewClassify(rd *bufio.Reader, e Event, p payload) (Review, bool) {
	cands := classifyCandidates(p, e.Question)
	task := classifyTask(p)
	fmt.Println(reviewHeader(e))
	fmt.Printf("  Task: %s\n", truncate(strings.Join(strings.Fields(task), " "), 300))
	fmt.Printf("  jev %s → %s (%.2f, %s)", e.Question, e.Choice, e.Confidence, e.Decision)
	if ch := asMap(p.Response["answers"]); ch != nil {
		if r := rankProbs(asMap(asMap(ch[e.Question])["probabilities"])); len(r) > 1 {
			fmt.Printf("   next: %s %.2f, %s %.2f", r[0].Name, r[0].Prob, r[1].Name, r[1].Prob)
		}
	}
	fmt.Println()
	base := Review{Span: e.Span, Trace: e.Trace, Kind: "classify", Profile: e.Profile, Role: "jev",
		Question: e.Question, Task: task, Choice: e.Choice, Decision: e.Decision, Confidence: e.Confidence}
	for {
		fmt.Printf("  Was %q right?  [Enter]=yes  n=no  or type the right one (%s)  i=ignore  s=skip  q=quit\n  > ", e.Choice, strings.Join(cands, "/"))
		line, ok := readLine(rd)
		if !ok {
			return Review{}, true
		}
		act, correct, note := parseClassifyAnswer(line, cands)
		switch act {
		case actOK:
			base.Verdict, base.Note = "ok", note
			return base, false
		case actFix:
			base.Verdict, base.Correct, base.Note = "fixed", correct, note
			if correct == e.Choice {
				base.Verdict, base.Correct = "ok", ""
			}
			return base, false
		case actBad:
			fmt.Print("  Which one was right? (name, or Enter if none fit)\n  > ")
			l2, ok := readLine(rd)
			if !ok {
				return Review{}, true
			}
			if c, found := matchCandidate(l2, cands); found {
				base.Verdict, base.Correct, base.Note = "fixed", c, note
			} else {
				base.Verdict, base.Note = "bad", strings.TrimSpace(note+" "+l2)
			}
			return base, false
		case actIgnore:
			base.Verdict = "ignore"
			return base, false
		case actSkip:
			return Review{}, false
		case actQuit:
			return Review{}, true
		}
		fmt.Println("  (didn't understand that)")
	}
}

func reviewAsk(rd *bufio.Reader, e Event, p payload) (Review, bool) {
	q, a := askQA(p)
	fmt.Println(reviewHeader(e))
	fmt.Printf("  %s/%s · %s · in=%d out=%d\n", e.Profile, e.Role, fmtDur(e.DurationMS), e.PromptTokens, e.CompletionTokens)
	fmt.Printf("  Q: %s\n", truncate(strings.Join(strings.Fields(q), " "), 400))
	fmt.Printf("  A: %s\n", truncate(strings.Join(strings.Fields(a), " "), 700))
	base := Review{Span: e.Span, Trace: e.Trace, Kind: "ask", Profile: e.Profile, Role: e.Role, Task: q}
	for {
		fmt.Print("  Helpful?  [Enter]=yes  n [why]=no  f=full answer  i=ignore  s=skip  q=quit\n  > ")
		line, ok := readLine(rd)
		if !ok {
			return Review{}, true
		}
		act, note := parseAskAnswer(line)
		switch act {
		case actOK:
			base.Verdict, base.Note = "ok", note
			return base, false
		case actBad:
			base.Verdict, base.Note = "bad", note
			return base, false
		case actFull:
			fmt.Printf("\n%s\n\n", a)
		case actIgnore:
			base.Verdict = "ignore"
			return base, false
		case actSkip:
			return Review{}, false
		case actQuit:
			return Review{}, true
		default:
			fmt.Println("  (didn't understand that)")
		}
	}
}

// exportReviews appends reviewed routing calls as eval cases, de-duplicated
// against what the file already holds, so it is safe to run repeatedly.
func exportReviews(path string) int {
	_, all := readReviews()
	latest := map[string]Review{}
	for _, r := range all {
		latest[r.Span] = r
	}
	seen := map[string]bool{}
	if data, err := os.ReadFile(path); err == nil {
		if cases, err := parseEvalCases(string(data)); err == nil {
			for _, c := range cases {
				seen[c.Question+"\x00"+c.Task] = true
			}
		}
	}
	var spans []string
	for s := range latest {
		spans = append(spans, s)
	}
	sort.Slice(spans, func(i, j int) bool { return latest[spans[i]].TS.Before(latest[spans[j]].TS) })

	var lines []string
	for _, s := range spans {
		r := latest[s]
		expect := ""
		switch {
		case r.Kind != "classify" || r.Task == "":
			continue
		case r.Verdict == "ok":
			expect = r.Choice
		case r.Verdict == "fixed":
			expect = r.Correct
		default:
			continue
		}
		key := r.Question + "\x00" + r.Task
		if seen[key] {
			continue
		}
		seen[key] = true
		b, _ := json.Marshal(evalCase{Question: r.Question, Task: r.Task, Expect: expect})
		lines = append(lines, string(b))
	}
	if len(lines) == 0 {
		fmt.Println("No new reviewed routing calls to export.")
		return 0
	}
	fi, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fail("%v", err)
	}
	defer fi.Close()
	if st, _ := fi.Stat(); st != nil && st.Size() == 0 {
		fmt.Fprintln(fi, "# Reviewed real-traffic routing cases (from `ai-mode review`). Contains real task text.")
	}
	for _, l := range lines {
		fmt.Fprintln(fi, l)
	}
	fmt.Printf("Appended %d case(s) to %s. Score them: ai-mode classify --eval %s\n", len(lines), path, path)
	return 0
}

// stdinIsTerminal is true for an interactive terminal; /dev/null and pipes are not.
func stdinIsTerminal() bool {
	st, err := os.Stdin.Stat()
	if err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(st, null) {
		return false
	}
	return true
}
