package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func readUserMessage(parts []string) string {
	if len(parts) > 0 {
		return strings.TrimSpace(strings.Join(parts, " "))
	}
	if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice == 0 {
		data, _ := io.ReadAll(os.Stdin)
		return strings.TrimSpace(string(data))
	}
	return ""
}

func assistantText(msg map[string]any) string {
	for _, k := range []string{"content", "reasoning_content"} {
		if v := msg[k]; v != nil {
			if s := strings.TrimSpace(asString(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

func cmdAsk(args []string) int {
	fs := newFlags("ask")
	profile := fs.String("profile", "", "Profile (default: active)")
	maxTokens := fs.Int("max-tokens", 2048, "Max tokens")
	timeout := fs.Float64("timeout", 600, "Request timeout seconds")
	asJSON := fs.Bool("json", false, "Print full API response JSON")
	raw := fs.Bool("raw", false, "Print message.content only (no reasoning fallback)")
	traceF := fs.String("trace", "", "Trace id to attach this call to (default: $AI_MODE_TRACE_ID or new)")
	parentF := fs.String("parent", "", "Parent span id (default: $AI_MODE_PARENT_SPAN)")
	callerF := fs.String("caller", "", "Who is asking, e.g. chief (default: $AI_MODE_CALLER or cli)")
	verbose := fs.Bool("v", false, "Print trace/span ids and timings to stderr")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) < 1 {
		return fail("usage: ai-mode ask <role> [message...] [--profile P] [--max-tokens N] [--timeout S] [--json] [--raw] [--trace ID] [--parent SPAN] [--caller NAME] [-v]")
	}
	role := pos[0]
	if role == "jev" {
		return fail("Role 'jev' is a router, not a chat model. Use chat/completions with another role.")
	}

	p, code := activeProfile(*profile, "Use: ai-mode use <name> or ai-mode ask --profile <name>")
	if code != 0 {
		return code
	}
	if !contains(iniSectionsFile(p.Ini), role) {
		return fail("Role '%s' is not configured in %s", role, filepathBase(p.Ini))
	}
	_, _, system, code := agentForRole(role)
	if code != 0 {
		return code
	}
	user := readUserMessage(pos[1:])
	if user == "" {
		return fail("Empty question: pass a message argument or pipe stdin.")
	}

	// From here on the call is traced, success or failure.
	trace, parent := traceContext(*traceF, *parentF)
	ev := Event{
		Kind: "ask", Trace: trace, Span: newSpanID(), Parent: parent,
		Caller:  firstNonEmpty(*callerF, os.Getenv("AI_MODE_CALLER"), "cli"),
		Profile: p.Name, Role: role, MaxTokens: *maxTokens,
		PromptChars: len(user), SystemSHA: shortSHA(system),
	}
	start := time.Now()
	ev.TS = start.UTC()
	finish := func(code int, errMsg string, req, resp any) int {
		ev.DurationMS = time.Since(start).Milliseconds()
		if errMsg != "" {
			ev.Status, ev.Error = "error", errMsg
		}
		ev.Payload = savePayload(ev.Trace, ev.Span, req, resp)
		emit(ev)
		pruneTraces()
		if *verbose {
			fmt.Fprintf(os.Stderr, "trace=%s span=%s %s %dms in=%d out=%d finish=%s\n",
				ev.Trace, ev.Span, ev.Status, ev.DurationMS, ev.PromptTokens, ev.CompletionTokens, ev.FinishReason)
		}
		if errMsg != "" {
			fmt.Fprintln(os.Stderr, errMsg)
		}
		return code
	}

	hc := secs(*timeout)
	if hc > 30*time.Second {
		hc = 30 * time.Second
	}
	models, reachable := getJSON(p.BaseURL()+"/models", hc)
	if !reachable {
		return finish(1, fmt.Sprintf("API not reachable at %s", p.BaseURL()), nil, nil)
	}
	if items, _ := models["data"].([]any); items != nil {
		for _, it := range items {
			if m := asMap(it); m != nil && asString(m["id"]) == role {
				ev.Cold = modelStatus(m) != "loaded"
			}
		}
	}

	body := map[string]any{
		"model":      role,
		"max_tokens": *maxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	if role != "architect" {
		body["chat_template_kwargs"] = map[string]any{"enable_thinking": false}
	}
	data, err := postJSON(p.BaseURL()+"/chat/completions", body, secs(*timeout))
	if err != nil {
		return finish(1, err.Error(), body, nil)
	}
	recordUsage(&ev, data)

	if *asJSON {
		ev.ResponseChars = len(prettyJSON(data))
		fmt.Print(prettyJSON(data))
		return finish(0, "", body, data)
	}

	choices, _ := data["choices"].([]any)
	if len(choices) == 0 {
		return finish(1, "No choices in response", body, data)
	}
	first := asMap(choices[0])
	if first == nil {
		return finish(1, "Invalid choice in response", body, data)
	}
	msg := asMap(first["message"])
	if msg == nil {
		return finish(1, "No message in response choice", body, data)
	}

	var text string
	if *raw {
		if msg["content"] == nil {
			return finish(1, "Empty message content", body, data)
		}
		text = asString(msg["content"])
	} else if text = assistantText(msg); text == "" {
		return finish(1, "Empty assistant content (content and reasoning_content)", body, data)
	}
	ev.ResponseChars = len(text)
	if strings.TrimSpace(asString(msg["content"])) == "" {
		ev.Detail = "content empty; used reasoning_content"
	}
	fmt.Print(text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Println()
	}
	return finish(0, "", body, data)
}

// recordUsage copies llama-server's usage/timings into the event.
func recordUsage(ev *Event, data map[string]any) {
	if u := asMap(data["usage"]); u != nil {
		ev.PromptTokens = toInt(u["prompt_tokens"])
		ev.CompletionTokens = toInt(u["completion_tokens"])
		if d := asMap(u["prompt_tokens_details"]); d != nil {
			ev.CachedTokens = toInt(d["cached_tokens"])
		}
	}
	if t := asMap(data["timings"]); t != nil {
		ev.PromptTPS = toFloat(t["prompt_per_second"])
		ev.GenTPS = toFloat(t["predicted_per_second"])
	}
	if ch, _ := data["choices"].([]any); len(ch) > 0 {
		if c := asMap(ch[0]); c != nil {
			ev.FinishReason = asString(c["finish_reason"])
		}
	}
}

func toFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}

func toInt(v any) int { return int(toFloat(v)) }

func filepathBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
