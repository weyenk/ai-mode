# Story Contract: Core Timer Engine

- **Story:** docs/pipeline/stories/C0004-core-timer-engine.md · **Verdict:** ready (product, qa, architect) · **Meeting:** 20261011-175234-ca2b46 · **Flag:** none

## Story

As a macOS user, I want a lightweight terminal-based Pomodoro timer, so that I can manage focus sessions and breaks without distractions.

## Rules

- R1 The timer must transition between focus sessions, short breaks, and long breaks according to the four-session cycle logic, with each session's duration not exceeding 24 hours.
- R2 The timer must update the in-place status line every second using carriage return and ANSI erase-to-end-of-line, showing the correct session label and countdown for Focus, ShortBreak, and LongBreak states.
- R3 On SIGINT, the program must clear the current in-place status line, print exactly 'Completed N focus sessions\n' where N counts only fully finished focus sessions, and exit with status 0 without a stack trace.
- R4 The program must validate that each configured duration (focus, short break, long break) does not exceed 24 hours at startup, printing a descriptive error to stderr and exiting with a non-zero code if violated.
- R5 The timer must transition between focus, short, and long breaks in a 4-session cycle.
- R6 On SIGINT, the program must clear the current in-place status line and print the completed count.
- R7 Each configured duration must not exceed 24 hours.
- R8 When a focus session is interrupted by SIGINT, the interrupted session must not be counted toward the 'Completed N focus sessions' output.

## Scenarios

Each scenario is one example from the Story Contract; S<n> is example n. Tests carry the id in their name.

- **S1** (R1) After 4 focus sessions (each 25 minutes), the next break is a long break of 15 minutes instead of a short break of 5 minutes.
- **S2** (R1) Timer starts in Focus state, Session 1. At t=25m, it transitions to ShortBreak state for 5m. At t=30m, it transitions back to Focus state, Session 2. After 3 more focus sessions and 3 short breaks (t=25+5+25+5+25+5+25+5 = 115m), the 4th focus session ends. The timer must transition to LongBreak state (not ShortBreak) for 15 minutes. After the long break, the cycle restarts at Session 5.
- **S3** (R1) Timer is in LongBreak state (15:00). At t=10m of the long break, SIGINT is received. The program must print 'Completed 4 focus sessions' (counting only fully finished focus sessions) and exit with status 0. It must NOT print the interrupted long break status or a stack trace.
- **S4** (R1) User configures focusDuration = 25h. The program must reject this configuration or fail fast at startup, because each session length (focus, short break, long break) individually must not exceed 24 hours. This constraint applies per-duration, not to the total cycle time.
- **S5** (R1) Terminal output for Focus Session 1 at t=0: the line shows 'Session 1: 25:00\r\x1b[K'. At t=24m59s: 'Session 1: 00:01\r\x1b[K'. At t=25m: transitions to ShortBreak, line shows 'Break: 05:00\r\x1b[K'. Each update overwrites the previous line in-place using carriage return and ANSI erase sequence.
- **S6** (R1) Timer is in Focus Session 3 (25:00). User presses Ctrl-C at t=20m. The program prints 'Completed 2 focus sessions' (Sessions 1 and 2 fully completed; Session 3 was interrupted) and exits with status 0. The interrupted Session 3 is NOT counted.
- **S7** (R2) At t=0 in Focus Session 1 with default 25m duration, the writer receives 'Session 1: 25:00\r\x1b[K'. At t=1439s, it receives 'Session 1: 00:01\r\x1b[K'. At t=1440s, the state transitions to ShortBreak and the writer receives 'Break: 05:00\r\x1b[K'.
- **S8** (R2) At t=0 in LongBreak with default 15m duration, the writer receives 'Long break: 15:00\r\x1b[K'. Note the label is 'Long break' (two words) not 'Break' to distinguish it from the short break.
- **S9** (R3) Timer is in Focus Session 3 at t=1200s (20m in, 5m remaining). SIGINT arrives. The writer first receives '\r\x1b[K' to clear the 'Session 3: 05:00' remnant, then 'Completed 2 focus sessions\n'. Exit code is 0. Session 3 is not counted because it was interrupted.
- **S10** (R3) Timer is in ShortBreak after Session 4 (the long break trigger has not fired yet—wait, Session 4 just completed, so the next break is LongBreak). Correction: Timer is in LongBreak after 4 completed focus sessions, t=600s into the 15m break. SIGINT arrives. Output is '\r\x1b[KCompleted 4 focus sessions\n'. Exit code 0.
- **S11** (R3) SIGINT arrives immediately at t=0 before any focus session has started or completed. Output is '\r\x1b[KCompleted 0 focus sessions\n'. Exit code 0. No stack trace.
- **S12** (R4) User passes --focus 25h. The program prints 'error: focus duration 25h exceeds 24h limit' to stderr and exits with code 1. No timer loop starts.
- **S13** (R4) User passes --short 24h --focus 25h. Both exceed the limit. The program should report both violations or the first one encountered, print descriptive errors to stderr, and exit with a non-zero code. The exact order of error reporting for multiple violations is an implementation detail, but at least one must be reported.
- **S14** (R2) Boundary: duration is exactly 1 minute. At t=0, output is 'Session 1: 01:00\r\x1b[K'. At t=59s, output is 'Session 1: 00:01\r\x1b[K'. At t=60s, transition to break. The countdown does not show negative values or skip 00:00.
- **S15** (R2) Negative or zero duration passed to constructor. The 24-hour validation only checks the upper bound. Behavior for 0 or negative durations is unspecified by the contract. This is a potential edge case that should be addressed.
- **S16** (R5) After 4 focus sessions (each 25 minutes), the next break is a long break of 15 minutes instead of a short one.
- **S17** (R6) Timer is in Focus Session 3 at t=1200s. SIGINT arrives. Output is '\r\x1b[KCompleted 2 focus sessions\n'. Exit code is 0.
- **S18** (R2) At t=0 in Focus Session 1 with 25m duration, the writer receives 'Session 1: 25:00\r\x1b[K'.
- **S19** (R7) User passes --focus 25h. The program prints 'error: focus duration 25h exceeds 24h limit' to stderr and exits with code 1.
- **S20** (R8) Timer is in Focus Session 5 (the first session of the second 4-session cycle) at t=100s (1:40 elapsed, 23:20 remaining). SIGINT arrives. Output is '\r\x1b[KCompleted 4 focus sessions\n'. The 4 completed focus sessions from the first cycle are counted; Session 5 is interrupted and NOT counted. Exit code 0.
- **S21** (R8) Timer is in Focus Session 1 at t=1s. SIGINT arrives. Output is '\r\x1b[KCompleted 0 focus sessions\n'. Session 1 is interrupted and NOT counted. Exit code 0. This confirms that 'completed' means the full 25:00 elapsed, not merely 'started'.
- **S22** (R8) Timer is in Focus Session 4 (the 4th of the cycle) at t=1439s (24:59 elapsed, 00:01 remaining). SIGINT arrives. Output is '\r\x1b[KCompleted 3 focus sessions\n'. Session 4 is interrupted 1 second before completion and NOT counted. Exit code 0.

## Layer plan

| Layer | Name | Scenarios | Test level |
| --- | --- | --- | --- |
| 1 | Core Timer Engine | S1, S2, S3, S4, S5, S6, S7, S8, S9, S10, S11, S12, S13, S14, S15, S16, S17, S18, S19, S20, S21, S22 | functional |

## Gates

- static-analysis: active
- mutation: active
- performance: not-available
- ai-evals: not-available
- dynamic-security: not-applicable

## Resolved questions

- What happens if the user interrupts the timer during a long break? → The completed focus sessions from the current cycle are counted and printed, but the interrupted long break is not counted as a completed session.
- What is the behavior when SIGINT is received during a focus session that has already been completed (i.e., the timer just transitioned to a break)? Does the completed focus session count in the 'Completed N focus sessions' message? → Yes, the completed focus session is counted because it was fully finished before the transition.
- What happens if SIGINT is received before the timer has started its first focus session (e.g., during initialization or if durations are zero)? → The program will print 'Completed 0 focus sessions' and exit.
- What is the exact error behavior when a duration exceeds 24 hours: does the program exit at startup with an error message, or does it panic/crash? → The program must validate durations at startup and exit with a clear error message and a non-zero exit code, rather than panicking.
- Should the 'Completed N focus sessions' line include a trailing newline, and is it the ONLY output on Ctrl-C (no partial timer line)? → The message must include a trailing newline to ensure the shell prompt starts on a new line and does not append to the message. To ensure it is the only visible output, the implementation should use a carriage return or overwrite the existing in-place timer line to clear any leftover partial timer text.
- Should the program exit with a specific error message or error code if the 24-hour duration constraint is violated at startup? → The program should exit with a descriptive error message to stderr (e.g., 'error: focus duration 25h exceeds 24h limit') and a standard non-zero exit code (e.g., 1) to ensure failure is detectable by calling processes or scripts.
- Should the 'Completed N focus sessions' message include a trailing newline, or should it strictly be the only thing printed on that line? → It should include a trailing newline to ensure the shell prompt starts on a new line. To ensure it is the only content on that line and no remnants of the previous timer status remain, the implementation should use a carriage return (\r) and the ANSI erase-to-end-of-line sequence (\x1b[K) before printing the message and the newline.
- How should durations be provided to the engine (e.g., via command-line flags or hardcoded defaults for this first iteration)? → The `Timer` struct will be decoupled from input methods, accepting `time.Duration` parameters via its constructor. `main.go` will use the standard `flag` package to provide command-line overrides for hardcoded defaults (25m focus, 5m short, 15m long), ensuring a 'zero-config' experience while allowing customization without a config file.
- What happens if a duration is zero or negative? The 24-hour constraint is an upper bound, but is there a lower bound (e.g., minimum 1 second)? Should the program reject non-positive durations at startup, or allow them and immediately transition? → The program should enforce a minimum duration of 1 second and reject non-positive durations at startup with a descriptive error to stderr. This follows the fail-fast principle established for the 24-hour upper bound and prevents the state machine from entering an unstable 'rapid-fire' transition loop.
- When SIGINT arrives, is the '\r\x1b[K' prefix before 'Completed N focus sessions\n' guaranteed, or is it acceptable to simply print 'Completed N focus sessions\n' and let the ANSI erase sequence from the last timer update leave a remnant? The contract says 'clear the current in-place status line' but the exact byte sequence for the final line should be pinned. → The '\r\x1b[K' prefix is required. The contract and existing examples (specifically Examples 9, 10, and 11) explicitly mandate clearing the current in-place line to prevent remnants of a longer previous status line from remaining. The sequence should be pinned to '\r\x1b[K' followed by the completion message.

## Global Constraints

- Language: Go (standard library only).
- Platform: macOS Terminal.
- No external dependencies or network/disk access.
- Maximum session length must not exceed 24 hours to prevent resource exhaustion.

## Contracts (from the feature plan)

The seams the stories share, decided now so no story depends on another story's internals:
- `main.go` (package `main`) will contain the core logic.
- Interfaces for `Clock` and `Writer` will be defined in `main.go` to allow dependency injection for testing.
- The logic will be encapsulated in a `Timer` struct that manages the state machine and countdown.

```go
package main

import (
	"io"
	"time"
)

// Clock defines the interface for time-related operations to allow for testing.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// TimerState represents the current phase of the Pomodoro timer.
type TimerState int

const (
	Focus TimerState = iota
	ShortBreak
	LongBreak
)

// Timer manages the Pomodoro countdown and session transitions.
type Timer struct {
	focusDuration  time.Duration
	shortDuration  time.Duration
	longDuration   time.Duration
	clock          Clock
	out            io.Writer
	completedFocus int
	currentSession int
	// internal state tracking
}

// Run starts the timer loop and handles transitions.
func (t *Timer) Run() error {
	return nil
}
```

- The test suite will use a `FakeClock` implementation of `Clock` and a `bytes.Buffer` as the `io.Writer`.

## Answers from the requester

- Every fourth focus session (the 4th, 8th, 12th, ...) is followed by the long break instead of a short one, so the four-session cycle starts over after each long break; the completed-focus total printed on Ctrl-C counts the whole run. The in-place line is written as a carriage return, the status text, then the ANSI erase-to-end-of-line sequence ESC[K so a shorter line leaves no leftovers. I have no preference on internal signatures: Run taking a context.Context that main cancels on Ctrl-C (signal.NotifyContext) is fine. (requester (answered by the operator, Claude, from the original request), 2026-10-11)
- Status lines, updated in place each second: a focus session shows 'Session N: MM:SS' (N counts focus sessions from 1), a short break shows 'Break: MM:SS', a long break shows 'Long break: MM:SS'. On Ctrl-C it prints exactly one line, 'Completed N focus sessions' (only fully finished focus sessions count; nothing about the interrupted one), and exits with status 0. The 24-hour maximum applies to each length on its own (focus, short break, long break), not to a whole cycle. (requester (answered by the operator, Claude, from the original request), 2026-10-11)
