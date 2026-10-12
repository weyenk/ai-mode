package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// runFlagValidation invokes the flag-validation seam (T06, R4/R7) on the given
// per-phase durations, capturing what the seam wrote to its (fake) stderr and
// the exit status it returned.
//
// The clock is never involved and there is no goroutine or sleep: flag
// validation is pure input → (message, exit code) logic, so using a
// bytes.Buffer as the io.Writer keeps the suite fast, deterministic and
// sleep-free (unit layer). In Phase A the seam is a not-implemented stub that
// writes nothing and returns the zero exit code, so every test below fails on
// its own assertion (red) rather than on a build or setup error. Phase B
// replaces the stub with the real validation and these same assertions turn
// green unchanged.
func runFlagValidation(focus, short, long time.Duration) (string, int) {
	var buf bytes.Buffer
	code := validateFlagsFn(focus, short, long, &buf)
	return buf.String(), code
}

// S4 (R1): focusDuration is 25h, 1h over the per-duration 24h limit. The
// configuration must be rejected / fail fast so the timer loop never starts.
// The constraint is per-duration (each of focus, short, long), not the total
// cycle, so the other durations here are valid and only focus trips the rule.
func TestFlags_S4_TooLong(t *testing.T) {
	// Arrange: focus is 25h; short and long stay within the limit.
	// Act.
	out, code := runFlagValidation(25*time.Hour, shortDur, longDur)

	// Assert: the 25h focus duration is rejected (fail fast, no timer start)
	// with a descriptive message that identifies the offending duration.
	if code == 0 {
		t.Errorf("exit code = 0, want non-zero (25h focus duration must be rejected)")
	}
	if out == "" {
		t.Errorf("stderr is empty, want a descriptive rejection of the 25h focus duration")
	}
	if !strings.Contains(out, "focus") {
		t.Errorf("rejection %q does not identify the offending focus duration", out)
	}
}

// S12 (R4): the user passes --focus 25h. The program prints the exact pinned
// line 'error: focus duration 25h exceeds 24h limit' to stderr and exits with
// code 1; the timer loop must not start.
func TestFlags_S12_ErrorMsg(t *testing.T) {
	// Arrange: --focus 25h; short and long are valid.
	// Act.
	out, code := runFlagValidation(25*time.Hour, shortDur, longDur)

	// Assert: exactly the pinned stderr message and the standard non-zero code 1.
	if want := "error: focus duration 25h exceeds 24h limit"; out != want {
		t.Errorf("stderr = %q, want %q", out, want)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

// S13 (R4): two configured durations both exceed the 24h limit. The program
// must report at least one violation descriptively to stderr and exit non-zero.
// NOTE: the scenario text names "24h" as a violation, but R4 says a duration
// must not *exceed* 24h, so 24h exactly is valid; to genuinely produce two
// violations (the scenario's intent) both offending durations are set just
// past the limit. The scenario allows reporting both or only the first, so the
// assertion requires "at least one" rather than both.
func TestFlags_S13_MultipleErrors(t *testing.T) {
	// Arrange: focus and short each exceed the 24h limit; long is valid.
	over := 24*time.Hour + time.Hour // 25h, just past the limit
	// Act.
	out, code := runFlagValidation(over, over, longDur)

	// Assert: a non-zero exit and at least one reported violation naming one of
	// the offending durations (focus or short).
	if code == 0 {
		t.Errorf("exit code = 0, want non-zero (two durations exceed the 24h limit)")
	}
	if !strings.Contains(out, "focus") && !strings.Contains(out, "short") {
		t.Errorf("stderr = %q, want at least one reported violation naming 'focus' or 'short'", out)
	}
}

// S15 (R2/edge): a non-positive duration is passed. R4 only bounds the upper
// end; the contract's resolved question fixes the lower end — a duration of
// zero or less must be rejected at startup (fail fast) with a descriptive
// error and a non-zero exit, so the state machine never enters a rapid-fire
// transition loop. The lower-bound boundary value is zero; a negative value is
// checked alongside it.
func TestFlags_S15_NonPositive(t *testing.T) {
	// Arrange + Act: the lower-bound boundary (zero).
	zeroOut, zeroCode := runFlagValidation(0, shortDur, longDur)

	// Arrange + Act: a negative value.
	negOut, negCode := runFlagValidation(-time.Second, shortDur, longDur)

	// Assert: each non-positive duration is rejected with a descriptive message
	// and a non-zero exit.
	if zeroCode == 0 {
		t.Errorf("focus=0: exit code = 0, want non-zero (non-positive duration must be rejected)")
	}
	if zeroOut == "" {
		t.Errorf("focus=0: stderr is empty, want a descriptive rejection of the non-positive duration")
	}
	if negCode == 0 {
		t.Errorf("focus=-1s: exit code = 0, want non-zero (non-positive duration must be rejected)")
	}
	if negOut == "" {
		t.Errorf("focus=-1s: stderr is empty, want a descriptive rejection of the non-positive duration")
	}
}

// S19 (R7): the user passes --focus 25h again. This is the standalone rule that
// each configured duration must not exceed 24h; the check must still fire even
// though the same 25h focus input was already exercised in S4/S12. The expected
// output is identical to S12.
func TestFlags_S19_DuplicateCheck(t *testing.T) {
	// Arrange: --focus 25h; short and long are valid.
	// Act.
	out, code := runFlagValidation(25*time.Hour, shortDur, longDur)

	// Assert: identical to S12 — the exact pinned stderr message and code 1,
	// confirming the 24h check still runs for this input.
	if want := "error: focus duration 25h exceeds 24h limit"; out != want {
		t.Errorf("stderr = %q, want %q", out, want)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}
