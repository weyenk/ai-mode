package main

import (
	"fmt"
	"io"
	"time"
)

// updateOutputFn writes the current in-place status line for the timer's state
// (R2). Given the active state, the current focus-session number and the time
// remaining in the current phase, it emits exactly one line of the form
//
//	"\r\x1b[K<label>: MM:SS"
//
// where the carriage return and ANSI erase-to-end-of-line prefix clears the
// previous line before printing the new one. The label is "Session N" for
// Focus, "Break" for ShortBreak and "Long break" for LongBreak, then it
// returns nil.
var updateOutputFn = func(state TimerState, session int, remaining time.Duration, out io.Writer) error {
	var label string
	switch state {
	case Focus:
		label = fmt.Sprintf("Session %d", session)
	case ShortBreak:
		label = "Break"
	case LongBreak:
		label = "Long break"
	default:
		label = "Unknown"
	}

	// Format remaining time as MM:SS.
	minutes := int(remaining.Minutes())
	seconds := int(remaining.Seconds()) % 60

	_, err := fmt.Fprintf(out, "\r\x1b[K%s: %02d:%02d", label, minutes, seconds)
	return err
}
