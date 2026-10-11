# Pomodoro CLI — design spec

**Card:** C0002 · **Request:** docs/pipeline/requests/C0001-simple-pomodoro-timer.md · **Viability:** docs/pipeline/viability/C0001-simple-pomodoro-timer.md

## Goal
A lightweight, terminal-based Pomodoro timer for macOS users to manage focus sessions and breaks without distractions.

## Users and success metrics
Users: Individuals seeking a distraction-free productivity tool.
Success metrics:
- User successfully starts and stops the timer.
- Timer cycles through focus and break sessions as expected.
- User can configure custom session lengths at startup.

## Scope
### In scope
- Terminal-based countdown timer logic.
- Flag parsing for custom session lengths (focus, short break, long break).
- Signal handling (SIGINT) for graceful exit and summary printing.
- Terminal bell notification (`\a`) at session transitions.
- In-place terminal updates using carriage returns (`\r`).
- Tracking and reporting completed focus sessions.

### Out of scope
- Persistent storage of session data.
- Integration with external tools or platforms.
- Multi-platform support beyond macOS terminal.
- User authentication or account features.

## User flows
1. **Start Timer:** User runs the program (optionally with flags). The timer begins the first focus session immediately.
2. **Monitor Progress:** User watches the terminal as the time remaining decrements and the session number increments.
3. **Session Transition:** When a session ends, a terminal bell sounds, and the next session type (short or long break) begins automatically.
4. **Interrupt Timer (Graceful Exit):** User presses `Ctrl-C`. The program catches `SIGINT`, stops the timer, and prints the total number of completed focus sessions before exiting.
5. **Invalid Configuration:** User provides invalid, negative, or excessively large session lengths (e.g., > 24h). The program should error out and exit before starting.

## Behaviour rules
- R1 Focus sessions default to 25 minutes.
- R2 Short breaks default to 5 minutes.
- R3 Long breaks default to 15 minutes.
- R4 After every fourth completed focus session, the next session is a long break. Otherwise, the next session is a short break.
- R5 Session end notification must include the ASCII bell character (`\a`).
- R6 Terminal output must update the same line using `\r` to show remaining time.
- R7 On exit, print one line to stdout: `Completed [X] focus sessions` where [X] is the total count of finished focus sessions.

## Scenarios
- **S1 — Standard Cycle.** Given default durations, when 25m focus and 5m short break pass, then the terminal shows "Session 1: 25:00" → "Session 1: 00:00" → "Break: 05:00".
- **S2 — Long Break Trigger.** Given default durations, when 4 focus sessions and 3 short breaks complete, then the 4th break must be 15 minutes long.
- **S3 — Custom Configuration.** Given flags `-focus 50 -short 10`, when the timer starts, then the first session lasts 50 minutes.
- **S4 — Graceful Exit.** Given a running timer, when `SIGINT` is received, then the program prints the completed count and exits without a stack trace.
- **S5 — Invalid Input.** Given `-focus -1`, when the program starts, then it prints an error and exits.

## Global Constraints
- Language: Go (standard library only).
- Platform: macOS Terminal.
- No external dependencies or network/disk access.
- Maximum session length must not exceed 24 hours to prevent resource exhaustion.

## Technical approach
- **Package:** `main` (single-file approach preferred for simplicity).
- **Core Logic:**
    - A `Timer` struct or loop driven by `time.Ticker` (1-second intervals).
    - A state machine: `Focus` → `ShortBreak` | `LongBreak`.
    - A counter for completed focus sessions.
- **Abstractions for Testability:**
    - `Clock` abstraction (e.g., a provider or interface) to control time progression and ticker intervals, enabling instantaneous test execution.
    - Dependency injection of `io.Writer` (replacing `os.Stdout`) to allow verifying terminal output in tests.
- **Concurrency:** A goroutine to handle the ticker/countdown and a separate goroutine/signal listener to handle `os.Interrupt`.

## Open questions
None.

## Review log
product: none; no blocking or non-blocking items. No edits applied.
research: non-blocking; noted terminal bell behavior and `` handling, but no changes needed.
security: proposed edit applied—added upper bound (24h) to prevent resource exhaustion; added negative duration check in invalid input rule.
ux: proposed edits applied—rewrote goal for clarity; reworded R4 for precision; rewrote R7 for stronger output format; added explicit negative duration error message.
architect: proposed edits applied—updated `Clock` and `Writer` descriptions to use idiomatic Go patterns (interface + dependency injection).
