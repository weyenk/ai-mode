package main

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// errSIGINTNotImplemented is returned by the handleSIGINTFn stub so the red
// tests fail on a clear signal rather than silently producing empty output.
var errSIGINTNotImplemented = errors.New("handleSIGINTFn: not implemented")

// handleSIGINTFn handles a SIGINT (Ctrl-C) while a session is in flight.
//
// It must (R3/R6/R8):
//  1. clear the current in-place status line by writing the exact sequence
//     "\r\x1b[K" (carriage return + ANSI erase-to-end-of-line);
//  2. print exactly "Completed N focus sessions\n", where N counts only the
//     fully finished focus sessions (t.completedFocus) and the interrupted
//     session is NOT counted;
//  3. exit with status 0 and no stack trace.
//
// out is the writer the timer uses for its in-place status line; writing the
// completion message there guarantees the shell prompt lands on a fresh line
// with no timer remnant.
//
// This is a Phase A (test-first) stub: it performs no I/O and returns 0 with
// errSIGINTNotImplemented so every interrupt test fails on its assertion.
func handleSIGINTFn(t *Timer, ctx context.Context, out io.Writer) (int, error) {
	_ = fmt.Sprint() // keep fmt import available for the real implementation
	_ = t
	_ = ctx
	_ = out
	return 0, errSIGINTNotImplemented
}
