package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestCLI_S7_LongTooLarge (S7, R1): -long 1441 is above the 24-hour upper
// bound. The program must print the exact error naming the offending flag and
// exit with status 2, and must not start a session.
func TestCLI_S7_LongTooLarge(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	got := runCLISec([]string{"-long", "1441"}, &out)

	if got != 2 {
		t.Fatalf("exit status = %d, want 2", got)
	}
	const wantMsg = "Invalid value for -long: must be between 1 and 1440 minutes."
	if !strings.Contains(out.String(), wantMsg) {
		t.Errorf("output %q does not contain %q", out.String(), wantMsg)
	}
	if rec.called {
		t.Errorf("a session was started for an invalid configuration; it must not start")
	}
}

// TestCLI_S8_ShortTooLarge (S8, R2): -short 1441 is above the upper bound.
// The program must print the exact error naming the flag and exit with status 2.
func TestCLI_S8_ShortTooLarge(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	got := runCLISec([]string{"-short", "1441"}, &out)

	if got != 2 {
		t.Fatalf("exit status = %d, want 2", got)
	}
	const wantMsg = "Invalid value for -short: must be between 1 and 1440 minutes."
	if !strings.Contains(out.String(), wantMsg) {
		t.Errorf("output %q does not contain %q", out.String(), wantMsg)
	}
	if rec.called {
		t.Errorf("a session was started for an invalid configuration; it must not start")
	}
}

// TestCLI_S9_ValidationOrder (S9, R3, per architect amendment): given
// -focus 200 (valid) -short -1 (invalid) -long 5 (valid), validation checks
// -focus first (passes), then -short (fails). The program must name -short,
// exit with status 2, and never mention -focus.
func TestCLI_S9_ValidationOrder(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	got := runCLISec([]string{"-focus", "200", "-short", "-1", "-long", "5"}, &out)

	if got != 2 {
		t.Fatalf("exit status = %d, want 2", got)
	}
	const wantMsg = "Invalid value for -short: must be between 1 and 1440 minutes."
	if !strings.Contains(out.String(), wantMsg) {
		t.Errorf("output %q does not contain %q (first invalid flag is -short)", out.String(), wantMsg)
	}
	if strings.Contains(out.String(), "-focus") {
		t.Errorf("output %q mentions -focus, but the first invalid flag is -short and -focus must not be named", out.String())
	}
	if rec.called {
		t.Errorf("a session was started for an invalid configuration; it must not start")
	}
}

// TestCLI_S10_ValidationOrderZero (S10, R3): given -focus 0 (invalid) -short -1
// (invalid) -long 5 (valid), validation checks -focus first and stops there.
// The first error must name -focus (0), not -short, and exit with status 2.
func TestCLI_S10_ValidationOrderZero(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	got := runCLISec([]string{"-focus", "0", "-short", "-1", "-long", "5"}, &out)

	if got != 2 {
		t.Fatalf("exit status = %d, want 2", got)
	}
	const wantMsg = "Invalid value for -focus: must be between 1 and 1440 minutes."
	if !strings.Contains(out.String(), wantMsg) {
		t.Errorf("output %q does not contain %q (first invalid flag is -focus 0)", out.String(), wantMsg)
	}
	if strings.Contains(out.String(), "-short") {
		t.Errorf("output %q mentions -short, but validation must stop at -focus first", out.String())
	}
	if rec.called {
		t.Errorf("a session was started for an invalid configuration; it must not start")
	}
}

// TestCLI_S11_Defaults (S11, R4): with no flags supplied, the defaults apply to
// every phase: focus=25m, short=5m, long=15m. runTimerFn must be invoked with
// exactly those durations (the first countdown uses 25 minutes).
func TestCLI_S11_Defaults(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	got := runCLISec(nil, &out)

	if got != 0 {
		t.Fatalf("exit status = %d, want 0 for a valid default configuration", got)
	}
	if !rec.called {
		t.Fatalf("runTimerFn was not called for a valid default configuration")
	}
	if rec.focus != 25*time.Minute {
		t.Errorf("focus = %v, want default 25m", rec.focus)
	}
	if rec.short != 5*time.Minute {
		t.Errorf("short = %v, want default 5m", rec.short)
	}
	if rec.long != 15*time.Minute {
		t.Errorf("long = %v, want default 15m", rec.long)
	}
}

// TestCLI_S12_PartialFlags (S12, R4): with only -focus 50 supplied, the supplied
// focus (50m) is applied while the unsupplied phases keep their defaults
// (short=5m, long=15m). Defaults apply only to flags that were not supplied.
func TestCLI_S12_PartialFlags(t *testing.T) {
	rec := withRunTimer(t)
	var out bytes.Buffer

	got := runCLISec([]string{"-focus", "50"}, &out)

	if got != 0 {
		t.Fatalf("exit status = %d, want 0 for a valid configuration", got)
	}
	if !rec.called {
		t.Fatalf("runTimerFn was not called for a valid configuration")
	}
	if rec.focus != 50*time.Minute {
		t.Errorf("focus = %v, want supplied 50m", rec.focus)
	}
	if rec.short != 5*time.Minute {
		t.Errorf("short = %v, want default 5m", rec.short)
	}
	if rec.long != 15*time.Minute {
		t.Errorf("long = %v, want default 15m", rec.long)
	}
}
