package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// NewTimer creates a Timer with the given per-phase durations.
func NewTimer(focus, short, long time.Duration, clock Clock, out io.Writer) *Timer {
	return &Timer{
		focusDuration:  focus,
		shortDuration:  short,
		longDuration:   long,
		clock:          clock,
		out:            out,
		currentSession: 1,
	}
}

// Run starts the timer loop and handles phase transitions.
// It writes an in-place status line every second (R2) and transitions between
// Focus, ShortBreak and LongBreak according to the four-session cycle (R1/R5).
func (t *Timer) Run(ctx context.Context) error {
	state := Focus
	elapsed := 0

	// Register the first wait before emitting the initial line so the caller
	// can advance time right after the first write without losing the tick.
	w := t.clock.After(time.Second)
	t.emit(state, elapsed)

	for {
		select {
		case <-w:
		case <-ctx.Done():
			return nil
		}

		// Register the next tick's wait BEFORE emitting this line, so the
		// wait is already pending when the caller advances the clock again.
		w = t.clock.After(time.Second)

		// elapsed counts seconds within the current phase; it resets on each
		// transition so a phase runs its full duration before moving on.
		elapsed++
		if time.Duration(elapsed)*time.Second >= t.durationFor(state) {
			t.transition(&state)
			elapsed = 0
		}
		t.emit(state, elapsed)
	}
}

// durationFor returns the configured duration for the given phase.
func (t *Timer) durationFor(state TimerState) time.Duration {
	switch state {
	case Focus:
		return t.focusDuration
	case ShortBreak:
		return t.shortDuration
	case LongBreak:
		return t.longDuration
	}
	return 0
}

// transition advances the state machine to the next phase after the current
// one has elapsed. A focus→break step counts the finished focus session and
// selects a long break every fourth session; a break→focus step resumes the
// next numbered focus session.
func (t *Timer) transition(state *TimerState) {
	switch *state {
	case Focus:
		// A focus session has fully elapsed: count it and pick the next break.
		t.completedFocus++
		if t.completedFocus%4 == 0 {
			*state = LongBreak
		} else {
			*state = ShortBreak
		}
	default: // ShortBreak, LongBreak
		// Entering the next focus session advances its number.
		t.currentSession++
		*state = Focus
	}
}

// emit writes the in-place status line for the current phase: a carriage
// return, the label and countdown, then the ANSI erase-to-end-of-line so a
// shorter line leaves no remnant.
func (t *Timer) emit(state TimerState, elapsed int) {
	remaining := int64(t.durationFor(state)/time.Second) - int64(elapsed)
	if remaining < 0 {
		remaining = 0
	}
	m := int(remaining / 60)
	s := int(remaining % 60)
	label := ""
	switch state {
	case Focus:
		label = fmt.Sprintf("Session %d", t.currentSession)
	case ShortBreak:
		label = "Break"
	case LongBreak:
		label = "Long break"
	}
	fmt.Fprintf(t.out, "%s: %02d:%02d\r\x1b[K", label, m, s)
}
