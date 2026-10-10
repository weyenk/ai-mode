package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The debug proxy sits on a profile's public port while llama-server runs on a
// hidden upstream port, so clients (Qwen Code, curl, …) keep their configured
// baseUrl and every chat completion is recorded in the event log. It exists only
// when `ai-mode use <profile> --debug` started it.

const (
	internalHeader  = "X-AI-Mode-Internal" // set by the CLI's own calls; they are already logged
	maxCaptureBytes = 8 << 20              // response bytes kept for parsing
	maxRequestBytes = 64 << 20
	maxTextKeep     = 50 << 10 // response text kept in a payload
	maxUserKeep     = 20 << 10 // last user message kept in a "light" payload
	maxFullKeep     = 2 << 20  // request size cap for AI_MODE_PROXY_CAPTURE=full
)

func upstreamPort(p Profile) int { return p.Port + 10000 }

func (p Profile) UpstreamBaseURL() string {
	return fmt.Sprintf("http://%s:%d/v1", p.ClientHost(), upstreamPort(p))
}

func (p Profile) ProxyLogFile() string { return logDir() + "/" + p.Name + ".proxy.log" }

// ---- parsing ---------------------------------------------------------------

type respInfo struct {
	Content, Reasoning string
	Finish             string
	Usage, Timings     map[string]any
	ToolNames          []string
	ToolCalls          int
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// parseSSE folds an OpenAI-style event stream into one response summary.
func parseSSE(data []byte) respInfo {
	var r respInfo
	var content, reasoning strings.Builder
	tools := map[int]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		if u := asMap(chunk["usage"]); u != nil {
			r.Usage = u
		}
		if t := asMap(chunk["timings"]); t != nil {
			r.Timings = t
		}
		ch, _ := chunk["choices"].([]any)
		if len(ch) == 0 {
			continue
		}
		c := asMap(ch[0])
		if fr := asString(c["finish_reason"]); fr != "" {
			r.Finish = fr
		}
		d := asMap(c["delta"])
		if d == nil {
			continue
		}
		if s, ok := d["content"].(string); ok && content.Len() < maxTextKeep {
			content.WriteString(s)
		}
		if s, ok := d["reasoning_content"].(string); ok && reasoning.Len() < maxTextKeep {
			reasoning.WriteString(s)
		}
		if tcs, ok := d["tool_calls"].([]any); ok {
			for i, tc := range tcs {
				m := asMap(tc)
				idx := i
				if f, ok := m["index"].(float64); ok {
					idx = int(f)
				}
				name := asString(asMap(m["function"])["name"])
				if _, seen := tools[idx]; !seen || name != "" {
					tools[idx] = firstNonEmpty(name, tools[idx])
				}
			}
		}
	}
	r.Content, r.Reasoning = content.String(), reasoning.String()
	for i := 0; i < len(tools)+8 && len(r.ToolNames) < len(tools); i++ {
		if n, ok := tools[i]; ok {
			r.ToolNames = append(r.ToolNames, n)
		}
	}
	r.ToolCalls = len(tools)
	return r
}

func parseJSONResp(data []byte) respInfo {
	var r respInfo
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return r
	}
	r.Usage, r.Timings = asMap(m["usage"]), asMap(m["timings"])
	if ch, _ := m["choices"].([]any); len(ch) > 0 {
		c := asMap(ch[0])
		r.Finish = asString(c["finish_reason"])
		if msg := asMap(c["message"]); msg != nil {
			r.Content = clip(asString(msg["content"]), maxTextKeep)
			r.Reasoning = clip(asString(msg["reasoning_content"]), maxTextKeep)
			if tcs, ok := msg["tool_calls"].([]any); ok {
				r.ToolCalls = len(tcs)
				for _, tc := range tcs {
					r.ToolNames = append(r.ToolNames, asString(asMap(asMap(tc)["function"])["name"]))
				}
			}
		}
	}
	return r
}

// contentText flattens a message's content, which may be a string or a list of parts.
func contentText(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, p := range c {
			if m := asMap(p); m != nil {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// ---- per-request recorder --------------------------------------------------

type recKey struct{}

type recorder struct {
	profile string
	r       *http.Request
	start   time.Time
	body    map[string]any // parsed request, nil if not JSON
	stream  bool

	mu        sync.Mutex
	status    int
	firstByte time.Duration
	buf       bytes.Buffer
	capped    bool
	done      sync.Once
}

func callerFromUA(r *http.Request) string {
	if c := r.Header.Get("X-AI-Mode-Caller"); c != "" {
		return c
	}
	ua := strings.ToLower(r.UserAgent())
	for _, k := range []string{"qwen", "cursor", "curl", "python", "node", "openai"} {
		if strings.Contains(ua, k) {
			return k
		}
	}
	return "client"
}

func (rec *recorder) write(p []byte) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.firstByte == 0 && len(p) > 0 {
		rec.firstByte = time.Since(rec.start)
	}
	if room := maxCaptureBytes - rec.buf.Len(); room > 0 {
		if len(p) > room {
			p, rec.capped = p[:room], true
		}
		rec.buf.Write(p)
	} else {
		rec.capped = true
	}
}

// finish records the call exactly once. err is nil/io.EOF for a clean end.
func (rec *recorder) finish(err error) {
	rec.done.Do(func() {
		rec.mu.Lock()
		raw := append([]byte(nil), rec.buf.Bytes()...)
		first := rec.firstByte
		rec.mu.Unlock()

		var info respInfo
		if rec.stream {
			info = parseSSE(raw)
		} else {
			info = parseJSONResp(raw)
		}

		ev := Event{
			TS: rec.start.UTC(), Kind: "ask", Source: "proxy",
			Trace:   firstNonEmpty(rec.r.Header.Get("X-AI-Mode-Trace"), newTraceID()),
			Span:    newSpanID(),
			Parent:  rec.r.Header.Get("X-AI-Mode-Parent"),
			Caller:  callerFromUA(rec.r),
			Profile: rec.profile, Status: "ok",
			DurationMS: time.Since(rec.start).Milliseconds(),
			TTFTMS:     first.Milliseconds(),
			ToolCalls:  info.ToolCalls,
		}
		if rec.body != nil {
			ev.Role = asString(rec.body["model"])
			ev.MaxTokens = toInt(rec.body["max_tokens"])
			if ev.MaxTokens == 0 {
				ev.MaxTokens = toInt(rec.body["max_completion_tokens"])
			}
			msgs, _ := rec.body["messages"].([]any)
			for _, m := range msgs {
				mm := asMap(m)
				ev.PromptChars += len(contentText(mm["content"]))
				if ev.SystemSHA == "" && asString(mm["role"]) == "system" {
					ev.SystemSHA = shortSHA(contentText(mm["content"]))
				}
			}
		}
		ev.ResponseChars = len(info.Content) + len(info.Reasoning)
		recordUsage(&ev, map[string]any{
			"usage": info.Usage, "timings": info.Timings,
			"choices": []any{map[string]any{"finish_reason": info.Finish}},
		})

		switch {
		case err != nil && err != io.EOF:
			ev.Status = "error"
			if rec.r.Context().Err() != nil {
				ev.Error = "client cancelled"
			} else {
				ev.Error = err.Error()
			}
		case rec.status >= 400:
			ev.Status = "error"
			ev.Error = fmt.Sprintf("HTTP %d: %s", rec.status, clip(strings.TrimSpace(string(raw)), 300))
		}

		resp := map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": info.Finish,
				"message": map[string]any{"role": "assistant", "content": info.Content,
					"reasoning_content": info.Reasoning, "tool_calls": info.ToolNames},
			}},
			"usage": info.Usage, "timings": info.Timings,
		}
		ev.Payload = savePayload(ev.Trace, ev.Span, rec.requestPayload(), resp)
		emit(ev)
		pruneTraces()
	})
}

// requestPayload keeps the request small: model, counts, and the last user
// message (a chief turn resends the whole conversation, so storing all of it
// every call would balloon the log). AI_MODE_PROXY_CAPTURE=full keeps it all.
func (rec *recorder) requestPayload() any {
	if rec.body == nil {
		return nil
	}
	if envOr("AI_MODE_PROXY_CAPTURE", configValue("AI_MODE_PROXY_CAPTURE")) == "full" {
		if b, err := json.Marshal(rec.body); err == nil && len(b) <= maxFullKeep {
			return rec.body
		}
	}
	msgs, _ := rec.body["messages"].([]any)
	tools, _ := rec.body["tools"].([]any)
	lastUser := ""
	for _, m := range msgs {
		if mm := asMap(m); asString(mm["role"]) == "user" {
			lastUser = contentText(mm["content"])
		}
	}
	return map[string]any{
		"model": rec.body["model"], "stream": rec.stream, "max_tokens": rec.body["max_tokens"],
		"message_count": len(msgs), "tool_count": len(tools), "light": true,
		"messages": []map[string]string{{"role": "user", "content": clip(lastUser, maxUserKeep)}},
	}
}

// captureBody tees the upstream response into the recorder without delaying it.
type captureBody struct {
	rc  io.ReadCloser
	rec *recorder
}

func (c *captureBody) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	if n > 0 {
		c.rec.write(p[:n])
	}
	if err != nil {
		c.rec.finish(err)
	}
	return n, err
}

func (c *captureBody) Close() error {
	err := c.rc.Close()
	c.rec.finish(io.ErrUnexpectedEOF) // no-op if the body already ended cleanly
	return err
}

// ---- handler ---------------------------------------------------------------

type proxyHandler struct {
	profile string
	rp      *httputil.ReverseProxy
}

func newProxyHandler(profile string, upstream *url.URL) *proxyHandler {
	h := &proxyHandler{profile: profile}
	h.rp = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.Out.Header.Del("Accept-Encoding") // keep bodies plain so they can be parsed
		},
		FlushInterval: -1, // stream chunks through immediately
		ModifyResponse: func(resp *http.Response) error {
			if rec, ok := resp.Request.Context().Value(recKey{}).(*recorder); ok {
				rec.status = resp.StatusCode
				resp.Body = &captureBody{rc: resp.Body, rec: rec}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if rec, ok := r.Context().Value(recKey{}).(*recorder); ok {
				rec.finish(err)
			}
			http.Error(w, "ai-mode proxy: upstream error: "+err.Error(), http.StatusBadGateway)
		},
	}
	return h
}

func shouldRecord(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/chat/completions") &&
		r.Header.Get(internalHeader) == ""
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !shouldRecord(r) {
		h.rp.ServeHTTP(w, r)
		return
	}
	rec := &recorder{profile: h.profile, start: time.Now(), r: r}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes))
	r.Body.Close()
	if err != nil {
		http.Error(w, "ai-mode proxy: reading request: "+err.Error(), http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	if json.Unmarshal(body, &rec.body) == nil && rec.body != nil {
		rec.stream, _ = rec.body["stream"].(bool)
	}
	h.rp.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), recKey{}, rec)))
}

// ---- command ---------------------------------------------------------------

func cmdProxy(args []string) int {
	fs := newFlags("proxy")
	profile := fs.String("profile", "", "Profile (default: active)")
	listen := fs.String("listen", "", "Listen address (default: the profile's host:port)")
	upstream := fs.String("upstream", "", "Upstream host:port (default: the profile's port + 10000)")
	if _, code, ok := parse(fs, args); !ok {
		return code
	}
	p, code := activeProfile(*profile, "Use: ai-mode proxy --profile <name>")
	if code != 0 {
		return code
	}
	if *listen == "" {
		*listen = fmt.Sprintf("%s:%d", p.Host, p.Port)
	}
	if *upstream == "" {
		*upstream = fmt.Sprintf("%s:%d", p.ClientHost(), upstreamPort(p))
	}
	target, err := url.Parse("http://" + *upstream)
	if err != nil {
		return fail("bad --upstream: %v", err)
	}

	srv := &http.Server{Addr: *listen, Handler: newProxyHandler(p.Name, target), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	fmt.Printf("%s ai-mode proxy: %s → %s (profile %s)\n", nowISO(), *listen, *upstream, p.Name)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail("proxy: %v", err)
	}
	return 0
}
