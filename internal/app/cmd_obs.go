package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// parseSince accepts 30m, 24h, 7d (or a bare number of hours).
func parseSince(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	unit := time.Hour
	num := s
	switch s[len(s)-1] {
	case 'm':
		unit, num = time.Minute, s[:len(s)-1]
	case 'h':
		num = s[:len(s)-1]
	case 'd':
		unit, num = 24*time.Hour, s[:len(s)-1]
	}
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("bad duration %q (use 30m, 24h, 7d)", s)
	}
	return time.Duration(n * float64(unit)), nil
}

func fmtDur(ms int64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		return fmt.Sprintf("%dm%02ds", ms/60_000, (ms%60_000)/1000)
	}
}

// warnings flags call outcomes worth a second look.
func warnings(e Event) []string {
	var w []string
	if e.FinishReason == "length" {
		w = append(w, "truncated(max_tokens)")
	}
	if e.Cold {
		w = append(w, "cold-start")
	}
	if e.Detail != "" && e.Kind == "ask" {
		w = append(w, e.Detail)
	}
	if e.Kind == "classify" && e.Decision != "" && e.Decision != "clear" {
		w = append(w, e.Decision)
	}
	return w
}

func eventLine(e Event) string {
	who := e.Profile
	if e.Role != "" {
		who += "/" + e.Role
	}
	line := fmt.Sprintf("%s %-10s %-24s %-5s %7s", e.TS.Local().Format("15:04:05"), e.Kind, who, e.Status, fmtDur(e.DurationMS))
	if e.Kind == "classify" {
		line += fmt.Sprintf("  %s→%s (%.2f)", e.Question, e.Choice, e.Confidence)
		if e.Caller != "" {
			line += " by=" + e.Caller
		}
	}
	if e.Kind == "ask" {
		line += fmt.Sprintf("  in=%d out=%d", e.PromptTokens, e.CompletionTokens)
		if e.GenTPS > 0 {
			line += fmt.Sprintf(" %.0ft/s", e.GenTPS)
		}
		if e.Caller != "" {
			line += " by=" + e.Caller
		}
	}
	if e.Trace != "" {
		line += " trace=" + e.Trace
	}
	if e.Span != "" {
		line += " span=" + e.Span
	}
	if w := warnings(e); len(w) > 0 {
		line += "  ⚠ " + strings.Join(w, ", ")
	}
	if e.Error != "" {
		line += "  ✗ " + e.Error
	}
	return line
}

func filterEvents(evs []Event, kind, role, trace, status string, since time.Duration) []Event {
	var out []Event
	cutoff := time.Time{}
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}
	for _, e := range evs {
		switch {
		case kind != "" && e.Kind != kind,
			role != "" && e.Role != role,
			trace != "" && e.Trace != trace,
			status != "" && e.Status != status,
			!cutoff.IsZero() && e.TS.Before(cutoff):
			continue
		}
		out = append(out, e)
	}
	return out
}

func cmdEvents(args []string) int {
	fs := newFlags("events")
	n := fs.Int("n", 30, "Most recent events to show (0 = all)")
	followF := fs.Bool("f", false, "Follow new events")
	kind := fs.String("kind", "", "Filter: ask, classify, use, stop, restart, warm, warm_model")
	role := fs.String("role", "", "Filter by role")
	trace := fs.String("trace", "", "Filter by trace id")
	status := fs.String("status", "", "Filter: ok or error")
	sinceF := fs.String("since", "", "Only events newer than e.g. 30m, 24h, 7d")
	asJSON := fs.Bool("json", false, "Raw JSONL")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	since, err := parseSince(*sinceF)
	if err != nil {
		return fail("%v", err)
	}
	evs := filterEvents(readEvents(), *kind, *role, *trace, *status, since)
	if *n > 0 && len(evs) > *n {
		evs = evs[len(evs)-*n:]
	}
	show := func(e Event) {
		if *asJSON {
			b, _ := jsonLine(e)
			fmt.Println(b)
		} else {
			fmt.Println(eventLine(e))
		}
	}
	for _, e := range evs {
		show(e)
	}
	if len(evs) == 0 && !*followF {
		fmt.Fprintf(os.Stderr, "No events yet (%s)\n", eventsFile())
	}
	if *followF {
		seen := len(readEvents())
		for {
			time.Sleep(500 * time.Millisecond)
			all := readEvents()
			if len(all) < seen { // rotated
				seen = 0
			}
			for _, e := range filterEvents(all[seen:], *kind, *role, *trace, *status, 0) {
				show(e)
			}
			seen = len(all)
		}
	}
	return 0
}

// ---- trace -----------------------------------------------------------------

type traceSummary struct {
	id      string
	start   time.Time
	calls   int
	errs    int
	totalMS int64
	roles   []string
	callers map[string]bool
}

func summarizeTraces(evs []Event) []traceSummary {
	byID := map[string]*traceSummary{}
	var order []string
	for _, e := range evs {
		if !isCall(e) || e.Trace == "" {
			continue
		}
		t := byID[e.Trace]
		if t == nil {
			t = &traceSummary{id: e.Trace, start: e.TS, callers: map[string]bool{}}
			byID[e.Trace] = t
			order = append(order, e.Trace)
		}
		t.calls++
		t.totalMS += e.DurationMS
		if e.Status != "ok" {
			t.errs++
		}
		if !contains(t.roles, e.Role) {
			t.roles = append(t.roles, e.Role)
		}
	}
	out := make([]traceSummary, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

func cmdTrace(args []string) int {
	fs := newFlags("trace")
	list := fs.Bool("list", false, "List recent traces")
	n := fs.Int("n", 10, "With --list: how many")
	full := fs.Bool("full", false, "Show captured request/response for each span")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) == 1 && pos[0] == "new" {
		fmt.Println(newTraceID())
		return 0
	}
	evs := readEvents()
	sums := summarizeTraces(evs)

	if *list {
		if len(sums) > *n {
			sums = sums[len(sums)-*n:]
		}
		for _, t := range sums {
			errs := ""
			if t.errs > 0 {
				errs = fmt.Sprintf("  ✗ %d error(s)", t.errs)
			}
			fmt.Printf("%s  %s  %d call(s)  %7s  roles=%s%s\n", t.id, t.start.Local().Format("01-02 15:04:05"),
				t.calls, fmtDur(t.totalMS), strings.Join(t.roles, ","), errs)
		}
		return 0
	}

	id := ""
	if len(pos) > 0 {
		id = pos[0]
	} else if len(sums) > 0 {
		id = sums[len(sums)-1].id
	}
	if id == "" {
		return fail("No traces yet. Run: ai-mode ask <role> \"...\"")
	}
	var spans []Event
	for _, e := range filterEvents(evs, "", "", id, "", 0) {
		if isCall(e) {
			spans = append(spans, e)
		}
	}
	if len(spans) == 0 {
		return fail("No spans for trace %s (try: ai-mode trace --list)", id)
	}
	printTrace(id, spans, *full)
	return 0
}

func printTrace(id string, spans []Event, full bool) {
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].TS.Before(spans[j].TS) })
	children := map[string][]Event{}
	known := map[string]bool{}
	for _, s := range spans {
		known[s.Span] = true
	}
	var roots []Event
	for _, s := range spans {
		if s.Parent == "" || !known[s.Parent] {
			roots = append(roots, s)
		} else {
			children[s.Parent] = append(children[s.Parent], s)
		}
	}
	t0 := spans[0].TS
	var total int64
	for _, s := range spans {
		if end := s.TS.Sub(t0).Milliseconds() + s.DurationMS; end > total {
			total = end
		}
	}
	fmt.Printf("trace %s  %s  %d span(s)  wall %s\n\n", id, t0.Local().Format("2006-01-02 15:04:05"), len(spans), fmtDur(total))

	var walk func(e Event, depth int)
	walk = func(e Event, depth int) {
		indent := strings.Repeat("  ", depth)
		mark := "✓"
		if e.Status != "ok" {
			mark = "✗"
		}
		fmt.Printf("%s%s %s/%s  +%s  %s", indent, mark, e.Profile, e.Role,
			fmtDur(e.TS.Sub(t0).Milliseconds()), fmtDur(e.DurationMS))
		if e.Kind == "classify" {
			fmt.Printf("  classify %s→%s (%.2f)", e.Question, e.Choice, e.Confidence)
		} else {
			fmt.Printf("  in=%d out=%d", e.PromptTokens, e.CompletionTokens)
		}
		if e.GenTPS > 0 {
			fmt.Printf(" %.0ft/s", e.GenTPS)
		}
		fmt.Printf("  [%s] by=%s\n", e.Span, firstNonEmpty(e.Caller, "?"))
		if w := warnings(e); len(w) > 0 {
			fmt.Printf("%s  ⚠ %s\n", indent, strings.Join(w, ", "))
		}
		if e.Error != "" {
			fmt.Printf("%s  error: %s\n", indent, e.Error)
		}
		if full && e.Payload != "" {
			printPayload(indent+"  ", e.Payload)
		}
		for _, c := range children[e.Span] {
			walk(c, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
}

func printPayload(indent, rel string) {
	data, err := os.ReadFile(filepath.Join(tracesDir(), rel))
	if err != nil {
		fmt.Printf("%s(payload missing: %s)\n", indent, rel)
		return
	}
	var p map[string]any
	if jsonUnmarshal(data, &p) != nil {
		return
	}
	if req := asMap(p["request"]); req != nil {
		if msgs, ok := req["messages"].([]any); ok {
			for _, m := range msgs {
				mm := asMap(m)
				if asString(mm["role"]) == "user" {
					fmt.Printf("%sQ: %s\n", indent, indentBlock(asString(mm["content"]), indent+"   "))
				}
			}
		}
	}
	if req := asMap(p["request"]); req != nil && req["state"] != nil {
		fmt.Printf("%sTask: %s\n", indent, indentBlock(asString(req["state"]), indent+"      "))
	}
	if resp := asMap(p["response"]); resp != nil {
		if ch, _ := resp["choices"].([]any); len(ch) > 0 {
			if msg := asMap(asMap(ch[0])["message"]); msg != nil {
				fmt.Printf("%sA: %s\n", indent, indentBlock(assistantText(msg), indent+"   "))
			}
		}
	}
}

func indentBlock(s, indent string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n"+indent)
}

// ---- stats -----------------------------------------------------------------

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1)*p + 0.5)
	return sorted[idx]
}

func cmdStats(args []string) int {
	fs := newFlags("stats")
	sinceF := fs.String("since", "24h", "Window, e.g. 30m, 24h, 7d (empty = all time)")
	role := fs.String("role", "", "Only this role")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	since, err := parseSince(*sinceF)
	if err != nil {
		return fail("%v", err)
	}
	evs := filterEvents(readEvents(), "", *role, "", "", since)

	type agg struct {
		calls, errs, trunc, cold int
		durs                     []int64
		in, out                  int
		tps                      []float64
	}
	groups := map[string]*agg{}
	warm := map[string]*agg{}
	for _, e := range evs {
		var m map[string]*agg
		switch e.Kind {
		case "ask":
			m = groups
		case "warm_model":
			m = warm
		default:
			continue
		}
		key := e.Profile + "/" + e.Role
		a := m[key]
		if a == nil {
			a = &agg{}
			m[key] = a
		}
		a.calls++
		a.durs = append(a.durs, e.DurationMS)
		a.in += e.PromptTokens
		a.out += e.CompletionTokens
		if e.Status != "ok" {
			a.errs++
		}
		if e.FinishReason == "length" {
			a.trunc++
		}
		if e.Cold {
			a.cold++
		}
		if e.GenTPS > 0 {
			a.tps = append(a.tps, e.GenTPS)
		}
	}
	type cagg struct {
		calls, errs, unclear int
		durs                 []int64
		conf                 float64
		picks                map[string]int
	}
	cls := map[string]*cagg{}
	for _, e := range evs {
		if e.Kind != "classify" {
			continue
		}
		key := e.Profile + "/" + e.Question
		a := cls[key]
		if a == nil {
			a = &cagg{picks: map[string]int{}}
			cls[key] = a
		}
		a.calls++
		a.durs = append(a.durs, e.DurationMS)
		a.conf += e.Confidence
		if e.Status != "ok" {
			a.errs++
		} else {
			a.picks[e.Choice]++
		}
		if e.Decision != "" && e.Decision != "clear" {
			a.unclear++
		}
	}
	if len(groups) == 0 && len(warm) == 0 && len(cls) == 0 {
		fmt.Printf("No events in window (%s).\n", firstNonEmpty(*sinceF, "all time"))
		return 0
	}
	keys := func(m map[string]*agg) []string {
		ks := make([]string, 0, len(m))
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return ks
	}
	win := firstNonEmpty(*sinceF, "all time")
	if len(groups) > 0 {
		fmt.Printf("ask — last %s\n", win)
		fmt.Printf("%-26s %6s %4s %6s %5s %8s %8s %9s %9s %7s\n", "profile/role", "calls", "err", "trunc", "cold", "p50", "p95", "tok in", "tok out", "gen t/s")
		for _, k := range keys(groups) {
			a := groups[k]
			sort.Slice(a.durs, func(i, j int) bool { return a.durs[i] < a.durs[j] })
			avg := 0.0
			for _, t := range a.tps {
				avg += t
			}
			if len(a.tps) > 0 {
				avg /= float64(len(a.tps))
			}
			fmt.Printf("%-26s %6d %4d %6d %5d %8s %8s %9d %9d %7.0f\n", k, a.calls, a.errs, a.trunc, a.cold,
				fmtDur(percentile(a.durs, 0.5)), fmtDur(percentile(a.durs, 0.95)), a.in, a.out, avg)
		}
	}
	if len(cls) > 0 {
		fmt.Printf("\nclassify (jev routing) — last %s\n", win)
		fmt.Printf("%-26s %6s %4s %8s %8s %8s  %s\n", "profile/question", "calls", "err", "p50", "avg conf", "unclear", "picks")
		ks := make([]string, 0, len(cls))
		for k := range cls {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			a := cls[k]
			sort.Slice(a.durs, func(i, j int) bool { return a.durs[i] < a.durs[j] })
			var picks []string
			for name, n := range a.picks {
				picks = append(picks, fmt.Sprintf("%s×%d", name, n))
			}
			sort.Strings(picks)
			fmt.Printf("%-26s %6d %4d %8s %8.2f %8d  %s\n", k, a.calls, a.errs, fmtDur(percentile(a.durs, 0.5)),
				a.conf/float64(a.calls), a.unclear, strings.Join(picks, " "))
		}
	}
	if len(warm) > 0 {
		fmt.Printf("\nwarm (load/download) — last %s\n", win)
		fmt.Printf("%-26s %6s %4s %9s %9s\n", "profile/role", "runs", "err", "median", "max")
		for _, k := range keys(warm) {
			a := warm[k]
			sort.Slice(a.durs, func(i, j int) bool { return a.durs[i] < a.durs[j] })
			fmt.Printf("%-26s %6d %4d %9s %9s\n", k, a.calls, a.errs, fmtDur(percentile(a.durs, 0.5)), fmtDur(a.durs[len(a.durs)-1]))
		}
	}
	return 0
}

// ---- ps --------------------------------------------------------------------

func cmdPS(args []string) int {
	fs := newFlags("ps")
	profile := fs.String("profile", "", "Profile (default: active)")
	all := fs.Bool("all", false, "Include non-role entries (raw HF cache models)")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	p, code := activeProfile(*profile, "Use: ai-mode use <name>")
	if code != 0 {
		return code
	}
	cat := fetchCatalog(p, 5*time.Second)
	if len(cat) == 0 {
		return fail("API not reachable at %s", p.BaseURL())
	}
	roles := iniSectionsFile(p.Ini)
	ids := make([]string, 0, len(cat))
	for id := range cat {
		if *all || contains(roles, id) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	// Last-call stats per role from the event log.
	last := map[string]Event{}
	for _, e := range readEvents() {
		if e.Kind == "ask" && e.Profile == p.Name {
			last[e.Role] = e
		}
	}
	fmt.Printf("%s  %s  models-max=%d\n\n", p.Name, p.BaseURL(), p.ModelsMax)
	fmt.Printf("%-34s %-9s %8s  %s\n", "model", "status", "ctx", "last ask")
	for _, id := range ids {
		m := cat[id]
		ctx := "-"
		if st := asMap(m["status"]); st != nil {
			if args, ok := st["args"].([]any); ok {
				for i, a := range args {
					if (asString(a) == "--ctx-size" || asString(a) == "-c") && i+1 < len(args) {
						ctx = asString(args[i+1])
					}
				}
			}
		}
		la := "-"
		if e, ok := last[id]; ok {
			la = fmt.Sprintf("%s ago (%s, %s)", fmtDur(time.Since(e.TS).Milliseconds()), e.Status, fmtDur(e.DurationMS))
		}
		fmt.Printf("%-34s %-9s %8s  %s\n", id, firstNonEmpty(modelStatus(m), "?"), ctx, la)
	}
	return 0
}
