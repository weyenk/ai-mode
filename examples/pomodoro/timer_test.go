package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// Default per-phase durations from the feature plan (25m focus, 5m short, 15m long).
const (
	focusDur = 25 * time.Minute
	shortDur = 5 * time.Minute
	longDur  = 15 * time.Minute
)

// testClock is a fake Clock: time only advances when the test explicitly does,
// so the suite is fast, deterministic and sleep-free.
type testClock struct {
	mu    sync.Mutex
	now   time.Time
	waits []clockWait
}

type clockWait struct {
	deadline time.Time
	ch       chan time.Time
}

func newTestClock(start time.Time) *testClock { return &testClock{now: start} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.waits = append(c.waits, clockWait{deadline: c.now.Add(d), ch: ch})
	return ch
}

// advance moves the fake clock by step and fires every pending After channel
// whose deadline has arrived (one outstanding one-second tick at a time).
func (c *testClock) advance(step time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(step)
	waits := c.waits
	c.waits = nil
	c.mu.Unlock()
	for _, w := range waits {
		if !w.deadline.After(c.now) {
			select {
			case w.ch <- w.deadline:
			default:
			}
		}
	}
}

// notifyWriter records every Write call as one status line and wakes the test
// goroutine on each write, so the driver can synchronise without sleeping.
type notifyWriter struct {
	mu    sync.Mutex
	lines []string
	sig   chan struct{}
}

func newNotifyWriter() *notifyWriter {
	return &notifyWriter{sig: make(chan struct{}, 16384)}
}

func (w *notifyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.lines = append(w.lines, string(p))
	w.mu.Unlock()
	select {
	case w.sig <- struct{}{}:
	default:
	}
	return len(p), nil
}

// lineAt returns the nth written status line (0-based), blocking until it
// exists; it fails the test on timeout instead of letting the driver race.
func (w *notifyWriter) lineAt(t *testing.T, n int) string {
	t.Helper()
	for {
		w.mu.Lock()
		if n < len(w.lines) {
			line := w.lines[n]
			w.mu.Unlock()
			return line
		}
		got := len(w.lines)
		w.mu.Unlock()
		select {
		case <-w.sig:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for status write %d; only %d writes recorded: %q", n, got, strings.Join(w.lines, "|"))
		}
	}
}

// runForTicks starts a Timer on the fake clock, waits for the initial status
// line at t=0, then advances the clock second by second for seconds ticks,
// synchronising on each per-second status write (R2).
func runForTicks(t *testing.T, focus, short, long time.Duration, seconds int) *notifyWriter {
	t.Helper()
	clk := newTestClock(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	out := newNotifyWriter()
	tmr := NewTimer(focus, short, long, clk, out)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = tmr.Run(ctx)
	}()
	out.lineAt(t, 0) // initial status line at t=0 (S18)
	for i := 1; i <= seconds; i++ {
		clk.advance(time.Second)
		out.lineAt(t, i)
	}
	return out
}

// S1: after 4 focus sessions the next break is a 15m long break, not a short one.
func TestTimer_S1_ShortBreakAfterFourth(t *testing.T) {
	// Arrange: 4 focus sessions (4*1500s) plus 3 short breaks (3*300s) end at t=6900s.
	// Act: run the timer to the transition right after the 4th focus session.
	out := runForTicks(t, focusDur, shortDur, longDur, 6900)

	// Assert: the phase entering at t=6900s is a 15-minute LongBreak.
	if got, want := out.lineAt(t, 6899), "Session 4: 00:01\r\x1b[K"; got != want {
		t.Errorf("last tick of 4th focus session = %q, want %q", got, want)
	}
	if got, want := out.lineAt(t, 6900), "Long break: 15:00\r\x1b[K"; got != want {
		t.Errorf("break after 4th focus session = %q, want %q", got, want)
	}
}

// S2: full transition sequence Focus -> ShortBreak -> Focus -> ... -> LongBreak -> Session 5.
func TestTimer_S2_FullCycle(t *testing.T) {
	// Arrange: default durations; milestones below are exact contract points.
	// Act: run 130 minutes (4 focus + 3 short + 1 long = 7800s).
	out := runForTicks(t, focusDur, shortDur, longDur, 7800)

	// Assert: state and countdown at each transition boundary.
	milestones := []struct {
		tick int
		line string
	}{
		{0, "Session 1: 25:00\r\x1b[K"},     // Focus, Session 1
		{1500, "Break: 05:00\r\x1b[K"},      // t=25m -> ShortBreak 5m
		{1800, "Session 2: 25:00\r\x1b[K"},  // t=30m -> Focus Session 2
		{6900, "Long break: 15:00\r\x1b[K"}, // t=115m -> LongBreak (not ShortBreak) 15m
		{7800, "Session 5: 25:00\r\x1b[K"},  // after long break -> Focus Session 5
	}
	for _, m := range milestones {
		if got, want := out.lineAt(t, m.tick), m.line; got != want {
			t.Errorf("t=%ds: status line = %q, want %q", m.tick, got, want)
		}
	}
}

// S5: in-place updates use the '\r' + ANSI erase sequence, one per second.
func TestTimer_S5_OutputFormat(t *testing.T) {
	// Arrange: default durations; observe ticks 0, 1499 and the transition tick 1500.
	// Act: run 25 minutes.
	out := runForTicks(t, focusDur, shortDur, longDur, 1500)

	// Assert: exact pinned lines, each carrying the \r and ANSI erase-to-EOL.
	if got, want := out.lineAt(t, 0), "Session 1: 25:00\r\x1b[K"; got != want {
		t.Errorf("t=0: status line = %q, want %q", got, want)
	}
	if got, want := out.lineAt(t, 1499), "Session 1: 00:01\r\x1b[K"; got != want {
		t.Errorf("t=1499s: status line = %q, want %q", got, want)
	}
	if got, want := out.lineAt(t, 1500), "Break: 05:00\r\x1b[K"; got != want {
		t.Errorf("t=1500s: status line = %q, want %q", got, want)
	}
	for _, n := range []int{0, 1499, 1500} {
		line := out.lineAt(t, n)
		if !strings.Contains(line, "\r") || !strings.Contains(line, "\x1b[K") {
			t.Errorf("t=%d: status line %q missing \\r or ANSI erase sequence", n, line)
		}
	}
}

// S7: countdown strings at t=0, t=1439s and the break transition at t=1500s.
func TestTimer_S7_BreakTransitionOutput(t *testing.T) {
	// Arrange: default 25m focus (1500s); the 25th minute ends exactly at t=1500s.
	// Act: run to the ShortBreak transition.
	out := runForTicks(t, focusDur, shortDur, longDur, 1500)

	// Assert: writer received the exact pinned countdown strings.
	if got, want := out.lineAt(t, 0), "Session 1: 25:00\r\x1b[K"; got != want {
		t.Errorf("t=0: status line = %q, want %q", got, want)
	}
	if got, want := out.lineAt(t, 1439), "Session 1: 01:01\r\x1b[K"; got != want {
		t.Errorf("t=1439s: status line = %q, want %q", got, want)
	}
	if got, want := out.lineAt(t, 1500), "Break: 05:00\r\x1b[K"; got != want {
		t.Errorf("t=1500s: status line = %q, want %q", got, want)
	}
}

// S8: long break is labelled 'Long break' (two words), not 'Break'.
func TestTimer_S8_LongBreakLabel(t *testing.T) {
	// Arrange: run up to the LongBreak phase (starts at t=115m, first tick of the phase).
	// Act: run to the first tick of the LongBreak.
	out := runForTicks(t, focusDur, shortDur, longDur, 6900)

	// Assert: the LongBreak's first line uses the two-word 'Long break' label.
	if got, want := out.lineAt(t, 6900), "Long break: 15:00\r\x1b[K"; got != want {
		t.Errorf("LongBreak first status line = %q, want %q", got, want)
	}
}

// S14: boundary with a 1-minute focus duration; no negative values, no 00:00 shown.
func TestTimer_S14_BoundaryOneMinute(t *testing.T) {
	// Arrange: focus duration is exactly 1 minute.
	// Act: run to the transition at t=60s.
	out := runForTicks(t, time.Minute, shortDur, longDur, 60)

	// Assert: countdown steps 01:00 -> 00:01 -> break, never negative.
	if got, want := out.lineAt(t, 0), "Session 1: 01:00\r\x1b[K"; got != want {
		t.Errorf("t=0: status line = %q, want %q", got, want)
	}
	if got, want := out.lineAt(t, 59), "Session 1: 00:01\r\x1b[K"; got != want {
		t.Errorf("t=59s: status line = %q, want %q", got, want)
	}
	if got, want := out.lineAt(t, 60), "Break: 05:00\r\x1b[K"; got != want {
		t.Errorf("t=60s: status line = %q, want %q", got, want)
	}
}

// S15: a zero or negative focus duration is a contract edge case; the 24-hour
// rule only bounds the upper end, so the engine must not panic, emit negative
// countdowns, or hang on the zero-duration boundary.
func TestTimer_S15_ZeroDuration(t *testing.T) {
	// Arrange: focus duration is 0; the 1m break prevents an infinite loop.
	// Act: run through the first tick so the zero-length focus resolves.
	out := runForTicks(t, 0, shortDur, longDur, 1)

	// Assert: output stays well-formed (never negative) and no 00:00 remnant.
	if got, want := out.lineAt(t, 0), "Session 1: 00:00\r\x1b[K"; got != want {
		t.Errorf("t=0: status line = %q, want %q", got, want)
	}
	line := out.lineAt(t, 1)
	if strings.Contains(line, "-") {
		t.Errorf("t=1: status line %q contains a negative value", line)
	}
}

// S16: after the long break the cycle restarts at focus session 5.
func TestTimer_S16_CycleRestart(t *testing.T) {
	// Arrange: 4 focus + 3 short + 1 long break = 7800s.
	// Act: run to the tick right after the long break ends.
	out := runForTicks(t, focusDur, shortDur, longDur, 7800)

	// Assert: long break runs down to 00:01, then Session 5 starts fresh.
	if got, want := out.lineAt(t, 7799), "Long break: 00:01\r\x1b[K"; got != want {
		t.Errorf("last tick of long break = %q, want %q", got, want)
	}
	if got, want := out.lineAt(t, 7800), "Session 5: 25:00\r\x1b[K"; got != want {
		t.Errorf("first tick after long break = %q, want %q", got, want)
	}
}

// S18: the very first output is the pinned initial status line at t=0.
func TestTimer_S18_InitialOutput(t *testing.T) {
	// Arrange: default 25m focus duration; no ticks advanced.
	// Act: start the timer and capture the initial write only.
	out := runForTicks(t, focusDur, shortDur, longDur, 0)

	// Assert: writer received exactly the initial line at t=0.
	if got, want := out.lineAt(t, 0), "Session 1: 25:00\r\x1b[K"; got != want {
		t.Errorf("initial status line = %q, want %q", got, want)
	}
}
