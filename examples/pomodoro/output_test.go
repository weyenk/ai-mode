package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// writeStatus invokes the output seam for one (state, session, remaining) tuple
// and returns the single line it wrote. In Phase A the seam is a not-implemented
// stub that writes nothing and returns an error; the error is intentionally
// ignored here so the test fails on its value assertion (empty output vs the
// expected line) rather than on a build or setup error, exactly as test-first
// red requires. Once the real formatter lands and returns nil, this simply
// returns the formatted line.
func writeStatus(t *testing.T, state TimerState, session int, remaining time.Duration) string {
	t.Helper()
	var buf bytes.Buffer
	_ = updateOutputFn(state, session, remaining, &buf)
	return buf.String()
}

// S5: in-place focus output is the exact pinned line, prefixed by a carriage
// return and the ANSI erase-to-end-of-line sequence (to clear the previous
// line before printing the new one), at t=0, the last second and on the
// ShortBreak transition.
func TestOutput_S5_Formatting(t *testing.T) {
	// Arrange: default 25m focus and 5m short-break durations.
	// Act + Assert: each update must be the exact in-place line and must carry
	// the leading "\r\x1b[K" that clears the previous line before writing the
	// new status.
	if got, want := writeStatus(t, Focus, 1, 25*time.Minute), "\r\x1b[KSession 1: 25:00"; got != want {
		t.Errorf("t=0: status line = %q, want %q", got, want)
	}
	if got, want := writeStatus(t, Focus, 1, 1*time.Second), "\r\x1b[KSession 1: 00:01"; got != want {
		t.Errorf("t=24m59s: status line = %q, want %q", got, want)
	}
	if got, want := writeStatus(t, ShortBreak, 1, 5*time.Minute), "\r\x1b[KBreak: 05:00"; got != want {
		t.Errorf("t=25m (ShortBreak): status line = %q, want %q", got, want)
	}
	for _, line := range []string{
		writeStatus(t, Focus, 1, 25*time.Minute),
		writeStatus(t, Focus, 1, 1*time.Second),
		writeStatus(t, ShortBreak, 1, 5*time.Minute),
	} {
		if !strings.HasPrefix(line, "\r\x1b[K") {
			t.Errorf("status line %q is not prefixed by \\r + ANSI erase-to-end-of-line", line)
		}
	}
}

// S7: the focus countdown reaches 00:01 one second before the session ends, and
// the transition to ShortBreak shows the 5:00 break.
func TestOutput_S7_ShortBreak(t *testing.T) {
	// Arrange: default 25m focus and 5m short-break durations.
	// Act + Assert: the countdown shows 00:01 at the last second and the break
	// line shows 05:00 once the phase is ShortBreak.
	if got, want := writeStatus(t, Focus, 1, 1500*time.Second), "\r\x1b[KSession 1: 25:00"; got != want {
		t.Errorf("focus start: status line = %q, want %q", got, want)
	}
	if got, want := writeStatus(t, Focus, 1, 1*time.Second), "\r\x1b[KSession 1: 00:01"; got != want {
		t.Errorf("last second of focus: status line = %q, want %q", got, want)
	}
	if got, want := writeStatus(t, ShortBreak, 1, 300*time.Second), "\r\x1b[KBreak: 05:00"; got != want {
		t.Errorf("ShortBreak transition: status line = %q, want %q", got, want)
	}
}

// S8: the long break is labelled "Long break" (two words), not "Break".
func TestOutput_S8_LongBreak(t *testing.T) {
	// Arrange: default 15m long-break duration.
	// Act + Assert: the long break line uses the two-word "Long break" label.
	if got, want := writeStatus(t, LongBreak, 1, 15*time.Minute), "\r\x1b[KLong break: 15:00"; got != want {
		t.Errorf("LongBreak: status line = %q, want %q", got, want)
	}
}

// S18: the very first output at t=0 is the pinned initial status line.
func TestOutput_S18_Initial(t *testing.T) {
	// Arrange: default 25m focus duration; the timer has just started.
	// Act + Assert: the writer receives exactly the initial line at t=0.
	if got, want := writeStatus(t, Focus, 1, 25*time.Minute), "\r\x1b[KSession 1: 25:00"; got != want {
		t.Errorf("initial status line = %q, want %q", got, want)
	}
}
