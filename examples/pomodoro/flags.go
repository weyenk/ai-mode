package main

import (
	"fmt"
	"io"
	"time"
)

// maxDuration is the per-duration upper bound (R1/R4/R7): each configured
// duration — focus, short break, long break — individually must not exceed 24
// hours. The limit applies per-duration, not to the total cycle time.
const maxDuration = 24 * time.Hour

// formatDuration renders a duration in a compact form: whole numbers of hours
// as "Nh" (e.g. 25h), otherwise the standard Duration string (e.g. 25m0s).
func formatDuration(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(d/time.Hour))
	}
	return d.String()
}

// minDuration is the lower bound resolved for non-positive durations (S15): a
// duration below 1 second (including zero and negative) is rejected at startup
// rather than allowed to collapse the state machine into a rapid-fire
// transition loop.
const minDuration = time.Second

// validateFlagsFn validates the configured per-phase durations before the timer
// loop starts (R4/R7). It checks focus, short, then long in order; for the
// first duration that falls outside the valid range (below minDuration or above
// maxDuration) it writes a single descriptive "error: <name> duration ... "
// message to out (stderr in production) and returns a non-zero exit code (1) so
// the process fails fast and the timer loop never starts. On a fully valid
// configuration it writes nothing and returns 0.
//
// This is the T06 seam; the tests in flags_test.go are written test-first
// against it.
var validateFlagsFn = func(focus, short, long time.Duration, out io.Writer) int {
	durations := []struct {
		name string
		dur  time.Duration
	}{
		{"focus", focus},
		{"short", short},
		{"long", long},
	}
	for _, d := range durations {
		switch {
		case d.dur < minDuration:
			fmt.Fprintf(out, "error: %s duration %s must be at least %s", d.name, formatDuration(d.dur), formatDuration(minDuration))
			return 1
		case d.dur > maxDuration:
			fmt.Fprintf(out, "error: %s duration %s exceeds 24h limit", d.name, formatDuration(d.dur))
			return 1
		}
	}
	return 0
}
