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
//	"<label>: MM:SS\r\x1b[K"
//
// where the label is "Session N" for Focus, "Break" for ShortBreak and "Long
// break" for LongBreak, then returns nil.
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

	_, err := fmt.Fprintf(out, "%s: %02d:%02d\r\x1b[K", label, minutes, seconds)
	return err
}
