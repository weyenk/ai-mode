package app

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var systemoneWarmBody = map[string]any{
	"state": "ping",
	"questions": map[string]any{
		"route": map[string]any{
			"type":         "choice",
			"instructions": "pick",
			"criteria":     map[string]any{"coder": nil, "chief": nil},
		},
	},
}

type catalog map[string]map[string]any

func fetchCatalog(p Profile, timeout time.Duration) catalog {
	out := catalog{}
	data, ok := getJSON(p.BaseURL()+"/models", timeout)
	if !ok {
		return out
	}
	items, _ := data["data"].([]any)
	for _, it := range items {
		if m := asMap(it); m != nil && m["id"] != nil && asString(m["id"]) != "" {
			out[asString(m["id"])] = m
		}
	}
	return out
}

func modelStatus(entry map[string]any) string {
	if entry == nil {
		return ""
	}
	if st := asMap(entry["status"]); st != nil && st["value"] != nil {
		return asString(st["value"])
	}
	return ""
}

func modelIsDecision(entry map[string]any) bool {
	if arch := asMap(entry["architecture"]); arch != nil {
		if outs, ok := arch["output_modalities"].([]any); ok {
			for _, o := range outs {
				if asString(o) == "decisions" {
					return true
				}
			}
		}
	}
	return strings.EqualFold(asString(entry["id"]), "jev")
}

func triggerLoad(p Profile, id string, decision bool, timeout time.Duration) error {
	if decision {
		body := map[string]any{"model": id}
		for k, v := range systemoneWarmBody {
			body[k] = v
		}
		_, err := postJSON(p.BaseURL()+"/systemone", body, timeout)
		return err
	}
	body := map[string]any{
		"model":      id,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	}
	_, err := postJSON(p.BaseURL()+"/chat/completions", body, timeout)
	return err
}

// warmOne times warmOneInner and records a warm_model event, so download/load
// durations per role are visible in `ai-mode stats`.
func warmOne(p Profile, id string, timeout time.Duration, cat catalog) bool {
	start := time.Now()
	ok := warmOneInner(p, id, timeout, cat)
	e := Event{TS: start.UTC(), Kind: "warm_model", Trace: newTraceID(), Span: newSpanID(),
		Profile: p.Name, Role: id, DurationMS: time.Since(start).Milliseconds()}
	if !ok {
		e.Status, e.Error = "error", "warm failed"
	}
	emit(e)
	return ok
}

// warmOneInner loads/downloads one model until status=loaded or timeout.
func warmOneInner(p Profile, id string, timeout time.Duration, cat catalog) bool {
	if cat == nil {
		cat = fetchCatalog(p, 5*time.Second)
	}
	entry, ok := cat[id]
	if !ok {
		// Catalog can lag right after router start; refresh once.
		entry, ok = fetchCatalog(p, 5*time.Second)[id]
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "  %s: failed (not in preset /v1/models)\n", id)
		return false
	}
	st := modelStatus(entry)
	if st == "loaded" {
		fmt.Printf("  %s: ready\n", id)
		return true
	}

	decision := modelIsDecision(entry)
	probe := "completions"
	if decision {
		probe = "systemone"
	}
	fmt.Printf("  %s: warming via %s…\n", id, probe)

	type result struct{ err error }
	done := make(chan result, 1)
	go func() { done <- result{triggerLoad(p, id, decision, timeout)} }()
	var probeErr error
	probeDone := false

	deadline := time.Now().Add(timeout)
	last := st
	for time.Now().Before(deadline) {
		st = modelStatus(fetchCatalog(p, 3*time.Second)[id])
		if st == "loaded" {
			fmt.Printf("  %s: ready\n", id)
			return true
		}
		if st != "" && st != last {
			switch st {
			case "loading":
				fmt.Printf("  %s: loading…\n", id)
			case "unloaded":
				fmt.Printf("  %s: waiting for download…\n", id)
			}
			last = st
		}
		if !probeDone {
			select {
			case r := <-done:
				probeDone, probeErr = true, r.err
			default:
			}
		}
		if probeDone && probeErr != nil && st != "loaded" && st != "loading" {
			fmt.Fprintf(os.Stderr, "  %s: failed (%v)\n", id, probeErr)
			return false
		}
		time.Sleep(1200 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "  %s: failed (timeout after %ds)\n", id, int(timeout.Seconds()))
	return false
}

// warmModels warms sequentially (safe for models-max). Returns an exit code.
func warmModels(p Profile, names []string, timeout time.Duration) int {
	if len(names) == 0 {
		fmt.Println("No models configured to warm for this profile.")
		return 0
	}
	if _, ok := getJSON(p.BaseURL()+"/models", 3*time.Second); !ok {
		fmt.Fprintf(os.Stderr, "Router not reachable at %s\n", p.BaseURL())
		return 1
	}
	if len(names) > p.ModelsMax {
		fmt.Printf("Note: models-max=%d — only %d model(s) stay loaded at once; "+
			"sequential warm still downloads/prefetches weights for faster later requests.\n",
			p.ModelsMax, p.ModelsMax)
	}

	cat := fetchCatalog(p, 5*time.Second)
	var failed []string
	fmt.Printf("Warming %d model(s) on %s…\n", len(names), p.BaseURL())
	for i, id := range names {
		fmt.Printf("[%d/%d]\n", i+1, len(names))
		if !warmOne(p, id, timeout, cat) {
			failed = append(failed, id)
		}
		cat = fetchCatalog(p, 5*time.Second)
	}
	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "Warm failed: %s\n", strings.Join(failed, ", "))
		return 1
	}
	fmt.Println("All models ready.")
	bad, _ := ctxMismatchesFor(p, cat)
	for _, b := range bad {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", b)
	}
	return 0
}
