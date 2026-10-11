# Viability: Simple pomodoro timer

- **Card:** C0001
- **Context:** hobby (a personal project: value is usefulness and enjoyment to its owner, cost is the owner's spare time, risk tolerance is high, no compliance needs)
- **Decision:** GO
- **ROI:** 4.00 value points per engineering day

## Checks

Computed by `ai-mode pipeline` from the role estimates below and the context rubric.

- pass: feasible = true (research, confidence 0.9)
- pass: value 4.0 ≥ 2.0 (product)
- pass: 3 success metric(s) named (product)
- pass: effort 1.0 days ≤ 10 (architect)
- pass: ROI 4.00 = value 4.0 / effort 1.0 days ≥ 0.20

## Target user

Individuals seeking a lightweight, distraction-free pomodoro timer for productivity

## Success metrics

- User successfully starts and stops the timer
- Timer cycles through focus and break sessions as expected
- User can configure custom session lengths at startup

## MVP scope

- 25-minute focus session with 5-minute breaks
- Longer 15-minute break after every fourth session
- Display of current session number and remaining time
- Terminal bell when a session ends
- Ctrl-C to stop the timer and show completed focus sessions
- Configurable session lengths at startup

## Out of scope

- Persistent storage of session data
- Integration with external tools or platforms
- Multi-platform support beyond macOS terminal
- User authentication or account features

## Approach (architect)

A single-file Go CLI using time.Ticker for the countdown and os/signal to handle graceful exit and summary printing. (effort 1.0 days, complexity 1/5)

## Role findings

### research

Findings:  
Go standard library (time, os/signal) enables precise timer loops and SIGINT handling. Terminal bell (`\a`) works in macOS terminals per Apple's terminal standards. Session tracking (25m focus → 5m break, every 4th → 15m) requires only counter logic and `time.Sleep`—no external dependencies needed.  

Risks:  
Time precision is acceptable for hobby use (OS timers suffice for 5-25 min durations). Signal handling via `os/signal` avoids terminal corruption on Ctrl-C. No compliance or storage risks exist per constraints.  

Open questions:  
How to count sessions interrupted mid-session (spec implies only *completed* focus sessions count; verified as non-issue via signal handler cleanup logic).

Estimate: `{"feasible": true, "prior_art": ["CLI pomodoro tools (e.g., `pomo` in npm, `pomodoro` Python CLI)"], "known_pitfalls": ["Time.Sleep precision limitations (acceptable for hobby use)"], "confidence": 0.9}`

### product

# Findings

This request is for a **simple pomodoro timer** designed to be run in the terminal. It targets users who want a **lightweight, distraction-free time management tool**, likely focused on productivity and task management. The job-to-be-done is to help them stay on track with focus sessions and breaks without requiring complex features or external dependencies.

The **smallest valuable first version** (MVP) would include:
- A timer that cycles through 25-minute focus sessions and 5-minute breaks.
- After every fourth session, a 15-minute break is used instead.
- Display of the current session number and remaining time.
- Notification when a session ends (e.g., terminal bell).
- Ability to stop the timer with Ctrl-C and show how many focus sessions were completed.
- Configurable session lengths at startup.

The **value rating** for this context is **4/5**. It's a **must-have** for users who value productivity and simplicity in their workflow, especially in a hobby project where small, functional tools are preferred over feature overload.

# Risks
- The timer may not be robust enough to handle edge cases (e.g., time zone changes, interruptions during sessions).
- Configuring session lengths might require more complex parsing than expected.
- Terminal bells may not be supported or noticeable in all environments.

# Open questions
- Should the program support custom session names or labels?
- What is the behavior if the user tries to configure invalid time values (e.g., negative numbers)?
- Will the timer handle interruptions during a session gracefully?

Estimate: `{ "value": 4, "target_user": "Individuals seeking a lightweight, distraction-free pomodoro timer for productivity", "success_metrics": ["User successfully starts and stops the timer", "Timer cycles through focus and break sessions as expected", "User can configure custom session lengths at startup"], "mvp_scope": ["25-minute focus session with 5-minute breaks", "Longer 15-minute break after every fourth session", "Display of current session number and remaining time", "Terminal bell when a session ends", "Ctrl-C to stop the timer and show completed focus sessions", "Configurable session lengths at startup"], "out_of_scope": ["Persistent storage of session data", "Integration with external tools or platforms", "Multi-platform support beyond macOS terminal", "User authentication or account features"] }`

### architect

## Findings
This is a low-complexity utility with high feasibility. The requirements are well-defined and perfectly suited for Go's standard library. The core logic is a simple state machine (Focus $\rightarrow$ Short Break $\rightarrow$ Long Break) driven by `time.Ticker`. Go's `flag` package can easily handle the customizable intervals, and `os/signal` provides the necessary hook for the Ctrl-C summary.

## Risks
- **Signal Interrupts:** Ensuring the final summary prints correctly when `SIGINT` is caught, without the terminal prompt interfering or the process exiting too abruptly.
- **Terminal UI:** Using carriage returns (`\r`) for in-place updates is simple but can occasionally flicker or leave artifacts if the terminal window is resized.
- **Assumption:** The macOS terminal emulator honors the ASCII bell character (`\a`) by default.

## Open questions
None. The constraints and functionality are self-contained.

Estimate: `{"effort_days": 1, "complexity": 1, "approach": "A single-file Go CLI using time.Ticker for the countdown and os/signal to handle graceful exit and summary printing.", "technical_risks": ["Graceful summary printing on SIGINT", "Terminal bell compatibility"]}`
