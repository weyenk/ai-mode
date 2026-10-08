package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const sseSample = `data: {"choices":[{"delta":{"role":"assistant","content":""}}]}

data: {"choices":[{"delta":{"content":"Hel"}}]}

data: {"choices":[{"delta":{"reasoning_content":"think"}}]}

data: {"choices":[{"delta":{"content":"lo"}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"read_file","arguments":"{}"}}]}}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":120,"completion_tokens":9,"prompt_tokens_details":{"cached_tokens":100}},"timings":{"predicted_per_second":55.5,"prompt_per_second":900}}

data: [DONE]

`

func TestParseSSE(t *testing.T) {
	r := parseSSE([]byte(sseSample))
	if r.Content != "Hello" || r.Reasoning != "think" || r.Finish != "tool_calls" || r.ToolCalls != 1 || r.ToolNames[0] != "read_file" {
		t.Fatalf("%+v", r)
	}
	var ev Event
	recordUsage(&ev, map[string]any{"usage": r.Usage, "timings": r.Timings, "choices": []any{map[string]any{"finish_reason": r.Finish}}})
	if ev.PromptTokens != 120 || ev.CompletionTokens != 9 || ev.CachedTokens != 100 || ev.GenTPS != 55.5 {
		t.Fatalf("%+v", ev)
	}
}

func TestParseJSONResp(t *testing.T) {
	r := parseJSONResp([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"hi","tool_calls":[{"function":{"name":"x"}}]}}],"usage":{"prompt_tokens":3}}`))
	if r.Content != "hi" || r.Finish != "stop" || r.ToolCalls != 1 || toInt(r.Usage["prompt_tokens"]) != 3 {
		t.Fatalf("%+v", r)
	}
}

func proxyFor(t *testing.T, upstream http.Handler) *httptest.Server {
	t.Helper()
	t.Setenv("AI_MODE_STATE", t.TempDir())
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	u, _ := url.Parse(up.URL)
	px := httptest.NewServer(newProxyHandler("dev-shop", u))
	t.Cleanup(px.Close)
	return px
}

func lastEvent(t *testing.T) Event {
	t.Helper()
	// the recorder finishes asynchronously after the client sees EOF
	for i := 0; i < 50; i++ {
		if evs := readEvents(); len(evs) > 0 {
			return evs[len(evs)-1]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no event recorded")
	return Event{}
}

func TestProxyStreamsAndRecords(t *testing.T) {
	release := make(chan struct{})
	px := proxyFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		first, rest, _ := strings.Cut(sseSample, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}")
		io.WriteString(w, first)
		w.(http.Flusher).Flush()
		<-release // the proxy must deliver the first chunk before the upstream finishes
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}"+rest)
	}))
	req, _ := http.NewRequest("POST", px.URL+"/v1/chat/completions", strings.NewReader(
		`{"model":"chief","stream":true,"max_tokens":50,"messages":[{"role":"system","content":"sys"},{"role":"user","content":"hello there"}]}`))
	req.Header.Set("User-Agent", "QwenCode/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	got := make(chan int, 1)
	go func() { n, _ := resp.Body.Read(buf); got <- n }()
	select {
	case n := <-got:
		if n == 0 {
			t.Fatal("empty first read")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first chunk was buffered until the upstream finished (streaming broken)")
	}
	close(release)
	rest, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(buf)+string(rest), "[DONE]") {
		t.Fatal("body not passed through intact")
	}

	ev := lastEvent(t)
	if ev.Kind != "ask" || ev.Source != "proxy" || ev.Role != "chief" || ev.Caller != "qwen" || ev.Status != "ok" ||
		ev.PromptTokens != 120 || ev.CompletionTokens != 9 || ev.ToolCalls != 1 || ev.FinishReason != "tool_calls" ||
		ev.MaxTokens != 50 || ev.PromptChars != len("sys")+len("hello there") || ev.TTFTMS < 0 || ev.Payload == "" {
		t.Fatalf("%+v", ev)
	}
	p, ok := loadPayload(ev.Payload)
	if !ok {
		t.Fatal("payload missing")
	}
	if q, a := askQA(p); q != "hello there" || a != "Hello" {
		t.Fatalf("payload q=%q a=%q", q, a)
	}
}

func TestProxyNonStreamAndInternalHeader(t *testing.T) {
	px := proxyFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":4,"completion_tokens":1}}`)
	}))
	// the CLI's own calls carry the internal header and must not be double-logged
	req, _ := http.NewRequest("POST", px.URL+"/v1/chat/completions", strings.NewReader(`{"model":"fast","messages":[]}`))
	req.Header.Set(internalHeader, "1")
	resp, _ := http.DefaultClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	// non-chat paths pass through unrecorded
	resp, _ = http.Get(px.URL + "/v1/models")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	time.Sleep(100 * time.Millisecond)
	if evs := readEvents(); len(evs) != 0 {
		t.Fatalf("expected no events, got %+v", evs)
	}

	resp, err := http.Post(px.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"fast","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `"content":"ok"`) {
		t.Fatalf("body: %s", body)
	}
	ev := lastEvent(t)
	if ev.Role != "fast" || ev.PromptTokens != 4 || ev.Status != "ok" || ev.Caller != "client" {
		t.Fatalf("%+v", ev)
	}
}

func TestProxyUpstreamErrorAndLinkHeaders(t *testing.T) {
	px := proxyFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"context size exceeded"}`, http.StatusBadRequest)
	}))
	req, _ := http.NewRequest("POST", px.URL+"/v1/chat/completions", strings.NewReader(`{"model":"chief","messages":[]}`))
	req.Header.Set("X-AI-Mode-Trace", "T1")
	req.Header.Set("X-AI-Mode-Parent", "P1")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 400 {
		t.Fatalf("status %d must pass through", resp.StatusCode)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	ev := lastEvent(t)
	if ev.Status != "error" || !strings.Contains(ev.Error, "HTTP 400") || !strings.Contains(ev.Error, "context size") ||
		ev.Trace != "T1" || ev.Parent != "P1" {
		t.Fatalf("%+v", ev)
	}
}

func TestProxyClientCancelPropagatesUpstream(t *testing.T) {
	var upstreamCancelled atomic.Bool
	px := proxyFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			upstreamCancelled.Store(true)
		case <-time.After(5 * time.Second):
		}
	}))
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", px.URL+"/v1/chat/completions", strings.NewReader(`{"model":"chief","stream":true,"messages":[]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	resp.Body.Read(buf)
	cancel() // like pressing Ctrl-C in the client
	resp.Body.Close()
	for i := 0; i < 100 && !upstreamCancelled.Load(); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if !upstreamCancelled.Load() {
		t.Fatal("upstream kept generating after the client disconnected")
	}
	ev := lastEvent(t)
	if ev.Status != "error" || ev.Error != "client cancelled" {
		t.Fatalf("%+v", ev)
	}
}

func TestProxyUpstreamDown(t *testing.T) {
	t.Setenv("AI_MODE_STATE", t.TempDir())
	u, _ := url.Parse("http://127.0.0.1:1") // nothing listens here
	px := httptest.NewServer(newProxyHandler("dev-shop", u))
	defer px.Close()
	resp, err := http.Post(px.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"chief","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ev := lastEvent(t); ev.Status != "error" || ev.Error == "" {
		t.Fatalf("%+v", ev)
	}
}
