package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"
)

// init installs the real CLI implementation over the package-level runCLI
// seam declared as a stub in cli_contract.go. init() runs before main and
// before any test, so callers always see the real parsing logic.
func init() {
	runCLI = func(args []string, out io.Writer) int {
		fs := flag.NewFlagSet("pomodoro", flag.ContinueOnError)
		fs.SetOutput(out)

		focus := fs.Int("focus", 25, "focus duration in minutes")
		short := fs.Int("short", 5, "short break duration in minutes")
		long := fs.Int("long", 15, "long break duration in minutes")

		if err := fs.Parse(args); err != nil {
			// flag package already printed the error; exit 2.
			return 2
		}

		// Validate in fixed order: -focus, -short, -long.
		if *focus < 1 || *focus > 1440 {
			fmt.Fprintf(out, "Invalid value for -focus: must be between 1 and 1440 minutes.\n")
			return 2
		}
		if *short < 1 || *short > 1440 {
			fmt.Fprintf(out, "Invalid value for -short: must be between 1 and 1440 minutes.\n")
			return 2
		}
		if *long < 1 || *long > 1440 {
			fmt.Fprintf(out, "Invalid value for -long: must be between 1 and 1440 minutes.\n")
			return 2
		}

		// All valid; start the timer.
		focusDur := time.Duration(*focus) * time.Minute
		shortDur := time.Duration(*short) * time.Minute
		longDur := time.Duration(*long) * time.Minute

		ctx := context.Background()
		if err := runTimerFn(ctx, focusDur, shortDur, longDur); err != nil {
			fmt.Fprintln(out, err)
			return 1
		}
		return 0
	}
}
