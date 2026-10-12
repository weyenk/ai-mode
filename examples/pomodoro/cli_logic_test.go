package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// timerCall records whether runTimerFn was invoked and, if so, the per-phase
// durations it was called with. Tests swap the package-level runTimerFn seam
// for this capturing stub so the CLI parsing logic (the unit under test) can
// be observed at the functional boundary without running a real countdown.
type timerCall struct {
	called bool
	focus  time.Duration
	short  time.Duration
	long   time.Duration
}

// withRunTimer replaces runTimerFn with a capturing stub for the duration of
// the test and restores the original afterwards, so no mutable state leaks
// across tests (isolation). It returns a pointer to the capture for asserting.
func withRunTimer(t *testing.T) *timerCall {
	t.Helper()
	rec := &timerCall{}
	orig := runTimerFn
	runTimerFn = func(ctx context.Context, focus, short, long time.Duration) error {
		rec.called = true
		rec.focus = focus
		rec.short = short
		rec.long = long
		return nil
	}
	t.Cleanup(func() { runTimerFn = orig })
	return rec
}

// S2: -focus -1 is below the 1-minute lower bound; the program must print an
// error naming the offending flag and exit with status 2 before starting any
// session (R1).
func TestCLI_S2_FocusNegative(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	got := runCLI([]string{"-focus", "-1"}, &out)

	if got != 2 {
		t.Fatalf("runCLI exit status = %d, want 2", got)
	}
	if out.Len() == 0 {
		t.Fatalf("runCLI printed no error message, want one naming -focus")
	}
	if !strings.Contains(out.String(), "-focus") {
		t.Errorf("runCLI error %q does not name the offending flag -focus", out.String())
	}
	if rec.called {
		t.Fatalf("runTimerFn was called for an invalid config; it must not start a session")
	}
}

// S3: -focus 0 is below the lower bound; the program must print the exact
// message and exit with status 2.
func TestCLI_S3_FocusZero(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	got := runCLI([]string{"-focus", "0"}, &out)

	if got != 2 {
		t.Fatalf("runCLI exit status = %d, want 2", got)
	}
	const wantMsg = "Invalid value for -focus: must be between 1 and 1440 minutes."
	if !strings.Contains(out.String(), wantMsg) {
		t.Fatalf("runCLI output %q does not contain %q", out.String(), wantMsg)
	}
	if rec.called {
		t.Fatalf("runTimerFn was called for an invalid config; it must not start a session")
	}
}

// S4: -focus 1441 is above the 24-hour upper bound; the program must print the
// exact message and exit with status 2.
func TestCLI_S4_FocusTooLarge(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	got := runCLI([]string{"-focus", "1441"}, &out)

	if got != 2 {
		t.Fatalf("runCLI exit status = %d, want 2", got)
	}
	const wantMsg = "Invalid value for -focus: must be between 1 and 1440 minutes."
	if !strings.Contains(out.String(), wantMsg) {
		t.Fatalf("runCLI output %q does not contain %q", out.String(), wantMsg)
	}
	if rec.called {
		t.Fatalf("runTimerFn was called for an invalid config; it must not start a session")
	}
}

// S5: -focus 1440 is valid (the 24-hour upper bound); the timer must start with
// focus=1440m and the default short=5m, long=15m.
func TestCLI_S5_FocusValid(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	_ = runCLI([]string{"-focus", "1440"}, &out)

	if !rec.called {
		t.Fatalf("runTimerFn was not called for a valid -focus value")
	}
	if rec.focus != 1440*time.Minute {
		t.Fatalf("focus = %v, want 1440m (24h)", rec.focus)
	}
	if rec.short != 5*time.Minute {
		t.Fatalf("short = %v, want default 5m", rec.short)
	}
	if rec.long != 15*time.Minute {
		t.Fatalf("long = %v, want default 15m", rec.long)
	}
}

// S6: -short 1 is valid; the timer must start with the default focus=25m, the
// supplied short=1m, and the default long=15m.
func TestCLI_S6_ShortValid(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	_ = runCLI([]string{"-short", "1"}, &out)

	if !rec.called {
		t.Fatalf("runTimerFn was not called for a valid -short value")
	}
	if rec.focus != 25*time.Minute {
		t.Fatalf("focus = %v, want default 25m", rec.focus)
	}
	if rec.short != 1*time.Minute {
		t.Fatalf("short = %v, want 1m", rec.short)
	}
	if rec.long != 15*time.Minute {
		t.Fatalf("long = %v, want default 15m", rec.long)
	}
}
