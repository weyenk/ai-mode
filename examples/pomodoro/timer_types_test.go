package main

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"
)

// TestMain_S1_Journey (R1 / S1) exercises the program entry point as a user
// would drive it: running the program with "-focus 50 -short 10" must start a
// first focus session of 50 minutes, a short break of 10 minutes, and the
// default long break of 15 minutes, exit with status 0, and print no error.
//
// The functional boundary is the public entry point (main). Everything inside
// it (flag parsing, validation, the runCLI wiring) is real; the only doubles
// at the boundary are the timer engine (captured via the shared withRunTimer
// seam) and the process output/exit (captured via os.Pipe and the osExit
// seam). No sleeping, no real network or disk I/O.
func TestMain_S1_Journey(t *testing.T) {
	rec := withRunTimer(t)

	// Capture the exit code main() would hand to os.Exit.
	var exitCode int
	osExit = func(code int) { exitCode = code }

	// Capture output so a happy (or erroneous) run cannot write to the real
	// terminal.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create output pipe: %v", err)
	}

	// Route the real entry point at the scenario's flags and output. Both
	// globals are restored afterwards so no state leaks across tests.
	oldArgs := os.Args
	os.Args = []string{"pomodoro", "-focus", "50", "-short", "10"}
	oldStdout := os.Stdout
	os.Stdout = pw
	t.Cleanup(func() {
		os.Args = oldArgs
		os.Stdout = oldStdout
	})

	// Act: run the program's real entry point.
	main()

	// Close the write end so the read side reaches EOF, then drain it
	// synchronously (main() has already returned, so this is deterministic).
	pw.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, pr); err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}
	pr.Close()

	// Assert: the timer engine was actually invoked ...
	if !rec.called {
		t.Fatalf("main() did not invoke the timer engine for valid flags -focus 50 -short 10")
	}
	// ... with the supplied durations applied to the corresponding phases ...
	if rec.focus != 50*time.Minute {
		t.Errorf("first focus session = %v, want 50m", rec.focus)
	}
	if rec.short != 10*time.Minute {
		t.Errorf("short break = %v, want 10m", rec.short)
	}
	// ... and the default applied to the flag that was not supplied.
	if rec.long != 15*time.Minute {
		t.Errorf("long break = %v, want default 15m", rec.long)
	}
	// A successful run exits 0 and prints no error.
	if exitCode != 0 {
		t.Errorf("exit status = %d, want 0", exitCode)
	}
	if out.Len() != 0 {
		t.Errorf("unexpected output on success: %q", out.String())
	}
}
