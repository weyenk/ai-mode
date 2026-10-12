package main

import (
	"context"
	"fmt"
	"io"
)

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
// with no timer remnant. The returned int is the exit status (always 0 on a
// clean interrupt); the error is nil.
func handleSIGINTFn(t *Timer, ctx context.Context, out io.Writer) (int, error) {
	// Clear the in-place status line and print the completion message. Only
	// fully finished focus sessions are counted; the interrupted one is not.
	_, err := fmt.Fprintf(out, "\r\x1b[KCompleted %d focus sessions\n", t.completedFocus)
	if err != nil {
		return 1, err
	}
	return 0, nil
}
