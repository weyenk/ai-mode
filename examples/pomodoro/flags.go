package main

import (
	"io"
	"time"
)

// maxDuration is the per-duration upper bound (R1/R4/R7): each configured
// duration — focus, short break, long break — individually must not exceed 24
// hours. The limit applies per-duration, not to the total cycle time.
const maxDuration = 24 * time.Hour

// minDuration is the lower bound resolved for non-positive durations (S15): a
// duration of zero or less is rejected at startup rather than allowed to
// collapse the state machine into a rapid-fire transition loop.
const minDuration = time.Second

// validateFlagsFn validates the configured per-phase durations before the timer
// loop starts (R4/R7). For each duration that falls outside the valid range it
// writes a descriptive "error: <name> duration ... " message to out (stderr in
// production) and, if any violation exists, returns a non-zero exit code (1)
// so the process fails fast and the timer loop never starts. On a fully valid
// configuration it writes nothing and returns 0.
//
// This is the T06 seam; the tests in flags_test.go are written test-first
// against it. The body below is a deliberate not-implemented stub: it performs
// no validation and returns the zero value, so every scenario test fails on
// its own assertion (red) rather than on a build or setup error. Phase B
// replaces the body with the real validation.
var validateFlagsFn = func(focus, short, long time.Duration, out io.Writer) int {
	_ = out
	_ = focus
	_ = short
	_ = long
	return 0 // stub: not implemented
}
