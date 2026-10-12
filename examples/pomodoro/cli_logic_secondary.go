package main

import "io"

// runCLISec is the secondary CLI entry point for T02: it parses the duration
// flags (-focus, -short, -long), validates them in the fixed order
// -focus, -short, -long (R3), applies defaults to unsupplied flags (R4), and
// starts the timer via runTimerFn. It returns the exit code (2 on a
// validation failure, 0 once a valid configuration is handed to runTimerFn).
//
// This Phase-A stub exists only so the test-first suite in
// cli_logic_secondary_test.go compiles; it carries no behaviour, so every test
// that asserts on its result fails on an assertion, never on a build error.
var runCLISec = func(args []string, out io.Writer) int {
	return 0
}
