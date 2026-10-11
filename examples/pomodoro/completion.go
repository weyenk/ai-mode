package main

import (
	"context"
	"fmt"
	"io"
)

// completionClear is the exact byte prefix the SIGINT handler must write first
// (R3): a carriage return plus the ANSI erase-to-end-of-line sequence, used to
// clear the current in-place status line before the completion message is
// printed so no timer remnant survives (see the resolved question pinning the
// prefix to "\r\x1b[K").
const completionClear = "\r\x1b[K"

// completionMessage formats the single completion line the program prints on
// SIGINT: "Completed N focus sessions\n", where N counts only fully finished
// focus sessions (R3/R8). The trailing newline keeps the shell prompt on a
// fresh line.
func completionMessage(n int) string {
	return fmt.Sprintf("Completed %d focus sessions\n", n)
}

// runCompletion is the T05 seam (produced as handleSIGINTFn): it clears the
// current in-place status line on out, prints exactly the completion message
// and returns the process exit status. Per R3/R6/R8 the count is only the fully
// finished focus sessions, so the interrupted session is never counted.
//
// The count is t.completedFocus: only focus sessions that fully elapsed before
// the interrupt are counted; the interrupted session (if any) is not (R8).
var runCompletion = func(t *Timer, ctx context.Context, out io.Writer) (int, error) {
	_, err := fmt.Fprint(out, completionClear+completionMessage(t.completedFocus))
	if err != nil {
		return 1, err
	}
	return 0, nil
}
