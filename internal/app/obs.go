package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Event is one line of events.jsonl. Observability must never break a command,
// so every write path swallows errors.
type Event struct {
	TS               time.Time `json:"ts"`
	Kind             string    `json:"kind"` // ask | classify | use | stop | restart | warm | warm_model
	Trace            string    `json:"trace_id,omitempty"`
	Span             string    `json:"span_id,omitempty"`
	Parent           string    `json:"parent_id,omitempty"`
	Caller           string    `json:"caller,omitempty"`
	Profile          string    `json:"profile,omitempty"`
	Role             string    `json:"role,omitempty"`
	Status           string    `json:"status"` // ok | error
	Error            string    `json:"error,omitempty"`
	DurationMS       int64     `json:"duration_ms"`
	Cold             bool      `json:"cold,omitempty"` // model was not loaded when the call started
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	CachedTokens     int       `json:"cached_tokens,omitempty"`
	PromptTPS        float64   `json:"prompt_tps,omitempty"`
	GenTPS           float64   `json:"gen_tps,omitempty"`
	FinishReason     string    `json:"finish_reason,omitempty"`
	MaxTokens        int       `json:"max_tokens,omitempty"`
	PromptChars      int       `json:"prompt_chars,omitempty"`
	ResponseChars    int       `json:"response_chars,omitempty"`
	SystemSHA        string    `json:"system_sha,omitempty"`
	Payload          string    `json:"payload,omitempty"` // path relative to traces dir
	Detail           string    `json:"detail,omitempty"`
	Question         string    `json:"question,omitempty"`   // classify: role | test_layer
	Choice           string    `json:"choice,omitempty"`     // classify: jev's pick
	Confidence       float64   `json:"confidence,omitempty"` // classify: jev's confidence
	Decision         string    `json:"decision,omitempty"`   // classify: clear | ambiguous | low-confidence
}

// isCall reports whether an event is a traced model call (a span in a trace).
func isCall(e Event) bool { return e.Kind == "ask" || e.Kind == "classify" }

const (
	maxEventsBytes = 10 << 20 // rotate events.jsonl past 10 MiB (keeps one .1 backup)
	defaultRetain  = 14       // days of per-call payloads to keep
)

func eventsFile() string { return filepath.Join(stateDir(), "events.jsonl") }
func tracesDir() string  { return filepath.Join(stateDir(), "traces") }

func newID(nbytes int) string {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

func newTraceID() string { return newID(6) }
func newSpanID() string  { return newID(4) }

func shortSHA(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:12]
}

// emit appends one event. A single O_APPEND write of a short line is atomic
// enough for concurrent `ai-mode ask` processes.
func emit(e Event) {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	if e.Status == "" {
		e.Status = "ok"
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	ensureDirs()
	path := eventsFile()
	if st, err := os.Stat(path); err == nil && st.Size() > maxEventsBytes {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

func captureEnabled() bool {
	v := envOr("AI_MODE_CAPTURE", configValue("AI_MODE_CAPTURE"))
	return v != "0" && v != "false" && v != "off"
}

// savePayload stores the full request/response for a span; returns the path
// relative to tracesDir (or "" if capture is off or failed).
func savePayload(trace, span string, request, response any) string {
	if !captureEnabled() || trace == "" || span == "" {
		return ""
	}
	dir := filepath.Join(tracesDir(), trace)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	data, err := json.MarshalIndent(map[string]any{"request": request, "response": response}, "", "  ")
	if err != nil {
		return ""
	}
	rel := filepath.Join(trace, span+".json")
	if err := os.WriteFile(filepath.Join(tracesDir(), rel), data, 0o644); err != nil {
		return ""
	}
	return rel
}

// pruneTraces deletes payload dirs older than the retention window; it runs at
// most once a day (guarded by a marker file).
func pruneTraces() {
	marker := filepath.Join(stateDir(), ".pruned")
	if st, err := os.Stat(marker); err == nil && time.Since(st.ModTime()) < 24*time.Hour {
		return
	}
	days := atoi(envOr("AI_MODE_RETAIN_DAYS", configValue("AI_MODE_RETAIN_DAYS")), defaultRetain)
	entries, _ := os.ReadDir(tracesDir())
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && e.IsDir() && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(tracesDir(), e.Name()))
		}
	}
	_ = os.WriteFile(marker, nil, 0o644)
}

// traceContext resolves the trace/parent for this process: explicit flags win,
// then AI_MODE_TRACE_ID / AI_MODE_PARENT_SPAN, else a fresh trace.
func traceContext(flagTrace, flagParent string) (trace, parent string) {
	trace = firstNonEmpty(flagTrace, os.Getenv("AI_MODE_TRACE_ID"))
	parent = firstNonEmpty(flagParent, os.Getenv("AI_MODE_PARENT_SPAN"))
	if trace == "" {
		trace = newTraceID()
	}
	return trace, parent
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// readEvents loads events (oldest first), including the rotated backup.
func readEvents() []Event {
	var out []Event
	for _, p := range []string{eventsFile() + ".1", eventsFile()} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			var e Event
			if json.Unmarshal([]byte(line), &e) == nil {
				out = append(out, e)
			}
		}
	}
	return out
}

// lifecycle times a command and records it as one event.
func lifecycle(kind, profile string, fn func() int) int {
	start := time.Now()
	code := fn()
	e := Event{
		TS: start.UTC(), Kind: kind, Trace: newTraceID(), Span: newSpanID(),
		Profile: profile, DurationMS: time.Since(start).Milliseconds(),
	}
	if code != 0 {
		e.Status, e.Error = "error", "exit "+strconv.Itoa(code)
	}
	emit(e)
	return code
}

func jsonLine(e Event) (string, error) {
	b, err := json.Marshal(e)
	return string(b), err
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
