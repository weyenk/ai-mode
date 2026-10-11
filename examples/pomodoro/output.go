package main

import (
	"errors"
	"io"
	"time"
)

// updateOutputFn writes the current in-place status line for the timer's state
// (R2). It is the seam exercised by T03's output tests: given the active state,
// the current focus-session number and the time remaining in the current phase,
// it must emit exactly one line of the form
//
//	"<label>: MM:SS" + "\r" + "\x1b[K"
//
// where the label is "Session N" for Focus, "Break" for ShortBreak and "Long
// break" for LongBreak, then returns nil.
//
// Phase A stub: returns a not-implemented error and writes nothing, so every
// output test fails on its value assertion before the real formatting exists.
var updateOutputFn = func(state TimerState, session int, remaining time.Duration, out io.Writer) error {
	return errors.New("output: updateOutputFn not implemented")
}
