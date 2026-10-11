package main

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// runCompletionTest invokes the completion (SIGINT) seam on a Timer placed in
// the given state and returns (the bytes the handler wrote, the exit status).
//
// The clock is never advanced and no goroutine or sleep is involved: an
// interrupted session is not counted (R8), so the completion count depends only
// on how many focus sessions fully finished before the interrupt, i.e. on
// t.completedFocus. The fake clock and the bytes.Buffer writer are the
// dependency-injection seams for time and output the contract requires, keeping
// the suite fast, deterministic and sleep-free (unit layer).
//
// The handler's error return is intentionally ignored here so the red failure
// lands on the assertion of the scenario's concrete output value (empty output
// vs the expected line) rather than on an early setup error, exactly as
// test-first red requires. Once the real handler writes the message and returns
// nil, the value assertions below simply document the no-stack-trace contract
// (R3) without changing the value assertion.
func runCompletionTest(t *testing.T, completedFocus, currentSession int) (string, int) {
	t.Helper()
	clk := newTestClock(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	var buf bytes.Buffer
	tmr := NewTimer(focusDur, shortDur, longDur, clk, &buf)
	tmr.completedFocus = completedFocus
	tmr.currentSession = currentSession
	code, _ := runCompletion(tmr, context.Background(), &buf)
	return buf.String(), code
}

// S3 (R1): the timer is in the LongBreak after 4 completed focus sessions; at
// t=10m into the break SIGINT arrives. The four fully finished focus sessions
// are counted; the interrupted long break is not. Output is
// "\r\x1b[KCompleted 4 focus sessions\n"; exit 0.
func TestCompletion_S3_LongBreak(t *testing.T) {
	// Arrange: four focus sessions fully finished; the timer is in the long
	// break (10 of its 15 minutes elapsed) when SIGINT arrives.
	// Act.
	out, code := runCompletionTest(t, 4, 5)

	// Assert: exactly the pinned SIGINT line; the interrupted break is not
	// counted and the exit status is 0.
	if want := completionClear + completionMessage(4); out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// S9 (R3): the timer is in Focus Session 3 at t=1200s (5m remaining). SIGINT
// arrives. The writer must first receive "\r\x1b[K" to clear the "Session 3:
// 05:00" remnant, then "Completed 2 focus sessions\n". Session 3 was
// interrupted and is NOT counted. Exit 0.
func TestCompletion_S9_ClearLine(t *testing.T) {
	// Arrange: two focus sessions fully finished; the third is in progress
	// (20 of its 25 minutes elapsed) when SIGINT arrives.
	// Act.
	out, code := runCompletionTest(t, 2, 3)

	// Assert: the line-clearing prefix precedes the completion message, and
	// the interrupted session is not counted.
	if want := completionClear + completionMessage(2); out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// S10 (R3): the timer is in the LongBreak after 4 completed focus sessions,
// t=600s into the 15m break. SIGINT arrives. Output is
// "\r\x1b[KCompleted 4 focus sessions\n"; exit 0.
func TestCompletion_S10_AfterSession4(t *testing.T) {
	// Arrange: four focus sessions fully finished; the timer is in the long
	// break (10 minutes elapsed) when SIGINT arrives.
	// Act.
	out, code := runCompletionTest(t, 4, 5)

	// Assert: the four completed sessions are counted; exit status is 0.
	if want := completionClear + completionMessage(4); out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// S11 (R3): SIGINT arrives immediately at t=0, before any focus session has
// started or completed. Output is "\r\x1b[KCompleted 0 focus sessions\n"; exit
// 0; no stack trace.
func TestCompletion_S11_Start(t *testing.T) {
	// Arrange: the timer has just started; no focus session has completed.
	// Act.
	out, code := runCompletionTest(t, 0, 1)

	// Assert: zero completed sessions and exit status 0.
	if want := completionClear + completionMessage(0); out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// S17 (R6): the timer is in Focus Session 3 at t=1200s. SIGINT arrives. Output
// is "\r\x1b[KCompleted 2 focus sessions\n"; exit 0. The interrupted session is
// not counted.
func TestCompletion_S17_FocusInterrupted(t *testing.T) {
	// Arrange: two focus sessions fully finished; the third is in progress
	// (20 of its 25 minutes elapsed) when SIGINT arrives.
	// Act.
	out, code := runCompletionTest(t, 2, 3)

	// Assert: the interrupted session is not counted; exit status is 0.
	if want := completionClear + completionMessage(2); out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// S20 (R8): after the four focus sessions of the first cycle have fully
// completed, the fifth period is the LongBreak (per the amended cycle model,
// consistent with S3/S10 where 4 completed focus sessions leaves the next
// period as the break); the timer is at t=100s into it. SIGINT arrives. The
// four completed focus sessions are counted; the interrupted break period is
// NOT counted. Output is "\r\x1b[KCompleted 4 focus sessions\n"; exit 0.
func TestCompletion_S20_Session5Start(t *testing.T) {
	// Arrange: the four focus sessions of the first cycle fully finished; the
	// timer is in the fifth period, the long break (100s elapsed) when SIGINT
	// arrives.
	// Act.
	out, code := runCompletionTest(t, 4, 5)

	// Assert: the whole-run total (4) is counted, the interrupted break period
	// is not, and the exit status is 0.
	if want := completionClear + completionMessage(4); out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}
