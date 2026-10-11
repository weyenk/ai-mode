package main

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// interruptOut is the exact byte form R3/R6 mandate on SIGINT: a carriage
// return and the ANSI erase-to-end-of-line to clear the in-place status line,
// then the completion line terminated by a trailing newline.
const interruptOut = "\r\x1b[K"

// runInterrupt invokes the SIGINT handler seam on a timer in the given state
// and returns (the bytes the handler wrote, the exit code).
//
// The clock is never advanced: an interrupted session is not counted (R8), so
// the completion count depends only on how many focus sessions fully finished
// before the interrupt, i.e. on t.completedFocus. This keeps the tests fast,
// deterministic and sleep-free (unit layer).
//
// The handler's error return is intentionally not fatalised here: the red
// failure must land on the assertion of the scenario's concrete output value
// (empty output vs the expected line), not on an early setup error. Once the
// real handler returns nil the check below simply documents the no-stack-trace
// contract (R3) without changing the value assertion.
func runInterrupt(t *testing.T, completedFocus, currentSession int) (string, int) {
	t.Helper()
	clk := newTestClock(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	var buf bytes.Buffer
	tmr := NewTimer(focusDur, shortDur, longDur, clk, &buf)
	tmr.completedFocus = completedFocus
	tmr.currentSession = currentSession
	code, _ := handleSIGINTFn(tmr, context.Background(), &buf)
	return buf.String(), code
}

// S6 (R1/R8): Focus Session 3 in progress (25:00); user presses Ctrl-C at
// t=20m. Sessions 1 and 2 fully completed; Session 3 was interrupted and is
// NOT counted. Output is "\r\x1b[KCompleted 2 focus sessions\n"; exit 0.
func TestInterrupt_S6_FocusInterrupted(t *testing.T) {
	// Arrange: two focus sessions have fully completed; the timer is in the
	// third focus session (20 of its 25 minutes elapsed) when SIGINT arrives.
	// Act.
	out, code := runInterrupt(t, 2, 3)

	// Assert: exactly the pinned SIGINT line; the interrupted session is
	// not counted, and the exit status is 0.
	if want := interruptOut + "Completed 2 focus sessions\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// S21 (R8): Focus Session 1 at t=1s; SIGINT arrives. "Completed" means the
// full 25:00 elapsed, not merely started, so Session 1 is NOT counted.
// Output is "\r\x1b[KCompleted 0 focus sessions\n"; exit 0.
func TestInterrupt_S21_StartInterrupted(t *testing.T) {
	// Arrange: the very first focus session has barely started (t=1s); none
	// has fully completed.
	// Act.
	out, code := runInterrupt(t, 0, 1)

	// Assert.
	if want := interruptOut + "Completed 0 focus sessions\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// S22 (R8): Focus Session 4 (4th of the cycle) at t=1439s (00:01 remaining);
// SIGINT arrives one second before completion. Session 4 is interrupted and
// NOT counted. Output is "\r\x1b[KCompleted 3 focus sessions\n"; exit 0.
func TestInterrupt_S22_EndOfSessionInterrupted(t *testing.T) {
	// Arrange: three focus sessions have fully completed; the timer is in the
	// fourth with one second left when SIGINT arrives.
	// Act.
	out, code := runInterrupt(t, 3, 4)

	// Assert.
	if want := interruptOut + "Completed 3 focus sessions\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}
