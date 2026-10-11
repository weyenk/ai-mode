package main

import (
	"context"
	"errors"
	"io"
	"time"
)

// errNotImplemented is returned by stubbed timer behaviour until T02 implements
// the engine logic.
var errNotImplemented = errors.New("timer: not implemented")

// NewTimer creates a Timer with the given per-phase durations.
// Stub: stores configuration only, no behaviour yet.
func NewTimer(focus, short, long time.Duration, clock Clock, out io.Writer) *Timer {
	return &Timer{
		focusDuration: focus,
		shortDuration: short,
		longDuration:  long,
		clock:         clock,
		out:           out,
	}
}

// Run starts the timer loop and handles phase transitions.
// Stub: not implemented yet.
func (t *Timer) Run(ctx context.Context) error {
	_ = t
	_ = ctx
	return errNotImplemented
}
